// BRE-11: response playbooks & tasks. breach.playbooks (a tenant's own, or a global default — same
// ORG-07-style tenant_read/tenant_write split as master data) names the set of steps a given incident_type
// should trigger; breach.response_tasks are the real, assignable, trackable work items a playbook's steps
// become once applied to a real incident. Both tables were already fully specified in the baseline
// migrations — no migration needed, only this service and its HTTP layer.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	breachstore "pdpa-platform/internal/breach/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
)

// ResponseTaskStatuses: open is the starting state every applied step begins in.
var ResponseTaskStatuses = []string{"open", "in_progress", "done", "cancelled"}

// PlaybookStep is one item of a playbook's own `steps` jsonb array.
type PlaybookStep struct {
	Code  string `json:"code"`
	Title string `json:"title"`
}

// Playbook is a reusable set of response steps for one kind of incident.
type Playbook struct {
	ID           uuid.UUID
	IncidentType string
	Name         string
	Steps        []PlaybookStep
	VersionNo    int32
	RowVersion   int32
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ResponseTask is one real, trackable task against an incident — either applied from a playbook step or
// (step_code/playbook_id both nil) added by hand.
type ResponseTask struct {
	ID              uuid.UUID
	IncidentID      uuid.UUID
	PlaybookID      *uuid.UUID
	StepCode        string
	Title           string
	AssigneeUserID  *uuid.UUID
	AssigneeGroupID *uuid.UUID
	AssigneeName    string
	Status          string
	DueAt           *time.Time
	CompletedAt     *time.Time
	RowVersion      int32
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func toPlaybook(id uuid.UUID, incidentType, name string, steps []byte, versionNo, rowVersion int32, createdAt, updatedAt time.Time) Playbook {
	p := Playbook{ID: id, IncidentType: incidentType, Name: name, VersionNo: versionNo, RowVersion: rowVersion,
		CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC()}
	_ = json.Unmarshal(steps, &p.Steps)
	if p.Steps == nil {
		p.Steps = []PlaybookStep{}
	}
	return p
}

func validatePlaybook(name, incidentType string, steps []PlaybookStep) error {
	var errs []FieldError
	if strings.TrimSpace(name) == "" {
		errs = append(errs, FieldError{"name", "required"})
	}
	if strings.TrimSpace(incidentType) == "" || len([]rune(incidentType)) > 40 {
		errs = append(errs, FieldError{"incident_type", "invalid"})
	}
	if len(steps) == 0 {
		errs = append(errs, FieldError{"steps", "required"})
	}
	seen := map[string]bool{}
	for i, st := range steps {
		if strings.TrimSpace(st.Code) == "" || strings.TrimSpace(st.Title) == "" {
			errs = append(errs, FieldError{"steps", "invalid"})
			break
		}
		if seen[st.Code] {
			errs = append(errs, FieldError{"steps", "duplicate_code"})
			break
		}
		seen[st.Code] = true
		_ = i
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// CreatePlaybook authors a new playbook for the tenant (never a global default — those come only from a
// migration, the same convention ORG-07's own master data already established).
func (s *Service) CreatePlaybook(ctx context.Context, incidentType, name string, steps []PlaybookStep) (Playbook, error) {
	if !has(ctx, PermCreate) {
		return Playbook{}, ErrForbidden
	}
	if err := validatePlaybook(name, incidentType, steps); err != nil {
		return Playbook{}, err
	}
	raw, err := json.Marshal(steps)
	if err != nil {
		return Playbook{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Playbook{}, err
	}
	row, err := breachstore.New(pdb.MustTxFromContext(ctx)).InsertPlaybook(ctx, breachstore.InsertPlaybookParams{
		ID: id, IncidentType: incidentType, Name: name, Steps: raw,
	})
	if err != nil {
		return Playbook{}, err
	}
	out := toPlaybook(row.ID, row.IncidentType, row.Name, row.Steps, row.VersionNo, row.RowVersion, row.CreatedAt.Time, row.UpdatedAt.Time)
	return out, s.audit(ctx, "breach.playbook.create", PlaybookType, out.ID, nil, map[string]any{"incident_type": incidentType, "name": name})
}

// UpdatePlaybook edits the tenant's own playbook (never a global default — RLS's own tenant_write policy
// already refuses that write outright).
func (s *Service) UpdatePlaybook(ctx context.Context, id uuid.UUID, rowVersion int32, name string, steps []PlaybookStep) (Playbook, error) {
	if !has(ctx, PermUpdate) {
		return Playbook{}, ErrForbidden
	}
	cur, err := s.GetPlaybook(ctx, id)
	if err != nil {
		return Playbook{}, err
	}
	if err := validatePlaybook(name, cur.IncidentType, steps); err != nil {
		return Playbook{}, err
	}
	raw, err := json.Marshal(steps)
	if err != nil {
		return Playbook{}, err
	}
	row, err := breachstore.New(pdb.MustTxFromContext(ctx)).UpdatePlaybook(ctx, breachstore.UpdatePlaybookParams{
		ID: id, RowVersion: rowVersion, Name: name, Steps: raw,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Playbook{}, ErrVersionMismatch
	}
	if err != nil {
		return Playbook{}, err
	}
	out := toPlaybook(row.ID, row.IncidentType, row.Name, row.Steps, row.VersionNo, row.RowVersion, row.CreatedAt.Time, row.UpdatedAt.Time)
	return out, s.audit(ctx, "breach.playbook.update", PlaybookType, out.ID, nil, map[string]any{"name": name})
}

func (s *Service) GetPlaybook(ctx context.Context, id uuid.UUID) (Playbook, error) {
	if !has(ctx, PermRead) {
		return Playbook{}, ErrForbidden
	}
	row, err := breachstore.New(pdb.MustTxFromContext(ctx)).GetPlaybook(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Playbook{}, ErrNotFound
	}
	if err != nil {
		return Playbook{}, err
	}
	return toPlaybook(row.ID, row.IncidentType, row.Name, row.Steps, row.VersionNo, row.RowVersion, row.CreatedAt.Time, row.UpdatedAt.Time), nil
}

// ListPlaybooks returns every playbook visible to this tenant (global defaults + its own), optionally
// narrowed to one incident_type — the picker driving the acceptance criterion itself.
func (s *Service) ListPlaybooks(ctx context.Context, incidentType *string) ([]Playbook, error) {
	if !has(ctx, PermRead) {
		return nil, ErrForbidden
	}
	rows, err := breachstore.New(pdb.MustTxFromContext(ctx)).ListPlaybooks(ctx, incidentType)
	if err != nil {
		return nil, err
	}
	out := make([]Playbook, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPlaybook(r.ID, r.IncidentType, r.Name, r.Steps, r.VersionNo, r.RowVersion, r.CreatedAt.Time, r.UpdatedAt.Time))
	}
	return out, nil
}

func fromResponseTaskRow(id, incidentID uuid.UUID, playbookID pgtype.UUID, stepCode *string, title string,
	assigneeUserID, assigneeGroupID pgtype.UUID, status string, dueAt, completedAt pgtype.Timestamptz,
	rowVersion int32, createdAt, updatedAt pgtype.Timestamptz) ResponseTask {
	return ResponseTask{
		ID: id, IncidentID: incidentID, PlaybookID: uuidPtr(playbookID), StepCode: deref(stepCode), Title: title,
		AssigneeUserID: uuidPtr(assigneeUserID), AssigneeGroupID: uuidPtr(assigneeGroupID), Status: status,
		DueAt: timePtr(dueAt), CompletedAt: timePtr(completedAt), RowVersion: rowVersion,
		CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(),
	}
}

// ApplyPlaybook is the acceptance criterion itself ("เลือกประเภทเหตุแล้วได้ชุดงานตาม playbook"): given an
// incident, resolve the playbook to use — explicitly, or by the incident's own incident_type when none is
// given — and turn every one of its steps into a real, trackable breach.response_tasks row. Re-applying
// the same playbook to the same incident is a harmless no-op (returns the tasks already created from it),
// never a duplicate set.
func (s *Service) ApplyPlaybook(ctx context.Context, incidentID uuid.UUID, playbookID *uuid.UUID) ([]ResponseTask, error) {
	if !has(ctx, PermExecute) {
		return nil, ErrForbidden
	}
	inc, err := s.Get(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	var pb Playbook
	if playbookID != nil {
		pb, err = s.GetPlaybook(ctx, *playbookID)
		if err != nil {
			return nil, err
		}
	} else {
		if inc.IncidentType == "" {
			return nil, &ValidationError{Fields: []FieldError{{"incident_type", "required"}}}
		}
		matches, err := s.ListPlaybooks(ctx, &inc.IncidentType)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, &ValidationError{Fields: []FieldError{{"incident_type", "no_playbook"}}}
		}
		pb = matches[0]
	}
	existingCount, err := q.CountResponseTasksByIncidentAndPlaybook(ctx, breachstore.CountResponseTasksByIncidentAndPlaybookParams{
		IncidentID: incidentID, PlaybookID: pgUUID(&pb.ID),
	})
	if err != nil {
		return nil, err
	}
	if existingCount > 0 {
		return s.ListResponseTasks(ctx, incidentID)
	}
	created := make([]ResponseTask, 0, len(pb.Steps))
	for _, st := range pb.Steps {
		taskID, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}
		row, err := q.InsertResponseTask(ctx, breachstore.InsertResponseTaskParams{
			ID: taskID, IncidentID: incidentID, PlaybookID: pgUUID(&pb.ID), StepCode: optText(st.Code), Title: st.Title,
		})
		if err != nil {
			return nil, err
		}
		created = append(created, fromResponseTaskRow(row.ID, row.IncidentID, row.PlaybookID, row.StepCode, row.Title,
			row.AssigneeUserID, row.AssigneeGroupID, row.Status, row.DueAt, row.CompletedAt, row.RowVersion, row.CreatedAt, row.UpdatedAt))
	}
	if err := s.audit(ctx, "breach.playbook.apply", IncidentType, incidentID, nil,
		map[string]any{"playbook_id": pb.ID, "task_count": len(created)}); err != nil {
		return nil, err
	}
	return s.resolveTaskNames(ctx, created)
}

// ListResponseTasks is the per-incident read side.
func (s *Service) ListResponseTasks(ctx context.Context, incidentID uuid.UUID) ([]ResponseTask, error) {
	if !has(ctx, PermRead) {
		return nil, ErrForbidden
	}
	rows, err := breachstore.New(pdb.MustTxFromContext(ctx)).ListResponseTasksByIncident(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	out := make([]ResponseTask, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromResponseTaskRow(r.ID, r.IncidentID, r.PlaybookID, r.StepCode, r.Title,
			r.AssigneeUserID, r.AssigneeGroupID, r.Status, r.DueAt, r.CompletedAt, r.RowVersion, r.CreatedAt, r.UpdatedAt))
	}
	return s.resolveTaskNames(ctx, out)
}

// SetTaskStatus moves a task through open -> in_progress -> done | cancelled, stamping completed_at once
// it reaches a terminal state.
func (s *Service) SetTaskStatus(ctx context.Context, id uuid.UUID, rowVersion int32, status string) (ResponseTask, error) {
	if !has(ctx, PermExecute) {
		return ResponseTask{}, ErrForbidden
	}
	ok := false
	for _, v := range ResponseTaskStatuses {
		if v == status {
			ok = true
			break
		}
	}
	if !ok {
		return ResponseTask{}, &ValidationError{Fields: []FieldError{{"status", "invalid"}}}
	}
	row, err := breachstore.New(pdb.MustTxFromContext(ctx)).SetResponseTaskStatus(ctx, breachstore.SetResponseTaskStatusParams{
		ID: id, RowVersion: rowVersion, Status: status,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ResponseTask{}, ErrVersionMismatch
	}
	if err != nil {
		return ResponseTask{}, err
	}
	out := fromResponseTaskRow(row.ID, row.IncidentID, row.PlaybookID, row.StepCode, row.Title,
		row.AssigneeUserID, row.AssigneeGroupID, row.Status, row.DueAt, row.CompletedAt, row.RowVersion, row.CreatedAt, row.UpdatedAt)
	if err := s.audit(ctx, "breach.response_task.status", ResponseTaskType, out.ID, nil, map[string]any{"status": status}); err != nil {
		return ResponseTask{}, err
	}
	results, err := s.resolveTaskNames(ctx, []ResponseTask{out})
	if err != nil {
		return ResponseTask{}, err
	}
	return results[0], nil
}

// AssignTask sets (or clears) a task's assignee — a user, a group, or neither.
func (s *Service) AssignTask(ctx context.Context, id uuid.UUID, rowVersion int32, userID, groupID *uuid.UUID) (ResponseTask, error) {
	if !has(ctx, PermExecute) {
		return ResponseTask{}, ErrForbidden
	}
	if userID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*userID})
		if err != nil {
			return ResponseTask{}, err
		}
		if _, ok := names[*userID]; !ok {
			return ResponseTask{}, &ValidationError{Fields: []FieldError{{"assignee_user_id", "not_found"}}}
		}
	}
	if groupID != nil {
		names, err := iamservice.GroupNames(ctx, []uuid.UUID{*groupID})
		if err != nil {
			return ResponseTask{}, err
		}
		if _, ok := names[*groupID]; !ok {
			return ResponseTask{}, &ValidationError{Fields: []FieldError{{"assignee_group_id", "not_found"}}}
		}
	}
	row, err := breachstore.New(pdb.MustTxFromContext(ctx)).AssignResponseTask(ctx, breachstore.AssignResponseTaskParams{
		ID: id, RowVersion: rowVersion, AssigneeUserID: pgUUID(userID), AssigneeGroupID: pgUUID(groupID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ResponseTask{}, ErrVersionMismatch
	}
	if err != nil {
		return ResponseTask{}, err
	}
	out := fromResponseTaskRow(row.ID, row.IncidentID, row.PlaybookID, row.StepCode, row.Title,
		row.AssigneeUserID, row.AssigneeGroupID, row.Status, row.DueAt, row.CompletedAt, row.RowVersion, row.CreatedAt, row.UpdatedAt)
	if err := s.audit(ctx, "breach.response_task.assign", ResponseTaskType, out.ID, nil,
		map[string]any{"assignee_user_id": userID, "assignee_group_id": groupID}); err != nil {
		return ResponseTask{}, err
	}
	results, err := s.resolveTaskNames(ctx, []ResponseTask{out})
	if err != nil {
		return ResponseTask{}, err
	}
	return results[0], nil
}

func (s *Service) resolveTaskNames(ctx context.Context, tasks []ResponseTask) ([]ResponseTask, error) {
	var ids []uuid.UUID
	for _, t := range tasks {
		if t.AssigneeUserID != nil {
			ids = append(ids, *t.AssigneeUserID)
		}
	}
	if len(ids) == 0 {
		return tasks, nil
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		if tasks[i].AssigneeUserID != nil {
			tasks[i].AssigneeName = names[*tasks[i].AssigneeUserID]
		}
	}
	return tasks, nil
}
