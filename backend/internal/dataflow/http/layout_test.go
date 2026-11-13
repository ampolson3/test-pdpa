package dataflowhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	dataflowhttp "pdpa-platform/internal/dataflow/http"
	dataflowservice "pdpa-platform/internal/dataflow/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
)

// Contract test for DFG-05's own layout overlay endpoints behind the real OpenAPI validator and AuthZ:
// GET (defaults before any save), PUT (ETag/If-Match, 428 missing / 412 stale), POST .../reset.
func TestDataflowLayout_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "dataflowlayouthttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM dataflow.layouts`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.legal_entities`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	svc := &dataflowservice.Service{Org: orgSvc}

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
	reader := uuid.New() // ropa.dataflow.read only — no .update, proves the 403 path on PUT/reset
	grants := map[string][]string{
		tenant.UserID.String(): {"ropa.dataflow.read", "ropa.dataflow.update"},
		reader.String():        {"ropa.dataflow.read"},
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
	strict := dataflowhttp.NewStrictHandlerWithOptions(dataflowhttp.NewStrict(svc),
		[]dataflowhttp.StrictMiddlewareFunc{authz.StrictMiddleware[dataflowhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		dataflowhttp.StrictHTTPServerOptions{
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
	dataflowhttp.HandlerWithOptions(strict, dataflowhttp.ChiServerOptions{BaseRouter: r})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	do := func(method, path string, user *uuid.UUID, ifMatch string, body any) (int, string, http.Header) {
		var rdr *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, srv.URL+path, rdr)
		req.Header.Set("Content-Type", "application/json")
		if user != nil {
			req.Header.Set("X-Test-User", user.String())
		}
		if ifMatch != "" {
			req.Header.Set("If-Match", ifMatch)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out bytes.Buffer
		_, _ = out.ReadFrom(res.Body)
		return res.StatusCode, out.String(), res.Header
	}
	admin := tenant.UserID

	if code, _, _ := do("GET", "/admin/v1/dataflow/layout", nil, "", nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}

	code, body, _ := do("GET", "/admin/v1/dataflow/layout", &admin, "", nil)
	if code != 200 {
		t.Fatalf("GET layout (never saved): %d %s", code, body)
	}
	if !strings.Contains(body, `"row_version":0`) {
		t.Errorf("never-saved layout should read row_version 0: %s", body)
	}

	if code, _, _ := do("PUT", "/admin/v1/dataflow/layout", &admin, "", map[string]any{"positions": []any{}, "notes": []any{}}); code != 428 {
		t.Errorf("missing If-Match: %d, want 428", code)
	}
	if code, _, _ := do("PUT", "/admin/v1/dataflow/layout", &reader, `"0"`, map[string]any{"positions": []any{}, "notes": []any{}}); code != 403 {
		t.Errorf("reader has no ropa.dataflow.update: %d, want 403", code)
	}

	saveBody := map[string]any{
		"positions": []map[string]any{{"node_id": "activity:abc", "x": 10, "y": 20}},
		"notes":     []map[string]any{{"node_id": "activity:abc", "text": "ย้ายมาตรงนี้"}},
	}
	code, body, headers := do("PUT", "/admin/v1/dataflow/layout", &admin, `"0"`, saveBody)
	if code != 200 {
		t.Fatalf("PUT layout: %d %s", code, body)
	}
	if !strings.Contains(body, `"node_id":"activity:abc"`) || !strings.Contains(body, `"x":10`) {
		t.Errorf("saved layout missing the given position: %s", body)
	}
	savedETag := headers.Get("ETag")
	if savedETag == "" {
		t.Fatal("PUT response missing ETag")
	}

	if code, _, _ := do("PUT", "/admin/v1/dataflow/layout", &admin, `"0"`, saveBody); code != 412 {
		t.Errorf("stale If-Match (still '0'): %d, want 412", code)
	}

	if code, _, _ := do("POST", "/admin/v1/dataflow/layout/reset", &reader, "", nil); code != 403 {
		t.Errorf("reader reset: %d, want 403", code)
	}
	if code, _, _ := do("POST", "/admin/v1/dataflow/layout/reset", &admin, "", nil); code != 204 {
		t.Errorf("reset: %d, want 204", code)
	}

	code, body, _ = do("GET", "/admin/v1/dataflow/layout", &admin, "", nil)
	if code != 200 || !strings.Contains(body, `"row_version":0`) {
		t.Errorf("after reset, layout should read the empty defaults again: %d %s", code, body)
	}
}
