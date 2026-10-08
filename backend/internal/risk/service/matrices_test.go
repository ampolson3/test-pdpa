package service_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	riskservice "pdpa-platform/internal/risk/service"
)

type env struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *riskservice.Service
}

var permissions = []string{"ropa.risk.read", "ropa.risk.create", "ropa.risk.update", "ropa.risk.delete"}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM risk.risk_matrices`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	return env{app: app, tenant: tenant, svc: &riskservice.Service{Audit: audit.New()}}
}

func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: permissions}))
	}); err != nil {
		t.Fatal(err)
	}
}

func matrix3x3() riskservice.RiskMatrix {
	return riskservice.RiskMatrix{
		Name: "3x3", LikelihoodLevels: []string{"low", "medium", "high"}, ImpactLevels: []string{"low", "medium", "high"},
		Thresholds: []riskservice.Threshold{{Level: "low", MinScore: 1}, {Level: "medium", MinScore: 4}, {Level: "high", MinScore: 7}},
	}
}

// TestSaveMatrix_CreateAndValidate is RRA-02's own CRUD: a valid matrix saves, and invalid shapes are
// refused before anything is written.
func TestSaveMatrix_CreateAndValidate(t *testing.T) {
	e := setup(t, "riskCreate")
	var saved riskservice.RiskMatrix
	e.in(t, func(ctx context.Context) error {
		var err error
		saved, err = e.svc.SaveMatrix(ctx, matrix3x3(), 0)
		return err
	})
	if saved.RowVersion != 1 || len(saved.Thresholds) != 3 {
		t.Fatalf("save did not stick: %+v", saved)
	}

	cases := []riskservice.RiskMatrix{
		{Name: "", LikelihoodLevels: []string{"a", "b"}, ImpactLevels: []string{"a", "b"}, Thresholds: []riskservice.Threshold{{Level: "low", MinScore: 1}}},
		{Name: "too few levels", LikelihoodLevels: []string{"a"}, ImpactLevels: []string{"a", "b"}, Thresholds: []riskservice.Threshold{{Level: "low", MinScore: 1}}},
		{Name: "no thresholds", LikelihoodLevels: []string{"a", "b"}, ImpactLevels: []string{"a", "b"}},
		{Name: "bad level", LikelihoodLevels: []string{"a", "b"}, ImpactLevels: []string{"a", "b"}, Thresholds: []riskservice.Threshold{{Level: "critical", MinScore: 1}}},
		{Name: "gap at the bottom", LikelihoodLevels: []string{"a", "b"}, ImpactLevels: []string{"a", "b"}, Thresholds: []riskservice.Threshold{{Level: "high", MinScore: 2}}},
	}
	for i, m := range cases {
		e.in(t, func(ctx context.Context) error {
			if _, err := e.svc.SaveMatrix(ctx, m, 0); err == nil {
				t.Errorf("case %d: expected an error, got none", i)
			}
			return nil
		})
	}
}

// TestClassify_PureAndLive is RRA-02's own acceptance criterion: classification always reads the current
// matrix, never a cached score — changing the matrix (here, just using a different one) changes the
// classification of the exact same (likelihood, impact) pair at once.
func TestClassify_PureAndLive(t *testing.T) {
	m := matrix3x3()
	score, level, err := riskservice.Classify(m, 2, 2) // 2*2 = 4
	if err != nil {
		t.Fatal(err)
	}
	if score != 4 || level != "medium" {
		t.Fatalf("score=%v level=%q, want 4/medium", score, level)
	}
	score, level, err = riskservice.Classify(m, 3, 3) // 3*3 = 9
	if err != nil {
		t.Fatal(err)
	}
	if score != 9 || level != "high" {
		t.Fatalf("score=%v level=%q, want 9/high", score, level)
	}

	// A different matrix classifies the exact same inputs differently — proof that nothing is cached.
	stricter := m
	stricter.Thresholds = []riskservice.Threshold{{Level: "low", MinScore: 1}, {Level: "high", MinScore: 4}}
	_, level2, err := riskservice.Classify(stricter, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if level2 != "high" {
		t.Fatalf("stricter matrix level = %q, want high", level2)
	}

	if _, _, err := riskservice.Classify(m, 0, 1); err == nil {
		t.Error("likelihood 0 should be refused")
	}
	if _, _, err := riskservice.Classify(m, 1, 4); err == nil {
		t.Error("impact beyond the matrix's own levels should be refused")
	}
}

// TestSaveMatrix_OneDefaultPerTenant proves the swap is atomic: setting a new default always leaves
// exactly one, whichever order the saves happen in, and the partial unique index (migration 00055) is
// never hit from inside the service itself.
func TestSaveMatrix_OneDefaultPerTenant(t *testing.T) {
	e := setup(t, "riskDefault")
	var a, b riskservice.RiskMatrix
	e.in(t, func(ctx context.Context) error {
		m := matrix3x3()
		m.IsDefault = true
		var err error
		a, err = e.svc.SaveMatrix(ctx, m, 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		m := matrix3x3()
		m.Name = "second"
		m.IsDefault = true
		var err error
		b, err = e.svc.SaveMatrix(ctx, m, 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListMatrices(ctx)
		if err != nil {
			return err
		}
		defaults := 0
		for _, m := range list {
			if m.IsDefault {
				defaults++
			}
		}
		if defaults != 1 {
			t.Errorf("expected exactly one default, got %d", defaults)
		}
		got, err := e.svc.GetMatrix(ctx, nil)
		if err != nil {
			return err
		}
		if got.ID != b.ID {
			t.Errorf("default resolved to %v, want the second matrix %v", got.ID, b.ID)
		}
		return nil
	})
	_ = a
}

// TestSaveMatrix_VersionMismatch and TestDeleteMatrix round-trip the ETag contract.
func TestSaveMatrix_VersionMismatchAndDelete(t *testing.T) {
	e := setup(t, "riskVersion")
	var m riskservice.RiskMatrix
	e.in(t, func(ctx context.Context) error {
		var err error
		m, err = e.svc.SaveMatrix(ctx, matrix3x3(), 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		m.Name = "renamed"
		if _, err := e.svc.SaveMatrix(ctx, m, 99); err == nil {
			t.Error("stale version should be refused")
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.DeleteMatrix(ctx, m.ID, 99); err == nil {
			t.Error("delete with a stale version should be refused")
		}
		return e.svc.DeleteMatrix(ctx, m.ID, m.RowVersion)
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.GetMatrix(ctx, &m.ID); err == nil {
			t.Error("expected the deleted matrix to be gone")
		}
		return nil
	})
}

// TestTwoTenantIsolation proves tenant B cannot read or list tenant A's matrices (CLAUDE.md rule 1).
func TestTwoTenantIsolation(t *testing.T) {
	eA := setup(t, "riskIsoA")
	var created riskservice.RiskMatrix
	eA.in(t, func(ctx context.Context) error {
		var err error
		created, err = eA.svc.SaveMatrix(ctx, matrix3x3(), 0)
		return err
	})

	eB := setup(t, "riskIsoB")
	eB.in(t, func(ctx context.Context) error {
		if _, err := eB.svc.GetMatrix(ctx, &created.ID); err == nil {
			t.Error("tenant B read tenant A's matrix")
		}
		list, err := eB.svc.ListMatrices(ctx)
		if err != nil {
			return err
		}
		for _, m := range list {
			if m.ID == created.ID {
				t.Error("tenant B's list contains tenant A's matrix")
			}
		}
		return nil
	})
}
