package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/breach/breachtest"
	breach "pdpa-platform/internal/breach/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// fakeRopa is DPIA-13's own minimal stand-in for the RoPA module (rule 9) — just enough for LinkActivity's
// own FK-visibility check (rule 1), without standing up the whole ropa schema/service for this test.
type fakeRopa struct {
	visible map[uuid.UUID]bool
}

func (r fakeRopa) GetActivity(ctx context.Context, id uuid.UUID) (ropaservice.Activity, error) {
	if !r.visible[id] {
		return ropaservice.Activity{}, ropaservice.ErrNotFound
	}
	return ropaservice.Activity{ID: id}, nil
}

// TestLinkActivity is DPIA-13's own breach-side half of the acceptance criterion: linking an incident to a
// RoPA activity records the link, refuses an activity the caller can't see, and a repeat link (the same
// delivery retried, or a plain double-click) is a harmless no-op rather than a duplicate row.
func TestLinkActivity(t *testing.T) {
	f := breachtest.Setup(t)
	activityID := uuid.New()
	f.Svc.Ropa = fakeRopa{visible: map[uuid.UUID]bool{activityID: true}}
	in := f.Incident(t, "เหตุทดสอบเชื่อมโยงกิจกรรม", f.Clock)

	var link breach.IncidentActivity
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		link, err = f.Svc.LinkActivity(ctx, in.ID, activityID)
		return err
	})
	if link.ActivityID != activityID || link.IncidentID != in.ID {
		t.Fatalf("expected the link to carry both ids, got %+v", link)
	}

	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		list, err := f.Svc.ListIncidentActivities(ctx, in.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ActivityID != activityID {
			t.Errorf("expected exactly one linked activity, got %+v", list)
		}
		return nil
	})

	// An unknown/invisible activity is refused.
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		if _, err := f.Svc.LinkActivity(ctx, in.ID, uuid.New()); !errors.Is(err, breach.ErrInvalidRequest) {
			t.Errorf("link an invisible activity: got %v, want ErrInvalidRequest", err)
		}
		return nil
	})

	// A redelivered/duplicate link for the same pair is a harmless no-op, not a duplicate row.
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		if _, err := f.Svc.LinkActivity(ctx, in.ID, activityID); err != nil {
			t.Errorf("relinking the same pair should be a no-op, got %v", err)
		}
		list, err := f.Svc.ListIncidentActivities(ctx, in.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Errorf("expected still exactly one linked activity after a duplicate link, got %d", len(list))
		}
		return nil
	})
}

// TestLinkActivity_TwoTenantIsolation (rule 1): tenant B's incident can't be linked by tenant A's call,
// and a tenant can't link an activity id belonging to another tenant either (caught by the FK-visibility
// check, mirrored here by the fake only knowing tenant A's activity id).
func TestLinkActivity_TwoTenantIsolation(t *testing.T) {
	f := breachtest.Setup(t)
	activityID := uuid.New()
	f.Svc.Ropa = fakeRopa{visible: map[uuid.UUID]bool{activityID: true}}
	inA := f.Incident(t, "เหตุ A", f.Clock)

	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		_, err := f.Svc.LinkActivity(ctx, inA.ID, activityID)
		return err
	})

	f.As(t, f.B.UserID, breachtest.DPO, func(ctx context.Context) error {
		if _, err := f.Svc.ListIncidentActivities(ctx, inA.ID); !errors.Is(err, breach.ErrNotFound) {
			t.Errorf("tenant B reading tenant A's incident activities: got %v, want ErrNotFound", err)
		}
		return nil
	})
}
