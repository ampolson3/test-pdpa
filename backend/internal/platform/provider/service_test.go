package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/dbtest"
	provider "pdpa-platform/internal/platform/provider"
)

func newService(t *testing.T) *provider.Service {
	return &provider.Service{Platform: dbtest.PlatformPool(t), App: dbtest.Pool(t)}
}

func TestCreateTenant_CreatesAndRejectsDuplicateCode(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	code := "provider-test-" + uuid.NewString()[:8]

	tenant, err := svc.CreateTenant(ctx, code, "Test Tenant", "free")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	t.Cleanup(func() {
		_, _ = svc.Platform.Exec(context.Background(), `DELETE FROM platform.tenants WHERE id = $1`, tenant.ID)
	})
	if tenant.Status != "active" || tenant.RowVersion != 1 {
		t.Fatalf("unexpected tenant: %+v", tenant)
	}

	if _, err := svc.CreateTenant(ctx, code, "Another name", "free"); !errors.Is(err, provider.ErrDuplicateCode) {
		t.Fatalf("duplicate code: got %v, want ErrDuplicateCode", err)
	}
	if _, err := svc.CreateTenant(ctx, "", "x", "free"); !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("empty code: got %v, want ErrInvalid", err)
	}
}

// TestSuspendReactivateTenant_FollowsStateMachine is the acceptance criterion's prerequisite half:
// suspend/reactivate must follow the tiny trial/active <-> suspended machine, and reject a stale
// If-Match the same way every other ETag-guarded update in this codebase does.
func TestSuspendReactivateTenant_FollowsStateMachine(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	tenant, err := svc.CreateTenant(ctx, "provider-test-"+uuid.NewString()[:8], "Test", "free")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = svc.Platform.Exec(context.Background(), `DELETE FROM platform.tenants WHERE id = $1`, tenant.ID)
	})

	if _, err := svc.SuspendTenant(ctx, tenant.ID, tenant.RowVersion+1); !errors.Is(err, provider.ErrVersionMismatch) {
		t.Fatalf("stale version: got %v, want ErrVersionMismatch", err)
	}
	suspended, err := svc.SuspendTenant(ctx, tenant.ID, tenant.RowVersion)
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if suspended.Status != "suspended" || suspended.SuspendedAt == nil {
		t.Fatalf("unexpected suspended tenant: %+v", suspended)
	}
	if _, err := svc.SuspendTenant(ctx, tenant.ID, suspended.RowVersion); !errors.Is(err, provider.ErrInvalidTransition) {
		t.Fatalf("double suspend: got %v, want ErrInvalidTransition", err)
	}
	reactivated, err := svc.ReactivateTenant(ctx, tenant.ID, suspended.RowVersion)
	if err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if reactivated.Status != "active" || reactivated.SuspendedAt != nil {
		t.Fatalf("unexpected reactivated tenant: %+v", reactivated)
	}
}

// TestListAndSetTenantModule_DefaultEnabledUntilExplicitlyDisabled is IAM-03's own acceptance
// criterion, at the data layer: a module with no row is enabled by default (opt-out, not opt-in),
// setting it false persists, and a stale If-Match (or a first-time claim against an already-set row)
// is rejected rather than silently applied.
func TestListAndSetTenantModule_DefaultEnabledUntilExplicitlyDisabled(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	tenant, err := svc.CreateTenant(ctx, "provider-test-"+uuid.NewString()[:8], "Test", "free")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = svc.Platform.Exec(context.Background(), `DELETE FROM platform.tenants WHERE id = $1`, tenant.ID)
	})

	list, err := svc.ListTenantModules(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListTenantModules: %v", err)
	}
	if len(list) != len(provider.PurchasableModules) {
		t.Fatalf("got %d modules, want %d (every purchasable module, even with no row)", len(list), len(provider.PurchasableModules))
	}
	for _, m := range list {
		if !m.Enabled || m.RowVersion != 0 {
			t.Fatalf("module %s: got enabled=%v row_version=%d, want enabled=true row_version=0 (virtual default)", m.ModuleCode, m.Enabled, m.RowVersion)
		}
	}

	// First save against the virtual default row (row_version 0) disables ropa.
	m, err := svc.SetTenantModule(ctx, tenant.ID, "ropa", false, nil, json.RawMessage(`{}`), 0)
	if err != nil {
		t.Fatalf("SetTenantModule: %v", err)
	}
	if m.Enabled || m.RowVersion != 1 {
		t.Fatalf("unexpected module after disable: %+v", m)
	}

	// Re-reading now shows the real row, not the virtual default.
	list, err = svc.ListTenantModules(ctx, tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, mod := range list {
		if mod.ModuleCode == "ropa" {
			found = true
			if mod.Enabled || mod.RowVersion != 1 {
				t.Fatalf("ropa row not reflected in list: %+v", mod)
			}
		}
	}
	if !found {
		t.Fatal("ropa missing from ListTenantModules")
	}

	// A stale "first time" claim (row_version 0) against an already-set row is a version mismatch, not
	// a silent overwrite.
	if _, err := svc.SetTenantModule(ctx, tenant.ID, "ropa", true, nil, nil, 0); !errors.Is(err, provider.ErrVersionMismatch) {
		t.Fatalf("stale first-time claim: got %v, want ErrVersionMismatch", err)
	}
	// Re-enabling with the correct row_version succeeds.
	m, err = svc.SetTenantModule(ctx, tenant.ID, "ropa", true, nil, nil, m.RowVersion)
	if err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if !m.Enabled {
		t.Fatalf("expected re-enabled module, got %+v", m)
	}

	if _, err := svc.SetTenantModule(ctx, tenant.ID, "not_a_real_module", true, nil, nil, 0); !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("unknown module: got %v, want ErrInvalid", err)
	}
}

