package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dpiaservice "pdpa-platform/internal/dpia/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

// triggerEnv wires a real riskSvc AND a real dpiaSvc together exactly as cmd/api/main.go does
// (riskSvc.DpiaTrigger = dpiaSvc) — proving RRA-03's cross-module wiring actually works end to end,
// not just dpiaservice.Service.TriggerFromRiskScore in isolation.
type triggerEnv struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	risk   *riskservice.Service
	dpia   *dpiaservice.Service
	org    *orgservice.Service
	ropa   *ropaservice.Service
}

func triggerSetup(t *testing.T, suffix string) triggerEnv {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM assess.answers`, `DELETE FROM assess.assessments`, `DELETE FROM assess.screening_rules`,
				`DELETE FROM risk.activity_scores`, `DELETE FROM risk.risk_matrices`,
				`DELETE FROM ropa.activity_transfers`, `DELETE FROM ropa.activity_recipients`, `DELETE FROM ropa.activity_data`,
				`DELETE FROM ropa.processing_activities`, `DELETE FROM org.external_parties`, `DELETE FROM org.org_units`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	riskSvc := &riskservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskSvc}
	riskSvc.Ropa = wiring.RiskRopa{Ropa: ropaSvc, Org: orgSvc}
	formsSvc := wiring.Forms(nil, audit.New())
	dpiaSvc := &dpiaservice.Service{Audit: audit.New(), Forms: formsSvc, Ropa: ropaSvc, Org: orgSvc}
	riskSvc.DpiaTrigger = dpiaSvc
	return triggerEnv{app: app, tenant: tenant, risk: riskSvc, dpia: dpiaSvc, org: orgSvc, ropa: ropaSvc}
}

func (e triggerEnv) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	perms := []string{"ropa.risk.read", "ropa.risk.create", "ropa.risk.update", "org.structure.read", "org.structure.update",
		"ropa.activity.read", "ropa.activity.create", "assessment.dpia.read", "assessment.dpia.create", "assessment.dpia.update",
		"assessment.template.read", "assessment.template.update"}
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: perms}))
	}); err != nil {
		t.Fatal(err)
	}
}

// TestScore_OpensDpiaEndToEnd is RRA-03's own acceptance criterion exercised through the real wiring: an
// activity with enough risk factors to score "high" against the default 3x3 matrix opens a real DPIA round
// via the exact riskSvc.DpiaTrigger = dpiaSvc assignment cmd/api/main.go makes — not a mock.
func TestScore_OpensDpiaEndToEnd(t *testing.T) {
	e := triggerSetup(t, "rra03e2e")
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.risk)

		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		act, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-HIGH", Name: "กิจกรรมความเสี่ยงสูง", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = act.ID

		cat, err := e.org.CreateMaster(ctx, orgservice.KindDataCategories, orgservice.MasterItem{Code: "sens1", NameTh: "ข้อมูลอ่อนไหวทดสอบ", IsSensitive: true})
		if err != nil {
			return err
		}
		subj, err := e.org.CreateMaster(ctx, orgservice.KindSubjectTypes, orgservice.MasterItem{Code: "vuln1", NameTh: "กลุ่มเปราะบางทดสอบ", IsVulnerable: true})
		if err != nil {
			return err
		}
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้รับข้อมูลทดสอบ", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityData(ctx, ropaservice.ActivityData{
			ActivityID: activityID, DataCategoryID: *cat.ID, SubjectTypeID: *subj.ID, Source: "direct",
			IsSensitive: true, VolumeBand: "gt_100k",
		}); err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: activityID, PartyID: party.ID, RecipientRole: "processor"}); err != nil {
			return err
		}
		_, err = e.ropa.AddActivityTransfer(ctx, ropaservice.ActivityTransfer{ActivityID: activityID, CountryCode: "US", TransferBasis: "standard_clauses"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		s, err := e.risk.Score(ctx, activityID)
		if err != nil {
			return err
		}
		if s.Level != "high" && s.Level != "very_high" {
			t.Fatalf("score level = %q, want high or very_high (score=%v)", s.Level, s.Score)
		}

		rows, _, err := e.dpia.ListAssessments(ctx, dpiaservice.AssessmentFilter{ActivityID: &activityID})
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("got %d DPIA assessments after a %q score, want exactly 1", len(rows), s.Level)
		}
		a := rows[0]
		if a.Status != "in_progress" || a.ScreeningResult != "required" {
			t.Errorf("status/result = %q/%q, want in_progress/required", a.Status, a.ScreeningResult)
		}
		if a.RiskLevel != s.Level {
			t.Errorf("assessment risk_level = %q, want %q (the risk engine's own level)", a.RiskLevel, s.Level)
		}
		return nil
	})
}
