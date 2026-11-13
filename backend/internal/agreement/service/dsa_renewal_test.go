package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
)

// TestFireRenewalReminder_WorksForDsaAgreementsToo is DSA-11's own acceptance criterion directly: the
// renewal-reminder mechanism DPA-10 built (SetSchedule/scheduleRenewalReminder/FireRenewalReminder) never
// branches on agreement_type — it already operates on any agreement.agreements row — so the exact same
// checkpoint fires and notifies role LEGAL for a "dsa" agreement too, with no new service code needed.
func TestFireRenewalReminder_WorksForDsaAgreementsToo(t *testing.T) {
	e := renewalEnv(t, "dsarenew")
	var leID, activityID, counterpartyID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _, activityID = fixture(t, ctx, e)
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "หน่วยงานพันธมิตร DSA-11", CountryCode: "TH"}, 0)
		counterpartyID = party.ID
		return err
	})
	a := createDsaAgreement(t, e, leID, counterpartyID, []uuid.UUID{activityID})

	effectiveTo := time.Now().UTC().AddDate(0, 0, 10)
	e.in(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.SetSchedule(ctx, a.ID, a.RowVersion, agreementservice.ScheduleInput{EffectiveTo: &effectiveTo, RenewalNoticeDays: 7})
		return err
	})

	var scheduledAt time.Time
	e.in(t, func(ctx context.Context) error {
		row := e.app.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE kind = 'agreement.renewal_reminder'
			AND args->>'agreement_id' = $1`, a.ID.String())
		return row.Scan(&scheduledAt)
	})
	want := agreementservice.RenewalReminderAt(effectiveTo, 7)
	if scheduledAt.Sub(want).Abs() > time.Minute {
		t.Errorf("scheduled_at = %v, want ~%v (effective_to - 7d)", scheduledAt, want)
	}

	e.in(t, func(ctx context.Context) error {
		return e.svc.FireRenewalReminder(ctx, a.ID, effectiveTo, 7)
	})
	var count int
	e.in(t, func(ctx context.Context) error {
		return e.app.QueryRow(ctx, `SELECT count(*) FROM platform.notifications WHERE entity_type = 'agreement' AND entity_id = $1`, a.ID).Scan(&count)
	})
	if count == 0 {
		t.Error("expected at least one platform.notifications row for role LEGAL on a dsa agreement")
	}
}
