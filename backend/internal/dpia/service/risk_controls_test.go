package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	riskservice "pdpa-platform/internal/risk/service"
)

// controlID resolves a real risk.controls row by its seeded code (ROPA-09, migration 00040) through the
// same riskservice.Service the dpia env already wires.
func (e env) controlID(t *testing.T, ctx context.Context, code string) uuid.UUID {
	t.Helper()
	controls, err := e.risk.ListControls(ctx)
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

// TestDpiaAddRiskControl_ThroughAssessment is DPIA-07's own wrapper, exercised through the DPIA round it
// belongs to: linking a control to a risk identified on an in_progress assessment, then recomputing
// residual once implemented — same acceptance criterion as risk/service's own test, proven end to end here.
func TestDpiaAddRiskControl_ThroughAssessment(t *testing.T) {
	e := setup(t, "dpiactl1")
	var assessmentID, riskID, controlID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		_, a := e.screenedActivity(t, ctx, "CTL-01")
		assessmentID = a.ID
		r, err := e.svc.IdentifyRisk(ctx, assessmentID, riskservice.Risk{Title: "a", Likelihood: 2, Impact: 2})
		if err != nil {
			return err
		}
		riskID = r.ID
		controlID = e.controlID(t, ctx, "ORG_POLICY")
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		rc, err := e.svc.AddRiskControl(ctx, assessmentID, riskID, controlID, nil, nil)
		if err != nil {
			return err
		}
		if rc.ControlCode != "ORG_POLICY" {
			t.Errorf("control_code = %q, want ORG_POLICY", rc.ControlCode)
		}
		list, err := e.svc.ListRiskControls(ctx, assessmentID, riskID)
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Fatalf("ListRiskControls = %+v, want exactly 1", list)
		}
		updated, err := e.svc.UpdateRiskControlStatus(ctx, assessmentID, riskID, controlID, "implemented", list[0].RowVersion)
		if err != nil {
			return err
		}
		if updated.Status != "implemented" {
			t.Errorf("status = %q, want implemented", updated.Status)
		}
		r, err := e.risk.GetRisk(ctx, riskID)
		if err != nil {
			return err
		}
		if r.ResidualLikelihood == nil || *r.ResidualLikelihood != 1 {
			t.Errorf("residual likelihood = %v, want 1 (2 - one implemented control)", r.ResidualLikelihood)
		}
		if err := e.svc.RemoveRiskControl(ctx, assessmentID, riskID, controlID); err != nil {
			return err
		}
		list, err = e.svc.ListRiskControls(ctx, assessmentID, riskID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("ListRiskControls after remove = %+v, want empty", list)
		}
		return nil
	})
}

// TestDpiaAddRiskControl_RefusedOutsideEditableStatus mirrors DPIA-06's own screening-status guard.
func TestDpiaAddRiskControl_RefusedOutsideEditableStatus(t *testing.T) {
	e := setup(t, "dpiactl2")
	var notRequiredID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		act := e.activity(t, ctx, "CTL-02")
		a, err := e.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		notRequiredID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		controlID := e.controlID(t, ctx, "ORG_POLICY")
		if _, err := e.svc.AddRiskControl(ctx, notRequiredID, uuid.New(), controlID, nil, nil); !errors.Is(err, dpiaservice.ErrInvalidTransition) {
			t.Errorf("not_required round: %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

// TestDpiaAddRiskControl_RejectsRiskNotLinkedToThisAssessment proves one round can't touch another's risk's
// controls by guessing the risk id, even within the same tenant.
func TestDpiaAddRiskControl_RejectsRiskNotLinkedToThisAssessment(t *testing.T) {
	e := setup(t, "dpiactl3")
	var round1, round2, riskID, controlID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		_, a1 := e.screenedActivity(t, ctx, "CTL-03A")
		_, a2 := e.screenedActivity(t, ctx, "CTL-03B")
		round1, round2 = a1.ID, a2.ID
		r, err := e.svc.IdentifyRisk(ctx, round1, riskservice.Risk{Title: "a", Likelihood: 1, Impact: 1})
		if err != nil {
			return err
		}
		riskID = r.ID
		controlID = e.controlID(t, ctx, "ORG_POLICY")
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddRiskControl(ctx, round2, riskID, controlID, nil, nil); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("unlinked risk: %v, want ErrNotFound", err)
		}
		if _, err := e.svc.ListRiskControls(ctx, round2, riskID); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("list for unlinked risk: %v, want ErrNotFound", err)
		}
		return nil
	})
}
