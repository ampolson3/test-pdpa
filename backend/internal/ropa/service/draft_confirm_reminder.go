// RTG-11 (มอบหมายร่าง RoPA ให้หน่วยงานยืนยัน): a single pair of checkpoints scheduled once, at the moment
// an activity is created from a template (ROPA-05's CreateActivityFromTemplate, reused by RTG-04/RTG-07/
// RTG-08) — the same "schedule once at creation, check live status at fire time" pattern VEN-06's own
// certificate-expiry reminder already established, not a recurring sweep (an activity's own "still draft"
// state doesn't move on its own the way a recurring review cycle does). No migration: both checkpoints are
// plain scheduled River jobs that re-read the activity's own current status, with nothing new to persist.
package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// DraftConfirmReminderDays / DraftConfirmOverdueDays: tunable defaults (CLAUDE.md's "implement the listed
// default as config", not a docs/decisions.md question — these only shape when the reminder fires, never
// a contract or legal behaviour).
const (
	DraftConfirmReminderDays = 3
	DraftConfirmOverdueDays  = 7
)

// DraftConfirmCheckArgs is both ropa.draft_confirm_reminder and ropa.draft_confirm_overdue — the same
// payload shape, the job Kind alone decides which checkpoint fired.
type DraftConfirmCheckArgs struct {
	jobs.TenantArgs
	ActivityID string `json:"activity_id"`
	Overdue    bool   `json:"overdue"`
}

func (DraftConfirmCheckArgs) Kind() string { return "ropa.draft_confirm_check" }

func tenantFromCtx(ctx context.Context) (uuid.UUID, error) {
	g, _ := authz.FromContext(ctx)
	if t, err := uuid.Parse(g.TenantID); err == nil {
		return t, nil
	}
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(t)
}

// scheduleDraftConfirmReminder enqueues both checkpoints for a freshly created, template-sourced activity
// — a reminder at DraftConfirmReminderDays and an overdue escalation at DraftConfirmOverdueDays. Either
// fire is a harmless no-op once the activity has left draft (submitted, or edited into under_review).
func (s *Service) scheduleDraftConfirmReminder(ctx context.Context, activityID uuid.UUID) error {
	if s.River == nil {
		return nil
	}
	tenant, err := tenantFromCtx(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for _, c := range []struct {
		days    int
		overdue bool
	}{{DraftConfirmReminderDays, false}, {DraftConfirmOverdueDays, true}} {
		args := DraftConfirmCheckArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, ActivityID: activityID.String(), Overdue: c.overdue}
		if _, err := jobs.Enqueue(ctx, s.River, args, &river.InsertOpts{
			ScheduledAt: now.AddDate(0, 0, c.days), UniqueOpts: river.UniqueOpts{ByArgs: true},
		}); err != nil {
			return err
		}
	}
	return nil
}

// FireDraftConfirmCheck is the acceptance criterion itself ("ร่างที่ยังไม่ยืนยันถูกเตือนตาม SLA"): while
// the activity is still draft, it notifies the activity's own owner — falling back to role OWNER (the
// module doc's own actor for confirming the draft), the same "default recipients until real per-record
// routing exists" fallback BRE-07/PNG-04/VEN-06/VEN-10 already use.
func (s *Service) FireDraftConfirmCheck(ctx context.Context, activityID uuid.UUID, overdue bool) error {
	a, err := s.GetActivity(ctx, activityID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if a.Status != "draft" {
		return nil // already confirmed (submitted) or otherwise moved on
	}
	if s.Notify == nil {
		return nil
	}
	recipients, err := s.draftConfirmRecipients(ctx, a.OwnerUserID)
	if err != nil {
		return err
	}
	code := "ropa.draft_confirm_reminder"
	if overdue {
		code = "ropa.draft_confirm_overdue"
	}
	vars := map[string]any{"activity_name": a.Name, "activity_code": a.Code}
	for _, uid := range recipients {
		uid := uid
		for _, ch := range []string{"in_app", "email"} {
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: code, Channel: ch,
					RecipientUserID: &uid, Vars: vars, EntityType: ActivityCollabEntityType, EntityID: &activityID, Urgent: false})
				return err
			})
			if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
				return err
			}
		}
	}
	return nil
}

func (s *Service) draftConfirmRecipients(ctx context.Context, owner *uuid.UUID) ([]uuid.UUID, error) {
	if owner != nil {
		return []uuid.UUID{*owner}, nil
	}
	return iamservice.UsersWithRole(ctx, "OWNER")
}

// DraftConfirmCheckWorker works ropa.draft_confirm_check.
type DraftConfirmCheckWorker struct {
	river.WorkerDefaults[DraftConfirmCheckArgs]
	Service *Service
}

func (w *DraftConfirmCheckWorker) Work(ctx context.Context, job *river.Job[DraftConfirmCheckArgs]) error {
	id, err := uuid.Parse(job.Args.ActivityID)
	if err != nil {
		return river.JobCancel(err)
	}
	return w.Service.FireDraftConfirmCheck(ctx, id, job.Args.Overdue)
}
