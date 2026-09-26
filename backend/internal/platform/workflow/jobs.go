package workflow

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	workflowstore "pdpa-platform/internal/platform/workflow/store"
)

// TickArgs is workflow.sla_tick: look at one SLA timer at a moment it was scheduled for — a reminder
// or the due time — and send what is due. Scheduled with River's ScheduledAt; extra or stale ticks
// (after a pause, a resume or completion) find nothing to do.
type TickArgs struct {
	jobs.TenantArgs
	TimerID string `json:"timer_id"`
}

func (TickArgs) Kind() string { return "workflow.sla_tick" }

// Ticker works workflow.sla_tick.
type Ticker struct {
	river.WorkerDefaults[TickArgs]
	Service *Service
}

func (w *Ticker) Work(ctx context.Context, job *river.Job[TickArgs]) error {
	id, err := uuid.Parse(job.Args.TimerID)
	if err != nil {
		return river.JobCancel(err)
	}
	return w.Service.Tick(ctx, id)
}

// Tick sends the timer's reminders whose time has come and, once the due time has passed, marks it
// breached and escalates; then schedules the next tick. It is idempotent.
func (s *Service) Tick(ctx context.Context, timerID uuid.UUID) error {
	q := workflowstore.New(pdb.MustTxFromContext(ctx))
	t, err := q.LockTimer(ctx, timerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // gone with its instance
	}
	if err != nil {
		return err
	}
	if t.StoppedAt.Valid || t.PausedAt.Valid {
		return nil // finished, or resumed later by a transition that schedules its own tick
	}
	irow, err := q.GetInstance(ctx, t.InstanceID)
	if err != nil {
		return err
	}
	inst := toInstance(irow)
	def, err := s.GetDefinition(ctx, inst.DefinitionID)
	if err != nil {
		return err
	}
	now := s.now()
	var rs []reminder
	_ = json.Unmarshal(t.Reminders, &rs)
	st, _ := def.Definition.state(inst.State)
	vars := map[string]any{"workflow": def.Name, "state": st.Label["th"], "due": displayTime(t.DueAt.Time)}
	event := SLAEvent{InstanceID: inst.ID, EntityType: inst.EntityType, EntityID: inst.EntityID, Code: t.Code, DueAt: t.DueAt.Time}

	reminded := false
	for i := range rs {
		if rs[i].SentAt == nil && !rs[i].At.After(now) {
			sent := now
			rs[i].SentAt = &sent
			reminded = true
		}
	}
	status, escalated := t.Status, t.EscalatedAt
	breached := status == "running" && !t.DueAt.Time.After(now)
	if breached {
		status, escalated = "breached", pgtype.Timestamptz{Time: now, Valid: true}
	}
	if !reminded && !breached {
		return s.scheduleTick(ctx, t.ID, nextTick(rs, t.DueAt.Time, status))
	}
	body, _ := json.Marshal(rs)
	if err := q.UpdateTimer(ctx, workflowstore.UpdateTimerParams{ID: t.ID, DueAt: t.DueAt, Reminders: body, EscalatedAt: escalated,
		StoppedAt: t.StoppedAt, PausedAt: t.PausedAt, Status: status}); err != nil {
		return err
	}
	assignees, err := s.openAssignees(ctx, inst)
	if err != nil {
		return err
	}
	if reminded && !breached { // a breach supersedes reminders that were late in the same tick
		if err := s.notify(ctx, "workflow.sla_reminder", assignees, inst, vars); err != nil {
			return err
		}
		if err := s.slaHook(ctx, inst, event, "reminder"); err != nil {
			return err
		}
		if inst.SLAStatus == "on_track" {
			if _, err := s.setSLAStatus(ctx, inst, "at_risk"); err != nil {
				return err
			}
		}
		if err := s.audit(ctx, "platform.workflow.sla_reminder", EntityType, inst.ID, nil, map[string]any{"sla": t.Code, "due_at": t.DueAt.Time}); err != nil {
			return err
		}
	}
	if breached {
		to := assignees
		if sla := def.Definition.SLA; sla != nil {
			to = append(to, sla.EscalateUserIDs...)
			if sla.EscalateGroupID != nil {
				members, err := s.assigneesOf(ctx, nil, sla.EscalateGroupID)
				if err != nil {
					return err
				}
				to = append(to, members...)
			}
		}
		if err := s.notify(ctx, "workflow.sla_breached", to, inst, vars); err != nil {
			return err
		}
		if err := s.slaHook(ctx, inst, event, "breached"); err != nil {
			return err
		}
		if _, err := s.setSLAStatus(ctx, inst, "overdue"); err != nil {
			return err
		}
		if err := s.audit(ctx, "platform.workflow.sla_breached", EntityType, inst.ID, nil, map[string]any{"sla": t.Code, "due_at": t.DueAt.Time}); err != nil {
			return err
		}
	}
	return s.scheduleTick(ctx, t.ID, nextTick(rs, t.DueAt.Time, status))
}

func (s *Service) slaHook(ctx context.Context, inst Instance, e SLAEvent, kind string) error {
	p, ok := s.policy(inst.EntityType)
	if !ok || p.OnSLA == nil {
		return nil
	}
	e.Kind = kind
	return p.OnSLA(ctx, e)
}

// openAssignees are the people on the instance's open tasks (users, or the members of groups).
func (s *Service) openAssignees(ctx context.Context, inst Instance) ([]uuid.UUID, error) {
	tasks, err := workflowstore.New(pdb.MustTxFromContext(ctx)).ListTasks(ctx, inst.ID)
	if err != nil {
		return nil, err
	}
	var out []uuid.UUID
	for _, t := range tasks {
		if t.Status != "open" && t.Status != "in_progress" {
			continue
		}
		us, err := s.assigneesOf(ctx, uuidPtr(t.AssigneeUserID), uuidPtr(t.AssigneeGroupID))
		if err != nil {
			return nil, err
		}
		out = append(out, us...)
	}
	return out, nil
}
