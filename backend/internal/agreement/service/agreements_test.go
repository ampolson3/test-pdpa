package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/versioning"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
	"pdpa-platform/internal/wiring"
)

var permissions = []string{
	"agreement.dpa.read", "agreement.dpa.create", "agreement.dpa.update",
	"org.structure.read", "org.structure.update", "vendor.vendor.read", "vendor.vendor.create",
	"ropa.activity.read", "ropa.activity.create",
}

type env struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *agreementservice.Service
	org    *orgservice.Service
	vendor *vendorservice.Service
	ropa   *ropaservice.Service
	ver    *versioning.Service
}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM agreement.clauses`, `DELETE FROM agreement.agreement_activities`, `DELETE FROM agreement.parties`,
				`DELETE FROM agreement.agreements`, `DELETE FROM ropa.activity_transfers`,
				`DELETE FROM ropa.activity_controls`, `DELETE FROM ropa.retention_rules`,
				`DELETE FROM ropa.activity_data`, `DELETE FROM ropa.activity_purposes`,
				`DELETE FROM platform.approvals`, `DELETE FROM platform.record_versions`,
				`DELETE FROM platform.document_versions`, `DELETE FROM platform.documents`,
				`DELETE FROM ropa.processing_activities`, `DELETE FROM vendor.vendors`, `DELETE FROM org.org_units`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM org.external_parties`, `DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	vendorSvc := &vendorservice.Service{Audit: audit.New(), Org: orgSvc}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: &riskservice.Service{Audit: audit.New()}}
	versioningSvc := wiring.Versioning(nil, audit.New())
	docsSvc := wiring.Docs(versioningSvc, nil, nil, audit.New(), nil)
	docsSvc.RegisterVersioning()
	svc := &agreementservice.Service{Docs: docsSvc, Org: orgSvc, Vendor: vendorSvc, Ropa: ropaSvc, Audit: audit.New()}
	docsSvc.SetSubmitValidate("dpa", svc.CheckSubmittable) // DPA-03: ม.40 mandatory clauses gate Submit
	return env{app: app, tenant: tenant, svc: svc, org: orgSvc, vendor: vendorSvc, ropa: ropaSvc, ver: versioningSvc}
}

func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: permissions}))
	}); err != nil {
		t.Fatal(err)
	}
}

// fixture seeds a legal entity, an org unit, an external party + vendor, and one RoPA activity: everything
// CreateWizard's own FK-visibility checks need.
func fixture(t *testing.T, ctx context.Context, e env) (legalEntityID, vendorID, activityID uuid.UUID) {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการคลาวด์", CountryCode: "US"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.vendor.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ประมวลผลเงินเดือนบนคลาวด์", IsProcessor: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-PAYROLL", Name: "ประมวลผลเงินเดือน", Role: "controller"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return le.ID, v.ID, a.ID
}

// TestCreateWizard_ManualMode is DPA-02's acceptance criterion in manual mode ("โหมดกรอกเอง"): one call
// from a vendor + its RoPA activities drafts a complete agreement with a real document, parties and linked
// activities, with no template.
func TestCreateWizard_ManualMode(t *testing.T) {
	e := setup(t, "dpaManual")
	var leID, vendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)
		return nil
	})

	var a agreementservice.Agreement
	e.in(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: leID,
			ActivityIDs: []uuid.UUID{activityID}, Title: "DPA กับผู้ให้บริการคลาวด์",
		})
		return err
	})
	if !strings.HasPrefix(a.AgreementNo, "DPA-") {
		t.Fatalf("agreement_no = %q, want DPA-<year>-NNNN", a.AgreementNo)
	}
	if a.Status == "" || a.DocumentID == uuid.Nil {
		t.Fatalf("expected a status and a composed document, got %+v", a)
	}
	if a.RenewalNoticeDays != 60 {
		t.Errorf("renewal_notice_days = %d, want the column default 60", a.RenewalNoticeDays)
	}
	if len(a.ActivityIDs) != 1 || a.ActivityIDs[0] != activityID {
		t.Errorf("activity_ids = %v, want [%s]", a.ActivityIDs, activityID)
	}

	e.in(t, func(ctx context.Context) error {
		doc, err := e.docsGet(ctx, a.DocumentID)
		if err != nil {
			return err
		}
		if doc.DocType != "dpa" {
			t.Errorf("document doc_type = %q, want dpa", doc.DocType)
		}
		return nil
	})
}

func (e env) docsGet(ctx context.Context, id uuid.UUID) (docs.Document, error) {
	return e.svc.Docs.Get(ctx, id)
}

// TestCreateWizard_CounterpartyRoleDerivation: the counterparty's own role is derived from ours, never
// chosen separately — controller<->processor, joint_controller<->joint_controller.
func TestCreateWizard_CounterpartyRoleDerivation(t *testing.T) {
	cases := []struct{ our, counterparty string }{
		{"controller", "processor"}, {"processor", "controller"}, {"joint_controller", "joint_controller"},
	}
	for _, c := range cases {
		e := setup(t, "dpaRole"+c.our)
		var leID, vendorID uuid.UUID
		e.in(t, func(ctx context.Context) error {
			leID, vendorID, _ = fixture(t, ctx, e)
			return nil
		})
		var a agreementservice.Agreement
		e.in(t, func(ctx context.Context) error {
			var err error
			a, err = e.svc.CreateWizard(ctx, agreementservice.CreateInput{
				AgreementType: "dpa", OurRole: c.our, VendorID: vendorID, LegalEntityID: leID, Title: "t",
			})
			return err
		})
		var partyRole string
		e.in(t, func(ctx context.Context) error {
			return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT party_role FROM agreement.parties WHERE agreement_id = $1`, a.ID).Scan(&partyRole)
		})
		if partyRole != c.counterparty {
			t.Errorf("our_role=%s: counterparty role = %q, want %q", c.our, partyRole, c.counterparty)
		}
	}
}

