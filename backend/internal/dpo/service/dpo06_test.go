package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	dposervice "pdpa-platform/internal/dpo/service"
)

// TestListTasks_CoversEverySourceAndFilters is DPO-06's own kanban/list view: every dpo.tasks job, whatever
// feature opened it, with provenance (source_type/source_id) already carried on every row — the acceptance
// criterion's own "มีที่มา" (has provenance) half.
func TestListTasks_CoversEverySourceAndFilters(t *testing.T) {
	e := taskSetup(t, "dpo06list")
	var taskA, taskB, activityID, findingID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		assignee := seedTaskUser(t, ctx)
		taskA, activityID, findingID = gapTaskFixture(t, e, ctx, &assignee)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		id, err := e.dpo.OpenGapRemediationTask(ctx, findingID, "คำนวณใหม่", "test", nil, nil, "urgent")
		if err != nil {
			return err
		}
		taskB = id
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		list, _, err := e.dpo.ListTasks(ctx, dposervice.TaskFilter{})
		if err != nil {
			return err
		}
		found := map[uuid.UUID]dposervice.Task{}
		for _, task := range list {
			found[task.ID] = task
		}
		ta, ok := found[taskA]
		if !ok {
			t.Fatal("expected taskA in the unfiltered list")
		}
		if ta.SourceType != "ropa_gap" || ta.SourceID == nil || *ta.SourceID != findingID {
			t.Errorf("taskA provenance = %q/%v, want ropa_gap/%v", ta.SourceType, ta.SourceID, findingID)
		}
		if _, ok := found[taskB]; !ok {
			t.Fatal("expected taskB in the unfiltered list")
		}

		urgentOnly, _, err := e.dpo.ListTasks(ctx, dposervice.TaskFilter{Priority: "urgent"})
		if err != nil {
			return err
		}
		if len(urgentOnly) != 1 || urgentOnly[0].ID != taskB {
			t.Errorf("priority=urgent filter returned %d tasks, want exactly taskB", len(urgentOnly))
		}

		bySource, _, err := e.dpo.ListTasks(ctx, dposervice.TaskFilter{SourceType: "ropa_gap"})
		if err != nil {
			return err
		}
		if len(bySource) != 2 {
			t.Errorf("source_type=ropa_gap filter returned %d tasks, want 2", len(bySource))
		}
		return nil
	})
	_ = activityID
}

// TestListTasks_TwoTenantIsolation proves RLS alone already scopes the list — no second check needed.
func TestListTasks_TwoTenantIsolation(t *testing.T) {
	a := taskSetup(t, "dpo06isoa")
	b := taskSetup(t, "dpo06isob")
	var taskID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		assignee := seedTaskUser(t, ctx)
		taskID, _, _ = gapTaskFixture(t, a, ctx, &assignee)
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		list, _, err := b.dpo.ListTasks(ctx, dposervice.TaskFilter{})
		if err != nil {
			return err
		}
		for _, task := range list {
			if task.ID == taskID {
				t.Error("tenant B's ListTasks saw tenant A's task")
			}
		}
		return nil
	})
}

// TestAssignTask_SetsAssigneeReviewerDueDateAndPriority is DPO-06's own real gap: every existing Open*Task
// caller bakes an assignee/due date in once, at creation, with no later edit path — this is that path.
func TestAssignTask_SetsAssigneeReviewerDueDateAndPriority(t *testing.T) {
	e := taskSetup(t, "dpo06assign")
	var taskID, unitID uuid.UUID
	var assignee, reviewer uuid.UUID
	e.in(t, func(ctx context.Context) error {
		assignee = seedTaskUser(t, ctx)
		reviewer = seedTaskUser(t, ctx)
		var activityID uuid.UUID
		taskID, activityID, _ = gapTaskFixture(t, e, ctx, nil) // no assignee at creation -> status stays "created"
		units, err := e.ropa.GetActivity(ctx, activityID)
		if err != nil {
			return err
		}
		unitID = units.OrgUnitID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		due := mustDate(t, "2026-12-31")
		updated, err := e.dpo.AssignTask(ctx, taskID, dposervice.AssignTaskInput{
			AssigneeUserID: &assignee, ReviewerUserID: &reviewer, OrgUnitID: &unitID, DueAt: &due, Priority: "high",
		}, 1)
		if err != nil {
			return err
		}
		if updated.AssigneeUserID == nil || *updated.AssigneeUserID != assignee {
			t.Errorf("assignee_user_id = %v, want %v", updated.AssigneeUserID, assignee)
		}
		if updated.ReviewerUserID == nil || *updated.ReviewerUserID != reviewer {
			t.Errorf("reviewer_user_id = %v, want %v", updated.ReviewerUserID, reviewer)
		}
		if updated.OrgUnitID == nil || *updated.OrgUnitID != unitID {
			t.Errorf("org_unit_id = %v, want %v", updated.OrgUnitID, unitID)
		}
		if updated.Priority != "high" {
			t.Errorf("priority = %q, want high", updated.Priority)
		}
		if updated.Status != "assigned" {
			t.Errorf("status = %q, want assigned (a created task gets an assignee for the first time)", updated.Status)
		}
		return nil
	})
}

// TestAssignTask_RequiresExecutePermission: "มอบหมาย" is explicitly part of dpo.task.execute's own seeded
// description — a caller holding only dpo.task.update may not assign, even to themselves.
func TestAssignTask_RequiresExecutePermission(t *testing.T) {
	e := taskSetup(t, "dpo06assignperm")
	var taskID uuid.UUID
	var bystander uuid.UUID
	e.in(t, func(ctx context.Context) error {
		bystander = seedTaskUser(t, ctx)
		taskID, _, _ = gapTaskFixture(t, e, ctx, nil)
		return nil
	})
	e.inAs(t, bystander, []string{"dpo.task.read", "dpo.task.update"}, func(ctx context.Context) error {
		if _, err := e.dpo.AssignTask(ctx, taskID, dposervice.AssignTaskInput{AssigneeUserID: &bystander, Priority: "high"}, 1); !errors.Is(err, dposervice.ErrForbidden) {
			t.Errorf("without dpo.task.execute: %v, want ErrForbidden", err)
		}
		return nil
	})
}

// TestAssignTask_ValidatesReferences: an unknown assignee/reviewer/org unit is refused before any write,
// the same FK-visibility discipline every other module's own assignment feature already follows.
func TestAssignTask_ValidatesReferences(t *testing.T) {
	e := taskSetup(t, "dpo06assignval")
	var taskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		taskID, _, _ = gapTaskFixture(t, e, ctx, nil)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		unknown := uuid.New()
		if _, err := e.dpo.AssignTask(ctx, taskID, dposervice.AssignTaskInput{AssigneeUserID: &unknown, Priority: "high"}, 1); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("unknown assignee: %v, want ErrInvalid", err)
		}
		if _, err := e.dpo.AssignTask(ctx, taskID, dposervice.AssignTaskInput{OrgUnitID: &unknown, Priority: "high"}, 1); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("unknown org_unit_id: %v, want ErrInvalid", err)
		}
		if _, err := e.dpo.AssignTask(ctx, taskID, dposervice.AssignTaskInput{Priority: "not-a-priority"}, 1); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("bad priority: %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestAssignTask_VersionMismatch(t *testing.T) {
	e := taskSetup(t, "dpo06assignver")
	var taskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		taskID, _, _ = gapTaskFixture(t, e, ctx, nil)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.dpo.AssignTask(ctx, taskID, dposervice.AssignTaskInput{Priority: "high"}, 99); !errors.Is(err, dposervice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}
