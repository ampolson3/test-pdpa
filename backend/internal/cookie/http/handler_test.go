package cookiehttp_test

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

	cookiehttp "pdpa-platform/internal/cookie/http"
	cookieservice "pdpa-platform/internal/cookie/service"
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

// Contract test for the cookie-banner admin endpoints (CON-01) behind the real OpenAPI validator and AuthZ.
func TestCookieEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "cookiehttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM cookie.consent_records`, `DELETE FROM cookie.banner_configs`, `DELETE FROM cookie.domains`,
				`DELETE FROM cookie.categories`, `DELETE FROM platform.public_keys`, `DELETE FROM org.legal_entities`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	svc := &cookieservice.Service{Audit: audit.New(), Org: orgSvc}

	var legalEntity orgservice.LegalEntity
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท คุกกี้ จำกัด", IsController: true}, 0)
		legalEntity = le
		return err
	})

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
		tenant.UserID.String(): {"cookie.banner.read", "cookie.banner.create", "cookie.banner.update", "cookie.banner.publish", "cookie.banner.delete"},
		reader.String():        {"cookie.banner.read"},
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
	strict := cookiehttp.NewStrictHandlerWithOptions(cookiehttp.NewStrict(svc),
		[]cookiehttp.StrictMiddlewareFunc{authz.StrictMiddleware[cookiehttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		cookiehttp.StrictHTTPServerOptions{
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
	cookiehttp.HandlerWithOptions(strict, cookiehttp.ChiServerOptions{BaseRouter: r})
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

	if code, _ := do("GET", "/admin/v1/cookie/domains", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/cookie/domains", &reader, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("list (none yet): %d %s", code, body)
	}
	newDomain := map[string]any{"domain": "contract.example", "legal_entity_id": legalEntity.ID}
	if code, _ := do("POST", "/admin/v1/cookie/domains", &reader, newDomain, nil); code != 403 {
		t.Errorf("register with read only: %d, want 403", code)
	}
	code, body := do("POST", "/admin/v1/cookie/domains", &admin, newDomain, nil)
	if code != 201 || !strings.Contains(body, `"status":"pending"`) {
		t.Fatalf("register: %d %s", code, body)
	}
	var domain cookiehttp.CookieDomain
	_ = json.Unmarshal([]byte(body), &domain)

	if code, _ := do("GET", "/admin/v1/cookie/domains/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown domain: %d, want 404", code)
	}
	if code, _ := do("POST", "/admin/v1/cookie/domains/"+domain.Id.String()+"/verify", &admin, nil, nil); code != 428 {
		t.Errorf("verify without If-Match: %d, want 428", code)
	}
	code, body = do("POST", "/admin/v1/cookie/domains/"+domain.Id.String()+"/verify", &admin, nil, map[string]string{"If-Match": `"99"`})
	if code != 412 {
		t.Errorf("verify with stale If-Match: %d %s, want 412", code, body)
	}
	code, body = do("POST", "/admin/v1/cookie/domains/"+domain.Id.String()+"/verify", &admin, nil, map[string]string{"If-Match": `"1"`})
	if code != 200 || !strings.Contains(body, `"status":"verified"`) {
		t.Errorf("verify: %d %s", code, body)
	}

	code, body = do("POST", "/admin/v1/cookie/categories", &admin, map[string]any{"code": "analytics", "name_th": "วิเคราะห์"}, nil)
	if code != 201 || !strings.Contains(body, `"code":"analytics"`) {
		t.Fatalf("create category: %d %s", code, body)
	}

	newBanner := map[string]any{"layout": "bar", "texts": map[string]any{
		"title_th": "เราใช้คุกกี้", "accept_all_th": "ยอมรับทั้งหมด", "reject_all_th": "ปฏิเสธทั้งหมด", "settings_th": "ตั้งค่า",
	}}
	code, body = do("POST", "/admin/v1/cookie/domains/"+domain.Id.String()+"/banner-configs", &admin, newBanner, nil)
	if code != 201 || !strings.Contains(body, `"status":"draft"`) {
		t.Fatalf("create banner config: %d %s", code, body)
	}
	var bc cookiehttp.CookieBannerConfig
	_ = json.Unmarshal([]byte(body), &bc)

	if code, _ := do("POST", "/admin/v1/cookie/banner-configs/"+bc.Id.String()+"/publish", &reader, nil, map[string]string{"If-Match": `"1"`}); code != 403 {
		t.Errorf("publish with read only: %d, want 403", code)
	}
	code, body = do("POST", "/admin/v1/cookie/banner-configs/"+bc.Id.String()+"/publish", &admin, nil, map[string]string{"If-Match": `"1"`})
	if code != 200 || !strings.Contains(body, `"status":"published"`) {
		t.Errorf("publish: %d %s", code, body)
	}
}
