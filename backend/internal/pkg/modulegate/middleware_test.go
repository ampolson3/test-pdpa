package modulegate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/modulegate"
)

type handlerFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error)

// TestMiddleware_DefaultEnabledExplicitDisableBlocks is IAM-03's own acceptance criterion
// ("ปิดโมดูลที่ไม่ได้ซื้อแล้ว...API ของโมดูลนั้นใช้ไม่ได้") proven directly against the gate every
// request actually goes through: a tenant with no tenant_modules row at all passes through (opt-out,
// not opt-in), an explicit enabled=false row blocks with 403 platform.module_disabled, and a core
// module (iam/platform) is never gated, whatever the row says.
func TestMiddleware_DefaultEnabledExplicitDisableBlocks(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	platform := dbtest.PlatformPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, platform, "modulegate")

	requiredModule := func(operationID string) (string, bool) {
		switch operationID {
		case "RopaListActivities":
			return "ropa", true
		case "IamGetMe":
			return "iam", true
		default:
			return "", false
		}
	}
	mw := modulegate.Middleware[handlerFunc](requiredModule)

	call := func(ctx context.Context, operationID string) error {
		next := handlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			return "ok", nil
		})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		_, err := mw(next, operationID)(ctx, httptest.NewRecorder(), req, nil)
		return err
	}

	// No tenant_modules row at all: enabled by default.
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		if err := call(ctx, "RopaListActivities"); err != nil {
			t.Fatalf("no row, expected pass-through, got error: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Disable ropa explicitly, then the next request for a ropa-tagged operation is blocked.
	if _, err := platform.Exec(ctx, `INSERT INTO platform.tenant_modules (tenant_id, module_code, enabled) VALUES ($1, 'ropa', false)`, tenant.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = platform.Exec(context.Background(), `DELETE FROM platform.tenant_modules WHERE tenant_id = $1`, tenant.ID)
	})
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		err := call(ctx, "RopaListActivities")
		p, ok := err.(httpx.Problem)
		if !ok || p.Code != "platform.module_disabled" {
			t.Fatalf("got %v (ok=%v), want httpx.Problem{Code: platform.module_disabled}", err, ok)
		}
		if p.Status != http.StatusForbidden {
			t.Fatalf("got status %d, want 403", p.Status)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// iam is a core module — never gated, even though it's tagged with x-module too.
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		if err := call(ctx, "IamGetMe"); err != nil {
			t.Fatalf("iam must never be gated, got error: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestMiddleware_IsolatesAcrossTenants: disabling a module for one tenant must never affect another
// tenant's own request against the same module — the cross-tenant proof this gate's own RLS read
// depends on.
func TestMiddleware_IsolatesAcrossTenants(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	platform := dbtest.PlatformPool(t)
	tenantA := dbtest.SeedTenant(t, ctx, app, platform, "modulegate-a")
	tenantB := dbtest.SeedTenant(t, ctx, app, platform, "modulegate-b")

	requiredModule := func(operationID string) (string, bool) { return "ropa", true }
	mw := modulegate.Middleware[handlerFunc](requiredModule)
	call := func(ctx context.Context) error {
		next := handlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			return "ok", nil
		})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		_, err := mw(next, "RopaListActivities")(ctx, httptest.NewRecorder(), req, nil)
		return err
	}

	if _, err := platform.Exec(ctx, `INSERT INTO platform.tenant_modules (tenant_id, module_code, enabled) VALUES ($1, 'ropa', false)`, tenantA.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = platform.Exec(context.Background(), `DELETE FROM platform.tenant_modules WHERE tenant_id = $1`, tenantA.ID)
	})

	if err := pdb.WithTenantTx(ctx, app, tenantB.ID.String(), tenantB.UserID.String(), func(ctx context.Context) error {
		if err := call(ctx); err != nil {
			t.Fatalf("tenant B blocked by tenant A's own disabled module: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
