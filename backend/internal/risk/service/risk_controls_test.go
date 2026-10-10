package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	pdb "pdpa-platform/internal/pkg/db"
	riskservice "pdpa-platform/internal/risk/service"
)

// manualRisk identifies a plain "manual" risk (no DPIA round involved) for DPIA-07's own tests, which live
// at the risk/service layer directly — the dpia-side editable-window/link checks are exercised separately
// in internal/dpia/service/risk_controls_test.go.
func (e scoreEnv) manualRisk(t *testing.T, ctx context.Context, title string, likelihood, impact int) riskservice.Risk {
	t.Helper()
	r, err := e.svc.IdentifyRisk(ctx, riskservice.Risk{SourceType: "manual", Title: title, Likelihood: likelihood, Impact: impact})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// firstControlID resolves a real risk.controls row's id by its seeded code (ROPA-09, migration 00040).
func (e scoreEnv) controlID(t *testing.T, ctx context.Context, code string) uuid.UUID {
	t.Helper()
	controls, err := e.svc.ListControls(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range controls {
		if c.Code == code {
			return c.ID
		}
	}
	t.Fatalf("seeded control %q not found", code)
	return uuid.Nil
}

// TestAddRiskControl_RecomputesResidual is DPIA-07's own acceptance criterion directly: residual likelihood
// drops by one per implemented control (floored at 1) and is reclassified against the tenant's matrix —
// linking a control that's merely "planned" (not yet implemented) must not move it.
func TestAddRiskControl_RecomputesResidual(t *testing.T) {
	e := scoreSetup(t, "rra07resid")
	var riskID, c1, c2 uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = e.manualRisk(t, ctx, "ความเสี่ยงทดสอบ", 3, 3).ID
		c1 = e.controlID(t, ctx, "ORG_POLICY")
		c2 = e.controlID(t, ctx, "ORG_TRAINING")
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		rc, err := e.svc.AddRiskControl(ctx, riskID, c1, nil, nil)
		if err != nil {
			return err
		}
		if rc.Status != "planned" {
			t.Errorf("default status = %q, want planned", rc.Status)
		}
		r, err := e.svc.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if r.ResidualLikelihood == nil || *r.ResidualLikelihood != 3 {
			t.Errorf("planned-only control changed residual likelihood: %+v, want unchanged at 3", r.ResidualLikelihood)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.UpdateRiskControlStatus(ctx, riskID, c1, "implemented", 1); err != nil {
			return err
		}
		r, err := e.svc.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if r.ResidualLikelihood == nil || *r.ResidualLikelihood != 2 {
			t.Fatalf("after one implemented control: residual likelihood = %v, want 2", r.ResidualLikelihood)
		}
		if r.ResidualImpact == nil || *r.ResidualImpact != 3 {
			t.Errorf("residual impact should stay at the inherent impact: %v, want 3", r.ResidualImpact)
		}
		if r.ResidualScore == nil || *r.ResidualScore != 6 {
			t.Errorf("residual score = %v, want 2x3=6", r.ResidualScore)
		}
		if r.ResidualLevel == nil || *r.ResidualLevel != "medium" {
			t.Errorf("residual level = %v, want medium", r.ResidualLevel)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddRiskControl(ctx, riskID, c2, nil, nil); err != nil {
			return err
		}
		if _, err := e.svc.UpdateRiskControlStatus(ctx, riskID, c2, "implemented", 1); err != nil {
			return err
		}
		r, err := e.svc.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if r.ResidualLikelihood == nil || *r.ResidualLikelihood != 1 {
			t.Errorf("after two implemented controls: residual likelihood = %v, want 1", r.ResidualLikelihood)
		}
		return nil
	})
}

// TestAddRiskControl_FloorsAtOne proves more implemented controls than the inherent likelihood never drives
// the residual below 1 (a risk can never become "less than low likelihood").
func TestAddRiskControl_FloorsAtOne(t *testing.T) {
	e := scoreSetup(t, "rra07floor")
	var riskID, c1, c2 uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = e.manualRisk(t, ctx, "ความเสี่ยงต่ำ", 1, 2).ID
		c1 = e.controlID(t, ctx, "ORG_POLICY")
		c2 = e.controlID(t, ctx, "ORG_TRAINING")
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddRiskControl(ctx, riskID, c1, nil, nil); err != nil {
			return err
		}
		if _, err := e.svc.UpdateRiskControlStatus(ctx, riskID, c1, "implemented", 1); err != nil {
			return err
		}
		if _, err := e.svc.AddRiskControl(ctx, riskID, c2, nil, nil); err != nil {
			return err
		}
		if _, err := e.svc.UpdateRiskControlStatus(ctx, riskID, c2, "implemented", 1); err != nil {
			return err
		}
		r, err := e.svc.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if r.ResidualLikelihood == nil || *r.ResidualLikelihood != 1 {
			t.Errorf("residual likelihood = %v, want floored at 1", r.ResidualLikelihood)
		}
		return nil
	})
}

