package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dpiaservice "pdpa-platform/internal/dpia/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/platform/jobs"
	riskservice "pdpa-platform/internal/risk/service"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
	"pdpa-platform/internal/wiring"
)

// assessEnv wires org + risk + dpia + vendor together — VEN-07's own acceptance criterion needs all four:
// a vendor (vendor), a published VEN-04 template answered through dpia's own generic
// RecordSubjectAssessment, and a risk matrix (RRA-02) to classify the resulting score against.
type assessEnv struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *vendorservice.Service
	org    *orgservice.Service
	dpia   *dpiaservice.Service
	risk   *riskservice.Service
}

func assessSetup(t *testing.T, suffix string) assessEnv {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM vendor.remediation_items`, `DELETE FROM vendor.vendor_assessments`, `DELETE FROM assess.answers`,
				`DELETE FROM assess.assessments`, `DELETE FROM risk.risk_matrices`, `DELETE FROM vendor.vendors`,
				`DELETE FROM org.external_parties`, `DELETE FROM platform.audit_log`, `DELETE FROM platform.outbox_events`,
				`DELETE FROM river_job WHERE kind = 'outbox.dispatch'`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	org := &orgservice.Service{Audit: audit.New()}
	riskSvc := riskservice.New()
	riskSvc.Audit = audit.New()
	formsSvc := wiring.Forms(nil, audit.New())
	dpiaSvc := &dpiaservice.Service{Forms: formsSvc, Org: org, Audit: audit.New(), Risk: riskSvc}
	riverClient, err := jobs.NewInsertClient(app)
	if err != nil {
		t.Fatal(err)
	}
	vendorSvc := &vendorservice.Service{Audit: audit.New(), Org: org, Forms: formsSvc, Dpia: dpiaSvc, Risk: riskSvc,
		Events: &events.Publisher{River: riverClient}}
	return assessEnv{app: app, tenant: tenant, svc: vendorSvc, org: org, dpia: dpiaSvc, risk: riskSvc}
}

var assessPerms = []string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update",
	"vendor.assessment.read", "vendor.assessment.create", "vendor.assessment.approve",
	"assessment.template.read", "ropa.risk.read", "ropa.risk.create", "ropa.risk.update"}

func (e assessEnv) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: assessPerms}))
	}); err != nil {
		t.Fatal(err)
	}
}

func seedAssessMatrix(t *testing.T, ctx context.Context, risk *riskservice.Service) {
	t.Helper()
	_, err := risk.SaveMatrix(ctx, riskservice.RiskMatrix{
		Name: "default", LikelihoodLevels: []string{"low", "medium", "high"}, ImpactLevels: []string{"low", "medium", "high"},
		Thresholds: []riskservice.Threshold{{Level: "low", MinScore: 1}, {Level: "medium", MinScore: 4}, {Level: "high", MinScore: 7}},
		IsDefault:  true,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
}

func seedAssessVendor(t *testing.T, ctx context.Context, e assessEnv) vendorservice.Vendor {
	t.Helper()
	party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "คู่ค้าทดสอบ VEN-07", CountryCode: "US"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ทดสอบการให้คะแนนประเมิน"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// vendor_pdpa (migration 00053) has 5 yes_no questions, each worth 1 point for "yes" and 0 for "no" — a
// plain weighted sum, exercised directly here.
var allYes = forms.Answers{"has_dpo": "yes", "has_retention_policy": "yes", "has_breach_process": "yes",
	"has_subprocessor_list": "yes", "has_dpa_signed": "yes"}
var allNo = forms.Answers{"has_dpo": "no", "has_retention_policy": "no", "has_breach_process": "no",
	"has_subprocessor_list": "no", "has_dpa_signed": "no"}

// TestRecordAssessment_ScoresByWeightAndClassifiesResidualRisk is VEN-07's own acceptance criterion:
// "คะแนนคำนวณถูกต้องตามน้ำหนักทุกกรณีทดสอบ" — the score is exactly the sum of the chosen options' own
// weights in every case, and the residual risk level it classifies to moves with that score.
func TestRecordAssessment_ScoresByWeightAndClassifiesResidualRisk(t *testing.T) {
	e := assessSetup(t, "ven07score")
	var vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		seedAssessMatrix(t, ctx, e.risk)
		vendorID = seedAssessVendor(t, ctx, e).ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		best, err := e.svc.RecordAssessment(ctx, vendorID, "vendor_pdpa", allYes)
		if err != nil {
			return err
		}
		if best.Score != 5 {
			t.Errorf("all-yes score = %v, want 5 (every question's own weight)", best.Score)
		}
		if best.ResidualLevel != "low" {
			t.Errorf("all-yes residual_level = %q, want low (ratio 0 floors at the lowest threshold)", best.ResidualLevel)
		}
		if best.CycleNo != 1 {
			t.Errorf("first cycle = %d, want 1", best.CycleNo)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		worst, err := e.svc.RecordAssessment(ctx, vendorID, "vendor_pdpa", allNo)
		if err != nil {
			return err
		}
		if worst.Score != 0 {
			t.Errorf("all-no score = %v, want 0", worst.Score)
		}
		if worst.ResidualLevel != "high" {
			t.Errorf("all-no residual_level = %q, want high (worst possible answers)", worst.ResidualLevel)
		}
		if worst.CycleNo != 2 {
			t.Errorf("second cycle = %d, want 2", worst.CycleNo)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListAssessments(ctx, vendorID)
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Fatalf("expected 2 recorded cycles, got %d", len(list))
		}
		if list[0].CycleNo != 2 || list[1].CycleNo != 1 {
			t.Errorf("expected newest-first order, got cycles %d, %d", list[0].CycleNo, list[1].CycleNo)
		}
		got, err := e.svc.GetAssessment(ctx, list[0].ID)
		if err != nil {
			return err
		}
		if got.ID != list[0].ID {
			t.Errorf("GetAssessment returned a different row")
		}
		return nil
	})
}

func TestRecordAssessment_UnknownTemplateCodeRefused(t *testing.T) {
	e := assessSetup(t, "ven07badtemplate")
	var vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		seedAssessMatrix(t, ctx, e.risk)
		vendorID = seedAssessVendor(t, ctx, e).ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordAssessment(ctx, vendorID, "no_such_template", allYes); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("unknown template_code: %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestRecordAssessment_NoMatrixConfiguredRefused(t *testing.T) {
	e := assessSetup(t, "ven07nomatrix")
	var vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		vendorID = seedAssessVendor(t, ctx, e).ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordAssessment(ctx, vendorID, "vendor_pdpa", allYes); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("no risk matrix configured: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestRecordAssessment_TwoTenantIsolation mirrors VEN-01's own isolation test.
func TestRecordAssessment_TwoTenantIsolation(t *testing.T) {
	a := assessSetup(t, "ven07iso1")
	b := assessSetup(t, "ven07iso2")
	var vendorID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		seedAssessMatrix(t, ctx, a.risk)
		vendorID = seedAssessVendor(t, ctx, a).ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		seedAssessMatrix(t, ctx, b.risk)
		if _, err := b.svc.RecordAssessment(ctx, vendorID, "vendor_pdpa", allYes); !errors.Is(err, vendorservice.ErrNotFound) {
			t.Errorf("tenant B recording an assessment against tenant A's vendor: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// seedAssessedVendor seeds a vendor matrix, creates a vendor, and answers vendor_pdpa once — the vendor
// lands "onboarding" (ST-06#1's own "prospect -> onboarding" edge, driven by RecordAssessment) with one
// undecided assessment cycle, which is exactly VEN-08's own starting point.
func seedAssessedVendor(t *testing.T, ctx context.Context, e assessEnv, answers forms.Answers) (vendorservice.Vendor, vendorservice.VendorAssessment) {
	t.Helper()
	seedAssessMatrix(t, ctx, e.risk)
	v := seedAssessVendor(t, ctx, e)
	a, err := e.svc.RecordAssessment(ctx, v.ID, "vendor_pdpa", answers)
	if err != nil {
		t.Fatal(err)
	}
	v, err = e.svc.GetVendor(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "onboarding" {
		t.Fatalf("vendor status after its first assessment = %q, want onboarding", v.Status)
	}
	return v, a
}

// TestDecideAssessment_ApprovedTransitionsVendorAndPublishesEvent covers the clean-approval half of
// VEN-08: the vendor moves onboarding -> approved (ST-06#1), approved_at is stamped, and the already-
// catalogued "vendor.approved" event (docs/architecture/events.yaml) is published.
func TestDecideAssessment_ApprovedTransitionsVendorAndPublishesEvent(t *testing.T) {
	e := assessSetup(t, "ven08approve")
	var vendorID, assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		v, a := seedAssessedVendor(t, ctx, e, allYes)
		vendorID, assessmentID = v.ID, a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		decided, items, err := e.svc.DecideAssessment(ctx, assessmentID, "approved", nil)
		if err != nil {
			return err
		}
		if decided.Decision != "approved" {
			t.Errorf("decision = %q, want approved", decided.Decision)
		}
		if decided.DecidedBy == nil {
			t.Errorf("decided_by not set")
		}
		if len(items) != 0 {
			t.Errorf("approved decision created %d remediation items, want 0", len(items))
		}
		v, err := e.svc.GetVendor(ctx, vendorID)
		if err != nil {
			return err
		}
		if v.Status != "approved" {
			t.Errorf("vendor status = %q, want approved", v.Status)
		}
		if v.ApprovedAt == nil {
			t.Errorf("approved_at not stamped")
		}
		var n int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*)::int FROM platform.outbox_events WHERE event_type = 'vendor.approved' AND aggregate_id = $1`, vendorID).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("vendor.approved outbox rows = %d, want 1", n)
		}
		return nil
	})
}

