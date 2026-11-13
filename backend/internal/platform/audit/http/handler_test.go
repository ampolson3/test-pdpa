package audithttp_test

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
	audithttp "pdpa-platform/internal/platform/audit/http"
	audit "pdpa-platform/internal/platform/audit/service"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the audit-log endpoints behind the real OpenAPI validator and AuthZ.
func TestAuditEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "audithttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	svc := audit.New()
	uid := tenant.UserID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), "", func(ctx context.Context) error {
		return svc.Write(ctx, audit.Entry{TenantID: tenant.ID, ActorType: "user", ActorID: &uid, Action: "iam.role_assignment.create", EntityType: "user", EntityID: &uid})
	}); err != nil {
		t.Fatal(err)
	}

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
	other := uuid.New()
	grants := map[string][]string{tenant.UserID.String(): {"admin.audit.read", "admin.audit.export"}, other.String(): {"admin.audit.read"}}
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
	strict := audithttp.NewStrictHandlerWithOptions(audithttp.NewStrict(svc),
		[]audithttp.StrictMiddlewareFunc{authz.StrictMiddleware[audithttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		audithttp.StrictHTTPServerOptions{
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
	audithttp.HandlerWithOptions(strict, audithttp.ChiServerOptions{BaseRouter: r})
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
	sec, reader := tenant.UserID, other
	nobody := uuid.New()
	grants[nobody.String()] = nil
	base := "/admin/v1/platform/audit-log"
	if code, _ := do("GET", base, nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, _ := do("GET", base, &nobody, nil, nil); code != 403 {
		t.Errorf("without admin.audit.read: %d, want 403", code)
	}
	if code, _ := do("GET", base+"?kind=bogus", &reader, nil, nil); code != 400 {
		t.Errorf("bad kind: %d, want 400", code)
	}
	if code, _ := do("GET", base+"?cursor=nope", &reader, nil, nil); code != 400 {
		t.Errorf("bad cursor: %d, want 400", code)
	}
	if code, body := do("GET", base+"?entity_type=user&entity_id="+sec.String(), &reader, nil, nil); code != 200 || !strings.Contains(body, "iam.role_assignment.create") {
		t.Errorf("search: %d %s", code, body)
	}
	if code, _ := do("GET", base+"/export", &reader, nil, nil); code != 403 {
		t.Errorf("export without admin.audit.export: %d, want 403", code)
	}
	req, _ := http.NewRequest("GET", srv.URL+base+"/export?action_prefix=iam.", nil)
	req.Header.Set("X-Test-User", sec.String())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var csvBody bytes.Buffer
	_, _ = csvBody.ReadFrom(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/csv") || !strings.Contains(res.Header.Get("Content-Disposition"), "attachment") || !strings.Contains(csvBody.String(), "iam.role_assignment.create") {
		t.Errorf("export: %d %v %s", res.StatusCode, res.Header, csvBody.String())
	}
	if code, body := do("POST", base+"/verify", &reader, nil, nil); code != 200 || !strings.Contains(body, `"ok":true`) {
		t.Errorf("verify: %d %s", code, body)
	}
}
