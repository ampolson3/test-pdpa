package riskhttp_test

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
	audit "pdpa-platform/internal/platform/audit/service"
	riskhttp "pdpa-platform/internal/risk/http"
	riskservice "pdpa-platform/internal/risk/service"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the risk matrix endpoints (RRA-02) behind the real OpenAPI validator and AuthZ.
func TestRiskMatrixEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "riskhttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM risk.risk_matrices`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	svc := &riskservice.Service{Audit: audit.New()}

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
	reader := uuid.New()
	grants := map[string][]string{
		tenant.UserID.String(): {"ropa.risk.read", "ropa.risk.create", "ropa.risk.update", "ropa.risk.delete"},
		reader.String():        {"ropa.risk.read"},
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
	strict := riskhttp.NewStrictHandlerWithOptions(riskhttp.NewStrict(svc),
		[]riskhttp.StrictMiddlewareFunc{authz.StrictMiddleware[riskhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		riskhttp.StrictHTTPServerOptions{
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
	riskhttp.HandlerWithOptions(strict, riskhttp.ChiServerOptions{BaseRouter: r})
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

	if code, _ := do("GET", "/admin/v1/risk/matrices", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/risk/matrices", &reader, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("list (none yet): %d %s", code, body)
	}
	newMatrix := map[string]any{
		"name": "3x3", "likelihood_levels": []string{"low", "medium", "high"}, "impact_levels": []string{"low", "medium", "high"},
		"thresholds": []map[string]any{{"level": "low", "min_score": 1}, {"level": "medium", "min_score": 4}, {"level": "high", "min_score": 7}},
	}
	if code, _ := do("POST", "/admin/v1/risk/matrices", &reader, newMatrix, nil); code != 403 {
		t.Errorf("create with read only: %d, want 403", code)
	}
	if code, body := do("POST", "/admin/v1/risk/matrices", &admin, map[string]any{"name": "x"}, nil); code != 400 {
		t.Errorf("missing required fields: %d %s, want 400 (schema)", code, body)
	}
	if code, body := do("POST", "/admin/v1/risk/matrices", &admin, map[string]any{
		"name": "bad", "likelihood_levels": []string{"a", "b"}, "impact_levels": []string{"a", "b"},
		"thresholds": []map[string]any{{"level": "high", "min_score": 2}}, // no threshold covers score 1
	}, nil); code != 422 {
		t.Errorf("thresholds not covering every score: %d %s, want 422", code, body)
	}
	code, body := do("POST", "/admin/v1/risk/matrices", &admin, newMatrix, nil)
	if code != 201 || !strings.Contains(body, `"name":"3x3"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created riskhttp.RiskMatrix
	_ = json.Unmarshal([]byte(body), &created)
	item := "/admin/v1/risk/matrices/" + created.Id.String()

	if code, body := do("GET", item, &reader, nil, nil); code != 200 || !strings.Contains(body, `"thresholds"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/risk/matrices/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown matrix: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/risk/matrices", &reader, nil, nil); code != 200 || !strings.Contains(body, created.Id.String()) {
		t.Errorf("list: %d %s", code, body)
	}

	update := item
	if code, _ := do("PUT", update, &admin, newMatrix, nil); code != 428 {
		t.Errorf("update, no If-Match: %d, want 428", code)
	}
	if code, _ := do("PUT", update, &admin, newMatrix, map[string]string{"If-Match": `"99"`}); code != 412 {
		t.Errorf("update, stale version: %d, want 412", code)
	}
	if code, body := do("PUT", update, &reader, newMatrix, map[string]string{"If-Match": etagOf(int(created.RowVersion))}); code != 403 {
		t.Errorf("update without update permission: %d %s, want 403", code, body)
	}
	renamed := map[string]any{}
	for k, v := range newMatrix {
		renamed[k] = v
	}
	renamed["name"] = "renamed"
	code, body = do("PUT", update, &admin, renamed, map[string]string{"If-Match": etagOf(int(created.RowVersion))})
	if code != 200 || !strings.Contains(body, `"name":"renamed"`) {
		t.Fatalf("update: %d %s", code, body)
	}
	var updated riskhttp.RiskMatrix
	_ = json.Unmarshal([]byte(body), &updated)

	if code, _ := do("DELETE", item, &reader, nil, map[string]string{"If-Match": etagOf(int(updated.RowVersion))}); code != 403 {
		t.Errorf("delete without delete permission: %d, want 403", code)
	}
	if code, _ := do("DELETE", item, &admin, nil, map[string]string{"If-Match": `"99"`}); code != 412 {
		t.Errorf("delete, stale version: %d, want 412", code)
	}
	if code, _ := do("DELETE", item, &admin, nil, map[string]string{"If-Match": etagOf(int(updated.RowVersion))}); code != 204 {
		t.Errorf("delete: %d, want 204", code)
	}
	if code, _ := do("GET", item, &admin, nil, nil); code != 404 {
		t.Errorf("get after delete: %d, want 404", code)
	}
}

func etagOf(v int) string { return `"` + itoa(v) + `"` }

func itoa(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}
