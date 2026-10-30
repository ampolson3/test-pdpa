package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// TestScheduleDraftConfirmReminder_AcceptanceCriterion is RTG-11's own acceptance criterion directly
// ("ร่างที่ยังไม่ยืนยันถูกเตือนตาม SLA"): creating an activity from a template schedules both checkpoints;
// firing either while the activity is still draft notifies its owner, while an already-submitted activity
// is a harmless no-op on the same fire.
func TestScheduleDraftConfirmReminder_AcceptanceCriterion(t *testing.T) {
	e := setup(t, "rtg11accept")
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}
	e.svc.River = client

	var unit orgservice.OrgUnit
	var ownerID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", UnitType: "department", NameTh: "HR"})
		if err != nil {
			return err
		}
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, 'Draft Owner', 'active') RETURNING id`,
			uuid.NewString()[:8]+"@owner.example").Scan(&ownerID)
	})

	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		tplID := templateIDByCode(t, ctx, "recruitment_job_posting")
		a, err := e.svc.CreateActivityFromTemplate(ctx, tplID, unit.LegalEntityID, unit.ID, "HR-RTG11-01", &ownerID)
		if err != nil {
			return err
		}
		activityID = a.ID
		return nil
	})

	// Both checkpoints were scheduled at creation.
	if n := countJobs(t, e, "ropa.draft_confirm_check", activityID); n != 2 {
		t.Fatalf("expected 2 scheduled checkpoints, got %d", n)
	}

	// Firing the reminder while still draft notifies the owner.
	e.in(t, func(ctx context.Context) error {
		return e.svc.FireDraftConfirmCheck(ctx, activityID, false)
	})
	if n := countNotifications(t, e, activityID); n != 1 {
		t.Fatalf("expected 1 owner notified (in_app), got %d", n)
	}

	// Submitting the activity moves it out of draft — a later fire is a harmless no-op.
	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.GetActivity(ctx, activityID)
		if err != nil {
			return err
		}
		_, err = e.svc.SubmitActivity(ctx, activityID, a.RowVersion)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		return e.svc.FireDraftConfirmCheck(ctx, activityID, true)
	})
	if n := countNotifications(t, e, activityID); n != 1 {
		t.Fatalf("expected still 1 notification after submit (overdue fire was a no-op), got %d", n)
	}
}

func countJobs(t *testing.T, e env, kind string, activityID uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.app.QueryRow(context.Background(),
		`SELECT count(*)::int FROM river_job WHERE kind = $1 AND args->>'activity_id' = $2`, kind, activityID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func countNotifications(t *testing.T, e env, activityID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(DISTINCT recipient_user_id)::int FROM platform.notifications WHERE entity_id = $1 AND channel = 'in_app'`, activityID).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}
