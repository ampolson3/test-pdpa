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
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	dpiaservice "pdpa-platform/internal/dpia/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/platform/jobs"
	riskservice "pdpa-platform/internal/risk/service"
	vendorhttp "pdpa-platform/internal/vendormgmt/http"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
	"pdpa-platform/internal/wiring"
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

// TestVendorAssessmentEndpoints_Contract is VEN-07's own HTTP contract test: recording an assessment
// through the real validator/AuthZ chain produces a real score and residual risk level.
func TestVendorAssessmentEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "vendorassesshttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM vendor.vendor_assessments`, `DELETE FROM assess.answers`, `DELETE FROM assess.assessments`,
				`DELETE FROM risk.risk_matrices`, `DELETE FROM vendor.vendors`, `DELETE FROM org.external_parties`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	riskSvc := riskservice.New()
	riskSvc.Audit = audit.New()
	formsSvc := wiring.Forms(nil, audit.New())
	dpiaSvc := &dpiaservice.Service{Forms: formsSvc, Org: orgSvc, Audit: audit.New(), Risk: riskSvc}
	svc := &vendorservice.Service{Audit: audit.New(), Org: orgSvc, Forms: formsSvc, Dpia: dpiaSvc, Risk: riskSvc}

	var vendorID uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"vendor.vendor.create", "ropa.risk.create"}})
		if _, err := riskSvc.SaveMatrix(ctx, riskservice.RiskMatrix{
			Name: "default", LikelihoodLevels: []string{"low", "medium", "high"}, ImpactLevels: []string{"low", "medium", "high"},
			Thresholds: []riskservice.Threshold{{Level: "low", MinScore: 1}, {Level: "medium", MinScore: 4}, {Level: "high", MinScore: 7}},
			IsDefault:  true,
		}, 0); err != nil {
			return err
		}
		p, err := orgSvc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการ VEN-07", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		v, err := svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: p.ID, ServiceDescription: "ทดสอบ HTTP VEN-07"}, 0)
		vendorID = v.ID
		return err
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
	reader := uuid.New()
	grants := map[string][]string{
		tenant.UserID.String(): {"vendor.vendor.read", "vendor.assessment.read", "vendor.assessment.create", "assessment.template.read"},
		reader.String():        {"vendor.vendor.read", "vendor.assessment.read", "assessment.template.read"},
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
		var out bytes.Buffer
		_, _ = out.ReadFrom(res.Body)
		return res.StatusCode, out.String()
	}
	admin := tenant.UserID
	assessments := "/admin/v1/vendors/" + vendorID.String() + "/assessments"
	body := map[string]any{"template_code": "vendor_pdpa", "answers": map[string]any{
		"has_dpo": "yes", "has_retention_policy": "yes", "has_breach_process": "yes",
		"has_subprocessor_list": "yes", "has_dpa_signed": "yes",
	}}

	if code, _ := do("GET", assessments, nil, nil); code != 401 {
		t.Errorf("list, no principal: %d, want 401", code)
	}
	if code, _ := do("POST", assessments, &reader, body); code != 403 {
		t.Errorf("record with read permission only: %d, want 403", code)
	}
	code, resBody := do("POST", assessments, &admin, body)
	if code != 201 || !strings.Contains(resBody, `"score":5`) || !strings.Contains(resBody, `"residual_level":"low"`) {
		t.Fatalf("record assessment: %d %s", code, resBody)
	}
	if code, resBody := do("GET", assessments, &reader, nil); code != 200 || !strings.Contains(resBody, `"cycle_no":1`) {
		t.Errorf("list assessments: %d %s", code, resBody)
	}
	if code, _ := do("POST", assessments, &admin, map[string]any{"template_code": "no_such_template", "answers": map[string]any{}}); code != 422 {
		t.Errorf("unknown template_code: %d, want 422", code)
	}
	if code, _ := do("GET", "/admin/v1/vendors/"+uuid.New().String()+"/assessments", &admin, nil); code != 404 {
		t.Errorf("list assessments for unknown vendor: %d, want 404", code)
	}
}

