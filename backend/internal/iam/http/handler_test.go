package iamhttp_test

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

	iamhttp "pdpa-platform/internal/iam/http"
	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	"pdpa-platform/internal/platform/versioning"
	"pdpa-platform/internal/wiring"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// TestRoleAssignmentRequestEndpoints_Contract is IAM-06's HTTP contract test, behind the real OpenAPI
// validator and AuthZ, the same pattern every other module's own handler_test.go already uses.
func TestRoleAssignmentRequestEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "iam06http")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM iam.role_assignment_requests`, `DELETE FROM platform.approvals`,
				`DELETE FROM platform.record_versions`, `DELETE FROM iam.role_assignments`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})

	svc := &iamservice.Service{}
	versioningSvc := &versioning.Service{}
	wiring.RegisterIamVersioning(svc, versioningSvc)

	other := uuid.New()
	var target uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		return tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'Target', 'active') RETURNING id`,
			tenant.ID, uuid.NewString()[:8]+"@iam06http.example").Scan(&target)
	}); err != nil {
		t.Fatal(err)
	}

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
	grants := map[string][]string{
		tenant.UserID.String(): {"admin.user.update", "admin.user.read"},
		other.String():         {"admin.audit.read"},
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
	strict := iamhttp.NewStrictHandlerWithOptions(iamhttp.NewStrict(svc),
		[]iamhttp.StrictMiddlewareFunc{authz.StrictMiddleware[iamhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		iamhttp.StrictHTTPServerOptions{
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
	iamhttp.HandlerWithOptions(strict, iamhttp.ChiServerOptions{BaseRouter: r})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	do := func(method, path string, user *uuid.UUID, body any) (int, string) {
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
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var buf2 bytes.Buffer
		buf2.ReadFrom(res.Body)
		return res.StatusCode, buf2.String()
	}

	listPath := "/admin/v1/iam/role-assignment-requests"
	if code, _ := do("GET", listPath, nil, nil); code != 401 {
		t.Fatalf("GET list no principal = %d, want 401", code)
	}
	if code, _ := do("GET", listPath, &other, nil); code != 403 {
		t.Fatalf("GET list without admin.user.read = %d, want 403", code)
	}
	if code, _ := do("POST", listPath, &other, map[string]any{"target_user_id": target}); code != 403 {
		t.Fatalf("POST without admin.user.update = %d, want 403", code)
	}
	if code, _ := do("POST", listPath, &tenant.UserID, map[string]any{"target_user_id": uuid.New()}); code != 422 {
		t.Fatalf("POST unknown target = %d, want 422", code)
	}

	code, body := do("POST", listPath, &tenant.UserID, map[string]any{"target_user_id": target})
	if code != 201 {
		t.Fatalf("POST happy path = %d (%s), want 201", code, body)
	}
	var created struct {
		ID        uuid.UUID `json:"id"`
		RoleCode  string    `json:"role_code"`
		VersionID uuid.UUID `json:"version_id"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	if created.RoleCode != "ORGADMIN" {
		t.Errorf("role_code = %q, want ORGADMIN", created.RoleCode)
	}
	if created.VersionID == (uuid.UUID{}) {
		t.Error("expected a version_id on creation")
	}

	code, body = do("GET", listPath+"/"+created.ID.String(), &tenant.UserID, nil)
	if code != 200 {
		t.Fatalf("GET one = %d (%s), want 200", code, body)
	}
	if !strings.Contains(body, created.ID.String()) {
		t.Error("expected the request's own id in the response")
	}

	code, body = do("GET", listPath, &tenant.UserID, nil)
	if code != 200 || !strings.Contains(body, created.ID.String()) {
		t.Fatalf("GET list = %d (%s), want 200 containing %s", code, body, created.ID)
	}

	if code, _ := do("GET", listPath+"/"+uuid.New().String(), &tenant.UserID, nil); code != 404 {
		t.Fatalf("GET unknown id = %d, want 404", code)
	}
}
