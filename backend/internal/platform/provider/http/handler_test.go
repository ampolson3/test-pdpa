package providerhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	provider "pdpa-platform/internal/platform/provider"
	providerhttp "pdpa-platform/internal/platform/provider/http"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for /provider/v1 (IAM-03) behind the real OpenAPI validator and AuthZ. The acceptance
// criterion itself (a disabled module's API becomes unusable) is proven at the modulegate level
// (internal/pkg/modulegate); this test proves the provider console's own CRUD contract — a SUPER-only
// surface, so the super/non-super permission split is the 401/403 half every other module's own
// contract test already covers.
func TestProviderEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	platform := dbtest.PlatformPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, platform, "providerhttp")

	svc := &provider.Service{Platform: platform, App: app, Redis: rdb, HealthWindowMinutes: 5, ErrorRateThreshold: 0.05, LatencyMsThreshold: 1000}

	spec, err := openapi3.NewLoader().LoadFromFile("../../../../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	validateMw, _ := validate.Middleware(spec, nil)
	perms := map[string]string{}
	for _, item := range spec.Paths.Map() {
		for _, op := range item.Operations() {
			if code, ok := op.Extensions["x-permission"].(string); ok {
				perms[strings.ToUpper(op.OperationID[:1])+op.OperationID[1:]] = code
			}
		}
	}

	super, viewer := tenant.UserID, uuid.New()
	grants := map[string][]string{
		super.String():  {"admin.tenant.read", "admin.tenant.create", "admin.tenant.update", "admin.user.create", "platform.health.read"},
		viewer.String(): {"admin.tenant.read"},
	}
	cache := authz.NewCachedLoader(rdb, func(_ context.Context, tid, uid string) (authz.Grants, error) {
		return authz.Grants{TenantID: tid, UserID: uid, Permissions: grants[uid]}, nil
	})
	t.Cleanup(func() {
		for uid := range grants {
			_ = cache.Invalidate(context.Background(), tenant.ID.String(), uid)
		}
	})

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if u := req.Header.Get("X-Test-User"); u != "" {
				req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{TenantID: tenant.ID.String(), UserID: u, ActorType: "user"}))
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Use(validateMw)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			p, ok := httpx.PrincipalFromContext(req.Context())
			if !ok {
				next.ServeHTTP(w, req)
				return
			}
			_ = pdb.WithTenantTx(req.Context(), app, p.TenantID, p.UserID, func(ctx context.Context) error {
				next.ServeHTTP(w, req.WithContext(ctx))
				return nil
			})
		})
	})
	strict := providerhttp.NewStrictHandlerWithOptions(providerhttp.NewStrict(svc),
		[]providerhttp.StrictMiddlewareFunc{authz.StrictMiddleware[providerhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		providerhttp.StrictHTTPServerOptions{
			RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				httpx.WriteProblem(w, r, httpx.RequestInvalid(err.Error()))
			},
			ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				if p, ok := err.(httpx.Problem); ok {
					httpx.WriteProblem(w, r, p)
					return
				}
				httpx.WriteProblem(w, r, httpx.Internal())
			},
		})
	providerhttp.HandlerWithOptions(strict, providerhttp.ChiServerOptions{BaseRouter: r})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	do := func(method, path string, user *uuid.UUID, body any, headers map[string]string) (int, string) {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, &buf)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if user != nil {
			req.Header.Set("X-Test-User", user.String())
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out bytes.Buffer
		_, _ = out.ReadFrom(res.Body)
		return res.StatusCode, out.String()
	}

	code := "providerhttp-" + uuid.NewString()[:8]
	in := map[string]any{"code": code, "name": "Test Tenant", "plan_code": "free"}

	if code, _ := do("GET", "/provider/v1/tenants", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if status, _ := do("POST", "/provider/v1/tenants", &viewer, in, nil); status != 403 {
		t.Errorf("create with read permission only: %d, want 403", status)
	}
	status, body := do("POST", "/provider/v1/tenants", &super, in, nil)
	if status != 201 {
		t.Fatalf("create: %d %s", status, body)
	}
	var created providerhttp.ProviderTenant
	_ = json.Unmarshal([]byte(body), &created)
	t.Cleanup(func() {
		_, _ = platform.Exec(context.Background(), `DELETE FROM platform.tenants WHERE id = $1`, created.Id)
	})
	if created.Status != "active" || created.RowVersion != 1 {
		t.Fatalf("unexpected create response: %s", body)
	}

	if status, _ := do("POST", "/provider/v1/tenants", &super, in, nil); status != 422 {
		t.Errorf("duplicate code: %d, want 422", status)
	}

	item := "/provider/v1/tenants/" + created.Id.String()
	if status, b := do("GET", item, &viewer, nil, nil); status != 200 || !strings.Contains(b, code) {
		t.Errorf("get: %d %s", status, b)
	}
	if status, _ := do("GET", "/provider/v1/tenants/"+uuid.New().String(), &super, nil, nil); status != 404 {
		t.Errorf("unknown tenant: %d, want 404", status)
	}

	suspend := item + "/suspend"
	if status, _ := do("POST", suspend, &super, nil, nil); status != 428 {
		t.Errorf("suspend without If-Match: %d, want 428", status)
	}
	if status, _ := do("POST", suspend, &super, nil, map[string]string{"If-Match": `"9"`}); status != 412 {
		t.Errorf("stale If-Match: %d, want 412", status)
	}
	if status, _ := do("POST", suspend, &viewer, nil, map[string]string{"If-Match": `"1"`}); status != 403 {
		t.Errorf("suspend with read permission only: %d, want 403", status)
	}
	status, body = do("POST", suspend, &super, nil, map[string]string{"If-Match": `"1"`})
	if status != 200 || !strings.Contains(body, `"status":"suspended"`) {
		t.Fatalf("suspend: %d %s", status, body)
	}

	reactivate := item + "/reactivate"
	status, body = do("POST", reactivate, &super, nil, map[string]string{"If-Match": `"2"`})
	if status != 200 || !strings.Contains(body, `"status":"active"`) {
		t.Fatalf("reactivate: %d %s", status, body)
	}

	// Modules.
	modules := item + "/modules"
	status, body = do("GET", modules, &viewer, nil, nil)
	if status != 200 || !strings.Contains(body, `"module_code":"ropa"`) || !strings.Contains(body, `"enabled":true`) {
		t.Fatalf("list modules: %d %s", status, body)
	}
	setRopa := modules + "/ropa"
	if status, _ := do("PUT", setRopa, &super, map[string]any{"enabled": false}, nil); status != 428 {
		t.Errorf("set module without If-Match: %d, want 428", status)
	}
	status, body = do("PUT", setRopa, &super, map[string]any{"enabled": false}, map[string]string{"If-Match": `"0"`})
	if status != 200 || !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("disable ropa: %d %s", status, body)
	}
	if status, _ := do("PUT", setRopa, &super, map[string]any{"enabled": true}, map[string]string{"If-Match": `"0"`}); status != 412 {
		t.Errorf("stale first-time claim: %d, want 412", status)
	}

	// PLT-19: health dashboard.
	if status, _ := do("GET", "/provider/v1/platform/health", nil, nil, nil); status != 401 {
		t.Errorf("health no principal: %d, want 401", status)
	}
	if status, _ := do("GET", "/provider/v1/platform/health", &viewer, nil, nil); status != 403 {
		t.Errorf("health without platform.health.read: %d, want 403", status)
	}
	status, body = do("GET", "/provider/v1/platform/health", &super, nil, nil)
	if status != 200 || !strings.Contains(body, `"service":"api"`) || !strings.Contains(body, `"status":"healthy"`) {
		t.Fatalf("health: %d %s", status, body)
	}

	// First admin.
	admins := item + "/admins"
	in2 := map[string]any{"email": "first-admin@providerhttp.example", "display_name": "First Admin", "roles": []string{"ORGADMIN"}}
	if status, _ := do("POST", admins, &viewer, in2, nil); status != 403 {
		t.Errorf("create admin with read permission only: %d, want 403", status)
	}
	status, body = do("POST", admins, &super, in2, nil)
	if status != 201 {
		t.Fatalf("create first admin: %d %s", status, body)
	}
	if status, _ := do("POST", admins, &super, in2, nil); status != 409 {
		t.Errorf("duplicate admin: %d, want 409", status)
	}
}