// TestVendorDecideAssessmentEndpoints_Contract is VEN-08's own HTTP contract test: deciding an answered
// cycle through the real validator/AuthZ chain requires vendor.assessment.approve (not just .create), a
// conditional decision opens real remediation items visible on their own listing endpoint, and an
// already-decided cycle 409s on a second decision.
func TestVendorDecideAssessmentEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "vendordecidehttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM vendor.remediation_items`, `DELETE FROM vendor.vendor_assessments`, `DELETE FROM assess.answers`,
				`DELETE FROM assess.assessments`, `DELETE FROM risk.risk_matrices`, `DELETE FROM vendor.vendors`,
				`DELETE FROM org.external_parties`, `DELETE FROM platform.audit_log`, `DELETE FROM platform.outbox_events`,
				`DELETE FROM river_job WHERE kind = 'outbox.dispatch'`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	riskSvc := riskservice.New()
	riskSvc.Audit = audit.New()
	formsSvc := wiring.Forms(nil, audit.New())
	dpiaSvc := &dpiaservice.Service{Forms: formsSvc, Org: orgSvc, Audit: audit.New(), Risk: riskSvc}
	riverClient, err := jobs.NewInsertClient(app)
	if err != nil {
		t.Fatal(err)
	}
	svc := &vendorservice.Service{Audit: audit.New(), Org: orgSvc, Forms: formsSvc, Dpia: dpiaSvc, Risk: riskSvc,
		Events: &events.Publisher{River: riverClient}}

	var vendorID, assessmentID uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"vendor.vendor.create", "ropa.risk.create", "vendor.assessment.create", "assessment.template.read"}})
		if _, err := riskSvc.SaveMatrix(ctx, riskservice.RiskMatrix{
			Name: "default", LikelihoodLevels: []string{"low", "medium", "high"}, ImpactLevels: []string{"low", "medium", "high"},
			Thresholds: []riskservice.Threshold{{Level: "low", MinScore: 1}, {Level: "medium", MinScore: 4}, {Level: "high", MinScore: 7}},
			IsDefault:  true,
		}, 0); err != nil {
			return err
		}
		p, err := orgSvc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการ VEN-08", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		v, err := svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: p.ID, ServiceDescription: "ทดสอบ HTTP VEN-08"}, 0)
		if err != nil {
			return err
		}
		vendorID = v.ID
		a, err := svc.RecordAssessment(ctx, vendorID, "vendor_pdpa", forms.Answers{"has_dpo": "no", "has_retention_policy": "no",
			"has_breach_process": "no", "has_subprocessor_list": "no", "has_dpa_signed": "no"})
		assessmentID = a.ID
		return err
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
	decider := uuid.New()
	grants := map[string][]string{
		tenant.UserID.String(): {"vendor.vendor.read", "vendor.assessment.read"},
		decider.String():       {"vendor.vendor.read", "vendor.assessment.read", "vendor.assessment.approve"},
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
		var out bytes.Buffer
		_, _ = out.ReadFrom(res.Body)
		return res.StatusCode, out.String()
	}

	decidePath := "/admin/v1/vendors/" + vendorID.String() + "/assessments/" + assessmentID.String() + "/decision"
	remediationPath := "/admin/v1/vendors/" + vendorID.String() + "/remediation-items"

	conditionalBody := map[string]any{"decision": "conditional", "remediation_items": []map[string]any{
		{"title": "ต้องส่งหลักฐานการเข้ารหัสข้อมูลเพิ่มเติม", "severity": "high"},
	}}
	if code, _ := do("POST", decidePath, &tenant.UserID, conditionalBody); code != 403 {
		t.Errorf("decide without vendor.assessment.approve: %d, want 403", code)
	}
	code, resBody := do("POST", decidePath, &decider, conditionalBody)
	if code != 200 || !strings.Contains(resBody, `"decision":"conditional"`) || !strings.Contains(resBody, `"status":"open"`) {
		t.Fatalf("decide conditional: %d %s", code, resBody)
	}
	if code, resBody := do("GET", remediationPath, &tenant.UserID, nil); code != 200 || !strings.Contains(resBody, `"severity":"high"`) {
		t.Errorf("list remediation items: %d %s", code, resBody)
	}
	if code, _ := do("POST", decidePath, &decider, map[string]any{"decision": "approved"}); code != 409 {
		t.Errorf("re-deciding an already-decided cycle: %d, want 409", code)
	}
	if code, _ := do("POST", decidePath, &decider, map[string]any{"decision": "conditional"}); code != 422 {
		t.Errorf("conditional with no remediation items (input validation runs before the already-decided check): %d, want 422", code)
	}
}

type fakeOffboardingFiles struct {
	raw      map[uuid.UUID]bool
	attached map[uuid.UUID]files.File
}

func (f *fakeOffboardingFiles) Get(ctx context.Context, id uuid.UUID) (files.File, error) {
	if a, ok := f.attached[id]; ok {
		return a, nil
	}
	if !f.raw[id] {
		return files.File{}, pgx.ErrNoRows
	}
	return files.File{ID: id}, nil
}

