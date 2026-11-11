package connectorshttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
	"pdpa-platform/internal/platform/connectors"
	connectorshttp "pdpa-platform/internal/platform/connectors/http"
	"pdpa-platform/internal/platform/crypto"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func etagOf(v int) string { return `"` + strconv.Itoa(v) + `"` }

// Contract test for the connector endpoints (PLT-20) behind the real OpenAPI validator and AuthZ.
func TestConnectorEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "connectorshttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.connector_secrets`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.connectors`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})

	svc := connectors.New(connectors.Registry{
		"rest": connectors.RESTDriver(),
	}, &crypto.Keyring{KEK: crypto.NewLocalKEK()}, nil)

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
	reader := uuid.New()
	grants := map[string][]string{
		tenant.UserID.String(): {"ropa.discovery.read", "ropa.discovery.create", "ropa.discovery.update", "ropa.discovery.delete", "ropa.discovery.execute"},
		reader.String():        {"ropa.discovery.read"},
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
	strict := connectorshttp.NewStrictHandlerWithOptions(connectorshttp.NewStrict(svc),
		[]connectorshttp.StrictMiddlewareFunc{authz.StrictMiddleware[connectorshttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		connectorshttp.StrictHTTPServerOptions{
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
	connectorshttp.HandlerWithOptions(strict, connectorshttp.ChiServerOptions{BaseRouter: r})
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
	admin := tenant.UserID

	if code, _ := do("GET", "/admin/v1/platform/connector-types", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/platform/connector-types", &reader, nil, nil); code != 200 || !strings.Contains(body, `"rest"`) {
		t.Errorf("list types: %d %s", code, body)
	}

	if code, body := do("GET", "/admin/v1/platform/connectors", &reader, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("list (none yet): %d %s", code, body)
	}

	newConnector := map[string]any{"name": "REST demo", "connector_type": "rest",
		"config": map[string]any{"base_url": "http://example.invalid", "test_path": "/health"}, "secret": "tok123"}
	if code, _ := do("POST", "/admin/v1/platform/connectors", &reader, newConnector, nil); code != 403 {
		t.Errorf("create with read only: %d, want 403", code)
	}
	if code, body := do("POST", "/admin/v1/platform/connectors", &admin, map[string]any{"connector_type": "rest"}, nil); code != 400 {
		t.Errorf("missing name: %d %s, want 400 (schema)", code, body)
	}
	code, body := do("POST", "/admin/v1/platform/connectors", &admin, newConnector, nil)
	if code != 201 || !strings.Contains(body, `"status":"draft"`) || !strings.Contains(body, `"has_secret":true`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created connectorshttp.Connector
	_ = json.Unmarshal([]byte(body), &created)
	item := "/admin/v1/platform/connectors/" + created.Id.String()

	badType := map[string]any{"name": "x", "connector_type": "not_a_real_type"}
	if code, body := do("POST", "/admin/v1/platform/connectors", &admin, badType, nil); code != 422 {
		t.Errorf("unknown connector_type: %d %s, want 422", code, body)
	}

	if code, body := do("GET", item, &reader, nil, nil); code != 200 || !strings.Contains(body, `"connector_type":"rest"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/platform/connectors/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown connector: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/platform/connectors", &reader, nil, nil); code != 200 || !strings.Contains(body, created.Id.String()) {
		t.Errorf("list: %d %s", code, body)
	}

	if code, _ := do("PATCH", item, &admin, map[string]any{"name": "renamed", "connector_type": "rest"}, nil); code != 428 {
		t.Errorf("update, no If-Match: %d, want 428", code)
	}
	if code, _ := do("PATCH", item, &admin, map[string]any{"name": "renamed", "connector_type": "rest"}, map[string]string{"If-Match": `"99"`}); code != 412 {
		t.Errorf("update, stale version: %d, want 412", code)
	}
	if code, _ := do("PATCH", item, &reader, map[string]any{"name": "renamed", "connector_type": "rest"}, map[string]string{"If-Match": etagOf(created.RowVersion)}); code != 403 {
		t.Errorf("update with read only: %d, want 403", code)
	}
	code, body = do("PATCH", item, &admin, map[string]any{"name": "renamed", "connector_type": "rest"}, map[string]string{"If-Match": etagOf(created.RowVersion)})
	if code != 200 || !strings.Contains(body, "renamed") || !strings.Contains(body, `"has_secret":true`) {
		t.Errorf("update (secret omitted stays sealed): %d %s", code, body)
	}
	var updated connectorshttp.Connector
	_ = json.Unmarshal([]byte(body), &updated)

	// Test connection: the acceptance criterion's own "ทดสอบการเชื่อมต่อจากหน้าจอได้" — never a 5xx,
	// reported through the body's own ok field (the REST driver's target host doesn't resolve).
	testPath := item + "/test"
	if code, _ := do("POST", testPath, &reader, nil, nil); code != 403 {
		t.Errorf("test with read only: %d, want 403", code)
	}
	code, body = do("POST", testPath, &admin, nil, nil)
	if code != 200 || !strings.Contains(body, `"ok":false`) {
		t.Errorf("test connection (expected ok:false against an unreachable host): %d %s", code, body)
	}

	if code, _ := do("DELETE", item, &admin, nil, map[string]string{"If-Match": etagOf(updated.RowVersion)}); code != 204 {
		t.Errorf("delete: %d", code)
	}
	if code, _ := do("GET", item, &admin, nil, nil); code != 404 {
		t.Errorf("get after delete: %d, want 404", code)
	}
}
