package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dpostore "pdpa-platform/internal/dpo/store"
	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// OpenGapRemediationTask opens one dpo.tasks job ("GAP-<year>-NNNN", source_type "ropa_gap" — already in
// the table's own CHECK constraint, unused until now) for RRA-07: implements risk/service's own DpoTasks
// interface structurally alongside DPIA-07's OpenRiskControlTask, this time with the caller's own
// assignee/due date/priority instead of a fixed "high" (the module doc's own "กำหนดผู้ดำเนินการ วันครบกำหนด
// และความสำคัญ").
func (s *Service) OpenGapRemediationTask(ctx context.Context, findingID uuid.UUID, title, description string,
	assigneeUserID *uuid.UUID, dueAt *time.Time, priority string) (uuid.UUID, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	year := strconv.Itoa(time.Now().UTC().Year())
	if err := q.LockTaskNumbering(ctx, year); err != nil {
		return uuid.Nil, err
	}
	prefix := "GAP-" + year + "-"
	n, err := q.CountTasksInYear(ctx, prefix)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	g, _ := authz.FromContext(ctx)
	actor, _ := uuid.Parse(g.UserID)
	var actorPtr *uuid.UUID
	if actor != uuid.Nil {
		actorPtr = &actor
	}
	row, err := q.InsertTask(ctx, dpostore.InsertTaskParams{ID: id, TaskNo: fmt.Sprintf("%s%04d", prefix, n+1),
		Title: title, Description: opt(description), SourceType: "ropa_gap", SourceID: pgUUID(&findingID),
		Priority: priority, AssigneeUserID: pgUUID(assigneeUserID), DueAt: pgDate(dueAt), Actor: pgUUID(actorPtr)})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}

