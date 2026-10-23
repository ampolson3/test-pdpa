package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
)

// seedUser inserts a real, active iam.users row (dbtest.SeedTenant's own tenant user defaults to 'invited',
// which iamservice.Names doesn't count as usable — the same gotcha DPO-01's tests already documented). Cleaned
// up by setup()'s own teardown (email pattern assignee-%@dbtest.example).
func seedUser(t *testing.T, ctx context.Context, e env) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, 'Assignee', 'active') RETURNING id`,
		"assignee-"+uuid.NewString()+"@dbtest.example").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSLAStatus(t *testing.T) {
	now := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		dueAt time.Time
		want  string
	}{
		{"far off", now.AddDate(0, 0, 20), "on_track"},
		{"exactly 10 days left", now.AddDate(0, 0, 10), "at_risk"},
		{"7 days left", now.AddDate(0, 0, 7), "at_risk"},
		{"already past", now.AddDate(0, 0, -1), "overdue"},
		{"due this instant", now, "at_risk"},
	}
	for _, c := range cases {
		if got := dsarservice.SLAStatus(now, c.dueAt); got != c.want {
			t.Errorf("%s: SLAStatus = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestReminderAt(t *testing.T) {
	due := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	want := due.AddDate(0, 0, -7)
	if got := dsarservice.ReminderAt(due); !got.Equal(want) {
		t.Errorf("ReminderAt = %v, want %v", got, want)
	}
}

// TestCreateRequest_SchedulesSlaReminder is DSAR-07's acceptance criterion at the scheduling level: creating
// a request enqueues its single dsar.sla_reminder checkpoint at due_at minus 7 days.
func TestCreateRequest_SchedulesSlaReminder(t *testing.T) {
	e := setup(t, "dsarsla")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "sla@example.com", ContactKind: crypto.KindEmail})
		return err
	})

	var scheduledAt time.Time
	e.in(t, func(ctx context.Context) error {
		row := e.app.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE kind = 'dsar.sla_reminder'
			AND args->>'request_id' = $1`, r.ID.String())
		return row.Scan(&scheduledAt)
	})
	want := dsarservice.ReminderAt(r.DueAt)
	if scheduledAt.Sub(want).Abs() > time.Minute {
		t.Errorf("scheduled_at = %v, want ~%v (due_at - 7d)", scheduledAt, want)
	}
}

// TestFireReminder_NoopsForClosedOrUnknownRequest covers the guards that keep a stale or late checkpoint
// harmless: an unknown request (already deleted) and a request that has already reached a terminal ST-02
// status are both silently skipped rather than erroring.
func TestFireReminder_NoopsForClosedOrUnknownRequest(t *testing.T) {
	e := setup(t, "dsarslanoop")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "close@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		var err2 error
		r, _, err2 = e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "withdrawn"})
		return err2
	})
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.FireReminder(ctx, r.ID, r.DueAt); err != nil {
			t.Errorf("closed request: %v", err)
		}
		if err := e.svc.FireReminder(ctx, uuid.New(), time.Now()); err != nil {
			t.Errorf("unknown request: %v", err)
		}
		return nil
	})
}

// TestAssignRequest is the other half of "ผู้รับผิดชอบ": setting, clearing and validating the responsible
// person notified alongside role DPO when the SLA reminder fires.
func TestAssignRequest(t *testing.T) {
	e := setup(t, "dsarassign")
	var leID, typeID, userID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		userID = seedUser(t, ctx, e)
		return nil
	})
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "assign@example.com", ContactKind: crypto.KindEmail})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		var err error
		r, err = e.svc.AssignRequest(ctx, r.ID, r.RowVersion, &userID)
		if err != nil {
			return err
		}
		if r.AssigneeUserID == nil || *r.AssigneeUserID != userID {
			t.Errorf("assignee_user_id = %v, want %v", r.AssigneeUserID, userID)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.AssignRequest(ctx, r.ID, r.RowVersion, ptr(uuid.New()))
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("unknown user: err = %v, want ErrInvalid", err)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		var err error
		r, err = e.svc.AssignRequest(ctx, r.ID, r.RowVersion, nil)
		if err != nil {
			return err
		}
		if r.AssigneeUserID != nil {
			t.Errorf("expected unassigned, got %v", r.AssigneeUserID)
		}
		return nil
	})
}

func ptr(u uuid.UUID) *uuid.UUID { return &u }
