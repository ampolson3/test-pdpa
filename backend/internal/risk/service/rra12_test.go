package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestRescoreOnActivityChange is RRA-12's own acceptance criterion directly: a RoPA activity's score is
// recomputed and persisted the moment this is called (internal/wiring/events.go's own ropa.activity_approved
// subscriber calls it synchronously, so "within 1 minute" is trivially satisfied).
func TestRescoreOnActivityChange(t *testing.T) {
	e := scoreSetup(t, "rra12")
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		activityID = seedActivity(t, ctx, e)
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.LatestScore(ctx, activityID); err == nil {
			t.Error("expected ErrNotFound before any score exists")
		}
		if err := e.svc.RescoreOnActivityChange(ctx, activityID); err != nil {
			t.Fatalf("RescoreOnActivityChange: %v", err)
		}
		if _, err := e.svc.LatestScore(ctx, activityID); err != nil {
			t.Errorf("expected a score to now exist: %v", err)
		}
		return nil
	})
}

// TestRescoreOnActivityChange_NoMatrixIsHarmless: a tenant that never configured a default risk matrix
// (RRA-02) must not have the whole ropa.activity_approved event chain fail because of it.
func TestRescoreOnActivityChange_NoMatrixIsHarmless(t *testing.T) {
	e := scoreSetup(t, "rra12nomatrix")
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		activityID = seedActivity(t, ctx, e)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.RescoreOnActivityChange(ctx, activityID); err != nil {
			t.Errorf("expected a harmless no-op with no default matrix configured, got %v", err)
		}
		return nil
	})
}

// TestRescoreOnActivityChange_UnknownActivityIsHarmless: a stale/redelivered event naming an activity that
// no longer exists (or was never visible to this tenant) must not fail the subscriber either.
func TestRescoreOnActivityChange_UnknownActivityIsHarmless(t *testing.T) {
	e := scoreSetup(t, "rra12unknown")
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		if err := e.svc.RescoreOnActivityChange(ctx, uuid.Must(uuid.NewRandom())); err != nil {
			t.Errorf("expected a harmless no-op for an unknown activity, got %v", err)
		}
		return nil
	})
}
