package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

func liaAnswers(v string) map[string]any {
	return map[string]any{
		"purpose_legitimate": v, "necessary_for_purpose": v, "not_overridden_by_rights": v, "safeguards_in_place": v,
	}
}

// TestAssessLIA_OpensInProgressRound is DPIA-16's own answer step: a fresh round starts in_progress, never
// already approved — approval is DecideLIA's own job.
func TestAssessLIA_OpensInProgressRound(t *testing.T) {
	e := setup(t, "dpialiaopen")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error { act = e.activity(t, ctx, "LIA-OPEN-01"); return nil })

	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.AssessLIA(ctx, act.ID, liaAnswers("yes"))
		if err != nil {
			return err
		}
		if a.Status != "in_progress" || a.RoundNo != 1 || a.PreviousID != nil {
			t.Errorf("got %+v, want a fresh in_progress round 1", a)
		}
		if len(a.Factors) != 4 {
			t.Errorf("expected all 4 answers recorded, got %d", len(a.Factors))
		}
		return nil
	})
}

// TestAssessLIA_ReanswerOpensNewRound: answering again always opens a new round, chained via previous_id —
// never edits the prior round in place.
func TestAssessLIA_ReanswerOpensNewRound(t *testing.T) {
	e := setup(t, "dpialiaredo")
	var act ropaservice.Activity
	var first uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act = e.activity(t, ctx, "LIA-REDO-01")
		a, err := e.svc.AssessLIA(ctx, act.ID, liaAnswers("yes"))
		if err != nil {
			return err
		}
		first = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.AssessLIA(ctx, act.ID, liaAnswers("no"))
		if err != nil {
			return err
		}
		if a.RoundNo != 2 || a.PreviousID == nil || *a.PreviousID != first {
			t.Errorf("got %+v, want round 2 chained to round 1 (%s)", a, first)
		}
		return nil
	})
}

// TestDecideLIA_ApproveAndReject is the acceptance criterion's other half: a DPO decides an answered round,
// and HasApprovedLIA reads true only once approved.
func TestDecideLIA_ApproveAndReject(t *testing.T) {
	e := setup(t, "dpialiadecide")
	var act ropaservice.Activity
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act = e.activity(t, ctx, "LIA-DEC-01")
		a, err := e.svc.AssessLIA(ctx, act.ID, liaAnswers("yes"))
		if err != nil {
			return err
		}
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		ok, err := e.svc.HasApprovedLIA(ctx, act.ID)
		if err != nil {
			return err
		}
		if ok {
			t.Error("not yet approved, want false")
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.DecideLIA(ctx, assessmentID, 1, "approved", "")
		if err != nil {
			return err
		}
		if a.Status != "approved" {
			t.Errorf("got %q, want approved", a.Status)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		ok, err := e.svc.HasApprovedLIA(ctx, act.ID)
		if err != nil {
			return err
		}
		if !ok {
			t.Error("approved round on record, want true")
		}
		return nil
	})
}

// TestDecideLIA_RejectNeedsReason mirrors DPIA-10's own rejected/needs_review guard.
func TestDecideLIA_RejectNeedsReason(t *testing.T) {
	e := setup(t, "dpialiareject")
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "LIA-REJ-01")
		a, err := e.svc.AssessLIA(ctx, act.ID, liaAnswers("yes"))
		if err != nil {
			return err
		}
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.DecideLIA(ctx, assessmentID, 1, "rejected", ""); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("no reason: got %v, want ErrInvalid", err)
		}
		a, err := e.svc.DecideLIA(ctx, assessmentID, 1, "rejected", "ไม่มีผลประโยชน์ที่ชัดเจน")
		if err != nil {
			return err
		}
		if a.Status != "rejected" {
			t.Errorf("got %q, want rejected", a.Status)
		}
		return nil
	})
}

