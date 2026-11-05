package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

func necessityAnswers(v string) map[string]any {
	return map[string]any{
		"minimal_data": v, "purpose_specific": v, "lawful_basis_appropriate": v, "less_invasive_considered": v,
	}
}

// screenToInProgress is a small helper: screen an activity with enough high-risk factors to land in_progress,
// which is what every DPIA-05 test needs before it can answer the necessity checklist.
func screenToInProgress(t *testing.T, e env, ctx context.Context, act ropaservice.Activity) dpiaservice.Assessment {
	t.Helper()
	answers := allAnswers("no")
	answers["sensitive_data"] = "yes"
	answers["large_scale"] = "yes"
	a, err := e.svc.Screen(ctx, act.ID, answers)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestAssessNecessity_AllYesIsNecessary is DPIA-05's acceptance criterion: answering the checklist fully
// (every question "yes") gives an immediate "necessary" conclusion with nothing missing.
func TestAssessNecessity_AllYesIsNecessary(t *testing.T) {
	e := setup(t, "dpianecyes")
	var act ropaservice.Activity
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act = e.activity(t, ctx, "NEC-YES-01")
		a := screenToInProgress(t, e, ctx, act)
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		n, err := e.svc.AssessNecessity(ctx, assessmentID, necessityAnswers("yes"))
		if err != nil {
			return err
		}
		if n.Result != "necessary" || len(n.Missing) != 0 {
			t.Errorf("got %+v, want necessary with nothing missing", n)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetNecessity(ctx, assessmentID)
		if err != nil {
			return err
		}
		if got.Result != "necessary" {
			t.Errorf("re-read: got %q, want necessary", got.Result)
		}
		return nil
	})
}

// TestAssessNecessity_AnyNoNeedsReview: one "no" answer flags that item and the checklist reads needs_review.
func TestAssessNecessity_AnyNoNeedsReview(t *testing.T) {
	e := setup(t, "dpianecno")
	var act ropaservice.Activity
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act = e.activity(t, ctx, "NEC-NO-01")
		a := screenToInProgress(t, e, ctx, act)
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		answers := necessityAnswers("yes")
		answers["less_invasive_considered"] = "no"
		n, err := e.svc.AssessNecessity(ctx, assessmentID, answers)
		if err != nil {
			return err
		}
		if n.Result != "needs_review" || len(n.Missing) != 1 || n.Missing[0] != "less_invasive_considered" {
			t.Errorf("got %+v, want needs_review flagging less_invasive_considered", n)
		}
		return nil
	})
}

// TestAssessNecessity_ResubmitReplaces: answering again replaces the prior submission rather than
// accumulating duplicate answers or a second round.
func TestAssessNecessity_ResubmitReplaces(t *testing.T) {
	e := setup(t, "dpianecredo")
	var act ropaservice.Activity
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act = e.activity(t, ctx, "NEC-REDO-01")
		a := screenToInProgress(t, e, ctx, act)
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		answers := necessityAnswers("yes")
		answers["minimal_data"] = "no"
		if _, err := e.svc.AssessNecessity(ctx, assessmentID, answers); err != nil {
			return err
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		n, err := e.svc.AssessNecessity(ctx, assessmentID, necessityAnswers("yes"))
		if err != nil {
			return err
		}
		if n.Result != "necessary" || len(n.Missing) != 0 {
			t.Errorf("after redo: got %+v, want necessary", n)
		}
		if len(n.Answers) != 4 {
			t.Errorf("expected exactly 4 answers after redo (no duplicates), got %d", len(n.Answers))
		}
		return nil
	})
}

// TestAssessNecessity_DoesNotAffectScreeningFactors proves the section_id split: answering the necessity
// checklist never changes what GetAssessment reports as the screening's own Factors.
func TestAssessNecessity_DoesNotAffectScreeningFactors(t *testing.T) {
	e := setup(t, "dpianecfactors")
	var act ropaservice.Activity
	var assessmentID uuid.UUID
	var factorsBefore int
	e.in(t, func(ctx context.Context) error {
		act = e.activity(t, ctx, "NEC-FAC-01")
		a := screenToInProgress(t, e, ctx, act)
		assessmentID = a.ID
		factorsBefore = len(a.Factors)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AssessNecessity(ctx, assessmentID, necessityAnswers("yes")); err != nil {
			return err
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetAssessment(ctx, assessmentID)
		if err != nil {
			return err
		}
		if len(got.Factors) != factorsBefore {
			t.Errorf("screening factors changed after necessity was answered: got %d, want %d", len(got.Factors), factorsBefore)
		}
		return nil
	})
}

// TestGetNecessity_NotFoundBeforeAnswered: reading before ever answering is a plain 404, not an empty result.
func TestGetNecessity_NotFoundBeforeAnswered(t *testing.T) {
	e := setup(t, "dpianecunanswered")
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "NEC-UNANS-01")
		a := screenToInProgress(t, e, ctx, act)
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.GetNecessity(ctx, assessmentID); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestAssessNecessity_TwoTenantIsolation proves tenant B can't answer or read tenant A's necessity checklist.
func TestAssessNecessity_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpianeciso_A")
	b := setup(t, "dpianeciso_B")
	var assessmentID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		act := a.activity(t, ctx, "NEC-ISO-01")
		got := screenToInProgress(t, a, ctx, act)
		assessmentID = got.ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.AssessNecessity(ctx, assessmentID, necessityAnswers("yes")); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant AssessNecessity: %v, want ErrNotFound", err)
		}
		if _, err := b.svc.GetNecessity(ctx, assessmentID); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant GetNecessity: %v, want ErrNotFound", err)
		}
		return nil
	})
}
