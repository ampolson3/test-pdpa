package orghttp_test

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

	orghttp "pdpa-platform/internal/org/http"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the calendar endpoints behind the real OpenAPI validator and AuthZ.
func TestCalendarEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "orghttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM org.org_settings`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.holidays`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.business_calendars`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	svc := &orgservice.Service{Audit: audit.New()}

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
	grants := map[string][]string{tenant.UserID.String(): {"org.settings.read", "org.settings.update"}, other.String(): {"org.settings.read"}}
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
	strict := orghttp.NewStrictHandlerWithOptions(orghttp.NewStrict(svc),
		[]orghttp.StrictMiddlewareFunc{authz.StrictMiddleware[orghttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		orghttp.StrictHTTPServerOptions{
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
	orghttp.HandlerWithOptions(strict, orghttp.ChiServerOptions{BaseRouter: r})
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
	admin, dpo := tenant.UserID, other
	cal := map[string]any{"name": "HQ", "timezone": "Asia/Bangkok", "workdays": []int{1, 2, 3, 4, 5}}

	if code, _ := do("GET", "/admin/v1/org/calendars", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, _ := do("POST", "/admin/v1/org/calendars", &dpo, cal, nil); code != 403 {
		t.Errorf("create with read permission only: %d, want 403", code)
	}
	if code, _ := do("POST", "/admin/v1/org/calendars", &admin, map[string]any{"name": "HQ", "timezone": "Asia/Bangkok", "workdays": []int{9}}, nil); code != 400 {
		t.Errorf("workday 9: %d, want 400 (schema)", code)
	}
	if code, body := do("POST", "/admin/v1/org/calendars", &admin, map[string]any{"name": "HQ", "timezone": "Mars/Olympus", "workdays": []int{1}}, nil); code != 422 || !strings.Contains(body, "org.invalid_calendar") {
		t.Errorf("bad timezone: %d %s, want 422", code, body)
	}
	code, body := do("POST", "/admin/v1/org/calendars", &admin, cal, nil)
	if code != 201 || !strings.Contains(body, `"is_default":true`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created orghttp.BusinessCalendar
	_ = json.Unmarshal([]byte(body), &created)
	if code, body := do("POST", "/admin/v1/org/calendars", &admin, cal, nil); code != 422 || !strings.Contains(body, "org.duplicate_calendar_name") {
		t.Errorf("duplicate: %d %s, want 422", code, body)
	}
	item := "/admin/v1/org/calendars/" + created.Id.String()
	if code, _ := do("PATCH", item, &admin, cal, nil); code != 428 {
		t.Errorf("PATCH without If-Match: %d, want 428", code)
	}
	if code, _ := do("PATCH", item, &admin, cal, map[string]string{"If-Match": `"9"`}); code != 412 {
		t.Errorf("stale If-Match: %d, want 412", code)
	}
	if code, body := do("PATCH", item, &admin, map[string]any{"name": "HQ", "timezone": "Asia/Bangkok", "workdays": []int{1, 2, 3, 4, 5, 6}}, map[string]string{"If-Match": `"1"`}); code != 200 || !strings.Contains(body, `"row_version":2`) {
		t.Errorf("update: %d %s", code, body)
	}
	if code, _ := do("PUT", item+"/holidays/2026-04-13", &dpo, map[string]any{"name": "สงกรานต์"}, nil); code != 403 {
		t.Errorf("holiday with read permission only: %d, want 403", code)
	}
	if code, _ := do("PUT", item+"/holidays/13-04-2026", &admin, map[string]any{"name": "สงกรานต์"}, nil); code != 400 {
		t.Errorf("malformed date: %d, want 400", code)
	}
	if code, body := do("PUT", item+"/holidays/2026-04-13", &admin, map[string]any{"name": "สงกรานต์"}, nil); code != 200 || !strings.Contains(body, `"date":"2026-04-13"`) {
		t.Errorf("put holiday: %d %s", code, body)
	}
	if code, body := do("GET", item+"/holidays?year=2026", &dpo, nil, nil); code != 200 || !strings.Contains(body, "สงกรานต์") {
		t.Errorf("list holidays: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/org/calendars/"+uuid.New().String()+"/holidays", &admin, nil, nil); code != 404 {
		t.Errorf("unknown calendar: %d, want 404", code)
	}
	if code, _ := do("DELETE", item+"/holidays/2026-04-13", &admin, nil, nil); code != 204 {
		t.Errorf("delete holiday: %d, want 204", code)
	}
	if code, _ := do("DELETE", item+"/holidays/2026-04-13", &admin, nil, nil); code != 404 {
		t.Errorf("delete again: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/org/calendars", &dpo, nil, nil); code != 200 || !strings.Contains(body, `"name":"HQ"`) {
		t.Errorf("list: %d %s", code, body)
	}
}