// TestDecideLIA_RequiresApprovePermission: a caller holding only assessment.dpia.update cannot decide.
func TestDecideLIA_RequiresApprovePermission(t *testing.T) {
	e := setup(t, "dpialianoperm")
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "LIA-NOPERM-01")
		a, err := e.svc.AssessLIA(ctx, act.ID, liaAnswers("yes"))
		if err != nil {
			return err
		}
		assessmentID = a.ID
		return nil
	})
	e.inWithoutApprove(t, func(ctx context.Context) error {
		if _, err := e.svc.DecideLIA(ctx, assessmentID, 1, "approved", ""); !errors.Is(err, dpiaservice.ErrForbidden) {
			t.Errorf("got %v, want ErrForbidden", err)
		}
		return nil
	})
}

// TestDecideLIA_InvalidTransition: a decided round can't be decided again.
func TestDecideLIA_InvalidTransition(t *testing.T) {
	e := setup(t, "dpialiainvalid")
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "LIA-INV-01")
		a, err := e.svc.AssessLIA(ctx, act.ID, liaAnswers("yes"))
		if err != nil {
			return err
		}
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.DecideLIA(ctx, assessmentID, 1, "approved", ""); err != nil {
			return err
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.DecideLIA(ctx, assessmentID, 2, "rejected", "เปลี่ยนใจ"); !errors.Is(err, dpiaservice.ErrInvalidTransition) {
			t.Errorf("got %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

// TestHasApprovedLIA_UnknownActivityIsFalse: no LIA round at all reads false, not an error.
func TestHasApprovedLIA_UnknownActivityIsFalse(t *testing.T) {
	e := setup(t, "dpialianone")
	e.in(t, func(ctx context.Context) error {
		ok, err := e.svc.HasApprovedLIA(ctx, uuid.New())
		if err != nil {
			return err
		}
		if ok {
			t.Error("no round on record, want false")
		}
		return nil
	})
}

// TestListLIAForActivity_NewestFirst lists every round, newest first.
func TestListLIAForActivity_NewestFirst(t *testing.T) {
	e := setup(t, "dpialialist")
	var act ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		act = e.activity(t, ctx, "LIA-LIST-01")
		if _, err := e.svc.AssessLIA(ctx, act.ID, liaAnswers("yes")); err != nil {
			return err
		}
		if _, err := e.svc.AssessLIA(ctx, act.ID, liaAnswers("no")); err != nil {
			return err
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListLIAForActivity(ctx, act.ID)
		if err != nil {
			return err
		}
		if len(list) != 2 || list[0].RoundNo != 2 || list[1].RoundNo != 1 {
			t.Errorf("got %+v, want 2 rounds newest first", list)
		}
		return nil
	})
}

// TestAssessLIA_TwoTenantIsolation proves tenant B can't answer, decide or read tenant A's LIA rounds.
func TestAssessLIA_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpialiaiso_A")
	b := setup(t, "dpialiaiso_B")
	var activityID, assessmentID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		act := a.activity(t, ctx, "LIA-ISO-01")
		activityID = act.ID
		got, err := a.svc.AssessLIA(ctx, act.ID, liaAnswers("yes"))
		if err != nil {
			return err
		}
		assessmentID = got.ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.AssessLIA(ctx, activityID, liaAnswers("yes")); !errors.Is(err, dpiaservice.ErrInvalid) {
			t.Errorf("cross-tenant AssessLIA: %v, want ErrInvalid (activity not visible)", err)
		}
		if _, err := b.svc.DecideLIA(ctx, assessmentID, 1, "approved", ""); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant DecideLIA: %v, want ErrNotFound", err)
		}
		if _, err := b.svc.GetLIA(ctx, assessmentID); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant GetLIA: %v, want ErrNotFound", err)
		}
		ok, err := b.svc.HasApprovedLIA(ctx, activityID)
		if err != nil {
			return err
		}
		if ok {
			t.Error("tenant B sees tenant A's activity as having an approved LIA, want false")
		}
		return nil
	})
}
