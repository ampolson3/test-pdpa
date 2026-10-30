package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
)

// ORG-03 local representative (ม.37(5)): recorded on the legal entity, validated, and
// surfaced as merge fields only when set (so a notice composed from it shows the
// representative's details exactly when one is on record, and nothing otherwise).
func TestLegalEntity_Representative(t *testing.T) {
	e := setup(t, "orgrep")
	e.in(t, func(ctx context.Context) error {
		co, err := e.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		// No representative on record: merge fields don't carry any representative_* key.
		f, err := e.svc.MergeFields(ctx, co.ID)
		if err != nil {
			t.Fatalf("merge fields: %v", err)
		}
		if _, ok := f["representative_name_th"]; ok {
			t.Errorf("unexpected representative merge field with none on record: %v", f)
		}

		// Invalid representative: no name, bad email, bad phone.
		for name, rep := range map[string]*orgservice.Representative{
			"no name":   {NameTh: " "},
			"bad email": {NameTh: "x", Email: "not-an-email"},
			"bad phone": {NameTh: "x", Phone: "???"},
		} {
			co.Representative = rep
			if _, err := e.svc.SaveLegalEntity(ctx, co, co.RowVersion); !errors.Is(err, orgservice.ErrInvalid) {
				t.Errorf("%s: %v", name, err)
			}
		}

		// Set a representative: it's on record and its merge fields appear.
		co.Representative = &orgservice.Representative{NameTh: "นายผู้แทน ใจดี", NameEn: "Mr. Representative Jaidee",
			Address: "99 ถนนสุขุมวิท กรุงเทพมหานคร", Email: "rep@example.co.th", Phone: "02-111-2222"}
		co, err = e.svc.SaveLegalEntity(ctx, co, co.RowVersion)
		if err != nil {
			t.Fatalf("set representative: %v", err)
		}
		got, err := e.svc.GetLegalEntity(ctx, co.ID)
		if err != nil || got.Representative == nil || got.Representative.NameTh != "นายผู้แทน ใจดี" || got.Representative.Email != "rep@example.co.th" {
			t.Errorf("representative round trip: %+v %v", got.Representative, err)
		}

		// Acceptance criterion: the merge fields a notice would render now carry the representative.
		f, err = e.svc.MergeFields(ctx, co.ID)
		if err != nil {
			t.Fatalf("merge fields: %v", err)
		}
		if f["representative_name_th"] != "นายผู้แทน ใจดี" || f["representative_name_en"] != "Mr. Representative Jaidee" ||
			f["representative_email"] != "rep@example.co.th" || f["representative_phone"] != "02-111-2222" {
			t.Errorf("representative merge fields: %v", f)
		}

		// Clearing the representative removes it from both the record and the merge fields.
		co.Representative = nil
		co, err = e.svc.SaveLegalEntity(ctx, co, co.RowVersion)
		if err != nil {
			t.Fatalf("clear representative: %v", err)
		}
		got, err = e.svc.GetLegalEntity(ctx, co.ID)
		if err != nil || got.Representative != nil {
			t.Errorf("representative not cleared: %+v %v", got.Representative, err)
		}
		f, err = e.svc.MergeFields(ctx, co.ID)
		if err != nil {
			t.Fatalf("merge fields: %v", err)
		}
		if _, ok := f["representative_name_th"]; ok {
			t.Errorf("representative merge field should be gone: %v", f)
		}
		return nil
	})
}

// Two-tenant isolation: a representative recorded for one tenant's legal entity is never visible to another.
func TestLegalEntity_Representative_TwoTenantIsolation(t *testing.T) {
	a, b := setup(t, "orgrepa"), setup(t, "orgrepb")
	var entID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		co, err := a.svc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "A", IsController: true,
			Representative: &orgservice.Representative{NameTh: "ผู้แทน A"}}, 0)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		entID = co.ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetLegalEntity(ctx, entID); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's legal entity: %v", err)
		}
		return nil
	})
}
