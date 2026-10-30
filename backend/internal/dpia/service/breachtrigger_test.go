package service_test

import (
	"context"
	"testing"
)

// TestMarkActivityAssessmentForReview is DPIA-13's own dpia-side half: breach's own LinkActivity event
// marks the activity's latest approved DPIA round needs_review (ST-05#2's already-declared "ครบรอบ /
// กิจกรรมเปลี่ยน" edge) — a no-op for an activity with no DPIA at all, or whose latest round was never
// actually approved (screening/not_required/in_progress/in_review/rejected/needs_review/closed).
func TestMarkActivityAssessmentForReview(t *testing.T) {
	e := setup(t, "dpia13")

	// No DPIA at all for this activity: a harmless no-op, not an error.
	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "DPIA13-NODPIA")
		return e.svc.MarkActivityAssessmentForReview(ctx, act.ID, "เชื่อมโยงกับเหตุละเมิดข้อมูล")
	})

	// A round that is in_progress (never approved) has nothing to re-open: also a no-op.
	e.in(t, func(ctx context.Context) error {
		a := inProgress(t, ctx, e, "DPIA13-INPROGRESS")
		if err := e.svc.MarkActivityAssessmentForReview(ctx, a.ActivityID, "เชื่อมโยงกับเหตุละเมิดข้อมูล"); err != nil {
			return err
		}
		got, err := e.svc.GetAssessment(ctx, a.ID)
		if err != nil {
			return err
		}
		if got.Status != "in_progress" {
			t.Errorf("expected in_progress untouched, got %q", got.Status)
		}
		return nil
	})

	// The acceptance criterion itself: an approved round is marked needs_review.
	e.in(t, func(ctx context.Context) error {
		a := inProgress(t, ctx, e, "DPIA13-APPROVED")
		reviewing, err := e.svc.Transition(ctx, a.ID, a.RowVersion, "in_review", "")
		if err != nil {
			return err
		}
		approved, err := e.svc.Transition(ctx, reviewing.ID, reviewing.RowVersion, "approved", "")
		if err != nil {
			return err
		}
		if err := e.svc.MarkActivityAssessmentForReview(ctx, approved.ActivityID, "เชื่อมโยงกับเหตุละเมิดข้อมูล BR-2026-0001"); err != nil {
			return err
		}
		got, err := e.svc.GetAssessment(ctx, approved.ID)
		if err != nil {
			return err
		}
		if got.Status != "needs_review" {
			t.Errorf("expected needs_review after the breach link, got %q", got.Status)
		}

		// A second call afterward is a no-op (the round is no longer approved, so nothing to re-mark).
		if err := e.svc.MarkActivityAssessmentForReview(ctx, approved.ActivityID, "again"); err != nil {
			t.Errorf("second call should be a harmless no-op, got %v", err)
		}
		got2, err := e.svc.GetAssessment(ctx, approved.ID)
		if err != nil {
			return err
		}
		if got2.Status != "needs_review" || got2.RowVersion != got.RowVersion {
			t.Errorf("a repeat call must not change an already needs_review round: before=%+v after=%+v", got, got2)
		}
		return nil
	})
}

// TestMarkActivityAssessmentForReview_UnknownActivity: an activity id with no assessment history at all
// (never screened) is also a harmless no-op.
func TestMarkActivityAssessmentForReview_UnknownActivity(t *testing.T) {
	e := setup(t, "dpia13unknown")
	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "DPIA13-UNKNOWN")
		return e.svc.MarkActivityAssessmentForReview(ctx, act.ID, "เชื่อมโยงกับเหตุละเมิดข้อมูล")
	})
}
