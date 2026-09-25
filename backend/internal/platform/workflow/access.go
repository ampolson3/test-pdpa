package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	workflowstore "pdpa-platform/internal/platform/workflow/store"
)

type access struct {
	read   bool // may see the instance
	act    bool // may move it (transitions)
	writer bool // holds the record's write permission (may also reassign work)
}

// access decides what the signed-in user may do with an instance: the record's permissions (Policy
// registered by its module) or being involved — assigned the task, directly or through a group. Only the
// people on the current open task (or writers) may move it on.
func (s *Service) access(ctx context.Context, inst Instance) (access, error) {
	user, g, ok := currentUser(ctx)
	if !ok {
		return access{}, nil
	}
	var a access
	if p, ok := s.policy(inst.EntityType); ok {
		a.writer = p.WritePermission != "" && g.Has(p.WritePermission)
		a.read = a.writer || (p.ReadPermission != "" && g.Has(p.ReadPermission))
		a.act = a.writer
	}
	tasks, err := workflowstore.New(pdb.MustTxFromContext(ctx)).ListTasks(ctx, inst.ID)
	if err != nil {
		return a, err
	}
	var groups []uuid.UUID
	for _, t := range tasks {
		if t.AssigneeGroupID.Valid && groups == nil {
			if groups, err = iamservice.GroupIDsOf(ctx, user); err != nil {
				return a, err
			}
			groups = append(groups, uuid.Nil) // loaded (possibly none)
		}
		involved := (t.AssigneeUserID.Valid && uuid.UUID(t.AssigneeUserID.Bytes) == user) ||
			(t.AssigneeGroupID.Valid && slices.Contains(groups, uuid.UUID(t.AssigneeGroupID.Bytes)))
		if involved {
			a.read = true
			if t.State == inst.State && (t.Status == "open" || t.Status == "in_progress") {
				a.act = true
			}
		}
	}
	return a, nil
}

// Task is a piece of work in a workflow.
type Task struct {
	ID              uuid.UUID
	InstanceID      uuid.UUID
	State           string
	Title           Text
	AssigneeUserID  *uuid.UUID
	AssigneeName    string
	AssigneeGroupID *uuid.UUID
	GroupName       string
	Status          string
	DueAt           *time.Time
	CompletedAt     *time.Time
	Outcome         string
	Comment         string
	RowVersion      int32
	CreatedAt       time.Time
}

// Timer is an SLA timer as shown to users.
type Timer struct {
	Code      string
	Mode      string
	StartedAt time.Time
	DueAt     time.Time
	Status    string // running | met | breached | stopped
	Paused    bool
	StoppedAt *time.Time
	Reminders []ReminderView
}

// ReminderView is one reminder of a timer.
type ReminderView struct {
	At   time.Time
	Sent bool
}

// HistoryEntry is one line of an instance's timeline.
type HistoryEntry struct {
	At        time.Time
	ActorName string // empty for the system
	Action    string
	Before    map[string]any
	After     map[string]any
}

// InstanceView is everything the workflow panel shows.
type InstanceView struct {
	Instance
	Workflow    DefinitionRecord
	Tasks       []Task
	Timers      []Timer
	History     []HistoryEntry
	Transitions []Transition // what the caller may do now
}

// Get returns an instance the caller may see.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (InstanceView, error) {
	q := workflowstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetInstance(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return InstanceView{}, ErrNotFound
	}
	if err != nil {
		return InstanceView{}, err
	}
	inst := toInstance(row)
	a, err := s.access(ctx, inst)
	if err != nil {
		return InstanceView{}, err
	}
	if !a.read {
		return InstanceView{}, ErrNotFound
	}
	def, err := s.GetDefinition(ctx, inst.DefinitionID)
	if err != nil {
		return InstanceView{}, err
	}
	v := InstanceView{Instance: inst, Workflow: def}
	if a.act && inst.CompletedAt == nil {
		v.Transitions = def.Definition.next(inst.State)
	}
	trows, err := q.ListTasks(ctx, id)
	if err != nil {
		return InstanceView{}, err
	}
	for _, t := range trows {
		v.Tasks = append(v.Tasks, toTask(t, def.Definition))
	}
	if err := s.nameTasks(ctx, v.Tasks); err != nil {
		return InstanceView{}, err
	}
	timers, err := q.ListTimers(ctx, id)
	if err != nil {
		return InstanceView{}, err
	}
	for _, t := range timers {
		v.Timers = append(v.Timers, toTimer(workflowstore.LockTimerRow(t)))
	}
	hist, err := q.InstanceHistory(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		return InstanceView{}, err
	}
	var actors []uuid.UUID
	for _, h := range hist {
		if h.ActorID.Valid {
			actors = append(actors, uuid.UUID(h.ActorID.Bytes))
		}
	}
	names, err := iamservice.Names(ctx, actors)
	if err != nil {
		return InstanceView{}, err
	}
	for _, h := range hist {
		e := HistoryEntry{At: h.OccurredAt.Time, Action: h.Action}
		if h.ActorID.Valid {
			e.ActorName = names[uuid.UUID(h.ActorID.Bytes)]
		}
		_ = json.Unmarshal(h.Before, &e.Before)
		_ = json.Unmarshal(h.After, &e.After)
		v.History = append(v.History, e)
	}
	return v, nil
}

