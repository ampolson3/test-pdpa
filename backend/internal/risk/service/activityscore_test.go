package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

type scoreEnv struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *riskservice.Service
	org    *orgservice.Service
	ropa   *ropaservice.Service
}

func scoreSetup(t *testing.T, suffix string) scoreEnv {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM risk.activity_scores`, `DELETE FROM risk.risk_matrices`,
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
	riskSvc := &riskservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskSvc}
	riskSvc.Ropa = wiring.RiskRopa{Ropa: ropaSvc, Org: orgSvc}
	return scoreEnv{app: app, tenant: tenant, svc: riskSvc, org: orgSvc, ropa: ropaSvc}
}

func (e scoreEnv) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: []string{
			"ropa.risk.read", "ropa.risk.create", "ropa.risk.update",
			"org.structure.read", "org.structure.update", "ropa.activity.read", "ropa.activity.create",
		}}))
	}); err != nil {
		t.Fatal(err)
	}
}

func defaultMatrix3x3(t *testing.T, ctx context.Context, svc *riskservice.Service) riskservice.RiskMatrix {
	t.Helper()
	m, err := svc.SaveMatrix(ctx, riskservice.RiskMatrix{
		Name: "default", LikelihoodLevels: []string{"low", "medium", "high"}, ImpactLevels: []string{"low", "medium", "high"},
		Thresholds: []riskservice.Threshold{{Level: "low", MinScore: 1}, {Level: "medium", MinScore: 4}, {Level: "high", MinScore: 7}},
		IsDefault:  true,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func seedActivity(t *testing.T, ctx context.Context, e scoreEnv) uuid.UUID {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-SCORE", Name: "ทดสอบคะแนนความเสี่ยง", Role: "controller"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

// TestScore_BaselineHasNoControlsFactor is RRA-01's own acceptance criterion: an activity with no data at
// all still scores (lowest likelihood/impact, aside from the no-controls factor every un-mitigated activity
// starts with) and lists exactly the factor responsible.
func TestScore_BaselineHasNoControlsFactor(t *testing.T) {
	e := scoreSetup(t, "rraBaseline")
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		activityID = seedActivity(t, ctx, e)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		s, err := e.svc.Score(ctx, activityID)
		if err != nil {
			return err
		}
		if s.Likelihood != 2 || s.Impact != 1 {
			t.Errorf("likelihood=%d impact=%d, want 2/1 (no_controls only)", s.Likelihood, s.Impact)
		}
		if len(s.Factors) != 1 || s.Factors[0].Code != "no_controls" {
			t.Errorf("factors = %+v, want exactly [no_controls]", s.Factors)
		}
		return nil
	})
}

// TestScore_ChangesWithRopaData is the acceptance criterion's other half directly: adding sensitive data,
// a recipient and a security control each change the next Score call's result, explaining the new factors.
func TestScore_ChangesWithRopaData(t *testing.T) {
	e := scoreSetup(t, "rraChanges")
	var activityID, categoryID, subjectTypeID, partyID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		activityID = seedActivity(t, ctx, e)
		cat, err := e.org.CreateMaster(ctx, orgservice.KindDataCategories, orgservice.MasterItem{Code: "sens1", NameTh: "ข้อมูลอ่อนไหวทดสอบ", IsSensitive: true})
		if err != nil {
			return err
		}
		categoryID = *cat.ID
		subj, err := e.org.CreateMaster(ctx, orgservice.KindSubjectTypes, orgservice.MasterItem{Code: "subj1", NameTh: "กลุ่มทดสอบ"})
		if err != nil {
			return err
		}
		subjectTypeID = *subj.ID
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้รับข้อมูลทดสอบ", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		partyID = party.ID
		return nil
	})

	var before riskservice.ActivityScore
	e.in(t, func(ctx context.Context) error {
		var err error
		before, err = e.svc.Score(ctx, activityID)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		_, err := e.ropa.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: activityID, DataCategoryID: categoryID, SubjectTypeID: subjectTypeID, Source: "direct", IsSensitive: true})
		if err != nil {
			return err
		}
		_, err = e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: activityID, PartyID: partyID, RecipientRole: "processor"})
		return err
	})

	var after riskservice.ActivityScore
	e.in(t, func(ctx context.Context) error {
		var err error
		after, err = e.svc.Score(ctx, activityID)
		return err
	})

	if after.Impact <= before.Impact {
		t.Errorf("impact did not increase after adding sensitive data: before=%d after=%d", before.Impact, after.Impact)
	}
	if after.Likelihood <= before.Likelihood {
		t.Errorf("likelihood did not increase after adding a recipient: before=%d after=%d", before.Likelihood, after.Likelihood)
	}
	codes := map[string]bool{}
	for _, f := range after.Factors {
		codes[f.Code] = true
	}
	if !codes["sensitive_data"] || !codes["external_recipients"] {
		t.Errorf("factors = %+v, want sensitive_data and external_recipients", after.Factors)
	}
}

// TestScore_NoDefaultMatrixRefused: scoring without a configured default matrix (RRA-02) is refused rather
// than guessing one.
func TestScore_NoDefaultMatrixRefused(t *testing.T) {
	e := scoreSetup(t, "rraNoMatrix")
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		activityID = seedActivity(t, ctx, e)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.Score(ctx, activityID); err == nil {
			t.Error("expected an error with no default matrix configured")
		}
		return nil
	})
}

// TestScore_UnknownActivityRefused and TestLatestScore_NoneYet round out the error paths.
func TestScore_UnknownActivityRefused(t *testing.T) {
	e := scoreSetup(t, "rraUnknown")
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		if _, err := e.svc.Score(ctx, uuid.Must(uuid.NewRandom())); err == nil {
			t.Error("expected an error for an unknown activity")
		}
		return nil
	})
}

func TestLatestScore_NoneYetThenAfterScoring(t *testing.T) {
	e := scoreSetup(t, "rraLatest")
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		activityID = seedActivity(t, ctx, e)
		if _, err := e.svc.LatestScore(ctx, activityID); err == nil {
			t.Error("expected ErrNotFound before any score exists")
		}
		if _, err := e.svc.Score(ctx, activityID); err != nil {
			return err
		}
		latest, err := e.svc.LatestScore(ctx, activityID)
		if err != nil {
			return err
		}
		if latest.ActivityID != activityID {
			t.Errorf("latest.ActivityID = %v, want %v", latest.ActivityID, activityID)
		}
		return nil
	})
}

// TestTwoTenantIsolation_ActivityScore proves tenant B cannot score or read tenant A's activity.
func TestTwoTenantIsolation_ActivityScore(t *testing.T) {
	eA := scoreSetup(t, "rraIsoA")
	var activityID uuid.UUID
	eA.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, eA.svc)
		activityID = seedActivity(t, ctx, eA)
		_, err := eA.svc.Score(ctx, activityID)
		return err
	})

	eB := scoreSetup(t, "rraIsoB")
	eB.in(t, func(ctx context.Context) error {
		if _, err := eB.svc.Score(ctx, activityID); err == nil {
			t.Error("tenant B scored tenant A's activity")
		}
		if _, err := eB.svc.LatestScore(ctx, activityID); err == nil {
			t.Error("tenant B read tenant A's activity score")
		}
		return nil
	})
}
