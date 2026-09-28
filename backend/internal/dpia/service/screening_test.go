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
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

type env struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *dpiaservice.Service
	org    *orgservice.Service
	ropa   *ropaservice.Service
}

var perms = []string{"assessment.dpia.read", "assessment.dpia.create", "assessment.dpia.update",
	"assessment.template.read", "assessment.template.update"}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.answers`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.assessments`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.screening_rules`)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.processing_activities`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.org_units`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.legal_entities`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	org := &orgservice.Service{Audit: audit.New()}
	ropa := &ropaservice.Service{Audit: audit.New(), Org: org, Risk: riskservice.New()}
	formsSvc := wiring.Forms(nil, audit.New())
	svc := &dpiaservice.Service{Audit: audit.New(), Forms: formsSvc, Ropa: ropa}
	return env{app: app, tenant: tenant, svc: svc, org: org, ropa: ropa}
}

// in runs fn as the tenant's admin in one transaction (as the Tx middleware would).
func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: perms}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

// activity creates a minimal RoPA processing activity for a screening to target.
func (e env) activity(t *testing.T, ctx context.Context, code string) ropaservice.Activity {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: code, Name: "กิจกรรมทดสอบ", Role: "controller"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// allNo/allYes are the 6 seeded TDPG screening questions (migration 00047).
func allAnswers(v string) forms.Answers {
	return forms.Answers{"sensitive_data": v, "large_scale": v, "monitoring": v, "automated_decision": v, "new_tech": v, "vulnerable_groups": v}
}

// TestScreen_NotRequiredWhenNoFactors is half of DPIA-01's acceptance criterion: no high-risk factors means
// not_required, and the activity's status lands not_required (ST-05).
func TestScreen_NotRequiredWhenNoFactors(t *testing.T) {
	e := setup(t, "dpianone")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "NONE-01"); return nil })

	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		if a.ScreeningResult != "not_required" || a.Status != "not_required" {
			t.Errorf("got %q/%q, want not_required/not_required", a.ScreeningResult, a.Status)
		}
		if a.RoundNo != 1 || a.PreviousID != nil {
			t.Errorf("round: %+v", a)
		}
		if len(a.Factors) != 6 {
			t.Errorf("expected all 6 answers recorded, got %d", len(a.Factors))
		}
		return nil
	})
}

// TestScreen_RequiredAtDefaultThreshold is the other half: the default threshold (min_factors=2, Q-26) —
// two or more high-risk factors screens as required, moving the assessment to in_progress.
func TestScreen_RequiredAtDefaultThreshold(t *testing.T) {
	e := setup(t, "dpiareq")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "REQ-01"); return nil })

	e.in(t, func(ctx context.Context) error {
		answers := allAnswers("no")
		answers["sensitive_data"] = "yes"
		answers["large_scale"] = "yes"
		a, err := e.svc.Screen(ctx, act.ID, answers)
		if err != nil {
			return err
		}
		if a.ScreeningResult != "required" || a.Status != "in_progress" {
			t.Errorf("got %q/%q, want required/in_progress", a.ScreeningResult, a.Status)
		}
		return nil
	})
}

// TestScreen_RecommendedBelowThreshold: exactly one factor is short of the default threshold of 2 —
// "recommended", not "required", and still in_progress (a human should still look at it).
func TestScreen_RecommendedBelowThreshold(t *testing.T) {
	e := setup(t, "dpiarec")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "REC-01"); return nil })

	e.in(t, func(ctx context.Context) error {
		answers := allAnswers("no")
		answers["new_tech"] = "yes"
		a, err := e.svc.Screen(ctx, act.ID, answers)
		if err != nil {
			return err
		}
		if a.ScreeningResult != "recommended" || a.Status != "in_progress" {
			t.Errorf("got %q/%q, want recommended/in_progress", a.ScreeningResult, a.Status)
		}
		return nil
	})
}

// TestSaveRules_ChangesNextScreeningRound is DPIA-02's acceptance criterion: lowering the threshold to 1
// changes what a fresh screening round of the same single-factor answers yields, without touching the
// template itself.
func TestSaveRules_ChangesNextScreeningRound(t *testing.T) {
	e := setup(t, "dpiarule")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "RULE-01"); return nil })

	answers := allAnswers("no")
	answers["monitoring"] = "yes"

	// Round 1, default threshold (2): one factor is only "recommended".
	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.Screen(ctx, act.ID, answers)
		if err != nil {
			return err
		}
		if a.ScreeningResult != "recommended" {
			t.Fatalf("round 1: %q, want recommended", a.ScreeningResult)
		}
		return nil
	})

	// Lower the threshold to 1.
	e.in(t, func(ctx context.Context) error {
		r, err := e.svc.SaveRules(ctx, 1, nil)
		if err != nil {
			return err
		}
		if r.MinFactors != 1 {
			t.Errorf("min_factors = %d, want 1", r.MinFactors)
		}
		return nil
	})

	// Round 2, same answers, new threshold: now required, and it's a new round chained to round 1.
	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.Screen(ctx, act.ID, answers)
		if err != nil {
			return err
		}
		if a.ScreeningResult != "required" {
			t.Errorf("round 2: %q, want required", a.ScreeningResult)
		}
		if a.RoundNo != 2 || a.PreviousID == nil {
			t.Errorf("round 2 chaining: %+v", a)
		}
		return nil
	})
}

// TestSaveRules_RejectsOutOfRange is DPIA-02's own input validation.
func TestSaveRules_RejectsOutOfRange(t *testing.T) {
	e := setup(t, "dpiabounds")
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveRules(ctx, 0, nil); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("min_factors=0: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.SaveRules(ctx, 7, nil); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("min_factors=7: %v, want ErrInvalid", err)
		}
		bad := -1.0
		if _, err := e.svc.SaveRules(ctx, 2, &bad); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("min_score<0: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestScreen_UnknownActivityRefused proves the activity FK is checked under the caller's own RLS before
// anything is written (rule 1).
func TestScreen_UnknownActivityRefused(t *testing.T) {
	e := setup(t, "dpiaunknown")
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.Screen(ctx, uuid.New(), allAnswers("no")); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("unknown activity: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestScreen_TwoTenantIsolation proves tenant B never sees tenant A's screening rounds, and can't screen
// tenant A's activity (it isn't visible under its own RLS).
func TestScreen_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpiaisoA")
	b := setup(t, "dpiaisoB")
	var act ropaservice.Activity
	var assessmentID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		act = a.activity(t, ctx, "ISO-01")
		got, err := a.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		assessmentID = got.ID
		return nil
	})

	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetAssessment(ctx, assessmentID); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant GetAssessment: %v, want ErrNotFound", err)
		}
		if _, err := b.svc.Screen(ctx, act.ID, allAnswers("no")); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("cross-tenant Screen: %v, want ErrInvalid", err)
		}
		return nil
	})
}
