package service_test

import (
	"context"
	"encoding/json"
	"testing"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/jobs"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// outboxChangedFields reads back the changed_fields array ropa.activity_approved's own payload carried for
// the given activity's most recently approved round (version), the PNG-10 producer-side acceptance
// criterion: approving an activity tells the notice module exactly which sections changed, so a redundant
// notice-review mark is never sent when nothing a notice could compose from actually moved.
func outboxChangedFields(t *testing.T, ctx context.Context, activityID interface{ String() string }, version int) []string {
	t.Helper()
	var raw []byte
	err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`SELECT payload->'data'->'changed_fields' FROM platform.outbox_events
		 WHERE event_type = 'ropa.activity_approved' AND aggregate_id = $1
		   AND (payload->'data'->>'version')::int = $2`, activityID.String(), version).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	var fields []string
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

// TestApproveActivity_PublishesChangedFieldsEvent is PNG-10's own producer-side half: approving an activity
// publishes ropa.activity_approved (PLT-11 outbox) naming exactly the sections that changed since the
// previous approved round — every section on a first approval (there is nothing to diff against yet), just
// the one field that was actually edited on a re-approval. internal/wiring/events.go's own subscriber turns
// this into "mark every notice composed from this activity for review" (notice/service/review.go's
// MarkNoticesForReviewByActivity) without ropa ever writing to the notice schema itself (rule 9).
func TestApproveActivity_PublishesChangedFieldsEvent(t *testing.T) {
	e := setup(t, "ropapng10")
	riverClient, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Events = &events.Publisher{River: riverClient}

	var a ropaservice.Activity
	e.in(t, func(ctx context.Context) error { a = completeActivity(t, ctx, e, "PNG10-01"); return nil })
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
		a = approved
		return nil
	})

	// First approval: nothing to diff against, so every section the snapshot carries counts as changed.
	e.in(t, func(ctx context.Context) error {
		fields := outboxChangedFields(t, ctx, a.ID, 1)
		want := map[string]bool{"name": false, "purposes": false, "data": false, "retention": false, "controls": false}
		for _, f := range fields {
			if _, ok := want[f]; ok {
				want[f] = true
			}
		}
		for f, seen := range want {
			if !seen {
				t.Errorf("first approval's changed_fields missing %q, got %v", f, fields)
			}
		}
		return nil
	})

	// Reopen, edit only the name, re-submit, re-approve -> the second round's own changed_fields names
	// exactly that one section, matching CompareActivityVersions' own diff for the identical edit
	// (TestActivityApproval_Lifecycle).
	e.in(t, func(ctx context.Context) error {
		reopened, err := e.svc.RequestActivityReview(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		a = reopened
		edited, err := e.svc.SaveActivity(ctx, ropaservice.Activity{ID: a.ID, LegalEntityID: a.LegalEntityID, OrgUnitID: a.OrgUnitID,
			Code: a.Code, Name: "กิจกรรมทดสอบ (แก้ไข)", Role: a.Role, RightsAndAccess: a.RightsAndAccess}, a.RowVersion)
		if err != nil {
			return err
		}
		a = edited
		submitted, err := e.svc.SubmitActivity(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		a = submitted
		approved, err := e.svc.ApproveActivity(ctx, a.ID, a.RowVersion)
		if err != nil {
			return err
		}
		a = approved
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		fields := outboxChangedFields(t, ctx, a.ID, 2)
		if len(fields) != 1 || fields[0] != "name" {
			t.Errorf("second approval's changed_fields = %v, want exactly [\"name\"]", fields)
		}
		return nil
	})
}

// TestApproveActivity_NoEventsPublisherIsHarmless proves the nil-means-skip convention: a Service with no
// Events field (every other ropa test in this package, and cmd/worker's own ropaSvc) still approves an
// activity exactly as before — PNG-10 adds nothing to approve, it only optionally observes it.
func TestApproveActivity_NoEventsPublisherIsHarmless(t *testing.T) {
	e := setup(t, "ropapng10nil")
	var a ropaservice.Activity
	e.in(t, func(ctx context.Context) error { a = completeActivity(t, ctx, e, "PNG10-02"); return nil })
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
			t.Errorf("expected active, got %s", approved.Status)
		}
		return nil
	})
}
