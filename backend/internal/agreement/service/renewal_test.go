package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// renewalEnv wires a real notify.Service + River insert client (the same minimal construction DSAR-07's own
// subtaskEnv uses) so SetSchedule's own reminder job is a real river_job row and FireRenewalReminder's own
// notification is a real platform.notifications row, plus a real iam.users row holding role LEGAL (the
// module doc's own actor) the same way notice_test.go seeds a DPO for PNG-07's own reminder.
func renewalEnv(t *testing.T, suffix string) env {
	t.Helper()
	e := setup(t, suffix)
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.River = client
	e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}

	owner := dbtest.OwnerPool(t)
	var legalUser uuid.UUID
	if err := pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'Legal Team', 'active') RETURNING id`,
			e.tenant.ID, uuid.NewString()[:8]+"@renewal.example").Scan(&legalUser); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
			SELECT $1, $2, id, 'tenant' FROM iam.roles WHERE code = 'LEGAL' AND tenant_id IS NULL`, e.tenant.ID, legalUser)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.role_assignments WHERE user_id = $1`, legalUser)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.users WHERE id = $1`, legalUser)
			return nil
		})
	})
	return e
}

func TestRenewalReminderAt(t *testing.T) {
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	want := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC) // 60 days before
	if got := agreementservice.RenewalReminderAt(to, 60); !got.Equal(want) {
		t.Errorf("RenewalReminderAt = %v, want %v", got, want)
	}
}

func TestSetSchedule_ValidatesAndSchedulesReminder(t *testing.T) {
	e := renewalEnv(t, "dparenew")
	var legalEntityID, vendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		legalEntityID, vendorID, activityID = fixture(t, ctx, e)
		return nil
	})
	var a agreementservice.Agreement
	e.in(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: legalEntityID,
			ActivityIDs: []uuid.UUID{activityID}, Title: "ทดสอบ DPA-10",
		})
		return err
	})

	effectiveTo := time.Now().UTC().AddDate(0, 0, 90)
	e.in(t, func(ctx context.Context) error {
		updated, err := e.svc.SetSchedule(ctx, a.ID, a.RowVersion, agreementservice.ScheduleInput{
			EffectiveTo: &effectiveTo, RenewalNoticeDays: 60, AutoRenew: true,
		})
		if err != nil {
			return err
		}
		if updated.EffectiveTo == nil || !updated.EffectiveTo.Equal(effectiveTo) {
			t.Errorf("effective_to = %v, want %v", updated.EffectiveTo, effectiveTo)
		}
		if updated.RenewalNoticeDays != 60 || !updated.AutoRenew {
			t.Errorf("renewal settings did not stick: %+v", updated)
		}
		if updated.RowVersion != a.RowVersion+1 {
			t.Errorf("row_version = %d, want %d", updated.RowVersion, a.RowVersion+1)
		}
		// stale version is refused
		if _, err := e.svc.SetSchedule(ctx, a.ID, a.RowVersion, agreementservice.ScheduleInput{RenewalNoticeDays: 30}); !errors.Is(err, agreementservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		// negative renewal_notice_days is refused
		if _, err := e.svc.SetSchedule(ctx, a.ID, updated.RowVersion, agreementservice.ScheduleInput{RenewalNoticeDays: -1}); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Errorf("negative renewal_notice_days: %v, want ErrInvalid", err)
		}
		return nil
	})

	var scheduledAt time.Time
	e.in(t, func(ctx context.Context) error {
		row := e.app.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE kind = 'agreement.renewal_reminder'
			AND args->>'agreement_id' = $1`, a.ID.String())
		return row.Scan(&scheduledAt)
	})
	want := agreementservice.RenewalReminderAt(effectiveTo, 60)
	if scheduledAt.Sub(want).Abs() > time.Minute {
		t.Errorf("scheduled_at = %v, want ~%v (effective_to - 60d)", scheduledAt, want)
	}
}

func TestFireRenewalReminder_NotifiesLegalAndNoopsWhenStale(t *testing.T) {
	e := renewalEnv(t, "dparenewfire")
	var legalEntityID, vendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		legalEntityID, vendorID, activityID = fixture(t, ctx, e)
		return nil
	})
	var a agreementservice.Agreement
	effectiveTo := time.Now().UTC().AddDate(0, 0, 10)
	e.in(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: legalEntityID,
			ActivityIDs: []uuid.UUID{activityID}, Title: "ทดสอบแจ้งเตือน",
		})
		if err != nil {
			return err
		}
		a, err = e.svc.SetSchedule(ctx, a.ID, a.RowVersion, agreementservice.ScheduleInput{EffectiveTo: &effectiveTo, RenewalNoticeDays: 7})
		return err
	})

	// a stale call (wrong effective_to, or an unknown agreement) is a harmless no-op
	e.in(t, func(ctx context.Context) error {
		stale := effectiveTo.AddDate(0, 0, 1)
		if err := e.svc.FireRenewalReminder(ctx, a.ID, stale, 7); err != nil {
			t.Errorf("stale fire: %v", err)
		}
		if err := e.svc.FireRenewalReminder(ctx, uuid.New(), effectiveTo, 7); err != nil {
			t.Errorf("unknown agreement fire: %v", err)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		return e.svc.FireRenewalReminder(ctx, a.ID, effectiveTo, 7)
	})
	var count int
	e.in(t, func(ctx context.Context) error {
		return e.app.QueryRow(ctx, `SELECT count(*) FROM platform.notifications WHERE entity_type = 'agreement' AND entity_id = $1`, a.ID).Scan(&count)
	})
	if count == 0 {
		t.Error("expected at least one platform.notifications row for role LEGAL")
	}
}