func (f *fakeOffboardingFiles) Attach(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error {
	eid := entityID
	f.attached[id] = files.File{ID: id, EntityType: entityType, EntityID: &eid}
	return nil
}

// TestVendorOffboardingEndpoints_Contract is VEN-14's own HTTP contract test: starting offboarding needs
// vendor.vendor.update, terminating needs the permanent-status-change vendor.vendor.delete, and terminating
// is refused until a destruction certificate and access revocation are both on record (the acceptance
// criterion), all through the real OpenAPI validator + AuthZ chain.
func TestVendorOffboardingEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "vendoroffboardhttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM vendor.offboardings`)
			_, _ = tx.Exec(ctx, `DELETE FROM vendor.vendors`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.external_parties`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	ff := &fakeOffboardingFiles{raw: map[uuid.UUID]bool{}, attached: map[uuid.UUID]files.File{}}
	svc := &vendorservice.Service{Audit: audit.New(), Org: orgSvc, Files: ff}

	var vendorID uuid.UUID
	var rowVersion int32
	certID := uuid.New()
	ff.raw[certID] = true
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		party, err := orgSvc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการทดสอบยุติ", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		v, err := svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ทดสอบยุติ", IsProcessor: true}, 0)
		if err != nil {
			return err
		}
		vendorID = v.ID
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`UPDATE vendor.vendors SET status = 'approved', row_version = row_version + 1 WHERE id = $1 RETURNING row_version`,
			v.ID).Scan(&rowVersion)
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
	limited := uuid.New() // update only, no delete — can start offboarding but never terminate
	grants := map[string][]string{
		tenant.UserID.String(): {"vendor.vendor.read", "vendor.vendor.create", "vendor.vendor.update", "vendor.vendor.delete"},
		limited.String():       {"vendor.vendor.read", "vendor.vendor.update"},
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

	do := func(method, path string, user *uuid.UUID, body any, headers map[string]string) (int, string, http.Header) {
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
		return res.StatusCode, out.String(), res.Header
	}
	admin := tenant.UserID
	vendorPath := "/admin/v1/vendors/" + vendorID.String()

	if code, _, _ := do("GET", vendorPath+"/offboarding", &admin, nil, nil); code != 404 {
		t.Errorf("no offboarding record yet: %d, want 404", code)
	}

	code, body, hdr := do("POST", vendorPath+"/transitions", &admin, map[string]any{"to": "terminated"}, map[string]string{"If-Match": etagOf(int(rowVersion))})
	if code != 409 || !strings.Contains(body, "vendor.invalid_transition") {
		t.Errorf("terminate from approved: %d %s, want 409 vendor.invalid_transition", code, body)
	}

	code, body, hdr = do("POST", vendorPath+"/transitions", &admin, map[string]any{"to": "offboarding"}, map[string]string{"If-Match": etagOf(int(rowVersion))})
	if code != 200 || !strings.Contains(body, `"status":"offboarding"`) {
		t.Fatalf("start offboarding: %d %s", code, body)
	}
	startedEtag := hdr.Get("ETag")

	code, body, hdr = do("GET", vendorPath+"/offboarding", &admin, nil, nil)
	if code != 200 || !strings.Contains(body, `"data_return_status":"pending"`) {
		t.Fatalf("get offboarding: %d %s", code, body)
	}
	offEtag := hdr.Get("ETag")

	if code, _, _ := do("PUT", vendorPath+"/offboarding", &admin, map[string]any{"data_return_status": "pending"}, nil); code != 428 {
		t.Errorf("update offboarding no If-Match: %d, want 428", code)
	}
	code, body, hdr = do("PUT", vendorPath+"/offboarding", &admin,
		map[string]any{"data_return_status": "pending", "access_revoked": true}, map[string]string{"If-Match": offEtag})
	if code != 200 || !strings.Contains(body, `"access_revoked_at"`) {
		t.Fatalf("record access revoked: %d %s", code, body)
	}
	offEtag = hdr.Get("ETag")

	// Access revoked but no certificate yet — terminating is still refused.
	if code, body, _ := do("POST", vendorPath+"/transitions", &admin, map[string]any{"to": "terminated"}, map[string]string{"If-Match": startedEtag}); code != 409 || !strings.Contains(body, "vendor.offboarding_incomplete") {
		t.Errorf("terminate with no certificate: %d %s, want 409 vendor.offboarding_incomplete", code, body)
	}

	code, body, hdr = do("PUT", vendorPath+"/offboarding", &admin,
		map[string]any{"data_return_status": "destroyed", "destruction_certificate_file_id": certID}, map[string]string{"If-Match": offEtag})
	if code != 200 || !strings.Contains(body, `"data_return_status":"destroyed"`) {
		t.Fatalf("record certificate: %d %s", code, body)
	}

	// A caller without vendor.vendor.delete can't terminate even once everything is in order.
	if code, _, _ := do("POST", vendorPath+"/transitions", &limited, map[string]any{"to": "terminated"}, map[string]string{"If-Match": startedEtag}); code != 403 {
		t.Errorf("terminate without .delete: %d, want 403", code)
	}

	code, body, _ = do("POST", vendorPath+"/transitions", &admin, map[string]any{"to": "terminated"}, map[string]string{"If-Match": startedEtag})
	if code != 200 || !strings.Contains(body, `"status":"terminated"`) {
		t.Fatalf("terminate: %d %s", code, body)
	}

	if code, body, _ := do("GET", vendorPath+"/offboarding", &admin, nil, nil); code != 200 || !strings.Contains(body, `"completed_at"`) {
		t.Errorf("offboarding record after termination: %d %s", code, body)
	}
}
