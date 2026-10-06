package service_test

import (
	"context"
	"errors"
	"testing"

	dpiaservice "pdpa-platform/internal/dpia/service"
)

// inProgress screens an activity into "in_progress" (required result) so ST-05#2's review/approval edges
// (DPIA-10) have something to act on.
func inProgress(t *testing.T, ctx context.Context, e env, code string) dpiaservice.Assessment {
	t.Helper()
	act := e.activity(t, ctx, code)
	answers := allAnswers("no")
	answers["sensitive_data"] = "yes"
	answers["large_scale"] = "yes"
	a, err := e.svc.Screen(ctx, act.ID, answers)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestTransition_FollowsST05 proves the allowed edges work and a disallowed one is refused — in_progress
// can't jump straight to approved, skipping in_review.
func TestTransition_FollowsST05(t *testing.T) {
	e := setup(t, "dpiatrans1")
	var a dpiaservice.Assessment
	e.in(t, func(ctx context.Context) error { a = inProgress(t, ctx, e, "TR-01"); return nil })

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.Transition(ctx, a.ID, a.RowVersion, "approved", ""); !errors.Is(err, dpiaservice.ErrInvalidTransition) {
			t.Errorf("in_progress -> approved: %v, want ErrInvalidTransition", err)
		}
		reviewing, err := e.svc.Transition(ctx, a.ID, a.RowVersion, "in_review", "")
		if err != nil {
			return err
		}
		if reviewing.Status != "in_review" {
			t.Errorf("status: %q, want in_review", reviewing.Status)
		}
		// rejected without a reason is refused.
		if _, err := e.svc.Transition(ctx, a.ID, reviewing.RowVersion, "rejected", ""); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("rejected without reason: %v, want ErrInvalid", err)
		}
		// a stale version is refused.
		if _, err := e.svc.Transition(ctx, a.ID, a.RowVersion, "approved", ""); !errors.Is(err, dpiaservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

// TestTransition_ApproveNeedsPermission is DPIA-10's actor line taken literally: a caller with only
// assessment.dpia.update (no .approve) can submit for review, but can't decide or close.
func TestTransition_ApproveNeedsPermission(t *testing.T) {
	e := setup(t, "dpiatrans2")
	var a dpiaservice.Assessment
	e.in(t, func(ctx context.Context) error { a = inProgress(t, ctx, e, "TR-02"); return nil })

	var reviewing dpiaservice.Assessment
	e.inWithoutApprove(t, func(ctx context.Context) error {
		var err error
		reviewing, err = e.svc.Transition(ctx, a.ID, a.RowVersion, "in_review", "")
		return err
	})
	e.inWithoutApprove(t, func(ctx context.Context) error {
		if _, err := e.svc.Transition(ctx, reviewing.ID, reviewing.RowVersion, "approved", ""); !errors.Is(err, dpiaservice.ErrForbidden) {
			t.Errorf("approve without .approve: %v, want ErrForbidden", err)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.Transition(ctx, reviewing.ID, reviewing.RowVersion, "approved", ""); err != nil {
			t.Errorf("approve with .approve: %v", err)
		}
		return nil
	})
}

// TestClose_RequiresAnOpinionUnlessNotRequired is DPIA-10's acceptance criterion directly: a round that was
// actually assessed (approved/rejected) can't close without at least one recorded DPO opinion; a
// not_required round (nothing to opine on) closes with none.
func TestClose_RequiresAnOpinionUnlessNotRequired(t *testing.T) {
	e := setup(t, "dpiaclose")
	var noOpinion, withOpinion, notRequired dpiaservice.Assessment
	e.in(t, func(ctx context.Context) error {
		a1 := inProgress(t, ctx, e, "CL-01")
		reviewing1, err := e.svc.Transition(ctx, a1.ID, a1.RowVersion, "in_review", "")
		if err != nil {
			return err
		}
		noOpinion, err = e.svc.Transition(ctx, reviewing1.ID, reviewing1.RowVersion, "approved", "")
		if err != nil {
			return err
		}

		a2 := inProgress(t, ctx, e, "CL-02")
		reviewing2, err := e.svc.Transition(ctx, a2.ID, a2.RowVersion, "in_review", "")
		if err != nil {
			return err
		}
		if _, err := e.svc.RecordOpinion(ctx, reviewing2.ID, "เห็นควรดำเนินการ", "proceed"); err != nil {
			return err
		}
		withOpinion, err = e.svc.Transition(ctx, reviewing2.ID, reviewing2.RowVersion, "approved", "")
		if err != nil {
			return err
		}

		act := e.activity(t, ctx, "CL-03")
		notRequired, err = e.svc.Screen(ctx, act.ID, allAnswers("no"))
		return err
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.Transition(ctx, noOpinion.ID, noOpinion.RowVersion, "closed", ""); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("close without an opinion: %v, want ErrInvalid", err)
		}
		closed, err := e.svc.Transition(ctx, withOpinion.ID, withOpinion.RowVersion, "closed", "")
		if err != nil {
			t.Errorf("close with an opinion: %v", err)
		} else if closed.Status != "closed" {
			t.Errorf("status: %q, want closed", closed.Status)
		}
		// not_required never had anything to opine on — closes with none.
		if _, err := e.svc.Transition(ctx, notRequired.ID, notRequired.RowVersion, "closed", ""); err != nil {
			t.Errorf("close not_required: %v", err)
		}
		return nil
	})
}

// TestRecordOpinion_ValidatesAndGatesOnStatus: an opinion can only be recorded on a round under review, and
// must carry a real recommendation value.
func TestRecordOpinion_ValidatesAndGatesOnStatus(t *testing.T) {
	e := setup(t, "dpiaopinion")
	var notRequired dpiaservice.Assessment
	var inReview dpiaservice.Assessment
	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "OP-01")
		var err error
		notRequired, err = e.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		a := inProgress(t, ctx, e, "OP-02")
		inReview, err = e.svc.Transition(ctx, a.ID, a.RowVersion, "in_review", "")
		return err
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordOpinion(ctx, notRequired.ID, "x", "proceed"); !errors.Is(err, dpiaservice.ErrInvalidTransition) {
			t.Errorf("opinion on not_required: %v, want ErrInvalidTransition", err)
		}
		if _, err := e.svc.RecordOpinion(ctx, inReview.ID, "", "proceed"); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("empty opinion: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.RecordOpinion(ctx, inReview.ID, "x", "bogus"); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("bad recommendation: %v, want ErrInvalid", err)
		}
		o, err := e.svc.RecordOpinion(ctx, inReview.ID, "เห็นควรดำเนินการแบบมีเงื่อนไข", "proceed_with_conditions")
		if err != nil {
			return err
		}
		list, err := e.svc.ListOpinions(ctx, inReview.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != o.ID {
			t.Errorf("list opinions: %+v", list)
		}
		return nil
	})
}

// TestTransition_Isolation: tenant B's transaction can't act on tenant A's assessment round.
func TestTransition_Isolation(t *testing.T) {
	a := setup(t, "dpiatransa")
	b := setup(t, "dpiatransb")
	var assessment dpiaservice.Assessment
	a.in(t, func(ctx context.Context) error { assessment = inProgress(t, ctx, a, "ISO-01"); return nil })

	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.Transition(ctx, assessment.ID, assessment.RowVersion, "in_review", ""); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("tenant B transitioning tenant A's assessment: %v, want ErrNotFound", err)
		}
		if _, err := b.svc.RecordOpinion(ctx, assessment.ID, "x", "proceed"); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("tenant B opinion on tenant A's assessment: %v, want ErrNotFound", err)
		}
		return nil
	})
}
