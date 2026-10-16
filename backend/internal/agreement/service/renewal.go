package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	agreementstore "pdpa-platform/internal/agreement/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// ScheduleInput is DPA-10's own mutable fields on an agreement: its registry start/end dates and renewal
// settings. Every other field (status, parties, clauses, activities, the document itself) belongs to a
// sibling feature — status transitions in particular are ST-04#1's own job (DPA-06/07/08/09, not built yet).
type ScheduleInput struct {
	EffectiveFrom     *time.Time
	EffectiveTo       *time.Time
	AutoRenew         bool
	RenewalNoticeDays int
}

// RenewalReminderAt is DPA-10's own single checkpoint: renewal_notice_days before the agreement's own end
// date (the column already on agreement.agreements since the baseline migration, configurable per
// agreement — not a tunable platform default, since the module doc's own frontend note has the user set it
// per contract).
func RenewalReminderAt(effectiveTo time.Time, renewalNoticeDays int) time.Time {
	return effectiveTo.AddDate(0, 0, -renewalNoticeDays)
}

// SetSchedule is DPA-10's acceptance criterion's data half: the registry's own start/end dates. Changing
// effective_to or renewal_notice_days reschedules the single reminder checkpoint (FireRenewalReminder checks
// the agreement's own current values at fire time, so a stale tick from before a change is a harmless no-op
// — the same pattern BRE-07's own aware_at reschedule and DSAR-07's own due_at use).
func (s *Service) SetSchedule(ctx context.Context, id uuid.UUID, version int32, in ScheduleInput) (Agreement, error) {
	if in.RenewalNoticeDays < 0 {
		return Agreement{}, fmt.Errorf("%w: renewal_notice_days", ErrInvalid)
	}
	if in.EffectiveFrom != nil && in.EffectiveTo != nil && in.EffectiveTo.Before(*in.EffectiveFrom) {
		return Agreement{}, fmt.Errorf("%w: effective_to before effective_from", ErrInvalid)
	}
	before, err := s.GetAgreement(ctx, id)
	if err != nil {
		return Agreement{}, err
	}

	q := agreementstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.UpdateAgreementSchedule(ctx, agreementstore.UpdateAgreementScheduleParams{
		ID: id, RowVersion: version, EffectiveFrom: pgDate(in.EffectiveFrom), EffectiveTo: pgDate(in.EffectiveTo),
		AutoRenew: in.AutoRenew, RenewalNoticeDays: int16(in.RenewalNoticeDays),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Agreement{}, ErrVersionMismatch
	}
	if err != nil {
		return Agreement{}, err
	}

	if err := s.scheduleRenewalReminder(ctx, id, in.EffectiveTo, in.RenewalNoticeDays); err != nil {
		return Agreement{}, err
	}
	if err := s.audit(ctx, "agreement.agreement.schedule", id,
		map[string]any{"effective_from": before.EffectiveFrom, "effective_to": before.EffectiveTo, "auto_renew": before.AutoRenew, "renewal_notice_days": before.RenewalNoticeDays},
		map[string]any{"effective_from": in.EffectiveFrom, "effective_to": in.EffectiveTo, "auto_renew": in.AutoRenew, "renewal_notice_days": in.RenewalNoticeDays}); err != nil {
		return Agreement{}, err
	}
	return toAgreement(row, before.ActivityIDs), nil
}

// ReminderArgs is agreement.renewal_reminder: the single checkpoint of one agreement's own end date.
// EffectiveTo/RenewalNoticeDays pin the schedule so a stale tick (the agreement's dates changed, or it was
// terminated, since this job was enqueued) is harmless.
type ReminderArgs struct {
	jobs.TenantArgs
	AgreementID       string `json:"agreement_id"`
	EffectiveTo       string `json:"effective_to"`
	RenewalNoticeDays int    `json:"renewal_notice_days"`
}

func (ReminderArgs) Kind() string { return "agreement.renewal_reminder" }

// scheduleRenewalReminder enqueues the single checkpoint at RenewalReminderAt, or immediately if that
// moment has already passed (an agreement whose end date is recorded with fewer days left than the notice
// period still alerts once, at once — the same "late-recorded event still alerts" rule PNG-04/DSAR-07 use).
// A nil EffectiveTo (the date was cleared) enqueues nothing — there is nothing to count down to.
func (s *Service) scheduleRenewalReminder(ctx context.Context, id uuid.UUID, effectiveTo *time.Time, renewalNoticeDays int) error {
	if s.River == nil || effectiveTo == nil {
		return nil
	}
	at := RenewalReminderAt(*effectiveTo, renewalNoticeDays)
	if now := time.Now().UTC(); at.Before(now) {
		at = now
	}
	tenant, err := tenantID(ctx)
	if err != nil {
		return err
	}
	args := ReminderArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, AgreementID: id.String(),
		EffectiveTo: effectiveTo.Format("2006-01-02"), RenewalNoticeDays: renewalNoticeDays}
	_, err = jobs.Enqueue(ctx, s.River, args, &river.InsertOpts{ScheduledAt: at, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}

// FireRenewalReminder runs the checkpoint: while the agreement's own end date and notice period still match
// what the job was scheduled for, and the agreement isn't terminated, it notifies role LEGAL (the module
// doc's own actor; SCHED is this job itself, not an RBAC role — there is no per-agreement owner column to
// route to more precisely yet).
func (s *Service) FireRenewalReminder(ctx context.Context, id uuid.UUID, effectiveTo time.Time, renewalNoticeDays int) error {
	a, err := s.GetAgreement(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if a.EffectiveTo == nil || !a.EffectiveTo.Equal(effectiveTo) || a.RenewalNoticeDays != renewalNoticeDays {
		return nil // stale schedule
	}
	if a.Status == "terminated" {
		return nil
	}
	if s.Notify == nil {
		return nil
	}
	to, err := iamservice.UsersWithRole(ctx, "LEGAL")
	if err != nil {
		return err
	}
	vars := map[string]any{"agreement_no": a.AgreementNo, "title": a.Title, "effective_to": a.EffectiveTo.Format("2 Jan 2006"), "days_left": renewalNoticeDays}
	for _, u := range to {
		uid := u
		for _, ch := range []string{"in_app", "email"} {
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "agreement.renewal_reminder", Channel: ch, RecipientUserID: &uid,
					Vars: vars, EntityType: EntityType, EntityID: &a.ID, Urgent: false})
				return err
			})
			if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
				return err
			}
		}
	}
	return nil
}

// ReminderWorker works agreement.renewal_reminder.
type ReminderWorker struct {
	river.WorkerDefaults[ReminderArgs]
	Service *Service
}

func (w *ReminderWorker) Work(ctx context.Context, job *river.Job[ReminderArgs]) error {
	id, err1 := uuid.Parse(job.Args.AgreementID)
	to, err2 := time.Parse("2006-01-02", job.Args.EffectiveTo)
	if err := errors.Join(err1, err2); err != nil {
		return river.JobCancel(err)
	}
	return w.Service.FireRenewalReminder(ctx, id, to, job.Args.RenewalNoticeDays)
}

func tenantID(ctx context.Context) (uuid.UUID, error) {
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(t)
}
