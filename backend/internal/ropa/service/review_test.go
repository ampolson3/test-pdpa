package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// Acceptance criterion (ROPA-15): an active activity's own owner gets a review-reminder task once its
// next_review_at arrives (ApproveActivity already set it, review_cycle_months out); the clock then
// restarts from today, review_cycle_months out.
func TestCheckDueReviews_NotifiesOwnerAndReschedules(t *testing.T) {
	e := setup(t, "ropareview")
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}

	var ownerID uuid.UUID
	activity := completeActivity(t, context.Background(), e, "REVIEW-01")
	e.in(t, func(ctx context.Context) error {
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, 'Activity Owner', 'active') RETURNING id`,
			uuid.NewString()[:8]+"@owner.example").Scan(&ownerID); err != nil {
			return err
		}
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE ropa.processing_activities SET owner_user_id = $2 WHERE id = $1`, activity.ID, ownerID)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		submitted, err := e.svc.SubmitActivity(ctx, activity.ID, activity.RowVersion)
		if err != nil {
			return err
		}
		activity = submitted
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		approved, err := e.svc.ApproveActivity(ctx, activity.ID, activity.RowVersion)
		if err != nil {
			return err
		}
		activity = approved
		return nil
	})

	// Backdate next_review_at to "now" (ApproveActivity just set it 12 months out).
	e.in(t, func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE ropa.processing_activities SET next_review_at = current_date WHERE id = $1`, activity.ID)
		return err
	})

	e.in(t, func(ctx context.Context) error { return e.svc.CheckDueReviews(ctx) })

	e.in(t, func(ctx context.Context) error {
		var count int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*) FROM platform.notifications WHERE recipient_user_id = $1 AND template_code = 'ropa.review_due'`,
			ownerID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			t.Error("expected at least one platform.notifications row for the activity's owner")
		}
		var next pgtype.Date
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT next_review_at FROM ropa.processing_activities WHERE id = $1`, activity.ID).Scan(&next); err != nil {
			return err
		}
		if !next.Valid || !next.Time.After(time.Now()) {
			t.Errorf("next_review_at should be rescheduled into the future, got %v", next)
		}
		return nil
	})

	// A second, immediate check is a harmless no-op.
	e.in(t, func(ctx context.Context) error { return e.svc.CheckDueReviews(ctx) })
	e.in(t, func(ctx context.Context) error {
		var count int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*) FROM platform.notifications WHERE recipient_user_id = $1 AND template_code = 'ropa.review_due'`,
			ownerID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("a not-yet-due activity should not be notified again, got %d notifications", count)
		}
		return nil
	})
}

// A notice with no owner falls back to role DPO — same "default recipients" pattern PNG-15/BRE-07 use.
func TestCheckDueReviews_FallsBackToDPOWithNoOwner(t *testing.T) {
	e := setup(t, "ropareviewdpo")
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}

	var dpoID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, 'Dee DPO', 'active') RETURNING id`,
			uuid.NewString()[:8]+"@dpo.example").Scan(&dpoID); err != nil {
			return err
		}
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
			SELECT current_setting('app.tenant_id')::uuid, $1, id, 'tenant' FROM iam.roles WHERE code = 'DPO' AND tenant_id IS NULL`, dpoID)
		return err
	})

	activity := completeActivity(t, context.Background(), e, "REVIEW-NODPO")
	e.in(t, func(ctx context.Context) error {
		submitted, err := e.svc.SubmitActivity(ctx, activity.ID, activity.RowVersion)
		if err != nil {
			return err
		}
		activity = submitted
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		approved, err := e.svc.ApproveActivity(ctx, activity.ID, activity.RowVersion)
		if err != nil {
			return err
		}
		activity = approved
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE ropa.processing_activities SET next_review_at = current_date WHERE id = $1`, activity.ID)
		return err
	})

	e.in(t, func(ctx context.Context) error { return e.svc.CheckDueReviews(ctx) })

	e.in(t, func(ctx context.Context) error {
		var count int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*) FROM platform.notifications WHERE recipient_user_id = $1 AND template_code = 'ropa.review_due'`,
			dpoID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			t.Error("expected the seeded DPO user to be notified when the activity has no owner")
		}
		return nil
	})
}

// Two-tenant isolation: CheckDueReviews only ever reads the calling tenant's own RLS-scoped rows.
func TestCheckDueReviews_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "ropareviewisoA")
	b := setup(t, "ropareviewisoB")
	for _, e := range []env{a, b} {
		client, err := jobs.NewInsertClient(e.app)
		if err != nil {
			t.Fatal(err)
		}
		e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}
	}

	activity := completeActivity(t, context.Background(), a, "REVIEW-ISO")
	a.in(t, func(ctx context.Context) error {
		submitted, err := a.svc.SubmitActivity(ctx, activity.ID, activity.RowVersion)
		if err != nil {
			return err
		}
		activity = submitted
		return nil
	})
	a.in(t, func(ctx context.Context) error {
		approved, err := a.svc.ApproveActivity(ctx, activity.ID, activity.RowVersion)
		if err != nil {
			return err
		}
		activity = approved
		return nil
	})
	a.in(t, func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE ropa.processing_activities SET next_review_at = current_date WHERE id = $1`, activity.ID)
		return err
	})

	// Tenant B's own check never sees tenant A's due activity (RLS).
	b.in(t, func(ctx context.Context) error { return b.svc.CheckDueReviews(ctx) })
	b.in(t, func(ctx context.Context) error {
		var count int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM ropa.processing_activities WHERE id = $1`, activity.ID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Error("tenant B should never see tenant A's activity under RLS")
		}
		return nil
	})
}