// TestRemoveRiskControl_RecomputesResidual proves removing an implemented control raises the residual back.
func TestRemoveRiskControl_RecomputesResidual(t *testing.T) {
	e := scoreSetup(t, "rra07remove")
	var riskID, c1 uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = e.manualRisk(t, ctx, "ความเสี่ยงทดสอบ", 3, 2).ID
		c1 = e.controlID(t, ctx, "ORG_POLICY")
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddRiskControl(ctx, riskID, c1, nil, nil); err != nil {
			return err
		}
		if _, err := e.svc.UpdateRiskControlStatus(ctx, riskID, c1, "implemented", 1); err != nil {
			return err
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		r, err := e.svc.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if *r.ResidualLikelihood != 2 {
			t.Fatalf("setup: residual likelihood = %v, want 2", r.ResidualLikelihood)
		}
		if err := e.svc.RemoveRiskControl(ctx, riskID, c1); err != nil {
			return err
		}
		r, err = e.svc.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if r.ResidualLikelihood == nil || *r.ResidualLikelihood != 3 {
			t.Errorf("after removing the only control: residual likelihood = %v, want back to 3", r.ResidualLikelihood)
		}
		return nil
	})
}

// TestAddRiskControl_UnknownControlRefused proves the control FK is checked before anything is written.
func TestAddRiskControl_UnknownControlRefused(t *testing.T) {
	e := scoreSetup(t, "rra07unknown")
	var riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = e.manualRisk(t, ctx, "x", 1, 1).ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddRiskControl(ctx, riskID, uuid.New(), nil, nil); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("unknown control: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestAddRiskControl_DuplicateRefused proves linking the same control twice is refused without aborting the
// transaction (pdb.Savepoint around the insert).
func TestAddRiskControl_DuplicateRefused(t *testing.T) {
	e := scoreSetup(t, "rra07dup")
	var riskID, c1 uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = e.manualRisk(t, ctx, "x", 1, 1).ID
		c1 = e.controlID(t, ctx, "ORG_POLICY")
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddRiskControl(ctx, riskID, c1, nil, nil); err != nil {
			return err
		}
		if _, err := e.svc.AddRiskControl(ctx, riskID, c1, nil, nil); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("duplicate link: %v, want ErrInvalid", err)
		}
		// The transaction must still be usable after the failed insert (savepoint rolled back only that).
		if _, err := e.svc.ListRiskControls(ctx, riskID); err != nil {
			t.Errorf("transaction poisoned after duplicate: %v", err)
		}
		return nil
	})
}

// TestAddRiskControl_OwnerAndDueDateOpensTask proves giving both an owner and a due date opens a real
// dpo.tasks job and records its id on the link — DPIA-07's own "ผู้รับผิดชอบและวันเสร็จ → task".
func TestAddRiskControl_OwnerAndDueDateOpensTask(t *testing.T) {
	e := scoreSetup(t, "rra07task")
	var riskID, c1 uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = e.manualRisk(t, ctx, "x", 2, 2).ID
		c1 = e.controlID(t, ctx, "ORG_POLICY")
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE iam.users SET status = 'active' WHERE id = $1`, e.tenant.UserID)
		return err
	})
	owner := e.tenant.UserID
	due := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	e.in(t, func(ctx context.Context) error {
		rc, err := e.svc.AddRiskControl(ctx, riskID, c1, &owner, &due)
		if err != nil {
			return err
		}
		if rc.TaskID == nil {
			t.Error("expected a dpo.tasks id to be recorded on the link")
		}
		if rc.OwnerUserID == nil || *rc.OwnerUserID != owner {
			t.Errorf("owner = %v, want %v", rc.OwnerUserID, owner)
		}
		return nil
	})
}

// TestAddRiskControl_UnknownOwnerRefused checks the owner FK under RLS before writing anything.
func TestAddRiskControl_UnknownOwnerRefused(t *testing.T) {
	e := scoreSetup(t, "rra07badowner")
	var riskID, c1 uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		riskID = e.manualRisk(t, ctx, "x", 1, 1).ID
		c1 = e.controlID(t, ctx, "ORG_POLICY")
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		bogus := uuid.New()
		if _, err := e.svc.AddRiskControl(ctx, riskID, c1, &bogus, nil); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("unknown owner: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestTwoTenantIsolation_RiskControls proves tenant B can neither link a control to tenant A's risk nor
// read its control list.
func TestTwoTenantIsolation_RiskControls(t *testing.T) {
	a := scoreSetup(t, "rra07isoA")
	b := scoreSetup(t, "rra07isoB")
	var riskID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, a.svc)
		riskID = a.manualRisk(t, ctx, "x", 1, 1).ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, b.svc)
		c1 := b.controlID(t, ctx, "ORG_POLICY")
		if _, err := b.svc.AddRiskControl(ctx, riskID, c1, nil, nil); !errors.Is(err, riskservice.ErrNotFound) {
			t.Errorf("cross-tenant risk id: %v, want ErrNotFound", err)
		}
		if _, err := b.svc.ListRiskControls(ctx, riskID); !errors.Is(err, riskservice.ErrNotFound) {
			t.Errorf("cross-tenant list: %v, want ErrNotFound", err)
		}
		return nil
	})
}
