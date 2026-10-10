package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	dsarstore "pdpa-platform/internal/dsar/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// ReminderWindow is how long before due_at the SLA reminder fires (DSAR-07's own acceptance criterion:
// "คำขอที่เหลือ 7 วันถูกแจ้งเตือน" — a request with 7 days left is notified).
const ReminderWindow = 7 * 24 * time.Hour

// AtRiskWindow marks a request at_risk once this close to its deadline — 10 days left, matching
// docs/legal/pdpa-rules.md's worked example of "at_risk วันที่ 20" for the default 30-day type (20 elapsed
// of 30 leaves 10) while staying correct for any other request_types.sla_days value.
const AtRiskWindow = 10 * 24 * time.Hour

// SLAStatus is on_track | at_risk | overdue, computed live off due_at — never persisted, so it can never go
// stale the way a stored column would.
func SLAStatus(now, dueAt time.Time) string {
	switch {
	case now.After(dueAt):
		return "overdue"
	case !dueAt.Add(-AtRiskWindow).After(now):
		return "at_risk"
	default:
		return "on_track"
	}
}

// ReminderAt is when DSAR-07's single reminder checkpoint fires for a request.
func ReminderAt(dueAt time.Time) time.Time {
	return dueAt.Add(-ReminderWindow)
}

// ReminderArgs is dsar.sla_reminder: the one checkpoint of a request's 30-day clock (ม.30 วรรคสาม). DueAt
// pins the schedule so a stale tick (after the request's due date changed, which cannot happen today, or the
// request closed) is harmless.
type ReminderArgs struct {
	jobs.TenantArgs
	RequestID string `json:"request_id"`
	DueAt     string `json:"due_at"`
}

func (ReminderArgs) Kind() string { return "dsar.sla_reminder" }

// scheduleReminder enqueues the single checkpoint at ReminderAt(dueAt), or immediately if that moment has
// already passed (a request created with fewer than 7 days left on its clock still alerts once, at once —
// the same "late-recorded event still alerts" rule PNG-04 uses).
func (s *Service) scheduleReminder(ctx context.Context, req Request) error {
	if s.River == nil {
		return nil
	}
	at := ReminderAt(req.DueAt)
	if now := s.now(); at.Before(now) {
		at = now
	}
	tenant, err := tenantID(ctx)
	if err != nil {
		return err
	}
	args := ReminderArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, RequestID: req.ID.String(),
		DueAt: req.DueAt.Format(time.RFC3339Nano)}
	_, err = jobs.Enqueue(ctx, s.River, args, &river.InsertOpts{ScheduledAt: at, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}

// FireReminder runs the checkpoint: while the request is still open, it notifies the assignee (if one is
// set) and role DPO that only ReminderWindow remains before due_at.
func (s *Service) FireReminder(ctx context.Context, id uuid.UUID, dueAt time.Time) error {
	req, err := s.GetRequest(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !req.DueAt.Equal(dueAt.UTC()) {
		return nil // stale schedule
	}
	if isClosed(req.Status) {
		return nil
	}
	daysLeft := int(req.DueAt.Sub(s.now()).Round(24*time.Hour) / (24 * time.Hour))
	if daysLeft < 0 {
		daysLeft = 0
	}
	vars := map[string]any{"request_no": req.RequestNo, "days_left": daysLeft, "due": req.DueAt.In(bangkokLoc()).Format("2 Jan 2006")}
	return s.alertSLA(ctx, req, vars)
}

func isClosed(status string) bool {
	switch status {
	case "completed", "rejected", "withdrawn":
		return true
	default:
		return false
	}
}

// alertSLA notifies the request's assignee (if set) plus every user with role DPO — there is no per-record
// routing yet (DSAR-08's workflow engine, not built), the same "default recipients until real routing exists"
// fallback BRE-07/PNG-04 already use.
func (s *Service) alertSLA(ctx context.Context, req Request, vars map[string]any) error {
	if s.Notify == nil {
		return nil
	}
	to, err := iamservice.UsersWithRole(ctx, "DPO")
	if err != nil {
		return err
	}
	if req.AssigneeUserID != nil {
		to = append(to, *req.AssigneeUserID)
	}
	seen := map[uuid.UUID]bool{}
	for _, u := range to {
		if seen[u] {
			continue
		}
		seen[u] = true
		uid := u
		for _, ch := range []string{"in_app", "email"} {
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "dsar.sla_reminder", Channel: ch, RecipientUserID: &uid,
					Vars: vars, EntityType: "dsar_request", EntityID: &req.ID, Urgent: false})
				return err
			})
			if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
				return err
			}
		}
	}
	return nil
}

// ReminderWorker works dsar.sla_reminder.
type ReminderWorker struct {
	river.WorkerDefaults[ReminderArgs]
	Service *Service
}

func (w *ReminderWorker) Work(ctx context.Context, job *river.Job[ReminderArgs]) error {
	id, err1 := uuid.Parse(job.Args.RequestID)
	due, err2 := time.Parse(time.RFC3339Nano, job.Args.DueAt)
	if err := errors.Join(err1, err2); err != nil {
		return river.JobCancel(err)
	}
	return w.Service.FireReminder(ctx, id, due)
}

// AssignRequest sets (or clears, with nil) the request's responsible person — DSAR-07's "ผู้รับผิดชอบ", who
// is notified alongside role DPO when the SLA reminder fires. DSAR-08 (workflow & subtasks) is the module
// that will eventually resolve this from a task claim; until then it is set directly.
func (s *Service) AssignRequest(ctx context.Context, id uuid.UUID, rowVersion int32, userID *uuid.UUID) (Request, error) {
	before, err := s.GetRequest(ctx, id)
	if err != nil {
		return Request{}, err
	}
	if userID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*userID})
		if err != nil {
			return Request{}, err
		}
		if _, ok := names[*userID]; !ok {
			return Request{}, fmt.Errorf("%w: assignee_user_id", ErrInvalid)
		}
	}
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.UpdateRequestAssignee(ctx, dsarstore.UpdateRequestAssigneeParams{ID: id, AssigneeUserID: pgUUID(userID), RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrVersionMismatch
	}
	if err != nil {
		return Request{}, err
	}
	out := toRequest(dsarstore.GetRequestRow(row))
	return out, s.audit(ctx, "dsar.request.assign", out.ID, requestAudit(before), requestAudit(out))
}

func requestAudit(r Request) map[string]any {
	return map[string]any{"status": r.Status, "assignee_user_id": r.AssigneeUserID}
}

func bangkokLoc() *time.Location {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		return time.UTC
	}
	return loc
}

func tenantID(ctx context.Context) (uuid.UUID, error) {
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(t)
}

func pgUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}
