package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	riskservice "pdpa-platform/internal/risk/service"
)

// acceptanceEnv wires a real River insert client + notify.Service (the same minimal construction VEN-06's
// own certEnv/DPA-10's own renewalEnv use) so RecordAcceptance's own reminder job is a real river_job row
// and FireAcceptanceReviewReminder's own notification is a real platform.notifications row, plus a real
// iam.users row holding role DPO (the acceptance criterion's own recipient).
func acceptanceEnv(t *testing.T, suffix string) (scoreEnv, uuid.UUID) {
	t.Helper()
	e := scoreSetup(t, suffix)
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.River = client
	e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}

	owner := dbtest.OwnerPool(t)
	var dpoUser uuid.UUID
	if err := pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'DPO', 'active') RETURNING id`,
			e.tenant.ID, uuid.NewString()[:8]+"@rra08.example").Scan(&dpoUser); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
			SELECT $1, $2, id, 'tenant' FROM iam.roles WHERE code = 'DPO' AND tenant_id IS NULL`, e.tenant.ID, dpoUser)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM risk.acceptances`)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.role_assignments WHERE user_id = $1`, dpoUser)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.users WHERE id = $1`, dpoUser)
			return nil
		})
	})
	return e, dpoUser
}

// inApprove runs fn in the tenant's own transaction with dpo.risk.approve granted, alongside the base
// permissions scoreEnv.in already grants.
func inApprove(t *testing.T, e scoreEnv, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: []string{
			"ropa.risk.read", "ropa.risk.create", "ropa.risk.update", "dpo.risk.read", "dpo.risk.approve",
			"org.structure.read", "org.structure.update", "ropa.activity.read", "ropa.activity.create",
		}}))
	}); err != nil {
		t.Fatal(err)
	}
}

func futureDate(days int) time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
}

// TestRecordAcceptance_SetsAcceptedAndSchedulesReminder is RRA-08's own acceptance criterion's data half:
// recording an acceptance flips the risk to "accepted" and schedules exactly one reminder at expires_at.
func TestRecordAcceptance_SetsAcceptedAndSchedulesReminder(t *testing.T) {
	e, _ := acceptanceEnv(t, "rra08record")
	var riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = mustIdentify(t, ctx, e.svc, riskservice.Risk{SourceType: "manual", Title: "residual risk", Likelihood: 1, Impact: 1}).ID
		return nil
	})

	expires := futureDate(180)
	var acc riskservice.Acceptance
	inApprove(t, e, func(ctx context.Context) error {
		var err error
		acc, err = e.svc.RecordAcceptance(ctx, riskID, "accepted for the quarter", expires)
		return err
	})
	if acc.Reason != "accepted for the quarter" || !acc.ExpiresAt.Equal(expires) {
		t.Fatalf("acceptance = %+v, want reason/expires_at to round-trip", acc)
	}

	e.in(t, func(ctx context.Context) error {
		r, err := e.svc.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if r.Status != "accepted" {
			t.Errorf("risk status = %q, want accepted", r.Status)
		}
		return nil
	})

	var scheduledAt time.Time
	e.in(t, func(ctx context.Context) error {
		row := e.app.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE kind = 'risk.acceptance_review'
			AND args->>'acceptance_id' = $1`, acc.ID.String())
		return row.Scan(&scheduledAt)
	})
	if scheduledAt.Sub(expires).Abs() > time.Minute {
		t.Errorf("scheduled_at = %v, want ~%v (expires_at itself)", scheduledAt, expires)
	}

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListAcceptances(ctx, riskID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != acc.ID {
			t.Errorf("ListAcceptances = %+v, want exactly the one acceptance", list)
		}
		return nil
	})
}

// TestRecordAcceptance_RequiresApprovePermission is the module doc's own actor line (DPO/EXEC) enforced
// directly in the service, the same permanent-decision gate ORG-06/DPIA-03/VEN-14 already use.
func TestRecordAcceptance_RequiresApprovePermission(t *testing.T) {
	e, _ := acceptanceEnv(t, "rra08forbid")
	var riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = mustIdentify(t, ctx, e.svc, riskservice.Risk{SourceType: "manual", Title: "no approve", Likelihood: 1, Impact: 1}).ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordAcceptance(ctx, riskID, "reason", futureDate(30)); !errors.Is(err, riskservice.ErrForbidden) {
			t.Errorf("RecordAcceptance without dpo.risk.approve: %v, want ErrForbidden", err)
		}
		return nil
	})
}

