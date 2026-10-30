package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dpiaservice "pdpa-platform/internal/dpia/service"
	dposervice "pdpa-platform/internal/dpo/service"
	govservice "pdpa-platform/internal/gov/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

type dpo10Env struct {
	app         *pgxpool.Pool
	tenant      dbtest.Tenant
	legalEntity uuid.UUID
	svc         *dposervice.Service
	ropa        *ropaservice.Service
	risk        *riskservice.Service
	org         *orgservice.Service
}

func dpo10Setup(t *testing.T, suffix string) dpo10Env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM risk.gap_findings`, `DELETE FROM assess.answers`, `DELETE FROM assess.assessments`,
				`DELETE FROM gov.training_attempts`, `DELETE FROM gov.training_assignments`, `DELETE FROM gov.courses`,
				`DELETE FROM ropa.processing_activities`, `DELETE FROM org.org_units`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc}
	riskSvc := &riskservice.Service{Audit: audit.New()}
	riskSvc.Ropa = wiring.RiskRopa{Ropa: ropaSvc, Org: orgSvc}
	dpiaSvc := &dpiaservice.Service{Audit: audit.New()}
	govSvc := &govservice.Service{Audit: audit.New()}
	dpoSvc := &dposervice.Service{Audit: audit.New(), Org: orgSvc, Ropa: ropaSvc, Risk: riskSvc, Dpia: dpiaSvc, Gov: govSvc}

	var legalEntity uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"org.structure.update"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท DPO-10 จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntity = le.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	return dpo10Env{app: app, tenant: tenant, legalEntity: legalEntity, svc: dpoSvc, ropa: ropaSvc, risk: riskSvc, org: orgSvc}
}

var dpo10Perms = []string{"dpo.report.read", "ropa.activity.read", "ropa.activity.create", "ropa.risk.read", "ropa.risk.create",
	"org.structure.read", "org.structure.update"}

func (e dpo10Env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: dpo10Perms}))
	}); err != nil {
		t.Fatal(err)
	}
}

// TestOrgUnitScorecard_ExplainsEveryComponent is DPO-10's own acceptance criterion directly ("คะแนน
// คำนวณซ้ำได้และอธิบายที่มาได้"): a bare activity under a fresh org unit produces a scorecard whose every
// component is named, and recomputing it twice gives the identical result (reproducible, never cached).
func TestOrgUnitScorecard_ExplainsEveryComponent(t *testing.T) {
	e := dpo10Setup(t, "dpo10bare")
	var unitID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: e.legalEntity, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		unitID = unit.ID
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: e.legalEntity, OrgUnitID: unit.ID, Code: "SC-01",
			Name: "กิจกรรมทดสอบ", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		_, err = e.risk.AnalyzeActivity(ctx, activityID)
		return err
	})

	var first dposervice.Scorecard
	e.in(t, func(ctx context.Context) error {
		sc, err := e.svc.OrgUnitScorecard(ctx, unitID)
		if err != nil {
			return err
		}
		first = sc
		return nil
	})

	if first.OrgUnitID != unitID {
		t.Errorf("org_unit_id = %v, want %v", first.OrgUnitID, unitID)
	}
	names := map[string]float64{}
	for _, c := range first.Components {
		names[c.Name] = c.Score
		if c.Detail == "" {
			t.Errorf("component %q has no explanation", c.Name)
		}
	}
	for _, want := range []string{"ropa_completeness", "legal_readiness", "dpia_readiness", "training_completion"} {
		if _, ok := names[want]; !ok {
			t.Errorf("expected a %q component, got %+v", want, first.Components)
		}
	}
	if names["ropa_completeness"] >= 100 {
		t.Errorf("ropa_completeness = %v, want less than 100 for a bare activity", names["ropa_completeness"])
	}
	if names["legal_readiness"] != 70 {
		t.Errorf("legal_readiness = %v, want 70 (100 - 20 no_lawful_basis - 10 no_retention)", names["legal_readiness"])
	}
	if names["dpia_readiness"] != 0 {
		t.Errorf("dpia_readiness = %v, want 0 (never screened)", names["dpia_readiness"])
	}
	if names["training_completion"] != 100 {
		t.Errorf("training_completion = %v, want 100 (no assignments)", names["training_completion"])
	}

	// Reproducibility: recomputing from the same, unchanged data gives the identical score.
	e.in(t, func(ctx context.Context) error {
		again, err := e.svc.OrgUnitScorecard(ctx, unitID)
		if err != nil {
			return err
		}
		if again.Overall != first.Overall {
			t.Errorf("recomputed overall = %v, want %v (reproducible)", again.Overall, first.Overall)
		}
		return nil
	})
}

func TestOrgUnitScorecard_NoActivitiesIsHarmless(t *testing.T) {
	e := dpo10Setup(t, "dpo10empty")
	var unitID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: e.legalEntity, Code: "EMPTY", NameTh: "Empty", UnitType: "department"})
		if err != nil {
			return err
		}
		unitID = unit.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		sc, err := e.svc.OrgUnitScorecard(ctx, unitID)
		if err != nil {
			return err
		}
		for _, c := range sc.Components {
			if c.Name == "ropa_completeness" {
				t.Error("expected no ropa_completeness component with zero activities")
			}
			if c.Name == "dpia_readiness" {
				t.Error("expected no dpia_readiness component with zero activities")
			}
		}
		return nil
	})
}

func TestOrgUnitScorecard_TwoTenantIsolation(t *testing.T) {
	a := dpo10Setup(t, "dpo10isoa")
	b := dpo10Setup(t, "dpo10isob")
	var unitID, activityID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: a.legalEntity, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		unitID = unit.ID
		act, err := a.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: a.legalEntity, OrgUnitID: unit.ID, Code: "ISO-01",
			Name: "กิจกรรม A", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = act.ID
		_, err = a.risk.AnalyzeActivity(ctx, activityID)
		return err
	})

	b.in(t, func(ctx context.Context) error {
		sc, err := b.svc.OrgUnitScorecard(ctx, unitID)
		if err != nil {
			return err
		}
		for _, c := range sc.Components {
			if c.Name == "ropa_completeness" || c.Name == "dpia_readiness" {
				t.Error("tenant B's scorecard saw tenant A's activity")
			}
		}
		return nil
	})
}
