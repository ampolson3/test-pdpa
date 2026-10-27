package ropahttp_test

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

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	riskservice "pdpa-platform/internal/risk/service"
	ropahttp "pdpa-platform/internal/ropa/http"
	ropaservice "pdpa-platform/internal/ropa/service"
	templatesservice "pdpa-platform/internal/ropa/templates"
)

// TestTemplateManagementEndpoints_Contract is RTG-08's own HTTP contract test — kept as a separate,
// self-contained fixture rather than extending TestAssetEndpoints_Contract (whose own batch-create
// assertion is pre-existing, unrelated, and already failing before RTG-08 ever touched this package —
// see docs/known-issues.md — so appending here would never actually run).
func TestTemplateManagementEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "ropartg08")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.activity_templates WHERE tenant_id IS NOT NULL`)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.template_sets WHERE tenant_id IS NOT NULL`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})

	svc := &ropaservice.Service{Audit: audit.New(), Org: &orgservice.Service{Audit: audit.New()}, Risk: riskservice.New(), Templates: templatesservice.New()}
	templates := templatesservice.New()
	templates.Audit = audit.New()

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
	viewer := uuid.New()
	grants := map[string][]string{
		tenant.UserID.String(): {"ropa.template.read", "ropa.template.create", "ropa.template.update", "ropa.template.publish", "ropa.template.delete"},
		viewer.String():        {"ropa.template.read"},
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
	strict := ropahttp.NewStrictHandlerWithOptions(ropahttp.NewStrict(svc, templates),
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
	admin := tenant.UserID

	if code, _ := do("GET", "/admin/v1/ropa/templates/custom", nil, nil, nil); code != 401 {
		t.Errorf("list own sets, no principal: %d, want 401", code)
	}
	if code, _ := do("POST", "/admin/v1/ropa/templates/custom", &viewer, map[string]any{"name": "custom set"}, nil); code != 403 {
		t.Errorf("create custom set, read-only caller: %d, want 403", code)
	}

	code, body := do("POST", "/admin/v1/ropa/templates/custom", &admin, map[string]any{"name": "group standard"}, nil)
	if code != 201 {
		t.Fatalf("create custom set: %d %s", code, body)
	}
	var customSet ropahttp.RopaTemplateSet
	_ = json.Unmarshal([]byte(body), &customSet)
	if customSet.SetType != "custom" || customSet.Status == nil || *customSet.Status != "draft" {
		t.Fatalf("expected a draft custom set, got %+v", customSet)
	}

	if code, body := do("GET", "/admin/v1/ropa/templates/custom", &admin, nil, nil); code != 200 || !strings.Contains(body, customSet.Id.String()) {
		t.Errorf("list own template sets: %d %s", code, body)
	}

	customActivity := map[string]any{
		"code": "grp_support", "name_th": "ซัพพอร์ตกลุ่มบริษัท", "job_category": "it_support", "role": "controller",
		"defaults":  map[string]any{"purposes": []any{map[string]any{"purpose_text_th": "x", "lawful_basis_code": "CONTRACT"}}},
		"rationale": map[string]any{"note": "test"},
	}
	code, body = do("POST", "/admin/v1/ropa/templates/"+customSet.Id.String()+"/activities", &admin, customActivity, nil)
	if code != 201 {
		t.Fatalf("add activity template: %d %s", code, body)
	}
	var customActivityWire ropahttp.RopaActivityTemplate
	_ = json.Unmarshal([]byte(body), &customActivityWire)
	if customActivityWire.RowVersion == nil {
		t.Fatalf("expected a row_version on the new custom activity, got %+v", customActivityWire)
	}

	// adding an activity to the global standard set is refused — a tenant can only write its own sets.
	stdCode, stdBody := do("GET", "/admin/v1/ropa/templates", &admin, nil, nil)
	if stdCode != 200 {
		t.Fatalf("list published sets: %d %s", stdCode, stdBody)
	}
	var stdResp struct {
		Data []ropahttp.RopaTemplateSet `json:"data"`
	}
	_ = json.Unmarshal([]byte(stdBody), &stdResp)
	var standardSetID uuid.UUID
	for _, s := range stdResp.Data {
		if s.SetType == "standard" {
			standardSetID = s.Id
		}
	}
	if standardSetID == uuid.Nil {
		t.Fatal("expected the seeded global standard set")
	}
	if code, _ := do("POST", "/admin/v1/ropa/templates/"+standardSetID.String()+"/activities", &admin, customActivity, nil); code != 422 {
		t.Errorf("add activity to the global standard set: %d, want 422", code)
	}

	publishURL := "/admin/v1/ropa/templates/" + customSet.Id.String() + "/publish"
	if code, _ := do("POST", publishURL, &admin, nil, nil); code != 428 {
		t.Errorf("publish with no If-Match: %d, want 428", code)
	}
	if code, body := do("POST", publishURL, &admin, nil, map[string]string{"If-Match": `"0"`}); code != 412 {
		t.Errorf("publish with a stale If-Match: %d %s, want 412", code, body)
	}
	code, body = do("POST", publishURL, &admin, nil, map[string]string{"If-Match": `"1"`})
	if code != 200 || !strings.Contains(body, `"status":"published"`) {
		t.Fatalf("publish: %d %s", code, body)
	}

	// the acceptance criterion itself: once published, it shows up on the exact same published browse
	// list RTG-01's own standard set already uses — every legal entity under this tenant can now create
	// activities from it.
	code, body = do("GET", "/admin/v1/ropa/templates", &admin, nil, nil)
	if code != 200 || !strings.Contains(body, customSet.Id.String()) {
		t.Errorf("published custom set must appear in the published browse list: %d %s", code, body)
	}

	retireURL := "/admin/v1/ropa/templates/" + customSet.Id.String() + "/retire"
	code, body = do("POST", retireURL, &admin, nil, map[string]string{"If-Match": `"2"`})
	if code != 200 || !strings.Contains(body, `"status":"retired"`) {
		t.Fatalf("retire: %d %s", code, body)
	}
	code, body = do("GET", "/admin/v1/ropa/templates", &admin, nil, nil)
	if code != 200 || strings.Contains(body, customSet.Id.String()) {
		t.Errorf("a retired custom set must no longer appear in the published browse list: %d %s", code, body)
	}
}
