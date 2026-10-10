package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dpostore "pdpa-platform/internal/dpo/store"
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
	t := Task{ID: r.ID, TaskNo: r.TaskNo, Title: r.Title, Description: deref(r.Description), SourceType: r.SourceType,
		SourceID: uuidPtr(r.SourceID), Status: r.Status, Priority: r.Priority, AssigneeUserID: uuidPtr(r.AssigneeUserID),
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.DueAt.Valid {
		t.DueAt = &r.DueAt.Time
	}
	if r.CompletedAt.Valid {
		t.CompletedAt = &r.CompletedAt.Time
	}
	return t
}

func toUpdatedTask(r dpostore.UpdateTaskStatusRow) Task {
	t := Task{ID: r.ID, TaskNo: r.TaskNo, Title: r.Title, Description: deref(r.Description), SourceType: r.SourceType,
		SourceID: uuidPtr(r.SourceID), Status: r.Status, Priority: r.Priority, AssigneeUserID: uuidPtr(r.AssigneeUserID),
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.DueAt.Valid {
		t.DueAt = &r.DueAt.Time
	}
	if r.CompletedAt.Valid {
		t.CompletedAt = &r.CompletedAt.Time
	}
	return t
}
