package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/bizcal"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	workflowstore "pdpa-platform/internal/platform/workflow/store"
)

// Instance is a running (or finished) workflow of one record.
type Instance struct {
	ID           uuid.UUID
	DefinitionID uuid.UUID
	EntityType   string
	EntityID     uuid.UUID
	State        string
	StartedAt    time.Time
	CompletedAt  *time.Time
	SLAStatus    string // on_track | at_risk | overdue | paused | done
	RowVersion   int32
}

// StartInput names the definition (by code: the tenant's newest version, else the global one) and the
// record. At is when the SLA starts counting (e.g. when a request was received); zero means now.
type StartInput struct {
	DefinitionCode string
	EntityType     string
	EntityID       uuid.UUID
	At             time.Time
}

// reminder is one entry of platform.sla_timers.reminders.
type reminder struct {
	Before int        `json:"before"`
	At     time.Time  `json:"at"`
	SentAt *time.Time `json:"sent_at,omitempty"`
}

// Start begins a workflow for a record. Modules call it from their own service (it is trusted: the
// module has already checked that the caller may create its record), in the request's transaction.
func (s *Service) Start(ctx context.Context, in StartInput) (Instance, error) {
	q := workflowstore.New(pdb.MustTxFromContext(ctx))
	dr, err := q.ActiveDefinitionByCode(ctx, in.DefinitionCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, fmt.Errorf("%w: no workflow %q", ErrNotFound, in.DefinitionCode)
	}
	if err != nil {
		return Instance{}, err
	}
	def, err := toDefinition(workflowstore.GetDefinitionRow(dr))
	if err != nil {
		return Instance{}, err
	}
	if def.EntityType != in.EntityType {
		return Instance{}, fmt.Errorf("%w: workflow %q is for %s, not %s", ErrInvalidRequest, def.Code, def.EntityType, in.EntityType)
	}
	now := s.now()
	start := in.At.UTC()
	if in.At.IsZero() {
		start = now
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Instance{}, err
	}
	row, err := q.InsertInstance(ctx, workflowstore.InsertInstanceParams{
		ID: id, DefinitionID: def.ID, EntityType: in.EntityType, EntityID: in.EntityID, CurrentState: def.Definition.Initial,
		StartedAt: pgtype.Timestamptz{Time: start, Valid: true},
	})
	if err != nil {
		return Instance{}, err
	}
	inst := toInstance(workflowstore.GetInstanceRow(row))
	initial, _ := def.Definition.state(def.Definition.Initial)
	if sla := def.Definition.SLA; sla != nil {
		if err := s.startTimer(ctx, inst, *sla, start, initial.PauseSLA); err != nil {
			return Instance{}, err
		}
		if initial.PauseSLA {
			if inst, err = s.setSLAStatus(ctx, inst, "paused"); err != nil {
				return Instance{}, err
			}
		}
	}
	if err := s.enterState(ctx, inst, def, initial, start); err != nil {
		return Instance{}, err
	}
	return inst, s.audit(ctx, "platform.workflow.start", EntityType, inst.ID, nil,
		map[string]any{"workflow": def.Code, "version": def.Version, "entity_type": in.EntityType, "entity_id": in.EntityID, "state": inst.State})
}

func (s *Service) startTimer(ctx context.Context, inst Instance, sla SLASpec, start time.Time, paused bool) error {
	cal, calID, err := s.Calendars.BusinessCalendar(ctx, sla.CalendarID)
	if err != nil {
		return err
	}
	due, err := DueAt(cal, start, sla.Duration)
	if err != nil {
		return err
	}
	rs, err := remindersOn(cal, start, due, sla)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(rs)
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	p := workflowstore.InsertTimerParams{ID: id, InstanceID: inst.ID, Code: sla.Code, Mode: sla.Mode,
		StartedAt: pgtype.Timestamptz{Time: start, Valid: true}, DueAt: pgtype.Timestamptz{Time: due, Valid: true}, Reminders: body}
	if calID != nil {
		p.CalendarID = pgtype.UUID{Bytes: *calID, Valid: true}
	}
	t, err := workflowstore.New(pdb.MustTxFromContext(ctx)).InsertTimer(ctx, p)
	if err != nil {
		return err
	}
	if paused {
		return workflowstore.New(pdb.MustTxFromContext(ctx)).UpdateTimer(ctx, workflowstore.UpdateTimerParams{
			ID: t.ID, DueAt: t.DueAt, Reminders: t.Reminders, Status: t.Status, PausedAt: pgtype.Timestamptz{Time: s.now(), Valid: true}})
	}
	return s.scheduleTick(ctx, t.ID, nextTick(rs, due, t.Status))
}