// TestRecordAcceptance_RefusesClosedRiskAndPastExpiry covers the two other validation rules: a closed risk
// has nothing left to accept, and expires_at must be a real future review date.
func TestRecordAcceptance_RefusesClosedRiskAndPastExpiry(t *testing.T) {
	e, _ := acceptanceEnv(t, "rra08invalid")
	var openID, closedID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		openID = mustIdentify(t, ctx, e.svc, riskservice.Risk{SourceType: "manual", Title: "open", Likelihood: 1, Impact: 1}).ID
		r := mustIdentify(t, ctx, e.svc, riskservice.Risk{SourceType: "manual", Title: "closed", Likelihood: 1, Impact: 1})
		closed := r
		closed.Status = "closed"
		var err error
		closedID = r.ID
		_, err = e.svc.UpdateRisk(ctx, r.ID, closed, r.RowVersion)
		return err
	})
	inApprove(t, e, func(ctx context.Context) error {
		if _, err := e.svc.RecordAcceptance(ctx, closedID, "reason", futureDate(30)); !errors.Is(err, riskservice.ErrInvalidTransition) {
			t.Errorf("RecordAcceptance on a closed risk: %v, want ErrInvalidTransition", err)
		}
		if _, err := e.svc.RecordAcceptance(ctx, openID, "reason", time.Now().UTC()); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("RecordAcceptance with expires_at not in the future: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.RecordAcceptance(ctx, openID, "  ", futureDate(30)); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("RecordAcceptance with a blank reason: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestFireAcceptanceReviewReminder_RevertsAndNotifiesDPO is the acceptance criterion itself: once the
// review date arrives, the risk reverts from accepted to open and every DPO is notified; a stale or
// superseded reminder is a harmless no-op.
func TestFireAcceptanceReviewReminder_RevertsAndNotifiesDPO(t *testing.T) {
	e, _ := acceptanceEnv(t, "rra08fire")
	var riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = mustIdentify(t, ctx, e.svc, riskservice.Risk{SourceType: "manual", Title: "expiring", Likelihood: 1, Impact: 1}).ID
		return nil
	})
	expires := futureDate(10)
	var acc riskservice.Acceptance
	inApprove(t, e, func(ctx context.Context) error {
		var err error
		acc, err = e.svc.RecordAcceptance(ctx, riskID, "reason", expires)
		return err
	})

	// a stale call (wrong expires_at, or an unknown acceptance) is a harmless no-op — the risk stays accepted.
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.FireAcceptanceReviewReminder(ctx, acc.ID, expires.AddDate(0, 0, 1)); err != nil {
			t.Errorf("stale fire: %v", err)
		}
		if err := e.svc.FireAcceptanceReviewReminder(ctx, uuid.New(), expires); err != nil {
			t.Errorf("unknown acceptance fire: %v", err)
		}
		r, err := e.svc.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if r.Status != "accepted" {
			t.Errorf("risk status after a stale fire = %q, want still accepted", r.Status)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		return e.svc.FireAcceptanceReviewReminder(ctx, acc.ID, expires)
	})
	e.in(t, func(ctx context.Context) error {
		r, err := e.svc.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if r.Status != "open" {
			t.Errorf("risk status after expiry = %q, want open", r.Status)
		}
		return nil
	})
	var count int
	e.in(t, func(ctx context.Context) error {
		row := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.notifications n
			JOIN platform.notification_templates t ON t.id = n.template_id
			WHERE t.code = 'risk.acceptance_review'`)
		return row.Scan(&count)
	})
	if count == 0 {
		t.Fatal("expected at least one risk.acceptance_review notification to a DPO user")
	}

	// a repeat fire after the risk already reverted is still a harmless no-op.
	e.in(t, func(ctx context.Context) error {
		return e.svc.FireAcceptanceReviewReminder(ctx, acc.ID, expires)
	})
}

// TestRecordAcceptance_TwoTenantIsolation is CLAUDE.md rule 1: a fresh acceptance belongs only to the
// tenant that recorded it, and its own risk is invisible to another tenant.
func TestRecordAcceptance_TwoTenantIsolation(t *testing.T) {
	e1, _ := acceptanceEnv(t, "rra08isoA")
	e2, _ := acceptanceEnv(t, "rra08isoB")
	var riskID uuid.UUID
	e1.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e1.svc)
		riskID = mustIdentify(t, ctx, e1.svc, riskservice.Risk{SourceType: "manual", Title: "tenant A risk", Likelihood: 1, Impact: 1}).ID
		return nil
	})
	inApprove(t, e1, func(ctx context.Context) error {
		_, err := e1.svc.RecordAcceptance(ctx, riskID, "reason", futureDate(30))
		return err
	})
	e2.in(t, func(ctx context.Context) error {
		if _, err := e2.svc.GetRisk(ctx, riskID); !errors.Is(err, riskservice.ErrNotFound) {
			t.Errorf("tenant B reading tenant A's risk: %v, want ErrNotFound", err)
		}
		return nil
	})
}