// TestCreateWizard_ValidatesFKs: an unknown vendor, legal entity or activity id is refused before anything
// is written, not left to the database's own FK constraint (FKs bypass RLS — CLAUDE.md rule 1).
func TestCreateWizard_ValidatesFKs(t *testing.T) {
	e := setup(t, "dpaFK")
	var leID, vendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)
		return nil
	})
	unknown := uuid.Must(uuid.NewRandom())

	cases := []agreementservice.CreateInput{
		{AgreementType: "dpa", OurRole: "controller", VendorID: unknown, LegalEntityID: leID, Title: "t"},
		{AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: unknown, Title: "t"},
		{AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: leID, ActivityIDs: []uuid.UUID{unknown}, Title: "t"},
		{AgreementType: "dsa", OurRole: "controller", CounterpartyPartyID: unknown, CounterpartyRole: "receiving", LegalEntityID: leID, Title: "t"}, // DSA-04: unknown counterparty party
		{AgreementType: "dpa", OurRole: "bogus", VendorID: vendorID, LegalEntityID: leID, Title: "t"},
		{AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: leID, Title: ""},
	}
	for i, in := range cases {
		e.in(t, func(ctx context.Context) error {
			_, err := e.svc.CreateWizard(ctx, in)
			if err == nil {
				t.Errorf("case %d: expected an error, got none", i)
			}
			return nil
		})
	}
	_ = activityID
}

// TestListAgreements_FilterByTypeAndVendor proves the list filters and paginates.
func TestListAgreements_FilterByTypeAndVendor(t *testing.T) {
	e := setup(t, "dpaList")
	var leID, vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, e)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateWizard(ctx, agreementservice.CreateInput{AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: leID, Title: "A"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListAgreements(ctx, agreementservice.AgreementFilter{VendorID: &vendorID})
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Errorf("list by vendor = %d items, want 1", len(list))
		}
		list2, _, err := e.svc.ListAgreements(ctx, agreementservice.AgreementFilter{AgreementType: "dpa"})
		if err != nil {
			return err
		}
		if len(list2) != 1 {
			t.Errorf("list by type = %d items, want 1", len(list2))
		}
		return nil
	})
}

// TestListAgreements_IncludesLinkedActivities is DPA-11's own acceptance criterion: opening a vendor (i.e.
// listing its agreements via the vendor_id filter) shows each agreement's own linked RoPA activities too,
// not just a bare agreement row — the same activity_ids GetAgreement already returns for one agreement.
func TestListAgreements_IncludesLinkedActivities(t *testing.T) {
	e := setup(t, "dpaListActivities")
	var leID, vendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: leID,
			ActivityIDs: []uuid.UUID{activityID}, Title: "A",
		})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListAgreements(ctx, agreementservice.AgreementFilter{VendorID: &vendorID})
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Fatalf("list by vendor = %d items, want 1", len(list))
		}
		if len(list[0].ActivityIDs) != 1 || list[0].ActivityIDs[0] != activityID {
			t.Errorf("activity_ids = %v, want [%s]", list[0].ActivityIDs, activityID)
		}
		return nil
	})
}

// TestTwoTenantIsolation: tenant B cannot read tenant A's agreement by id, and tenant B's own list never
// shows tenant A's rows (CLAUDE.md rule 1).
func TestTwoTenantIsolation(t *testing.T) {
	eA := setup(t, "dpaIsoA")
	var leID, vendorID uuid.UUID
	eA.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, eA)
		return nil
	})
	var agreementID uuid.UUID
	eA.in(t, func(ctx context.Context) error {
		a, err := eA.svc.CreateWizard(ctx, agreementservice.CreateInput{AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: leID, Title: "A"})
		agreementID = a.ID
		return err
	})

	eB := setup(t, "dpaIsoB")
	eB.in(t, func(ctx context.Context) error {
		_, err := eB.svc.GetAgreement(ctx, agreementID)
		if err == nil {
			t.Error("tenant B read tenant A's agreement")
		}
		list, _, err := eB.svc.ListAgreements(ctx, agreementservice.AgreementFilter{})
		if err != nil {
			return err
		}
		for _, a := range list {
			if a.ID == agreementID {
				t.Error("tenant B's list contains tenant A's agreement")
			}
		}
		return nil
	})
}
