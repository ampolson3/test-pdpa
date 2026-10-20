package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	dposervice "pdpa-platform/internal/dpo/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	riskservice "pdpa-platform/internal/risk/service"
)

// withDpo wires a real dpo.Service onto the env's own risk service (setup() leaves it nil, since no other
// dpia test needs a real dpo.tasks write) so SubmitRiskToRegister's own EnsureRemediationTask call has
// something real to open a task against.
func withDpo(e env) env {
	e.risk.Dpo = &dposervice.Service{Audit: audit.New()}
	return e
}

func countDpiaRiskTasks(t *testing.T, ctx context.Context, riskID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM dpo.tasks WHERE source_type = 'risk' AND source_id = $1`, riskID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestSubmitRiskToRegister_OpensTaskForSignificantRisk is DPIA-08's own acceptance criterion through the
// DPIA round it belongs to: a still-significant risk identified on this assessment already sits in the
// shared risk register (DPIA-06 writes risk.risks directly) — this call's own job is confirming that and
// opening its remediation task, exactly once.
func TestSubmitRiskToRegister_OpensTaskForSignificantRisk(t *testing.T) {
	e := withDpo(setup(t, "dpia08submit"))
	t.Cleanup(func() {
		owner := dbtest.OwnerPool(t)
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM dpo.tasks`)
			return err
		})
	})
	var assessmentID, riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		_, a := e.screenedActivity(t, ctx, "SUB-01")
		assessmentID = a.ID
		r, err := e.svc.IdentifyRisk(ctx, assessmentID, riskservice.Risk{Title: "ความเสี่ยงสูง", Likelihood: 3, Impact: 3})
		if err != nil {
			return err
		}
		riskID = r.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		risk, opened, err := e.svc.SubmitRiskToRegister(ctx, assessmentID, riskID)
		if err != nil {
			return err
		}
		if !opened {
			t.Error("opened = false, want true for a high-level risk with no task yet")
		}
		if risk.ID != riskID || risk.SourceType != "dpia" {
			t.Errorf("returned risk = %+v, want the same dpia-sourced risk", risk)
		}
		if n := countDpiaRiskTasks(t, ctx, riskID); n != 1 {
			t.Errorf("dpo.tasks for risk = %d, want 1", n)
		}
		// A second submit is a harmless no-op — never a duplicate task.
		_, opened, err = e.svc.SubmitRiskToRegister(ctx, assessmentID, riskID)
		if err != nil {
			return err
		}
		if opened {
			t.Error("second submit: opened = true, want false")
		}
		if n := countDpiaRiskTasks(t, ctx, riskID); n != 1 {
			t.Errorf("dpo.tasks for risk after second submit = %d, want still 1", n)
		}
		return nil
	})
}

// TestSubmitRiskToRegister_UnlinkedRiskRefused proves a risk id not actually linked to this assessment is
// refused, the same guard UpdateRisk/AddRiskControl already share.
func TestSubmitRiskToRegister_UnlinkedRiskRefused(t *testing.T) {
	e := withDpo(setup(t, "dpia08unlinked"))
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		e.defaultMatrix(t, ctx)
		_, a := e.screenedActivity(t, ctx, "SUB-02")
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.SubmitRiskToRegister(ctx, assessmentID, uuid.New()); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("unlinked risk id: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestSubmitRiskToRegister_TwoTenantIsolation proves tenant B can't submit tenant A's assessment/risk pair.
func TestSubmitRiskToRegister_TwoTenantIsolation(t *testing.T) {
	a := withDpo(setup(t, "dpia08isoA"))
	b := withDpo(setup(t, "dpia08isoB"))
	var assessmentID, riskID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		a.defaultMatrix(t, ctx)
		_, asmt := a.screenedActivity(t, ctx, "SUB-03")
		assessmentID = asmt.ID
		r, err := a.svc.IdentifyRisk(ctx, assessmentID, riskservice.Risk{Title: "x", Likelihood: 3, Impact: 3})
		if err != nil {
			return err
		}
		riskID = r.ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, _, err := b.svc.SubmitRiskToRegister(ctx, assessmentID, riskID); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant submit: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}
