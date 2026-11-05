package importerhttp_test

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
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/importer"
	importerhttp "pdpa-platform/internal/platform/importer/http"
	"pdpa-platform/internal/platform/jobs"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the import endpoints behind the real OpenAPI validator and AuthZ.
func TestImportEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "importerhttp")

	client, _ := jobs.NewInsertClient(app)
	svc := &importer.Service{Types: importer.Registry{"people": {Permission: "x.people.import", Columns: []importer.Column{{Key: "name", Required: true}}}},
		Files: &files.Service{}, River: client, Audit: audit.New()}

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
	grants := map[string][]string{tenant.UserID.String(): {"x.people.import"}, other.String(): {}}
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
	strict := importerhttp.NewStrictHandlerWithOptions(importerhttp.NewStrict(svc),
		[]importerhttp.StrictMiddlewareFunc{authz.StrictMiddleware[importerhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		importerhttp.StrictHTTPServerOptions{
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
	importerhttp.HandlerWithOptions(strict, importerhttp.ChiServerOptions{BaseRouter: r})
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
	missing := uuid.New().String()

	if code, _ := do("GET", "/admin/v1/platform/imports", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, _ := do("POST", "/admin/v1/platform/imports", &other, map[string]any{"import_type": "people", "file_id": missing}, nil); code != 403 {
		t.Errorf("create without the type's permission: %d, want 403", code)
	}
	if code, _ := do("POST", "/admin/v1/platform/imports", &admin, map[string]any{"import_type": "nope", "file_id": missing}, nil); code != 404 {
		t.Errorf("unregistered type: %d, want 404", code)
	}
	if code, _ := do("POST", "/admin/v1/platform/imports", &admin, map[string]any{"import_type": "people"}, nil); code != 400 {
		t.Errorf("missing file_id: %d, want 400", code)
	}
	if code, body := do("POST", "/admin/v1/platform/imports", &admin, map[string]any{"import_type": "people", "file_id": missing}, nil); code != 422 || !strings.Contains(body, "import.file_not_usable") {
		t.Errorf("someone else's / unknown file: %d %s, want 422", code, body)
	}
	if code, body := do("GET", "/admin/v1/platform/imports", &other, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("list: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/platform/imports/"+missing, &admin, nil, nil); code != 404 {
		t.Errorf("unknown import: %d, want 404", code)
	}
	if code, _ := do("PUT", "/admin/v1/platform/imports/"+missing+"/mapping", &admin, map[string]any{"columns": map[string]string{"name": "Name"}}, nil); code != 428 {
		t.Errorf("mapping without If-Match: %d, want 428", code)
	}
	if code, _ := do("POST", "/admin/v1/platform/imports/"+missing+"/confirm", &admin, nil, map[string]string{"If-Match": `"1"`}); code != 404 {
		t.Errorf("confirm unknown import: %d, want 404", code)
	}
	if code, _ := do("GET", "/admin/v1/platform/imports/"+missing+"/errors", &admin, nil, nil); code != 404 {
		t.Errorf("errors of unknown import: %d, want 404", code)
	}
}