// remindersOn computes a timer's reminders (none sent yet) from its SLA, in time order.
func remindersOn(cal bizcal.Calendar, start, due time.Time, sla SLASpec) ([]reminder, error) {
	var out []reminder
	for _, b := range sla.RemindBefore {
		t, err := reminderAt(cal, due, sla.Mode, b)
		if err != nil {
			return nil, err
		}
		if t.After(start) {
			out = append(out, reminder{Before: b, At: t})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// Transition moves an instance to another state: the open tasks of the current state are done with the
// target as outcome (and comment), the SLA clock pauses or resumes as the states say, and the target's
// task opens. A terminal target completes the instance; its SLA is then met unless it was breached.
func (s *Service) Transition(ctx context.Context, id uuid.UUID, version int32, to, comment string) (Instance, error) {
	q := workflowstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.LockInstance(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, err
	}
	inst := toInstance(workflowstore.GetInstanceRow(row))
	def, err := s.GetDefinition(ctx, inst.DefinitionID)
	if err != nil {
		return Instance{}, err
	}
	access, err := s.access(ctx, inst)
	if err != nil {
		return Instance{}, err
	}
	if !access.read {
		return Instance{}, ErrNotFound
	}
	if inst.CompletedAt != nil {
		return Instance{}, ErrInvalidTransition
	}
	if !access.act {
		return Instance{}, ErrForbidden
	}
	if inst.RowVersion != version {
		return Instance{}, ErrVersionMismatch
	}
	comment = strings.TrimSpace(comment)
	if len([]rune(comment)) > 2000 {
		return Instance{}, fmt.Errorf("%w: comment too long", ErrInvalidRequest)
	}
	if !def.Definition.allowed(inst.State, to) {
		return Instance{}, ErrInvalidTransition
	}
	from, _ := def.Definition.state(inst.State)
	target, _ := def.Definition.state(to)
	now := s.now()

	if _, err := q.CloseOpenTasks(ctx, workflowstore.CloseOpenTasksParams{InstanceID: inst.ID, Status: "done", Outcome: &to,
		Comment: optional(comment), CompletedAt: pgtype.Timestamptz{Time: now, Valid: true}}); err != nil {
		return Instance{}, err
	}
	slaStatus, err := s.moveTimers(ctx, inst, from, target, now)
	if err != nil {
		return Instance{}, err
	}
	var completed pgtype.Timestamptz
	if target.Terminal {
		completed = pgtype.Timestamptz{Time: now, Valid: true}
		slaStatus = "done"
	}
	if slaStatus == "" {
		slaStatus = inst.SLAStatus
	}
	updated, err := q.UpdateInstance(ctx, workflowstore.UpdateInstanceParams{ID: inst.ID, RowVersion: inst.RowVersion, CurrentState: to, CompletedAt: completed, SlaStatus: slaStatus})
	if err != nil {
		return Instance{}, err
	}
	next := toInstance(workflowstore.GetInstanceRow(updated))
	if err := s.enterState(ctx, next, def, target, now); err != nil {
		return Instance{}, err
	}
	if p, ok := s.policy(inst.EntityType); ok && p.OnTransition != nil {
		if err := p.OnTransition(ctx, TransitionEvent{InstanceID: inst.ID, EntityType: inst.EntityType, EntityID: inst.EntityID, From: from.Key, To: to, Completed: target.Terminal}); err != nil {
			return Instance{}, err
		}
	}
	after := map[string]any{"state": to}
	if target.Terminal {
		after["completed"] = true
	}
	return next, s.audit(ctx, "platform.workflow.transition", EntityType, inst.ID, map[string]any{"state": from.Key}, after)
}

// moveTimers pauses, resumes or stops the instance's timers for a move from → to and returns the
// instance's resulting SLA status ("" = unchanged).
func (s *Service) moveTimers(ctx context.Context, inst Instance, from, to State, now time.Time) (string, error) {
	q := workflowstore.New(pdb.MustTxFromContext(ctx))
	timers, err := q.ListTimers(ctx, inst.ID)
	if err != nil || len(timers) == 0 {
		return "", err
	}
	status := ""
	for _, tr := range timers {
		t, err := q.LockTimer(ctx, tr.ID)
		if err != nil {
			return "", err
		}
		if t.StoppedAt.Valid {
			continue
		}
		var rs []reminder
		_ = json.Unmarshal(t.Reminders, &rs)
		p := workflowstore.UpdateTimerParams{ID: t.ID, DueAt: t.DueAt, Reminders: t.Reminders, EscalatedAt: t.EscalatedAt, StoppedAt: t.StoppedAt, PausedAt: t.PausedAt, Status: t.Status}
		switch {
		case to.Terminal:
			p.StoppedAt = pgtype.Timestamptz{Time: now, Valid: true}
			p.PausedAt = pgtype.Timestamptz{}
			if t.Status == "running" {
				p.Status = "met"
			}
		case to.PauseSLA && !t.PausedAt.Valid:
			p.PausedAt = pgtype.Timestamptz{Time: now, Valid: true}
			status = "paused"
		case !to.PauseSLA && t.PausedAt.Valid:
			cal, _, err := s.Calendars.BusinessCalendar(ctx, uuidPtr(t.CalendarID))
			if err != nil {
				return "", err
			}
			due, err := Resume(cal, t.Mode, t.DueAt.Time, t.PausedAt.Time, now)
			if err != nil {
				return "", err
			}
			spec := SLASpec{Code: t.Code, Duration: Duration{Mode: t.Mode}}
			for _, r := range rs {
				spec.RemindBefore = append(spec.RemindBefore, r.Before)
			}
			fresh, err := remindersOn(cal, t.StartedAt.Time, due, spec)
			if err != nil {
				return "", err
			}
			for i := range fresh { // a reminder already sent is not sent again
				for _, old := range rs {
					if old.Before == fresh[i].Before {
						fresh[i].SentAt = old.SentAt
					}
				}
			}
			body, _ := json.Marshal(fresh)
			p.DueAt, p.Reminders, p.PausedAt = pgtype.Timestamptz{Time: due, Valid: true}, body, pgtype.Timestamptz{}
			if err := s.scheduleTick(ctx, t.ID, nextTick(fresh, due, t.Status)); err != nil {
				return "", err
			}
			status = statusOf(fresh, t.Status)
		default:
			continue
		}
		if err := q.UpdateTimer(ctx, p); err != nil {
			return "", err
		}
	}
	return status, nil
}

// statusOf is the instance's SLA status for a running timer: overdue once breached, at risk once a
// reminder went out, else on track.
func statusOf(rs []reminder, timerStatus string) string {
	if timerStatus == "breached" {
		return "overdue"
	}
	for _, r := range rs {
		if r.SentAt != nil {
			return "at_risk"
		}
	}
	return "on_track"
}

// enterState opens the state's task, if it has one, and notifies its assignees.
func (s *Service) enterState(ctx context.Context, inst Instance, def DefinitionRecord, st State, at time.Time) error {
	if st.Task == nil {
		return nil
	}
	var due pgtype.Timestamptz
	if st.Task.Due != nil {
		var calID *uuid.UUID
		if def.Definition.SLA != nil {
			calID = def.Definition.SLA.CalendarID
		}
		cal, _, err := s.Calendars.BusinessCalendar(ctx, calID)
		if err != nil {
			return err
		}
		d, err := DueAt(cal, at, *st.Task.Due)
		if err != nil {
			return err
		}
		due = pgtype.Timestamptz{Time: d, Valid: true}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	p := workflowstore.InsertTaskParams{ID: id, InstanceID: inst.ID, State: st.Key, Title: st.Task.Title["th"], DueAt: due}
	if st.Task.AssigneeUserID != nil {
		p.AssigneeUserID = pgtype.UUID{Bytes: *st.Task.AssigneeUserID, Valid: true}
	}
	if st.Task.AssigneeGroupID != nil {
		p.AssigneeGroupID = pgtype.UUID{Bytes: *st.Task.AssigneeGroupID, Valid: true}
	}
	t, err := workflowstore.New(pdb.MustTxFromContext(ctx)).InsertTask(ctx, p)
	if err != nil {
		return err
	}
	recipients, err := s.assigneesOf(ctx, uuidPtr(t.AssigneeUserID), uuidPtr(t.AssigneeGroupID))
	if err != nil {
		return err
	}
	dueText := "-"
	if due.Valid {
		dueText = displayTime(due.Time)
	} else if d, ok, err := s.slaDue(ctx, inst.ID); err != nil {
		return err
	} else if ok {
		dueText = displayTime(d)
	}
	return s.notify(ctx, "workflow.task_assigned", recipients, inst, map[string]any{"title": st.Task.Title["th"], "workflow": def.Name, "due": dueText})
}

// slaDue is the due time of the instance's first running timer.
func (s *Service) slaDue(ctx context.Context, instanceID uuid.UUID) (time.Time, bool, error) {
	timers, err := workflowstore.New(pdb.MustTxFromContext(ctx)).ListTimers(ctx, instanceID)
	if err != nil {
		return time.Time{}, false, err
	}
	for _, t := range timers {
		if !t.StoppedAt.Valid {
			return t.DueAt.Time, true, nil
		}
	}
	return time.Time{}, false, nil
}

// assigneesOf is the user, or the active members of the group.
func (s *Service) assigneesOf(ctx context.Context, user, group *uuid.UUID) ([]uuid.UUID, error) {
	if user != nil {
		return []uuid.UUID{*user}, nil
	}
	if group != nil {
		return iamservice.GroupMembers(ctx, *group)
	}
	return nil, nil
}

// notify sends an in-app notification to each distinct recipient (never to the acting user).
func (s *Service) notify(ctx context.Context, template string, to []uuid.UUID, inst Instance, vars map[string]any) error {
	if s.Notify == nil {
		return nil
	}
	_, actor, _ := actorOf(ctx)
	seen := map[uuid.UUID]bool{}
	for _, u := range to {
		if seen[u] || (actor != nil && *actor == u) {
			continue
		}
		seen[u] = true
		u := u
		id := inst.ID
		if _, err := s.Notify.Send(ctx, notify.Request{TemplateCode: template, Channel: notify.ChannelInApp, RecipientUserID: &u,
			Vars: vars, EntityType: EntityType, EntityID: &id}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) setSLAStatus(ctx context.Context, inst Instance, status string) (Instance, error) {
	if err := workflowstore.New(pdb.MustTxFromContext(ctx)).SetInstanceSLAStatus(ctx, workflowstore.SetInstanceSLAStatusParams{ID: inst.ID, SlaStatus: status}); err != nil {
		return inst, err
	}
	inst.SLAStatus = status
	inst.RowVersion++
	return inst, nil
}

// scheduleTick queues workflow.sla_tick for the timer at t (nothing when t is zero). Ticks that find
// nothing due are harmless, so pausing, resuming or moving a deadline just schedules another one.
func (s *Service) scheduleTick(ctx context.Context, timerID uuid.UUID, t time.Time) error {
	if t.IsZero() || s.River == nil {
		return nil
	}
	tenant, _, err := actorOf(ctx)
	if err != nil {
		return err
	}
	_, err = jobs.Enqueue(ctx, s.River, TickArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, TimerID: timerID.String()},
		&river.InsertOpts{ScheduledAt: t})
	return err
}

// nextTick is the next moment the timer needs attention: its first unsent reminder, else its due time
// while it has not been breached.
func nextTick(rs []reminder, due time.Time, status string) time.Time {
	for _, r := range rs {
		if r.SentAt == nil {
			return r.At
		}
	}
	if status == "running" {
		return due
	}
	return time.Time{}
}

// displayTime formats a deadline for notification text: Bangkok time, ISO-like and language neutral.
func displayTime(t time.Time) string {
	return t.In(bizcal.Bangkok).Format("2006-01-02 15:04")
}

func toInstance(r workflowstore.GetInstanceRow) Instance {
	i := Instance{ID: r.ID, DefinitionID: r.DefinitionID, EntityType: r.EntityType, EntityID: r.EntityID, State: r.CurrentState,
		StartedAt: r.StartedAt.Time, SLAStatus: r.SlaStatus, RowVersion: r.RowVersion}
	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time
		i.CompletedAt = &t
	}
	return i
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	u := uuid.UUID(v.Bytes)
	return &u
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
