package ropahttp_test

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

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	ropahttp "pdpa-platform/internal/ropa/http"
	ropaservice "pdpa-platform/internal/ropa/service"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the asset endpoints behind the real OpenAPI validator and AuthZ.
func TestAssetEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "ropahttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.assets`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.external_parties`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	svc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc}

	spec, err := openapi3.NewLoader().LoadFromFile("../../../../api/openapi/openapi.yaml")
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
	other := uuid.New()
	grants := map[string][]string{tenant.UserID.String(): {"ropa.inventory.read", "ropa.inventory.create", "ropa.inventory.update"},
		other.String(): {"ropa.inventory.read"}}
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
	strict := ropahttp.NewStrictHandlerWithOptions(ropahttp.NewStrict(svc),
		[]ropahttp.StrictMiddlewareFunc{authz.StrictMiddleware[ropahttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		ropahttp.StrictHTTPServerOptions{
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
	ropahttp.HandlerWithOptions(strict, ropahttp.ChiServerOptions{BaseRouter: r})
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
	admin, viewer := tenant.UserID, other
	asset := map[string]any{"name": "ระบบ HRIS", "asset_type": "application", "hosting_country_code": "TH"}

	if code, _ := do("GET", "/admin/v1/ropa/assets", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, _ := do("POST", "/admin/v1/ropa/assets", &viewer, asset, nil); code != 403 {
		t.Errorf("create with read permission only: %d, want 403", code)
	}
	if code, _ := do("POST", "/admin/v1/ropa/assets", &admin, map[string]any{"name": "x", "asset_type": "bogus"}, nil); code != 400 {
		t.Errorf("bad asset_type: %d, want 400 (schema)", code)
	}
	if code, body := do("POST", "/admin/v1/ropa/assets", &admin, map[string]any{"name": "x", "asset_type": "application", "org_unit_id": uuid.New()}, nil); code != 422 || !strings.Contains(body, "ropa.invalid_asset") {
		t.Errorf("unknown org unit: %d %s, want 422", code, body)
	}
	code, body := do("POST", "/admin/v1/ropa/assets", &admin, asset, nil)
	if code != 201 || !strings.Contains(body, `"status":"active"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created ropahttp.Asset
	_ = json.Unmarshal([]byte(body), &created)
	item := "/admin/v1/ropa/assets/" + created.Id.String()
	if code, _ := do("PATCH", item, &admin, asset, nil); code != 428 {
		t.Errorf("update without If-Match: %d, want 428", code)
	}
	if code, _ := do("PATCH", item, &admin, asset, map[string]string{"If-Match": `"9"`}); code != 412 {
		t.Errorf("stale If-Match: %d, want 412", code)
	}
	if code, body := do("PATCH", item, &admin, map[string]any{"name": "ระบบ HRIS (ใหม่)", "asset_type": "database", "status": "retired"}, map[string]string{"If-Match": `"1"`}); code != 200 || !strings.Contains(body, `"row_version":2`) || !strings.Contains(body, `"status":"retired"`) {
		t.Errorf("update: %d %s", code, body)
	}
	if code, body := do("GET", item, &viewer, nil, nil); code != 200 || !strings.Contains(body, `"asset_type":"database"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/ropa/assets/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown asset: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/ropa/assets?q=HRIS", &viewer, nil, nil); code != 200 || !strings.Contains(body, `"name":"ระบบ HRIS`) {
		t.Errorf("search: %d %s", code, body)
	}
}
