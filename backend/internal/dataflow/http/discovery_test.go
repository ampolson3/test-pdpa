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

// Contract test for DFG-09 discovery endpoints behind the real validator and AuthZ.
func TestDataflowDiscovery_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "dataflowdischttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM dataflow.discovery_findings`)
			_, _ = tx.Exec(ctx, `DELETE FROM dataflow.discovery_scans`)
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
	reader := uuid.New() // read only — proves the 403 path
	grants := map[string][]string{
		tenant.UserID.String(): {"ropa.discovery.read", "ropa.discovery.execute", "ropa.discovery.update"},
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

	if code, _, _ := do("GET", "/admin/v1/dataflow/discovery-scans", nil, "", nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, _, _ := do("POST", "/admin/v1/dataflow/discovery-scans", &reader, "", map[string]any{"connector_id": uuid.NewString()}); code != 403 {
		t.Errorf("reader run scan: %d, want 403", code)
	}
	if code, _, _ := do("GET", "/admin/v1/dataflow/discovery-scans", &reader, "", nil); code != 200 {
		t.Errorf("reader list: %d, want 200", code)
	}
	if code, _, _ := do("POST", "/admin/v1/dataflow/discovery-scans", &admin, "", map[string]any{"connector_id": uuid.NewString()}); code != 422 {
		t.Errorf("run scan without connectors: %d, want 422", code)
	}
	if code, _, _ := do("GET", "/admin/v1/dataflow/discovery-scans/"+uuid.NewString()+"/findings", &admin, "", nil); code != 404 {
		t.Errorf("unknown scan findings: %d, want 404", code)
	}
	if code, _, _ := do("POST", "/admin/v1/dataflow/discovery-findings/"+uuid.NewString()+"/review", &admin, "", map[string]any{"decision": "accepted"}); code != 428 {
		t.Errorf("review without If-Match: %d, want 428", code)
	}
	if code, _, _ := do("POST", "/admin/v1/dataflow/discovery-findings/"+uuid.NewString()+"/review", &admin, `"1"`, map[string]any{"decision": "accepted"}); code != 404 {
		t.Errorf("review unknown finding: %d, want 404", code)
	}
}
