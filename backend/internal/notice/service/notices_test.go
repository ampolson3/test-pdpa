package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/jobs"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

type env struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *noticeservice.Service
	org    *orgservice.Service
	ropa   *ropaservice.Service
	docs   *docs.Service
}

var noticePermissions = []string{"notice.document.read", "notice.document.create", "notice.document.update",
	"ropa.activity.read", "ropa.activity.create", "ropa.activity.update", "org.structure.read", "org.structure.update",
	"org.masterdata.read", "org.party.read", "org.party.create", "org.party.update"}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM notice.notice_activity_links`, `DELETE FROM notice.notices`,
				`DELETE FROM platform.document_versions`, `DELETE FROM platform.documents`,
				`DELETE FROM ropa.activity_transfers`, `DELETE FROM ropa.activity_recipients`,
				`DELETE FROM ropa.retention_rules`, `DELETE FROM ropa.activity_data`, `DELETE FROM ropa.activity_purposes`,
				`DELETE FROM ropa.processing_activities`, `DELETE FROM org.data_categories WHERE tenant_id IS NOT NULL`,
				`DELETE FROM org.external_parties`, `DELETE FROM org.org_units`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	client, err := jobs.NewInsertClient(app)
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := &orgservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc}
	versioningSvc := wiring.Versioning(nil, audit.New())
	docsSvc := wiring.Docs(versioningSvc, nil, client, audit.New(), nil)
	docsSvc.RegisterVersioning()
	svc := &noticeservice.Service{Audit: audit.New(), Org: orgSvc, Ropa: ropaSvc, Docs: docsSvc}
	return env{app: app, tenant: tenant, svc: svc, org: orgSvc, ropa: ropaSvc, docs: docsSvc}
}

func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: noticePermissions}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

// seedActivity builds one fully-populated processing activity (purpose+lawful basis, sensitive data category,
// retention rule, a foreign recipient with a logged transfer) so CreateWizard has real RoPA data to compose from.
func seedActivity(t *testing.T, ctx context.Context, e env, legalEntity uuid.UUID) uuid.UUID {
	t.Helper()
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: legalEntity, Code: "HR", NameTh: "HR", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการคลาวด์ต่างประเทศ", NameEn: "Overseas Cloud Processor", CountryCode: "US"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	cats, err := e.org.ListMaster(ctx, "data_categories")
	if err != nil || len(cats) == 0 {
		t.Fatal(err, "expected seeded data categories (ORG-07)")
	}
	subjectTypes, err := e.org.ListMaster(ctx, "data_subject_types")
	if err != nil || len(subjectTypes) == 0 {
		t.Fatal(err, "expected seeded data subject types (ORG-07)")
	}
	basisCode := lawfulBasisCode(t, ctx, e.org)

	a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: legalEntity, OrgUnitID: unit.ID, Code: "HR-01", Name: "การจ่ายเงินเดือนพนักงาน", Role: "controller"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ropa.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: a.ID, PurposeText: "จ่ายเงินเดือนและสวัสดิการพนักงาน", LawfulBasisCode: basisCode}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ropa.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: a.ID, DataCategoryID: *cats[0].ID, SubjectTypeID: *subjectTypes[0].ID, Source: "direct", IsSensitive: cats[0].IsSensitive}); err != nil {
		t.Fatal(err)
	}
	months := 84
	if _, err := e.ropa.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: a.ID, RetentionMonths: &months, RetentionBasis: "legal", TriggerEvent: "สิ้นสุดสัญญาจ้าง", DisposalMethod: "delete"}); err != nil {
		t.Fatal(err)
	}
	rc, err := e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: a.ID, PartyID: party.ID, RecipientRole: "processor", DisclosureBasis: "การประมวลผลตามสัญญา"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ropa.AddActivityTransfer(ctx, ropaservice.ActivityTransfer{ActivityID: a.ID, RecipientID: &rc.ID, CountryCode: "US", TransferBasis: "standard_clauses"}); err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func plainText(n render.Node, sb *strings.Builder) {
	if n.Type == "text" {
		sb.WriteString(n.Text)
	}
	for _, c := range n.Content {
		plainText(c, sb)
	}
}

// TestCreateWizard_ComposesFromActivity is PNG-01's acceptance criterion: a non-legal user who only picks a
// legal entity, notice type and the activity it covers gets a complete draft — every ม.23 topic present, filled
// in from the activity's own RoPA data rather than left as a placeholder — well inside the 30-minute budget
// since no manual authoring happens at all.
func TestCreateWizard_ComposesFromActivity(t *testing.T) {
	e := setup(t, "pngwiz")
	var le orgservice.LegalEntity
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", ContactEmail: "dpo@test.example", IsController: true}, 0)
		if err != nil {
			return err
		}
		activityID = seedActivity(t, ctx, e, le.ID)
		return nil
	})

	var n noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		n, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice",
			Title: "ประกาศความเป็นส่วนตัวสำหรับพนักงาน", Slug: "employee-notice", ActivityIDs: []uuid.UUID{activityID}})
		return err
	})
	if n.Status != "draft" {
		t.Fatalf("status = %q, want draft", n.Status)
	}
	if n.DocumentID == uuid.Nil {
		t.Fatal("expected a document to be created")
	}
	if len(n.ActivityIDs) != 1 || n.ActivityIDs[0] != activityID {
		t.Fatalf("activity_ids = %v", n.ActivityIDs)
	}

	e.in(t, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, n.DocumentID)
		if err != nil {
			return err
		}
		if doc.Draft == nil {
			t.Fatal("expected a draft")
		}
		var sb strings.Builder
		plainText(doc.Draft.Content["th"], &sb)
		text := sb.String()
		for _, want := range []string{
			"จ่ายเงินเดือนและสวัสดิการพนักงาน", // purpose
			"84 เดือน",                       // retention
			"ผู้ให้บริการคลาวด์ต่างประเทศ",         // recipient
			"standard_clauses",              // transfer basis
			"สิทธิของเจ้าของข้อมูลส่วนบุคคล",        // rights boilerplate
		} {
			if !strings.Contains(text, want) {
				t.Errorf("draft content missing %q", want)
			}
		}
		if strings.Contains(text, "[โปรดระบุวัตถุประสงค์และฐานทางกฎหมาย]") {
			t.Error("purpose placeholder should have been replaced by real activity data")
		}
		return nil
	})

	// Fetching again includes the linked activity.
	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetNotice(ctx, n.ID)
		if err != nil {
			return err
		}
		if len(got.ActivityIDs) != 1 {
			t.Errorf("GetNotice activity_ids = %v", got.ActivityIDs)
		}
		return nil
	})
}

// TestCreateWizard_NoActivities_Placeholders: with nothing linked, the draft is still complete — every topic
// present — just with bracketed placeholders instead of derived text, so the wizard never blocks on RoPA data
// existing yet.
func TestCreateWizard_NoActivities_Placeholders(t *testing.T) {
	e := setup(t, "pngwizempty")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		return err
	})
	var n noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		n, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "cookie_policy", Title: "นโยบายคุกกี้", Slug: "cookie-policy"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, n.DocumentID)
		if err != nil {
			return err
		}
		var sb strings.Builder
		plainText(doc.Draft.Content["th"], &sb)
		if !strings.Contains(sb.String(), "[โปรดระบุวัตถุประสงค์และฐานทางกฎหมาย]") {
			t.Error("expected a purpose placeholder when no activity is linked")
		}
		return nil
	})
}

func TestCreateWizard_Validation(t *testing.T) {
	e := setup(t, "pngwizval")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		return err
	})

	cases := []struct {
		name string
		in   noticeservice.WizardInput
	}{
		{"bad slug", noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "x", Slug: "Not A Slug!"}},
		{"bad notice_type", noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "not_a_type", Title: "x", Slug: "x"}},
		{"unknown legal entity", noticeservice.WizardInput{LegalEntityID: uuid.New(), NoticeType: "privacy_notice", Title: "x", Slug: "y"}},
		{"unknown activity", noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "x", Slug: "z", ActivityIDs: []uuid.UUID{uuid.New()}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e.in(t, func(ctx context.Context) error {
				_, err := e.svc.CreateWizard(ctx, c.in)
				if !errors.Is(err, noticeservice.ErrInvalid) {
					t.Errorf("err = %v, want ErrInvalid", err)
				}
				return nil
			})
		})
	}
}

func TestCreateWizard_DuplicateSlug(t *testing.T) {
	e := setup(t, "pngwizdup")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "หนึ่ง", Slug: "dup"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "สอง", Slug: "dup"})
		if !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("err = %v, want ErrInvalid (duplicate slug)", err)
		}
		return nil
	})
}

// TestCreateWizard_TenantIsolation: tenant B's transaction cannot see tenant A's legal entity or activity — the
// FK-visibility checks (rule 1) must refuse them exactly as they would an unknown id.
func TestCreateWizard_TenantIsolation(t *testing.T) {
	a := setup(t, "pngwiziso-a")
	b := setup(t, "pngwiziso-b")
	var le orgservice.LegalEntity
	var activityID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		var err error
		le, err = a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		activityID = seedActivity(t, ctx, a, le.ID)
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		_, err := b.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "x", Slug: "cross-tenant"})
		if !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("legal_entity_id across tenants: err = %v, want ErrInvalid", err)
		}
		return nil
	})
	var leB orgservice.LegalEntity
	b.in(t, func(ctx context.Context) error {
		var err error
		leB, err = b.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท บี จำกัด", IsController: true}, 0)
		return err
	})
	b.in(t, func(ctx context.Context) error {
		_, err := b.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: leB.ID, NoticeType: "privacy_notice", Title: "x", Slug: "cross-tenant-activity", ActivityIDs: []uuid.UUID{activityID}})
		if !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("activity_ids across tenants: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// lawfulBasisCode returns any lawful basis code, preferring one that doesn't require consent evidence so this
// fixture doesn't also need a consent.purposes row.
func lawfulBasisCode(t *testing.T, ctx context.Context, org *orgservice.Service) string {
	t.Helper()
	items, err := org.ListMaster(ctx, "lawful_bases")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if !it.RequiresConsent {
			return it.Code
		}
	}
	t.Fatal("expected at least one non-consent lawful basis to be seeded (ORG-07)")
	return ""
}
