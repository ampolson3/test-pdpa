package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/files"
)

// validID builds a valid 13-digit Thai id from 12 digits (the check digit is computed).
func validID(first12 string) string {
	sum := 0
	for i := 0; i < 12; i++ {
		sum += int(first12[i]-'0') * (13 - i)
	}
	return first12 + string(rune('0'+(11-sum%11)%10))
}

func TestValidThaiID(t *testing.T) {
	good := validID("010555612345")
	if !orgservice.ValidThaiID(good) {
		t.Errorf("%s should be valid", good)
	}
	bad := good[:12] + string(rune('0'+(int(good[12]-'0')+1)%10))
	for _, s := range []string{bad, "123", "01055561234AB", ""} {
		if orgservice.ValidThaiID(s) {
			t.Errorf("%q accepted", s)
		}
	}
}

type fakeFiles struct {
	file     files.File
	attached map[uuid.UUID]uuid.UUID
}

func (f *fakeFiles) Get(_ context.Context, id uuid.UUID) (files.File, error) {
	if id != f.file.ID {
		return files.File{}, files.ErrNotFound
	}
	return f.file, nil
}

func (f *fakeFiles) Attach(_ context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error {
	if entityType != orgservice.LegalEntityType {
		return files.ErrUnknownEntity
	}
	f.attached[id] = entityID
	return nil
}

func TestLegalEntities(t *testing.T) {
	e := setup(t, "orgle")
	logo := &fakeFiles{file: files.File{ID: uuid.New(), MimeType: "image/png", AVStatus: "clean"}, attached: map[uuid.UUID]uuid.UUID{}}
	e.in(t, func(ctx context.Context) error { // the logo's platform.files row (org.legal_entities.logo_file_id is a FK)
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `INSERT INTO platform.files (id, tenant_id, bucket, object_key, file_name, mime_type, size_bytes, sha256)
			VALUES ($1, current_setting('app.tenant_id')::uuid, 'b', $2, 'logo.png', 'image/png', 1, repeat('0', 64))`, logo.file.ID, logo.file.ID.String())
		return err
	})
	e.svc.Files = logo
	reg := validID("010555612345")
	e.in(t, func(ctx context.Context) error {
		for name, in := range map[string]orgservice.LegalEntity{
			"no name":        {NameTh: " "},
			"bad checksum":   {NameTh: "x", RegistrationNo: reg[:12] + "0"},
			"bad email":      {NameTh: "x", ContactEmail: "not-an-email"},
			"bad postcode":   {NameTh: "x", Address: orgservice.Address{PostalCode: "123"}},
			"unknown status": {NameTh: "x", Status: "gone"},
		} {
			if name == "bad checksum" && reg[12] == '0' {
				continue
			}
			if _, err := e.svc.SaveLegalEntity(ctx, in, 0); !errors.Is(err, orgservice.ErrInvalid) {
				t.Errorf("%s: %v", name, err)
			}
		}
		group, err := e.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "กลุ่มบริษัท ตัวอย่าง", IsController: true}, 0)
		if err != nil {
			return err
		}
		co, err := e.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", NameEn: "Example Co., Ltd.", ParentID: &group.ID,
			RegistrationNo: reg[:1] + "-" + reg[1:5] + "-" + reg[5:], TaxID: reg, ContactEmail: "dpo@example.co.th", ContactPhone: "02-123-4567",
			Address:      orgservice.Address{Line1: "99 ถนนสุขุมวิท", District: "วัฒนา", Province: "กรุงเทพมหานคร", PostalCode: "10110"},
			IsController: true, LogoFileID: &logo.file.ID}, 0)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if co.RegistrationNo != reg || co.Address.CountryCode != "TH" || logo.attached[logo.file.ID] != co.ID {
			t.Errorf("normalized and logo attached: %+v %v", co, logo.attached)
		}
		if _, err := e.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "ซ้ำ", RegistrationNo: reg}, 0); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("duplicate registration no: %v", err)
		}
		group.ParentID = &co.ID
		if _, err := e.svc.SaveLegalEntity(ctx, group, group.RowVersion); !errors.Is(err, orgservice.ErrCycle) {
			t.Errorf("parent cycle: %v", err)
		}
		co.NameEn = "Example Company Limited"
		if _, err := e.svc.SaveLegalEntity(ctx, co, co.RowVersion+1); !errors.Is(err, orgservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v", err)
		}
		if co, err = e.svc.SaveLegalEntity(ctx, co, co.RowVersion); err != nil || co.RowVersion != 2 {
			t.Errorf("update: %+v %v", co, err)
		}
		// Acceptance (ORG-01): documents show the organization as recorded.
		f, err := e.svc.MergeFields(ctx, co.ID)
		if err != nil || f["org_name_th"] != "บริษัท ตัวอย่าง จำกัด" || f["org_name_en"] != "Example Company Limited" || f["org_registration_no"] != reg ||
			f["org_address"] != "99 ถนนสุขุมวิท วัฒนา กรุงเทพมหานคร 10110" || f["org_email"] != "dpo@example.co.th" {
			t.Errorf("merge fields: %v %v", f, err)
		}
		other := &fakeFiles{file: files.File{ID: uuid.New(), MimeType: "application/pdf", AVStatus: "clean"}, attached: map[uuid.UUID]uuid.UUID{}}
		e.svc.Files = other
		co.LogoFileID = &other.file.ID
		if _, err := e.svc.SaveLegalEntity(ctx, co, co.RowVersion); !errors.Is(err, orgservice.ErrLogoNotUse) {
			t.Errorf("PDF as logo: %v", err)
		}
		var n int
		_ = pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.audit_log WHERE entity_type = 'legal_entity'`).Scan(&n)
		if n != 3 {
			t.Errorf("audit rows: %d", n)
		}
		return nil
	})
}

// Acceptance (ORG-04): moving a department moves everything below it, and scope checks follow at once.
func TestOrgUnits_MoveAndScope(t *testing.T) {
	e := setup(t, "orgunit")
	e.in(t, func(ctx context.Context) error {
		co, err := e.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ก"}, 0)
		if err != nil {
			return err
		}
		other, err := e.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ข"}, 0)
		if err != nil {
			return err
		}
		mk := func(code, name, typ string, parent *uuid.UUID) orgservice.OrgUnit {
			u, err := e.svc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: co.ID, ParentID: parent, Code: code, NameTh: name, UnitType: typ})
			if err != nil {
				t.Fatalf("create %s: %v", code, err)
			}
			return u
		}
		ops := mk("OPS", "ฝ่ายปฏิบัติการ", "division", nil)
		it := mk("IT", "ฝ่ายไอที", "division", nil)
		hr := mk("HR", "แผนกบุคคล", "department", &ops.ID)
		payroll := mk("PAY", "ทีมเงินเดือน", "team", &hr.ID)
		if payroll.Depth != 3 || !strings.HasPrefix(payroll.Path, ops.Path+".") {
			t.Errorf("tree path: %+v", payroll)
		}
		if _, err := e.svc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: co.ID, Code: "HR", NameTh: "ซ้ำ", UnitType: "team"}); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("duplicate code: %v", err)
		}
		if _, err := e.svc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: other.ID, ParentID: &ops.ID, Code: "X", NameTh: "x", UnitType: "team"}); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("parent of another legal entity: %v", err)
		}
		within := func(unit, scope uuid.UUID) bool {
			ok, err := e.svc.UnitWithin(ctx, unit, scope, true)
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}
		if !within(payroll.ID, ops.ID) || within(payroll.ID, it.ID) {
			t.Fatal("scope before the move")
		}
		moved, err := e.svc.MoveOrgUnit(ctx, hr.ID, hr.RowVersion, &it.ID)
		if err != nil || *moved.ParentID != it.ID {
			t.Fatalf("move: %+v %v", moved, err)
		}
		if within(payroll.ID, ops.ID) || !within(payroll.ID, it.ID) || !within(hr.ID, it.ID) {
			t.Error("scope did not follow the move of the department and its team")
		}
		if ok, _ := e.svc.UnitWithin(ctx, payroll.ID, it.ID, false); ok {
			t.Error("without descendants only the unit itself is in scope")
		}
		if _, err := e.svc.MoveOrgUnit(ctx, it.ID, it.RowVersion, &payroll.ID); !errors.Is(err, orgservice.ErrCycle) {
			t.Errorf("move under own descendant: %v", err)
		}
		list, _ := e.svc.ListOrgUnits(ctx, &co.ID, false)
		if len(list) != 4 || list[0].Depth != 1 {
			t.Errorf("list in path order: %+v", list)
		}
		if _, err := e.svc.CloseOrgUnit(ctx, hr.ID, moved.RowVersion); !errors.Is(err, orgservice.ErrHasActive) {
			t.Errorf("close with an active team below: %v", err)
		}
		cur, _ := e.svc.ListOrgUnits(ctx, &co.ID, false)
		var pay orgservice.OrgUnit
		for _, u := range cur {
			if u.ID == payroll.ID {
				pay = u
			}
		}
		if closed, err := e.svc.CloseOrgUnit(ctx, payroll.ID, pay.RowVersion); err != nil || closed.Status != "closed" || closed.ClosedAt == nil {
			t.Fatalf("close team: %+v %v", closed, err)
		}
		if active, _ := e.svc.ListOrgUnits(ctx, &co.ID, false); len(active) != 3 {
			t.Errorf("closed unit hidden by default: %d", len(active))
		}
		if all, _ := e.svc.ListOrgUnits(ctx, &co.ID, true); len(all) != 4 {
			t.Errorf("closed unit kept for history: %d", len(all))
		}
		upd, err := e.svc.UpdateOrgUnit(ctx, it.ID, it.RowVersion, orgservice.OrgUnit{Code: "IT", NameTh: "ฝ่ายเทคโนโลยี", UnitType: "division"})
		if err != nil || upd.NameTh != "ฝ่ายเทคโนโลยี" {
			t.Errorf("rename: %+v %v", upd, err)
		}
		return nil
	})
}

func TestStructure_TenantIsolation(t *testing.T) {
	a, b := setup(t, "orgsta"), setup(t, "orgstb")
	var co orgservice.LegalEntity
	var unit orgservice.OrgUnit
	a.in(t, func(ctx context.Context) error {
		var err error
		if co, err = a.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "A Co"}, 0); err != nil {
			return err
		}
		unit, err = a.svc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: co.ID, Code: "A1", NameTh: "a", UnitType: "department"})
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if list, _ := b.svc.ListLegalEntities(ctx); len(list) != 0 {
			t.Error("B lists A's legal entities")
		}
		if _, err := b.svc.GetLegalEntity(ctx, co.ID); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("B reads A's entity: %v", err)
		}
		if _, err := b.svc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: co.ID, Code: "B", NameTh: "b", UnitType: "team"}); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("B adds a unit to A's entity: %v", err)
		}
		mine, _ := b.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "B Co"}, 0)
		if _, err := b.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "B sub", ParentID: &co.ID}, 0); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("B links to A's entity as parent: %v", err)
		}
		if _, err := b.svc.MoveOrgUnit(ctx, unit.ID, unit.RowVersion, nil); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("B moves A's unit: %v", err)
		}
		if units, _ := b.svc.ListOrgUnits(ctx, nil, true); len(units) != 0 {
			t.Error("B lists A's units")
		}
		_ = mine
		return nil
	})
}