// TestDecideAssessment_ConditionalOpensRemediationItems is VEN-08's own literal acceptance criterion:
// "อนุมัติแบบมีเงื่อนไขสร้างแผนแก้ไขอัตโนมัติ" — a conditional decision automatically opens the given
// remediation items, the vendor lands "conditional" (ST-06#1), and no "vendor.approved" event fires.
func TestDecideAssessment_ConditionalOpensRemediationItems(t *testing.T) {
	e := assessSetup(t, "ven08conditional")
	var vendorID, assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		v, a := seedAssessedVendor(t, ctx, e, allNo)
		vendorID, assessmentID = v.ID, a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		decided, items, err := e.svc.DecideAssessment(ctx, assessmentID, "conditional", []vendorservice.RemediationItemInput{
			{Title: "ต้องมี DPA ลงนามก่อนเริ่มงาน", Severity: "high"},
			{Title: "ต้องมีนโยบายเก็บรักษาข้อมูลเป็นลายลักษณ์อักษร", Severity: "medium"},
		})
		if err != nil {
			return err
		}
		if decided.Decision != "conditional" {
			t.Errorf("decision = %q, want conditional", decided.Decision)
		}
		if len(items) != 2 {
			t.Fatalf("remediation items created = %d, want 2", len(items))
		}
		for _, it := range items {
			if it.Status != "open" || it.VendorAssessmentID == nil || *it.VendorAssessmentID != assessmentID {
				t.Errorf("remediation item %q: status=%q vendor_assessment_id=%v, want open linked to %s", it.Title, it.Status, it.VendorAssessmentID, assessmentID)
			}
		}
		v, err := e.svc.GetVendor(ctx, vendorID)
		if err != nil {
			return err
		}
		if v.Status != "conditional" {
			t.Errorf("vendor status = %q, want conditional", v.Status)
		}
		if v.ApprovedAt != nil {
			t.Errorf("approved_at stamped on a conditional decision")
		}
		var n int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*)::int FROM platform.outbox_events WHERE event_type = 'vendor.approved' AND aggregate_id = $1`, vendorID).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("vendor.approved outbox rows = %d, want 0 for a conditional decision", n)
		}
		list, err := e.svc.ListRemediationItems(ctx, vendorID)
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Errorf("ListRemediationItems = %d, want 2", len(list))
		}
		return nil
	})
}

func TestDecideAssessment_ConditionalWithoutItemsRefused(t *testing.T) {
	e := assessSetup(t, "ven08noitems")
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		_, a := seedAssessedVendor(t, ctx, e, allNo)
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.DecideAssessment(ctx, assessmentID, "conditional", nil); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("conditional with no remediation items: %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestDecideAssessment_RejectedTransitionsVendor(t *testing.T) {
	e := assessSetup(t, "ven08reject")
	var vendorID, assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		v, a := seedAssessedVendor(t, ctx, e, allNo)
		vendorID, assessmentID = v.ID, a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.DecideAssessment(ctx, assessmentID, "rejected", nil); err != nil {
			return err
		}
		v, err := e.svc.GetVendor(ctx, vendorID)
		if err != nil {
			return err
		}
		if v.Status != "rejected" {
			t.Errorf("vendor status = %q, want rejected", v.Status)
		}
		return nil
	})
}

// TestDecideAssessment_CannotRedecide is the guard behind "WHERE decision IS NULL": a cycle already
// decided cannot be decided again.
func TestDecideAssessment_CannotRedecide(t *testing.T) {
	e := assessSetup(t, "ven08redecide")
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		_, a := seedAssessedVendor(t, ctx, e, allYes)
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.DecideAssessment(ctx, assessmentID, "approved", nil); err != nil {
			return err
		}
		if _, _, err := e.svc.DecideAssessment(ctx, assessmentID, "rejected", nil); !errors.Is(err, vendorservice.ErrInvalidTransition) {
			t.Errorf("re-deciding an already-decided cycle: %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

// TestDecideAssessment_RequiresOnboardingStatus: a vendor not currently "onboarding" (e.g. still
// "prospect" — no assessment ever recorded) cannot be decided.
func TestDecideAssessment_RequiresOnboardingStatus(t *testing.T) {
	e := assessSetup(t, "ven08notonboarding")
	var vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		seedAssessMatrix(t, ctx, e.risk)
		vendorID = seedAssessVendor(t, ctx, e).ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		// Force an assessment row to decide against without ever onboarding the vendor (simulates a
		// direct cross-tenant/edge-case row — RecordAssessment always onboards, so this exercises the
		// guard DecideAssessment itself must still hold).
		a, err := e.svc.RecordAssessment(ctx, vendorID, "vendor_pdpa", allYes)
		if err != nil {
			return err
		}
		q := `UPDATE vendor.vendors SET status = 'prospect' WHERE id = $1`
		if _, err := pdb.MustTxFromContext(ctx).Exec(ctx, q, vendorID); err != nil {
			return err
		}
		if _, _, err := e.svc.DecideAssessment(ctx, a.ID, "approved", nil); !errors.Is(err, vendorservice.ErrInvalidTransition) {
			t.Errorf("deciding against a non-onboarding vendor: %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

// TestDecideAssessment_TwoTenantIsolation mirrors VEN-07's own isolation test.
func TestDecideAssessment_TwoTenantIsolation(t *testing.T) {
	a := assessSetup(t, "ven08iso1")
	b := assessSetup(t, "ven08iso2")
	var assessmentID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		_, as := seedAssessedVendor(t, ctx, a, allYes)
		assessmentID = as.ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, _, err := b.svc.DecideAssessment(ctx, assessmentID, "approved", nil); !errors.Is(err, vendorservice.ErrNotFound) {
			t.Errorf("tenant B deciding tenant A's assessment: %v, want ErrNotFound", err)
		}
		return nil
	})
}
