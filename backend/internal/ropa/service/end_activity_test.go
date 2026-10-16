package service_test

import (
	"context"
	"errors"
	"testing"

	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestEndActivity is ROPA-14's own acceptance criterion: ending an active activity keeps its full history
// (every child row untouched) and takes it out of the default list, while status=ended still finds it.
func TestEndActivity(t *testing.T) {
	e := setup(t, "ropaend")
	var a ropaservice.Activity
	e.in(t, func(ctx context.Context) error { a = completeActivity(t, ctx, e, "END-01"); return nil })

	// Ending a draft activity is refused — only active -> ended is a valid edge.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.EndActivity(ctx, a.ID, a.RowVersion, "ไม่ใช้งานแล้ว"); !errors.Is(err, ropaservice.ErrInvalidTransition) {
			t.Errorf("end a draft activity: got %v, want ErrInvalidTransition", err)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		submitted, err := e.svc.SubmitActivity(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		a = submitted
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		approved, err := e.svc.ApproveActivity(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		if approved.Status != "active" {
			t.Fatalf("expected active, got %s", approved.Status)
		}
		a = approved
		return nil
	})

	// Empty reason is refused.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.EndActivity(ctx, a.ID, a.RowVersion, "  "); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("end with an empty reason: got %v, want ErrInvalid", err)
		}
		return nil
	})

	// Stale ETag is refused.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.EndActivity(ctx, a.ID, a.RowVersion+1, "ไม่ใช้งานแล้ว"); !errors.Is(err, ropaservice.ErrVersionMismatch) {
			t.Errorf("end with a stale version: got %v, want ErrVersionMismatch", err)
		}
		return nil
	})

	var ended ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		ended, err = e.svc.EndActivity(ctx, a.ID, a.RowVersion, "เลิกให้บริการระบบ HR เดิม")
		return err
	})
	if ended.Status != "ended" || ended.EndedAt == nil || ended.EndReason == nil || *ended.EndReason != "เลิกให้บริการระบบ HR เดิม" {
		t.Fatalf("expected ended with ended_at/end_reason set, got %+v", ended)
	}

	// The default list excludes it; every child row (purpose/data/retention/control) survives untouched.
	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListActivities(ctx, ropaservice.ActivityFilter{})
		if err != nil {
			return err
		}
		for _, x := range list {
			if x.ID == a.ID {
				t.Errorf("default list still shows the ended activity")
			}
		}
		retrospective, _, err := e.svc.ListActivities(ctx, ropaservice.ActivityFilter{Status: "ended"})
		if err != nil {
			return err
		}
		found := false
		for _, x := range retrospective {
			if x.ID == a.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("status=ended search doesn't find the ended activity")
		}
		got, err := e.svc.GetActivity(ctx, a.ID)
		if err != nil {
			return err
		}
		if got.Completeness != 100 {
			t.Errorf("ending must keep history intact: completeness = %d, want 100", got.Completeness)
		}
		return nil
	})

	// Double-ending (already ended) is refused as an invalid transition, not a version mismatch.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.EndActivity(ctx, a.ID, ended.RowVersion, "อีกครั้ง"); !errors.Is(err, ropaservice.ErrInvalidTransition) {
			t.Errorf("end an already-ended activity: got %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

// TestEndActivity_TwoTenantIsolation (rule 1): a tenant can't end another tenant's activity by id.
func TestEndActivity_TwoTenantIsolation(t *testing.T) {
	e1 := setup(t, "ropaendiso1")
	e2 := setup(t, "ropaendiso2")
	var a ropaservice.Activity
	e1.in(t, func(ctx context.Context) error { a = completeActivity(t, ctx, e1, "ISO-01"); return nil })
	e1.in(t, func(ctx context.Context) error {
		s, err := e1.svc.SubmitActivity(ctx, a.ID, a.RowVersion)
		a = s
		return err
	})
	e1.in(t, func(ctx context.Context) error {
		ap, err := e1.svc.ApproveActivity(ctx, a.ID, a.RowVersion)
		a = ap
		return err
	})

	e2.in(t, func(ctx context.Context) error {
		if _, err := e2.svc.EndActivity(ctx, a.ID, a.RowVersion, "ข้ามเทนแนนต์"); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("end another tenant's activity: got %v, want ErrNotFound", err)
		}
		return nil
	})
}