// TestCreateFirstAdmin_CreatesUserAndRejectsDuplicate is the module doc's own "สร้างผู้ดูแล tenant
// คนแรก" — a DB-only iam.users row + role grant, following cmd/devseed's exact pattern since no
// Keycloak is available in this environment.
func TestCreateFirstAdmin_CreatesUserAndRejectsDuplicate(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	tenant, err := svc.CreateTenant(ctx, "provider-test-"+uuid.NewString()[:8], "Test", "free")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = svc.Platform.Exec(context.Background(), `DELETE FROM platform.tenants WHERE id = $1`, tenant.ID)
	})

	userID, err := svc.CreateFirstAdmin(ctx, tenant.ID, "admin@provider-test.example", "Admin One", []string{"ORGADMIN"})
	if err != nil {
		t.Fatalf("CreateFirstAdmin: %v", err)
	}
	if userID == uuid.Nil {
		t.Fatal("expected a real user id")
	}

	if _, err := svc.CreateFirstAdmin(ctx, tenant.ID, "admin@provider-test.example", "Admin Two", []string{"ORGADMIN"}); !errors.Is(err, provider.ErrAdminAlreadyExists) {
		t.Fatalf("duplicate email: got %v, want ErrAdminAlreadyExists", err)
	}
	if _, err := svc.CreateFirstAdmin(ctx, tenant.ID, "other@provider-test.example", "Admin Three", []string{"NOT_A_ROLE"}); !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("unknown role: got %v, want ErrInvalid", err)
	}
	if _, err := svc.CreateFirstAdmin(ctx, uuid.New(), "x@provider-test.example", "X", []string{"ORGADMIN"}); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("unknown tenant: got %v, want ErrNotFound", err)
	}
}

// TestTenants_AreIndependentAcrossTenants proves this cross-tenant console manages each tenant's own
// rows correctly and in isolation from another's — the shape of proof this package's own BYPASSRLS
// writes call for, in place of the ordinary per-tenant RLS isolation test every RLS-scoped repository
// in this codebase carries (CLAUDE.md rule 1): there is no RLS here to prove, since pdpa_platform
// bypasses it by design, so what must hold instead is that two tenants' own module licenses and
// statuses never leak into each other.
func TestTenants_AreIndependentAcrossTenants(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	tenantA, err := svc.CreateTenant(ctx, "provider-test-a-"+uuid.NewString()[:8], "Tenant A", "free")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = svc.Platform.Exec(context.Background(), `DELETE FROM platform.tenants WHERE id = $1`, tenantA.ID)
	})
	tenantB, err := svc.CreateTenant(ctx, "provider-test-b-"+uuid.NewString()[:8], "Tenant B", "free")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = svc.Platform.Exec(context.Background(), `DELETE FROM platform.tenants WHERE id = $1`, tenantB.ID)
	})

	if _, err := svc.SetTenantModule(ctx, tenantA.ID, "ropa", false, nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SuspendTenant(ctx, tenantA.ID, tenantA.RowVersion); err != nil {
		t.Fatal(err)
	}

	bModules, err := svc.ListTenantModules(ctx, tenantB.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range bModules {
		if m.ModuleCode == "ropa" && !m.Enabled {
			t.Fatal("tenant B's ropa module was disabled by tenant A's own change — cross-tenant leak")
		}
	}
	b, err := svc.GetTenant(ctx, tenantB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "active" {
		t.Fatalf("tenant B's own status was affected by tenant A's suspend: %+v", b)
	}
}
