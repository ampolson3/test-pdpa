package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	pdb "pdpa-platform/internal/pkg/db"
	riskservice "pdpa-platform/internal/risk/service"
)

func countTasksForRisk(t *testing.T, ctx context.Context, riskID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM dpo.tasks WHERE source_type = 'risk' AND source_id = $1`, riskID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestEnsureRemediationTask_OpensOnceForSignificantRisk is DPIA-08's own acceptance criterion directly: a
// still-open risk whose level is high gets exactly one dpo.tasks remediation job, and a repeat call never
// opens a second one.
func TestEnsureRemediationTask_OpensOnceForSignificantRisk(t *testing.T) {
	e := scoreSetup(t, "dpia08open")
	var riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = e.manualRisk(t, ctx, "ความเสี่ยงสูง", 3, 3).ID // score 9 -> high
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		opened, err := e.svc.EnsureRemediationTask(ctx, riskID)
		if err != nil {
			return err
		}
		if !opened {
			t.Error("first call: opened = false, want true for a high-level open risk")
		}
		if n := countTasksForRisk(t, ctx, riskID); n != 1 {
			t.Errorf("tasks for risk after first call = %d, want 1", n)
		}
		opened, err = e.svc.EnsureRemediationTask(ctx, riskID)
		if err != nil {
			return err
		}
		if opened {
			t.Error("second call: opened = true, want false (already has a task)")
		}
		if n := countTasksForRisk(t, ctx, riskID); n != 1 {
			t.Errorf("tasks for risk after second call = %d, want still 1 (no duplicate)", n)
		}
		return nil
	})
}

// TestEnsureRemediationTask_LowLevelSkipped proves a risk whose level doesn't warrant one never gets a
// task.
func TestEnsureRemediationTask_LowLevelSkipped(t *testing.T) {
	e := scoreSetup(t, "dpia08low")
	var riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = e.manualRisk(t, ctx, "ความเสี่ยงต่ำ", 1, 1).ID // score 1 -> low
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		opened, err := e.svc.EnsureRemediationTask(ctx, riskID)
		if err != nil {
			return err
		}
		if opened {
			t.Error("opened = true for a low-level risk, want false")
		}
		if n := countTasksForRisk(t, ctx, riskID); n != 0 {
			t.Errorf("tasks for risk = %d, want 0", n)
		}
		return nil
	})
}

// TestEnsureRemediationTask_AcceptedRiskSkipped proves a risk the DPO already accepted (or closed) never
// gets a remediation task, even at a high level — accepting is itself the decision.
func TestEnsureRemediationTask_AcceptedRiskSkipped(t *testing.T) {
	e := scoreSetup(t, "dpia08accepted")
	var riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		r := e.manualRisk(t, ctx, "ความเสี่ยงที่ยอมรับ", 3, 3)
		riskID = r.ID
		if _, err := e.svc.UpdateRisk(ctx, riskID, riskservice.Risk{Title: r.Title, Likelihood: 3, Impact: 3, Treatment: "accept", Status: "accepted"}, r.RowVersion); err != nil {
			return err
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		opened, err := e.svc.EnsureRemediationTask(ctx, riskID)
		if err != nil {
			return err
		}
		if opened {
			t.Error("opened = true for an accepted risk, want false")
		}
		return nil
	})
}

// TestEnsureRemediationTask_ResidualLevelTakesPriority proves a risk whose inherent level is high but whose
// residual level (after mitigation) has dropped to low is no longer significant.
func TestEnsureRemediationTask_ResidualLevelTakesPriority(t *testing.T) {
	e := scoreSetup(t, "dpia08residual")
	var riskID, controlID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		controlID = e.controlID(t, ctx, "ORG_POLICY")
		return nil
	})
	// Build a risk that starts high (3x3=9) then mitigate it down with one implemented control.
	e.in(t, func(ctx context.Context) error {
		r := e.manualRisk(t, ctx, "ความเสี่ยงที่ลดแล้ว", 3, 3)
		riskID = r.ID
		if _, err := e.svc.AddRiskControl(ctx, riskID, controlID, nil, nil); err != nil {
			return err
		}
		if _, err := e.svc.UpdateRiskControlStatus(ctx, riskID, controlID, "implemented", 1); err != nil {
			return err
		}
		r2, err := e.svc.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if r2.ResidualLevel == nil || *r2.ResidualLevel != "medium" {
			t.Fatalf("setup: residual level = %v, want medium (one implemented control on a 3x3 risk)", r2.ResidualLevel)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		opened, err := e.svc.EnsureRemediationTask(ctx, riskID)
		if err != nil {
			return err
		}
		if opened {
			t.Error("opened = true once the residual level dropped below high, want false")
		}
		return nil
	})
}

// TestEnsureRemediationTask_TwoTenantIsolation proves tenant B can't read or affect tenant A's risk.
func TestEnsureRemediationTask_TwoTenantIsolation(t *testing.T) {
	a := scoreSetup(t, "dpia08isoA")
	b := scoreSetup(t, "dpia08isoB")
	var riskID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, a.svc)
		riskID = a.manualRisk(t, ctx, "x", 3, 3).ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, b.svc)
		if _, err := b.svc.EnsureRemediationTask(ctx, riskID); err == nil {
			t.Error("cross-tenant EnsureRemediationTask: err = nil, want a not-found error")
		}
		return nil
	})
}
