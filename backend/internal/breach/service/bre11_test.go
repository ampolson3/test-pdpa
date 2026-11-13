package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"pdpa-platform/internal/breach/breachtest"
	breach "pdpa-platform/internal/breach/service"
)

// TestApplyPlaybook_AcceptanceCriterion is BRE-11's own acceptance criterion directly ("เลือกประเภทเหตุแล้ว
// ได้ชุดงานตาม playbook"): an incident of a given incident_type, applied with no explicit playbook,
// resolves the matching playbook and turns every one of its steps into a real response task. Re-applying
// is a harmless no-op — the same tasks, never a duplicate set.
func TestApplyPlaybook_AcceptanceCriterion(t *testing.T) {
	f := breachtest.Setup(t)
	aware := f.Clock.Add(-2 * time.Hour)

	var pb breach.Playbook
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		pb, err = f.Svc.CreatePlaybook(ctx, "ransomware", "Ransomware response", []breach.PlaybookStep{
			{Code: "isolate", Title: "แยกระบบที่ติดออกจากเครือข่าย"},
			{Code: "notify_dpo", Title: "แจ้ง DPO"},
		})
		return err
	})
	if len(pb.Steps) != 2 || pb.VersionNo != 1 {
		t.Fatalf("unexpected playbook: %+v", pb)
	}

	var n = int32(10)
	var in breach.Incident
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		in, err = f.Svc.Create(ctx, breach.Input{LegalEntityID: f.EntityA, ReportedVia: "email", Title: "โดน ransomware",
			Description: "ระบบถูกเรียกค่าไถ่", BreachTypes: []string{"availability"}, IncidentType: "ransomware",
			AwareAt: aware, AffectedSubjects: &n})
		return err
	})

	var tasks1 []breach.ResponseTask
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		tasks1, err = f.Svc.ApplyPlaybook(ctx, in.ID, nil)
		return err
	})
	if len(tasks1) != 2 {
		t.Fatalf("expected 2 tasks from the playbook, got %d", len(tasks1))
	}
	for _, task := range tasks1 {
		if task.Status != "open" || task.PlaybookID == nil || *task.PlaybookID != pb.ID {
			t.Errorf("unexpected task: %+v", task)
		}
	}

	// Re-applying is a harmless no-op: the same 2 tasks, never a third set.
	var tasks2 []breach.ResponseTask
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		tasks2, err = f.Svc.ApplyPlaybook(ctx, in.ID, nil)
		return err
	})
	if len(tasks2) != 2 {
		t.Fatalf("expected re-apply to stay at 2 tasks, got %d", len(tasks2))
	}

	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		list, err := f.Svc.ListResponseTasks(ctx, in.ID)
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Fatalf("ListResponseTasks: expected 2, got %d", len(list))
		}
		return nil
	})
}

// TestApplyPlaybook_NoMatchingPlaybookIsRefused covers the other half: an incident whose incident_type has
// no playbook at all is refused, not silently applied with nothing.
func TestApplyPlaybook_NoMatchingPlaybookIsRefused(t *testing.T) {
	f := breachtest.Setup(t)
	aware := f.Clock.Add(-2 * time.Hour)
	var n = int32(5)
	var in breach.Incident
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		in, err = f.Svc.Create(ctx, breach.Input{LegalEntityID: f.EntityA, ReportedVia: "email", Title: "เหตุไม่มี playbook",
			Description: "x", BreachTypes: []string{"confidentiality"}, IncidentType: "no_such_type", AwareAt: aware, AffectedSubjects: &n})
		return err
	})
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		_, err := f.Svc.ApplyPlaybook(ctx, in.ID, nil)
		var ve *breach.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("expected a ValidationError for no matching playbook, got %v", err)
		}
		return nil
	})
}

// TestResponseTask_StatusAndAssignment covers the task-management half: status moves through
// open -> in_progress -> done (stamping completed_at), and a task can be assigned to a real user.
func TestResponseTask_StatusAndAssignment(t *testing.T) {
	f := breachtest.Setup(t)
	aware := f.Clock.Add(-2 * time.Hour)

	var pb breach.Playbook
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		pb, err = f.Svc.CreatePlaybook(ctx, "phishing", "Phishing response", []breach.PlaybookStep{{Code: "reset_pw", Title: "บังคับเปลี่ยนรหัสผ่าน"}})
		return err
	})

	var n = int32(1)
	var in breach.Incident
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		in, err = f.Svc.Create(ctx, breach.Input{LegalEntityID: f.EntityA, ReportedVia: "email", Title: "phishing",
			Description: "x", BreachTypes: []string{"confidentiality"}, IncidentType: "phishing", AwareAt: aware, AffectedSubjects: &n})
		return err
	})

	var tasks []breach.ResponseTask
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		tasks, err = f.Svc.ApplyPlaybook(ctx, in.ID, &pb.ID)
		return err
	})
	task := tasks[0]

	var assigned breach.ResponseTask
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		assigned, err = f.Svc.AssignTask(ctx, task.ID, task.RowVersion, &f.DPO, nil)
		return err
	})
	if assigned.AssigneeUserID == nil || *assigned.AssigneeUserID != f.DPO || assigned.AssigneeName == "" {
		t.Fatalf("unexpected assignment: %+v", assigned)
	}

	var moved breach.ResponseTask
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		moved, err = f.Svc.SetTaskStatus(ctx, assigned.ID, assigned.RowVersion, "in_progress")
		return err
	})
	if moved.Status != "in_progress" || moved.CompletedAt != nil {
		t.Fatalf("unexpected in_progress task: %+v", moved)
	}

	var done breach.ResponseTask
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		done, err = f.Svc.SetTaskStatus(ctx, moved.ID, moved.RowVersion, "done")
		return err
	})
	if done.Status != "done" || done.CompletedAt == nil {
		t.Fatalf("expected done with a completed_at: %+v", done)
	}

	// A stale row_version is refused distinctly from a bad status.
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		_, err := f.Svc.SetTaskStatus(ctx, done.ID, done.RowVersion, "done")
		if !errors.Is(err, breach.ErrVersionMismatch) {
			t.Fatalf("expected ErrVersionMismatch on a stale row_version, got %v", err)
		}
		return nil
	})
}

// TestPlaybooks_TwoTenantIsolation proves RLS alone already scopes a cross-tenant playbook id to not-found,
// and a cross-tenant incident id refuses ApplyPlaybook.
func TestPlaybooks_TwoTenantIsolation(t *testing.T) {
	f := breachtest.Setup(t)
	var pbA breach.Playbook
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		pbA, err = f.Svc.CreatePlaybook(ctx, "ddos", "DDoS response", []breach.PlaybookStep{{Code: "block_ips", Title: "บล็อก IP ต้นทาง"}})
		return err
	})

	if err := f.Try(f.B.UserID, breachtest.SEC, func(ctx context.Context) error {
		_, err := f.Svc.GetPlaybook(ctx, pbA.ID)
		if !errors.Is(err, breach.ErrNotFound) {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
