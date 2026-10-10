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
	"pdpa-platform/internal/platform/forms"
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
				`DELETE FROM vendor.vendor_assessments`, `DELETE FROM assess.answers`, `DELETE FROM assess.assessments`,
				`DELETE FROM risk.risk_matrices`, `DELETE FROM vendor.vendors`, `DELETE FROM org.external_parties`,
				`DELETE FROM platform.audit_log`,
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
	vendorSvc := &vendorservice.Service{Audit: audit.New(), Org: org, Forms: formsSvc, Dpia: dpiaSvc, Risk: riskSvc}
	return assessEnv{app: app, tenant: tenant, svc: vendorSvc, org: org, dpia: dpiaSvc, risk: riskSvc}
}

var assessPerms = []string{"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update",
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