// TaskItem is a task on the caller's board, with its workflow.
type TaskItem struct {
	Task
	EntityType   string
	EntityID     uuid.UUID
	WorkflowName string
	WorkflowCode string
	StateLabel   Text
	SLAStatus    string
	SLADueAt     *time.Time
}

// MyTasks is the caller's open work: assigned to them, or to one of their groups and not yet claimed.
func (s *Service) MyTasks(ctx context.Context) ([]TaskItem, error) {
	user, _, ok := currentUser(ctx)
	if !ok {
		return nil, ErrForbidden
	}
	groups, err := iamservice.GroupIDsOf(ctx, user)
	if err != nil {
		return nil, err
	}
	if groups == nil {
		groups = []uuid.UUID{}
	}
	rows, err := workflowstore.New(pdb.MustTxFromContext(ctx)).MyTasks(ctx, workflowstore.MyTasksParams{UserID: pgtype.UUID{Bytes: user, Valid: true}, GroupIds: groups})
	if err != nil {
		return nil, err
	}
	defs := map[uuid.UUID]Definition{}
	out := make([]TaskItem, 0, len(rows))
	for _, r := range rows {
		d, ok := defs[r.DefinitionID]
		if !ok {
			rec, err := s.GetDefinition(ctx, r.DefinitionID)
			if err != nil {
				return nil, err
			}
			d, defs[r.DefinitionID] = rec.Definition, rec.Definition
		}
		t := toTask(workflowstore.ListTasksRow{ID: r.ID, InstanceID: r.InstanceID, State: r.State, Title: r.Title, AssigneeUserID: r.AssigneeUserID,
			AssigneeGroupID: r.AssigneeGroupID, Status: r.Status, DueAt: r.DueAt, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt}, d)
		st, _ := d.state(r.CurrentState)
		item := TaskItem{Task: t, EntityType: r.EntityType, EntityID: r.EntityID, WorkflowName: r.WorkflowName, WorkflowCode: r.WorkflowCode,
			StateLabel: st.Label, SLAStatus: r.SlaStatus}
		if r.SlaDueAt.Valid {
			tm := r.SlaDueAt.Time
			item.SLADueAt = &tm
		}
		out = append(out, item)
	}
	tasks := make([]Task, len(out))
	for i := range out {
		tasks[i] = out[i].Task
	}
	if err := s.nameTasks(ctx, tasks); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Task = tasks[i]
	}
	return out, nil
}

// TaskChange is what a user may change on a task: claim or reassign it, and mark it in progress.
type TaskChange struct {
	AssigneeUserID  *uuid.UUID
	AssigneeGroupID *uuid.UUID
	Status          string // "" | open | in_progress
}

