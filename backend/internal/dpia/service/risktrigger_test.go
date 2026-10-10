package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// activityWithOwner is like env.activity but sets OwnerUserID to the tenant's own seeded user — RRA-03's
// own acceptance criterion needs a real owner to assign automatically.
func (e env) activityWithOwner(t *testing.T, ctx context.Context, code string) ropaservice.Activity {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	owner := e.tenant.UserID
	// SeedTenant's user starts "invited" (iam.users' own default) — AddActivityOwner's FK-visibility check
	// goes through iamservice.Names, which only resolves "active" users, so activate it for this test.
	if _, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE iam.users SET status = 'active' WHERE id = $1`, owner); err != nil {
		t.Fatal(err)
	}
	a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{
		LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: code, Name: "กิจกรรมทดสอบ", Role: "controller", OwnerUserID: &owner,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestTriggerFromRiskScore_HighOpensInProgressWithOwner is RRA-03's own acceptance criterion: a "high"
// score opens a DPIA directly at in_progress, screening_result forced to required, risk_level/score carried
// over, and the activity's own owner assigned automatically.
func TestTriggerFromRiskScore_HighOpensInProgressWithOwner(t *testing.T) {
	e := setup(t, "rra03high")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activityWithOwner(t, ctx, "RRA03-01"); return nil })

	e.in(t, func(ctx context.Context) error {
		if err := e.svc.TriggerFromRiskScore(ctx, act.ID, 12.5, "high"); err != nil {
			return err
		}
		rows, _, err := e.svc.ListAssessments(ctx, dpiaservice.AssessmentFilter{ActivityID: &act.ID})
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("got %d assessments, want 1", len(rows))
		}
		a := rows[0]
		if a.Status != "in_progress" || a.ScreeningResult != "required" {
			t.Errorf("status/result = %q/%q, want in_progress/required", a.Status, a.ScreeningResult)
		}
		if a.RiskLevel != "high" || a.Score != 12.5 {
			t.Errorf("risk_level/score = %q/%v, want high/12.5", a.RiskLevel, a.Score)
		}
		if a.OwnerUserID == nil || *a.OwnerUserID != e.tenant.UserID {
			t.Errorf("owner_user_id = %v, want %v", a.OwnerUserID, e.tenant.UserID)
		}
		if a.RoundNo != 1 || a.PreviousID != nil {
			t.Errorf("round: %+v", a)
		}
		return nil
	})
}

// TestTriggerFromRiskScore_LowLevelIsNoOp proves medium/low/unknown levels never open a DPIA.
func TestTriggerFromRiskScore_LowLevelIsNoOp(t *testing.T) {
	e := setup(t, "rra03low")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "RRA03-02"); return nil })

	e.in(t, func(ctx context.Context) error {
		for _, level := range []string{"low", "medium", ""} {
			if err := e.svc.TriggerFromRiskScore(ctx, act.ID, 1, level); err != nil {
				t.Fatalf("level %q: %v", level, err)
			}
		}
		rows, _, err := e.svc.ListAssessments(ctx, dpiaservice.AssessmentFilter{ActivityID: &act.ID})
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Errorf("got %d assessments, want 0", len(rows))
		}
		return nil
	})
}

// TestTriggerFromRiskScore_SkipsWhenAlreadyOpen is the idempotency rule: Score() is called on every page
// view, so a second high score while a round is still open (in_progress) must not spawn a duplicate.
func TestTriggerFromRiskScore_SkipsWhenAlreadyOpen(t *testing.T) {
	e := setup(t, "rra03dup")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "RRA03-03"); return nil })

	e.in(t, func(ctx context.Context) error { return e.svc.TriggerFromRiskScore(ctx, act.ID, 10, "high") })
	e.in(t, func(ctx context.Context) error { return e.svc.TriggerFromRiskScore(ctx, act.ID, 11, "very_high") })

	e.in(t, func(ctx context.Context) error {
		rows, _, err := e.svc.ListAssessments(ctx, dpiaservice.AssessmentFilter{ActivityID: &act.ID})
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("got %d assessments, want 1 (no duplicate while one is still open)", len(rows))
		}
		return nil
	})
}

// TestTriggerFromRiskScore_NewRoundAfterClosed proves a fresh high score after the prior round left the
// open set (e.g. closed/not_required) opens a new, chained round rather than being silently skipped.
func TestTriggerFromRiskScore_NewRoundAfterClosed(t *testing.T) {
	e := setup(t, "rra03chain")
	var act ropaservice.Activity
	var firstID uuid.UUID
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "RRA03-04"); return nil })
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.TriggerFromRiskScore(ctx, act.ID, 10, "high"); err != nil {
			return err
		}
		rows, _, err := e.svc.ListAssessments(ctx, dpiaservice.AssessmentFilter{ActivityID: &act.ID})
		if err != nil {
			return err
		}
		firstID = rows[0].ID
		return nil
	})

	// Decide and close the round so it leaves the open set (TriggerFromRiskScore's own idempotency guard).
	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetAssessment(ctx, firstID)
		if err != nil {
			return err
		}
		got, err = e.svc.Transition(ctx, firstID, got.RowVersion, "in_review", "")
		if err != nil {
			return err
		}
		if _, err := e.svc.RecordOpinion(ctx, firstID, "เห็นควรดำเนินการ", "proceed"); err != nil {
			return err
		}
		got, err = e.svc.Transition(ctx, firstID, got.RowVersion, "approved", "")
		if err != nil {
			return err
		}
		_, err = e.svc.Transition(ctx, firstID, got.RowVersion, "closed", "")
		return err
	})

	e.in(t, func(ctx context.Context) error {
		if err := e.svc.TriggerFromRiskScore(ctx, act.ID, 15, "very_high"); err != nil {
			return err
		}
		rows, _, err := e.svc.ListAssessments(ctx, dpiaservice.AssessmentFilter{ActivityID: &act.ID})
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("got %d assessments, want 2", len(rows))
		}
		var second dpiaservice.Assessment
		for _, r := range rows {
			if r.ID != firstID {
				second = r
			}
		}
		if second.RoundNo != 2 || second.PreviousID == nil || *second.PreviousID != firstID {
			t.Errorf("round 2 chaining: %+v", second)
		}
		return nil
	})
}

// TestTriggerFromRiskScore_UnknownActivityRefused proves the activity FK is checked under the caller's own
// RLS before anything is written (rule 1).
func TestTriggerFromRiskScore_UnknownActivityRefused(t *testing.T) {
	e := setup(t, "rra03unknown")
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.TriggerFromRiskScore(ctx, uuid.New(), 10, "high"); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("unknown activity: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestTriggerFromRiskScore_TwoTenantIsolation proves tenant B's activity id (even a real one, from its own
// tenant) is invisible to tenant A's call, and that each tenant's own trigger only ever affects its own rows.
func TestTriggerFromRiskScore_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "rra03isoA")
	b := setup(t, "rra03isoB")
	var actA, actB ropaservice.Activity
	a.in(t, func(ctx context.Context) error { actA = a.activity(t, ctx, "ISO-01"); return nil })
	b.in(t, func(ctx context.Context) error { actB = b.activity(t, ctx, "ISO-01"); return nil })

	a.in(t, func(ctx context.Context) error { return a.svc.TriggerFromRiskScore(ctx, actA.ID, 10, "high") })

	b.in(t, func(ctx context.Context) error {
		if err := b.svc.TriggerFromRiskScore(ctx, actA.ID, 10, "high"); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("cross-tenant activity id: %v, want ErrInvalid", err)
		}
		rows, _, err := b.svc.ListAssessments(ctx, dpiaservice.AssessmentFilter{ActivityID: &actB.ID})
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Errorf("tenant B sees %d assessments, want 0", len(rows))
		}
		return nil
	})
}