// GetTask returns one dpo.tasks row by id, RLS-scoped to the caller's tenant.
func (s *Service) GetTask(ctx context.Context, id uuid.UUID) (Task, error) {
	row, err := dpostore.New(pdb.MustTxFromContext(ctx)).GetTask(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	return toGetTask(row), nil
}

var taskTransitions = map[string]map[string]bool{
	"created":   {"assigned": true, "in_review": true},
	"assigned":  {"in_review": true},
	"in_review": {"done": true},
	"done":      {"closed": true},
	"closed":    {},
}

// UpdateTaskStatus moves a dpo.tasks job forward through created -> assigned -> in_review -> done ->
// closed (checked against this small, local state machine — the first time any caller drives dpo.tasks'
// own status column through an API, so this is also the first place that column needs one). Closing a
// "ropa_gap" task (RRA-07's own acceptance criterion — "ปิดงานแล้วตรวจ rule ซ้ำ") re-runs that finding's
// activity through the risk module and lets the finding clear itself automatically if the rule no longer
// applies; the task closes either way — remediation tracking isn't held hostage by whether the fix
// actually worked.
func (s *Service) UpdateTaskStatus(ctx context.Context, id uuid.UUID, status string, version int32) (Task, error) {
	current, err := s.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if err := requireTaskAccess(ctx, current); err != nil {
		return Task{}, err
	}
	if !taskTransitions[current.Status][status] {
		return Task{}, fmt.Errorf("%w: cannot move task from %q to %q", ErrInvalid, current.Status, status)
	}
	g, _ := authz.FromContext(ctx)
	actor, _ := uuid.Parse(g.UserID)
	var actorPtr *uuid.UUID
	if actor != uuid.Nil {
		actorPtr = &actor
	}
	row, err := dpostore.New(pdb.MustTxFromContext(ctx)).UpdateTaskStatus(ctx, dpostore.UpdateTaskStatusParams{
		ID: id, Status: status, RowVersion: version, Actor: pgUUID(actorPtr),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrVersionMismatch
	}
	if err != nil {
		return Task{}, err
	}
	if status == "closed" && current.SourceType == "ropa_gap" && current.SourceID != nil && s.Risk != nil {
		if finding, err := s.Risk.GetGapFinding(ctx, *current.SourceID); err == nil {
			if _, err := s.Risk.AnalyzeActivity(ctx, finding.ActivityID); err != nil {
				return Task{}, err
			}
		}
	}
	if err := s.audit(ctx, "dpo.task.status", id, map[string]any{"status": current.Status},
		map[string]any{"status": status}); err != nil {
		return Task{}, err
	}
	return toUpdatedTask(row), nil
}

// requireTaskAccess is docs/security/permissions.md's own note on dpo.task ("แก้ได้เฉพาะงานที่ได้รับ
// มอบหมาย" — edit only tasks assigned to you): a caller holding dpo.task.execute (DPO/PRIVACY in the
// seeded RBAC) may update any task; everyone else holding only dpo.task.update (LEGAL/OWNER/IT/SEC) may
// update only a task actually assigned to them — the exact two-tier shape DSAR-08's own
// requireSubtaskAccess already established, minus the group-assignee case dpo.tasks has no column for.
func requireTaskAccess(ctx context.Context, t Task) error {
	g, _ := authz.FromContext(ctx)
	if g.Has("dpo.task.execute") {
		return nil
	}
	callerID, err := uuid.Parse(g.UserID)
	if err != nil {
		return ErrForbidden
	}
	if t.AssigneeUserID != nil && *t.AssigneeUserID == callerID {
		return nil
	}
	return ErrForbidden
}

func toGetTask(r dpostore.GetTaskRow) Task {
	return taskRow{ID: r.ID, TaskNo: r.TaskNo, Title: r.Title, Description: r.Description, SourceType: r.SourceType,
		SourceID: r.SourceID, Status: r.Status, Priority: r.Priority, AssigneeUserID: r.AssigneeUserID,
		ReviewerUserID: r.ReviewerUserID, OrgUnitID: r.OrgUnitID, DueAt: r.DueAt, CompletedAt: r.CompletedAt,
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt}.toTask()
}

func toUpdatedTask(r dpostore.UpdateTaskStatusRow) Task {
	return taskRow{ID: r.ID, TaskNo: r.TaskNo, Title: r.Title, Description: r.Description, SourceType: r.SourceType,
		SourceID: r.SourceID, Status: r.Status, Priority: r.Priority, AssigneeUserID: r.AssigneeUserID,
		ReviewerUserID: r.ReviewerUserID, OrgUnitID: r.OrgUnitID, DueAt: r.DueAt, CompletedAt: r.CompletedAt,
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt}.toTask()
}

// taskRow mirrors every dpo.tasks query's own identically-laid-out, separately-generated Row type (the
// established sqlc workaround this codebase uses throughout — e.g. BRE-15/VEN-03/VEN-14/DPX-01/DPX-03's own
// auditRow/courseRow/etc.), letting ListTasks/AssignTask's own generated Row types convert through it too.
type taskRow struct {
	ID             uuid.UUID
	TaskNo         string
	Title          string
	Description    *string
	SourceType     string
	SourceID       pgtype.UUID
	Status         string
	Priority       string
	AssigneeUserID pgtype.UUID
	ReviewerUserID pgtype.UUID
	OrgUnitID      pgtype.UUID
	DueAt          pgtype.Date
	CompletedAt    pgtype.Timestamptz
	RowVersion     int32
	CreatedAt      pgtype.Timestamptz
}

func (r taskRow) toTask() Task {
	t := Task{ID: r.ID, TaskNo: r.TaskNo, Title: r.Title, Description: deref(r.Description), SourceType: r.SourceType,
		SourceID: uuidPtr(r.SourceID), Status: r.Status, Priority: r.Priority, AssigneeUserID: uuidPtr(r.AssigneeUserID),
		ReviewerUserID: uuidPtr(r.ReviewerUserID), OrgUnitID: uuidPtr(r.OrgUnitID),
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.DueAt.Valid {
		t.DueAt = &r.DueAt.Time
	}
	if r.CompletedAt.Valid {
		t.CompletedAt = &r.CompletedAt.Time
	}
	return t
}

// TaskCursor is the kanban/list view's own (created_at, id) keyset cursor (DPO-06), the exact RRA-09
// RiskCursor shape ported to dpo.tasks.
type TaskCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// TaskFilter narrows ListTasks — every field optional, "" / nil meaning no filter.
type TaskFilter struct {
	Status         string
	SourceType     string
	Priority       string
	AssigneeUserID *uuid.UUID
	After          *TaskCursor
	Limit          int
}

const taskListPageSize = 50

// ListTasks is DPO-06's own kanban/list view: every dpo.tasks job regardless of which feature opened it
// (RRA-07/DPO-09/DPA-12/PNG-07/ROPA-19/DPIA-08/RRA-08/RTG-10), newest first, optionally narrowed by status
// (for a kanban column), source_type, priority or assignee.
func (s *Service) ListTasks(ctx context.Context, f TaskFilter) ([]Task, *TaskCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > taskListPageSize {
		limit = taskListPageSize
	}
	p := dpostore.ListTasksParams{Lim: int32(limit + 1)}
	if f.Status != "" {
		p.Status = &f.Status
	}
	if f.SourceType != "" {
		p.SourceType = &f.SourceType
	}
	if f.Priority != "" {
		p.Priority = &f.Priority
	}
	p.AssigneeUserID = pgUUID(f.AssigneeUserID)
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.CreatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := dpostore.New(pdb.MustTxFromContext(ctx)).ListTasks(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Task, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &TaskCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
		}
		out = append(out, taskRow{ID: r.ID, TaskNo: r.TaskNo, Title: r.Title, Description: r.Description, SourceType: r.SourceType,
			SourceID: r.SourceID, Status: r.Status, Priority: r.Priority, AssigneeUserID: r.AssigneeUserID,
			ReviewerUserID: r.ReviewerUserID, OrgUnitID: r.OrgUnitID, DueAt: r.DueAt, CompletedAt: r.CompletedAt,
			RowVersion: r.RowVersion, CreatedAt: r.CreatedAt}.toTask())
	}
	return out, nil, nil
}

// AssignTaskInput is DPO-06's own "มีผู้ตรวจ วันครบกำหนด และความสำคัญ": every field is a full replace (nil
// clears it), matching every other admin form's full-rebuild PATCH semantics in this codebase.
type AssignTaskInput struct {
	AssigneeUserID *uuid.UUID
	ReviewerUserID *uuid.UUID
	OrgUnitID      *uuid.UUID
	DueAt          *time.Time
	Priority       string
}

// AssignTask is DPO-06's own real, narrow gap against every other Open*Task caller in this codebase: none of
// them ever lets anyone change an assignee/reviewer/due date/priority once a task is auto-created. Gated by
// dpo.task.execute ("ดำเนินการ / มอบหมาย / ทำรายการ" — already seeded, no new permission code), the same
// permission requireTaskAccess already treats as the bypass tier, since assigning a task is itself the
// "มอบหมาย" action that permission already names. Every id given (assignee/reviewer/org unit) is checked
// visible under RLS first (rule 1) before the write.
func (s *Service) AssignTask(ctx context.Context, id uuid.UUID, in AssignTaskInput, version int32) (Task, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("dpo.task.execute") {
		return Task{}, ErrForbidden
	}
	if in.Priority == "" {
		in.Priority = "medium"
	}
	if !priorities[in.Priority] {
		return Task{}, fmt.Errorf("%w: priority", ErrInvalid)
	}
	ids := make([]uuid.UUID, 0, 2)
	if in.AssigneeUserID != nil {
		ids = append(ids, *in.AssigneeUserID)
	}
	if in.ReviewerUserID != nil {
		ids = append(ids, *in.ReviewerUserID)
	}
	if len(ids) > 0 {
		names, err := iamservice.Names(ctx, ids)
		if err != nil {
			return Task{}, err
		}
		if in.AssigneeUserID != nil {
			if _, ok := names[*in.AssigneeUserID]; !ok {
				return Task{}, fmt.Errorf("%w: assignee_user_id", ErrInvalid)
			}
		}
		if in.ReviewerUserID != nil {
			if _, ok := names[*in.ReviewerUserID]; !ok {
				return Task{}, fmt.Errorf("%w: reviewer_user_id", ErrInvalid)
			}
		}
	}
	if in.OrgUnitID != nil {
		if _, err := s.Org.GetOrgUnit(ctx, *in.OrgUnitID); err != nil {
			return Task{}, fmt.Errorf("%w: org_unit_id", ErrInvalid)
		}
	}
	current, err := s.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	g2, _ := authz.FromContext(ctx)
	actor, _ := uuid.Parse(g2.UserID)
	var actorPtr *uuid.UUID
	if actor != uuid.Nil {
		actorPtr = &actor
	}
	row, err := dpostore.New(pdb.MustTxFromContext(ctx)).AssignTask(ctx, dpostore.AssignTaskParams{
		ID: id, AssigneeUserID: pgUUID(in.AssigneeUserID), ReviewerUserID: pgUUID(in.ReviewerUserID),
		OrgUnitID: pgUUID(in.OrgUnitID), DueAt: pgDate(in.DueAt), Priority: in.Priority, RowVersion: version, Actor: pgUUID(actorPtr),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrVersionMismatch
	}
	if err != nil {
		return Task{}, err
	}
	out := taskRow{ID: row.ID, TaskNo: row.TaskNo, Title: row.Title, Description: row.Description, SourceType: row.SourceType,
		SourceID: row.SourceID, Status: row.Status, Priority: row.Priority, AssigneeUserID: row.AssigneeUserID,
		ReviewerUserID: row.ReviewerUserID, OrgUnitID: row.OrgUnitID, DueAt: row.DueAt, CompletedAt: row.CompletedAt,
		RowVersion: row.RowVersion, CreatedAt: row.CreatedAt}.toTask()
	if err := s.audit(ctx, "dpo.task.assign", id, map[string]any{"assignee_user_id": current.AssigneeUserID, "priority": current.Priority},
		map[string]any{"assignee_user_id": out.AssigneeUserID, "reviewer_user_id": out.ReviewerUserID, "priority": out.Priority}); err != nil {
		return Task{}, err
	}
	return out, nil
}

var priorities = map[string]bool{"low": true, "medium": true, "high": true, "urgent": true}