// UpdateTask applies a change: a member of the task's group may claim it (assign it to themselves); the
// assignee may start it; the record's writers may also reassign it to another user or group.
func (s *Service) UpdateTask(ctx context.Context, id uuid.UUID, version int32, c TaskChange) (Task, error) {
	q := workflowstore.New(pdb.MustTxFromContext(ctx))
	t, err := q.GetTask(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	irow, err := q.GetInstance(ctx, t.InstanceID)
	if err != nil {
		return Task{}, err
	}
	inst := toInstance(irow)
	a, err := s.access(ctx, inst)
	if err != nil {
		return Task{}, err
	}
	if !a.read {
		return Task{}, ErrNotFound
	}
	if t.RowVersion != version {
		return Task{}, ErrVersionMismatch
	}
	if t.Status != "open" && t.Status != "in_progress" {
		return Task{}, ErrInvalidTransition
	}
	if c.Status != "" && c.Status != "open" && c.Status != "in_progress" {
		return Task{}, fmt.Errorf("%w: status", ErrInvalidRequest)
	}
	me, _, _ := currentUser(ctx)
	user, group, status := t.AssigneeUserID, t.AssigneeGroupID, t.Status
	changed := false
	switch {
	case c.AssigneeGroupID != nil:
		if !a.writer {
			return Task{}, ErrForbidden
		}
		if n, err := iamservice.GroupNames(ctx, []uuid.UUID{*c.AssigneeGroupID}); err != nil {
			return Task{}, err
		} else if _, ok := n[*c.AssigneeGroupID]; !ok {
			return Task{}, fmt.Errorf("%w: group", ErrInvalidRequest)
		}
		group, user, changed = pgtype.UUID{Bytes: *c.AssigneeGroupID, Valid: true}, pgtype.UUID{}, true
		if c.AssigneeUserID != nil {
			return Task{}, fmt.Errorf("%w: one assignee", ErrInvalidRequest)
		}
	case c.AssigneeUserID != nil && (!user.Valid || uuid.UUID(user.Bytes) != *c.AssigneeUserID):
		claim := *c.AssigneeUserID == me && !user.Valid && group.Valid && a.read && t.State == inst.State
		if claim {
			groups, err := iamservice.GroupIDsOf(ctx, me)
			if err != nil {
				return Task{}, err
			}
			claim = slices.Contains(groups, uuid.UUID(group.Bytes))
		}
		if !claim && !a.writer {
			return Task{}, ErrForbidden
		}
		if n, err := iamservice.Names(ctx, []uuid.UUID{*c.AssigneeUserID}); err != nil {
			return Task{}, err
		} else if _, ok := n[*c.AssigneeUserID]; !ok {
			return Task{}, fmt.Errorf("%w: user", ErrInvalidRequest)
		}
		user, changed = pgtype.UUID{Bytes: *c.AssigneeUserID, Valid: true}, true
	}
	if c.Status != "" && c.Status != status {
		if !a.writer && !(user.Valid && uuid.UUID(user.Bytes) == me) {
			return Task{}, ErrForbidden
		}
		status = c.Status
	}
	r, err := q.UpdateTask(ctx, workflowstore.UpdateTaskParams{ID: id, RowVersion: version, AssigneeUserID: user, AssigneeGroupID: group, Status: status})
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrVersionMismatch
	}
	if err != nil {
		return Task{}, err
	}
	def, err := s.GetDefinition(ctx, inst.DefinitionID)
	if err != nil {
		return Task{}, err
	}
	out := []Task{toTask(workflowstore.ListTasksRow(r), def.Definition)}
	if err := s.nameTasks(ctx, out); err != nil {
		return Task{}, err
	}
	if changed {
		recipients, err := s.assigneesOf(ctx, uuidPtr(user), uuidPtr(group))
		if err != nil {
			return Task{}, err
		}
		due := "-"
		if r.DueAt.Valid {
			due = displayTime(r.DueAt.Time)
		}
		if err := s.notify(ctx, "workflow.task_assigned", recipients, inst, map[string]any{"title": r.Title, "workflow": def.Name, "due": due}); err != nil {
			return Task{}, err
		}
	}
	return out[0], s.audit(ctx, "platform.workflow.task_update", EntityType, inst.ID,
		map[string]any{"task_id": id, "assignee_user_id": uuidPtr(t.AssigneeUserID), "assignee_group_id": uuidPtr(t.AssigneeGroupID), "status": t.Status},
		map[string]any{"task_id": id, "assignee_user_id": uuidPtr(user), "assignee_group_id": uuidPtr(group), "status": status})
}

// nameTasks fills assignee and group names.
func (s *Service) nameTasks(ctx context.Context, ts []Task) error {
	var users, groups []uuid.UUID
	for _, t := range ts {
		if t.AssigneeUserID != nil {
			users = append(users, *t.AssigneeUserID)
		}
		if t.AssigneeGroupID != nil {
			groups = append(groups, *t.AssigneeGroupID)
		}
	}
	un, err := iamservice.Names(ctx, users)
	if err != nil {
		return err
	}
	gn, err := iamservice.GroupNames(ctx, groups)
	if err != nil {
		return err
	}
	for i := range ts {
		if ts[i].AssigneeUserID != nil {
			ts[i].AssigneeName = un[*ts[i].AssigneeUserID]
		}
		if ts[i].AssigneeGroupID != nil {
			ts[i].GroupName = gn[*ts[i].AssigneeGroupID]
		}
	}
	return nil
}

func toTask(r workflowstore.ListTasksRow, d Definition) Task {
	t := Task{ID: r.ID, InstanceID: r.InstanceID, State: r.State, Title: Text{"th": r.Title}, AssigneeUserID: uuidPtr(r.AssigneeUserID),
		AssigneeGroupID: uuidPtr(r.AssigneeGroupID), Status: r.Status, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if st, ok := d.state(r.State); ok && st.Task != nil {
		t.Title = st.Task.Title
	}
	if r.DueAt.Valid {
		v := r.DueAt.Time
		t.DueAt = &v
	}
	if r.CompletedAt.Valid {
		v := r.CompletedAt.Time
		t.CompletedAt = &v
	}
	if r.Outcome != nil {
		t.Outcome = *r.Outcome
	}
	if r.Comment != nil {
		t.Comment = *r.Comment
	}
	return t
}

func toTimer(r workflowstore.LockTimerRow) Timer {
	t := Timer{Code: r.Code, Mode: r.Mode, StartedAt: r.StartedAt.Time, DueAt: r.DueAt.Time, Status: r.Status, Paused: r.PausedAt.Valid}
	if r.StoppedAt.Valid {
		v := r.StoppedAt.Time
		t.StoppedAt = &v
	}
	var rs []reminder
	_ = json.Unmarshal(r.Reminders, &rs)
	for _, x := range rs {
		t.Reminders = append(t.Reminders, ReminderView{At: x.At, Sent: x.SentAt != nil})
	}
	return t
}
