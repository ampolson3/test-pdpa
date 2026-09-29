package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestCompareToPrevious_FirstRoundIsEmpty: a round with no PreviousID has nothing to diff against.
func TestCompareToPrevious_FirstRoundIsEmpty(t *testing.T) {
	e := setup(t, "dpiadiff1")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "DIFF-01"); return nil })

	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		d, err := e.svc.CompareToPrevious(ctx, a.ID)
		if err != nil {
			return err
		}
		if d.PreviousID != nil {
			t.Errorf("previous_id = %v, want nil", d.PreviousID)
		}
		if len(d.Changes) != 0 {
			t.Errorf("changes = %+v, want empty", d.Changes)
		}
		return nil
	})
}

// TestCompareToPrevious_FlagsChangedAnswer is the acceptance criterion: re-screening with one changed answer
// shows exactly that question's before/after in the diff, and leaves unchanged questions out.
func TestCompareToPrevious_FlagsChangedAnswer(t *testing.T) {
	e := setup(t, "dpiadiff2")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "DIFF-02"); return nil })

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.Screen(ctx, act.ID, allAnswers("no")); err != nil {
			return err
		}
		return nil
	})

	var second dpiaservice.Assessment
	e.in(t, func(ctx context.Context) error {
		answers := allAnswers("no")
		answers["sensitive_data"] = "yes"
		a, err := e.svc.Screen(ctx, act.ID, answers)
		if err != nil {
			return err
		}
		second = a
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		d, err := e.svc.CompareToPrevious(ctx, second.ID)
		if err != nil {
			return err
		}
		if d.PreviousID == nil {
			t.Fatalf("previous_id = nil, want the first round's id")
		}
		if len(d.Changes) != 1 {
			t.Fatalf("changes = %+v, want exactly 1", d.Changes)
		}
		c := d.Changes[0]
		if c.Question != "sensitive_data" {
			t.Errorf("question = %q, want sensitive_data", c.Question)
		}
		if c.Before != "no" || c.After != "yes" {
			t.Errorf("before/after = %v/%v, want no/yes", c.Before, c.After)
		}
		return nil
	})
}

// TestCompareToPrevious_UnchangedReScreenIsEmpty: identical answers across two rounds diff to nothing.
func TestCompareToPrevious_UnchangedReScreenIsEmpty(t *testing.T) {
	e := setup(t, "dpiadiff3")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "DIFF-03"); return nil })

	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.Screen(ctx, act.ID, allAnswers("no"))
		return err
	})
	var second dpiaservice.Assessment
	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		second = a
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		d, err := e.svc.CompareToPrevious(ctx, second.ID)
		if err != nil {
			return err
		}
		if len(d.Changes) != 0 {
			t.Errorf("changes = %+v, want empty for an unchanged re-screen", d.Changes)
		}
		return nil
	})
}

// TestCompareToPrevious_UnknownAssessmentRefused proves the assessment is resolved under the caller's own
// RLS (GetAssessment), not trusted blindly.
func TestCompareToPrevious_UnknownAssessmentRefused(t *testing.T) {
	e := setup(t, "dpiadiff4")
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.CompareToPrevious(ctx, uuid.New()); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("unknown assessment: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestCompareToPrevious_TwoTenantIsolation proves tenant B can't diff tenant A's assessment.
func TestCompareToPrevious_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpiadiffisoA")
	b := setup(t, "dpiadiffisoB")
	var assessmentID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		act := a.activity(t, ctx, "DIFF-ISO-01")
		got, err := a.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		assessmentID = got.ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.CompareToPrevious(ctx, assessmentID); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant CompareToPrevious: %v, want ErrNotFound", err)
		}
		return nil
	})
}
