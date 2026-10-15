package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/breach/breachtest"
	breach "pdpa-platform/internal/breach/service"
)

// Acceptance criterion (BRE-15): a corrective action is tracked, one at a time, until it's done.
func TestRootCause_CorrectiveActionsTrackedToCompletion(t *testing.T) {
	f := breachtest.Setup(t)
	in := f.Incident(t, "เหตุทดสอบวิเคราะห์สาเหตุ", f.Clock)

	var rc breach.RootCause
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		rc, err = f.Svc.AddRootCause(ctx, in.ID, "process_gap", "ไม่มีการตรวจสอบสิทธิ์ก่อนส่งออกข้อมูล",
			[]breach.CorrectiveAction{
				{Description: "เพิ่มการอนุมัติสองชั้นก่อนส่งออก"},
				{Description: "อบรมพนักงานเรื่องการจัดการข้อมูล"},
			}, nil)
		return err
	})
	if len(rc.CorrectiveActions) != 2 {
		t.Fatalf("expected 2 corrective actions, got %+v", rc.CorrectiveActions)
	}
	if rc.AllActionsDone() {
		t.Error("should not be done yet — two open actions")
	}
	for _, a := range rc.CorrectiveActions {
		if a.ID == "" {
			t.Error("every corrective action should have an assigned id")
		}
		if a.Status != "open" {
			t.Errorf("new actions should default to open, got %q", a.Status)
		}
	}

	first := rc.CorrectiveActions[0].ID
	second := rc.CorrectiveActions[1].ID

	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		rc, err = f.Svc.CompleteCorrectiveAction(ctx, rc.ID, first, rc.RowVersion)
		return err
	})
	if rc.AllActionsDone() {
		t.Error("should not be done yet — one action still open")
	}
	for _, a := range rc.CorrectiveActions {
		if a.ID == first && (a.Status != "done" || a.CompletedAt == nil) {
			t.Errorf("first action should be done with a completed_at: %+v", a)
		}
	}

	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		rc, err = f.Svc.CompleteCorrectiveAction(ctx, rc.ID, second, rc.RowVersion)
		return err
	})
	if !rc.AllActionsDone() {
		t.Errorf("expected all actions done, got %+v", rc.CorrectiveActions)
	}

	// Completing an already-done action again is a harmless no-op, not an error.
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		rc, err = f.Svc.CompleteCorrectiveAction(ctx, rc.ID, second, rc.RowVersion)
		return err
	})
	if !rc.AllActionsDone() {
		t.Error("redelivery of an already-done action should stay done")
	}
}

func TestRootCause_ValidationAndRiskVisibility(t *testing.T) {
	f := breachtest.Setup(t)
	in := f.Incident(t, "เหตุทดสอบ validation", f.Clock)

	if err := f.Try(f.Sec, breachtest.SEC, func(ctx context.Context) error {
		_, err := f.Svc.AddRootCause(ctx, in.ID, "not_a_category", "x", nil, nil)
		return err
	}); !errors.Is(err, breach.ErrInvalidRequest) {
		t.Errorf("bad cause_category: %v, want ErrInvalidRequest", err)
	}

	if err := f.Try(f.Sec, breachtest.SEC, func(ctx context.Context) error {
		_, err := f.Svc.AddRootCause(ctx, in.ID, "human_error", "", nil, nil)
		return err
	}); !errors.Is(err, breach.ErrInvalidRequest) {
		t.Errorf("empty description: %v, want ErrInvalidRequest", err)
	}

	unknownRisk := uuid.Must(uuid.NewV7())
	if err := f.Try(f.Sec, breachtest.SEC, func(ctx context.Context) error {
		_, err := f.Svc.AddRootCause(ctx, in.ID, "human_error", "x", nil, &unknownRisk)
		return err
	}); !errors.Is(err, breach.ErrInvalidRequest) {
		t.Errorf("unknown risk_id: %v, want ErrInvalidRequest", err)
	}

	// Without the incident-update permission, the write is refused.
	if err := f.Try(f.Employee, breachtest.Employee, func(ctx context.Context) error {
		_, err := f.Svc.AddRootCause(ctx, in.ID, "human_error", "x", nil, nil)
		return err
	}); !errors.Is(err, breach.ErrForbidden) {
		t.Errorf("no update permission: %v, want ErrForbidden", err)
	}
}

func TestRootCause_ListUpdateDelete(t *testing.T) {
	f := breachtest.Setup(t)
	in := f.Incident(t, "เหตุทดสอบ list/update", f.Clock)

	var rc breach.RootCause
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		rc, err = f.Svc.AddRootCause(ctx, in.ID, "technical_control", "ไม่มีการเข้ารหัส",
			[]breach.CorrectiveAction{{Description: "เข้ารหัสฐานข้อมูล"}}, nil)
		return err
	})

	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		list, err := f.Svc.ListRootCauses(ctx, in.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != rc.ID {
			t.Errorf("expected exactly the one root cause, got %+v", list)
		}
		return nil
	})

	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		updated, err := f.Svc.UpdateRootCause(ctx, rc.ID, rc.RowVersion, "third_party", "ผู้ให้บริการภายนอกรั่วไหล", rc.CorrectiveActions, nil)
		if err != nil {
			return err
		}
		if updated.CauseCategory != "third_party" {
			t.Errorf("update did not take effect: %+v", updated)
		}
		rc = updated
		return nil
	})

	// A stale row version is refused.
	if err := f.Try(f.Sec, breachtest.SEC, func(ctx context.Context) error {
		_, err := f.Svc.UpdateRootCause(ctx, rc.ID, rc.RowVersion-1, "third_party", "x", nil, nil)
		return err
	}); !errors.Is(err, breach.ErrVersionMismatch) {
		t.Errorf("stale version: %v, want ErrVersionMismatch", err)
	}

	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		return f.Svc.DeleteRootCause(ctx, in.ID, rc.ID)
	})
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		list, err := f.Svc.ListRootCauses(ctx, in.ID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("expected no root causes after delete, got %+v", list)
		}
		return nil
	})
}

// Two-tenant isolation: a root cause recorded in tenant A is invisible under tenant B's own RLS.
func TestRootCause_TwoTenantIsolation(t *testing.T) {
	f := breachtest.Setup(t)
	in := f.Incident(t, "เหตุทดสอบ isolation", f.Clock)
	var rc breach.RootCause
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		rc, err = f.Svc.AddRootCause(ctx, in.ID, "human_error", "x", nil, nil)
		return err
	})

	if err := f.Try(f.B.UserID, breachtest.SEC, func(ctx context.Context) error {
		_, err := f.Svc.GetRootCause(ctx, rc.ID)
		if !errors.Is(err, breach.ErrNotFound) {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
