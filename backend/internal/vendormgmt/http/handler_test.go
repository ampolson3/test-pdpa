package vendorhttp_test

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

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
	vendorhttp "pdpa-platform/internal/vendormgmt/http"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the vendor endpoints (VEN-01) behind the real OpenAPI validator and AuthZ.
func TestVendorEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "vendorhttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM vendor.vendors`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.external_parties`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	formsSvc := &forms.Service{}
	formsSvc.Register("intake", forms.Policy{Read: "vendor.vendor.read", Create: "vendor.vendor.approve",
		Update: "vendor.vendor.approve", Publish: "vendor.vendor.approve", Respond: "vendor.vendor.update"})
	svc := &vendorservice.Service{Audit: audit.New(), Org: orgSvc, Forms: formsSvc}

	var party orgservice.ExternalParty
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		p, err := orgSvc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการ", CountryCode: "US"}, 0)
		party = p
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
		tenant.UserID.String(): {"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update"},
		reader.String():        {"vendor.vendor.read"},
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
	strict := vendorhttp.NewStrictHandlerWithOptions(vendorhttp.NewStrict(svc),
		[]vendorhttp.StrictMiddlewareFunc{authz.StrictMiddleware[vendorhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		vendorhttp.StrictHTTPServerOptions{
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
	vendorhttp.HandlerWithOptions(strict, vendorhttp.ChiServerOptions{BaseRouter: r})
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

	if code, _ := do("GET", "/admin/v1/vendors", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/vendors", &reader, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("list (none yet): %d %s", code, body)
	}
	newVendor := map[string]any{"party_id": party.ID, "service_description": "ประมวลผลเงินเดือนบนคลาวด์", "processing_countries": []string{"TH"}}
	if code, _ := do("POST", "/admin/v1/vendors", &reader, newVendor, nil); code != 403 {
		t.Errorf("create with read only: %d, want 403", code)
	}
	if code, body := do("POST", "/admin/v1/vendors", &admin, map[string]any{"party_id": party.ID}, nil); code != 400 {
		t.Errorf("missing service_description: %d %s, want 400 (schema)", code, body)
	}
	code, body := do("POST", "/admin/v1/vendors", &admin, newVendor, nil)
	if code != 201 || !strings.Contains(body, `"status":"prospect"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created vendorhttp.Vendor
	_ = json.Unmarshal([]byte(body), &created)
	item := "/admin/v1/vendors/" + created.Id.String()

	if code, body := do("GET", item, &reader, nil, nil); code != 200 || !strings.Contains(body, `"service_description"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/vendors/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown vendor: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/vendors", &reader, nil, nil); code != 200 || !strings.Contains(body, created.Id.String()) {
		t.Errorf("list: %d %s", code, body)
	}

	update := "/admin/v1/vendors/" + created.Id.String()
	if code, _ := do("PATCH", update, &admin, map[string]any{"party_id": party.ID, "service_description": "x"}, nil); code != 428 {
		t.Errorf("update, no If-Match: %d, want 428", code)
	}
	if code, _ := do("PATCH", update, &admin, map[string]any{"party_id": party.ID, "service_description": "x"}, map[string]string{"If-Match": `"99"`}); code != 412 {
		t.Errorf("update, stale version: %d, want 412", code)
	}
	if code, body := do("PATCH", update, &reader, map[string]any{"party_id": party.ID, "service_description": "x"}, map[string]string{"If-Match": etagOf(int(created.RowVersion))}); code != 403 {
		t.Errorf("update with read only: %d %s, want 403", code, body)
	}
	code, body = do("PATCH", update, &admin, map[string]any{"party_id": party.ID, "service_description": "ประมวลผลเงินเดือน (แก้ไข)"}, map[string]string{"If-Match": etagOf(int(created.RowVersion))})
	if code != 200 || !strings.Contains(body, "แก้ไข") {
		t.Errorf("update: %d %s", code, body)
	}
	if code, body := do("PATCH", update, &admin, map[string]any{"party_id": uuid.New(), "service_description": "x"}, map[string]string{"If-Match": etagOf(int(created.RowVersion) + 1)}); code != 422 {
		t.Errorf("update unknown party: %d %s, want 422", code, body)
	}

	// VEN-02: intake tiering, against the real global "intake" form seeded by migration 00058.
	intakes := item + "/intakes"
	if code, body := do("GET", intakes, &reader, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("list intakes (none yet): %d %s", code, body)
	}
	criticalAnswers := map[string]any{"answers": map[string]any{
		"data_volume": "very_large", "sensitive_data": "yes", "system_access_level": "admin", "cross_border_transfer": "yes"}}
	if code, body := do("POST", intakes, &reader, criticalAnswers, nil); code != 403 {
		t.Errorf("record intake with read only: %d %s, want 403", code, body)
	}
	code, body = do("POST", intakes, &admin, criticalAnswers, nil)
	if code != 201 || !strings.Contains(body, `"tier_result":"critical"`) || !strings.Contains(body, "vendor_pdpa") {
		t.Fatalf("record intake: %d %s", code, body)
	}
	if code, body := do("GET", item, &admin, nil, nil); code != 200 || !strings.Contains(body, `"tier":"critical"`) {
		t.Errorf("vendor tier after intake: %d %s", code, body)
	}
	if code, body := do("GET", intakes, &admin, nil, nil); code != 200 || !strings.Contains(body, `"tier_result":"critical"`) {
		t.Errorf("list intakes: %d %s", code, body)
	}
	incompleteAnswers := map[string]any{"answers": map[string]any{"data_volume": "small"}}
	if code, body := do("POST", intakes, &admin, incompleteAnswers, nil); code != 422 {
		t.Errorf("record intake with missing answers: %d %s, want 422", code, body)
	}
	if code, _ := do("GET", "/admin/v1/vendors/"+uuid.New().String()+"/intakes", &admin, nil, nil); code != 404 {
		t.Errorf("list intakes for unknown vendor: %d, want 404", code)
	}
}

func etagOf(v int) string { return `"` + strconv.Itoa(v) + `"` }
