package dpiahttp_test

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

	dpiahttp "pdpa-platform/internal/dpia/http"
	dpiaservice "pdpa-platform/internal/dpia/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the dpia admin endpoints (DPIA-01/02) behind the real OpenAPI validator and AuthZ.
func TestDpiaEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "dpiahttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.answers`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.assessments`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.screening_rules`)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.processing_activities`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.org_units`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.legal_entities`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskservice.New()}
	formsSvc := wiring.Forms(nil, audit.New())
	svc := &dpiaservice.Service{Audit: audit.New(), Forms: formsSvc, Ropa: ropaSvc}

	var activityID uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"org.structure.create", "ropa.activity.create"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := orgSvc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := ropaSvc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HTTP-01", Name: "กิจกรรมทดสอบ", Role: "controller"}, 0)
		activityID = a.ID
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
		tenant.UserID.String(): {"assessment.dpia.read", "assessment.dpia.create", "assessment.template.read", "assessment.template.update"},
		reader.String():        {"assessment.dpia.read", "assessment.template.read"},
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
	strict := dpiahttp.NewStrictHandlerWithOptions(dpiahttp.NewStrict(svc),
		[]dpiahttp.StrictMiddlewareFunc{authz.StrictMiddleware[dpiahttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		dpiahttp.StrictHTTPServerOptions{
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
	dpiahttp.HandlerWithOptions(strict, dpiahttp.ChiServerOptions{BaseRouter: r})
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
	admin, reader2 := tenant.UserID, reader
	answers := map[string]any{"sensitive_data": "yes", "large_scale": "yes", "monitoring": "no",
		"automated_decision": "no", "new_tech": "no", "vulnerable_groups": "no"}

	if code, _ := do("GET", "/admin/v1/dpia/screening-rules", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/dpia/screening-rules", &admin, nil, nil); code != 200 || !strings.Contains(body, `"min_factors":2`) {
		t.Errorf("default rules: %d %s", code, body)
	}
	if code, _ := do("PUT", "/admin/v1/dpia/screening-rules", &reader2, map[string]any{"min_factors": 1}, nil); code != 403 {
		t.Errorf("save rules with read-only permission: %d, want 403", code)
	}
	if code, _ := do("PUT", "/admin/v1/dpia/screening-rules", &admin, map[string]any{"min_factors": 0}, nil); code != 400 {
		t.Errorf("min_factors=0: %d, want 400 (schema minimum)", code)
	}
	if code, body := do("PUT", "/admin/v1/dpia/screening-rules", &admin, map[string]any{"min_factors": 1}, nil); code != 200 || !strings.Contains(body, `"min_factors":1`) {
		t.Errorf("save rules: %d %s", code, body)
	}

	screenPath := "/admin/v1/dpia/activities/" + activityID.String() + "/screen"
	if code, _ := do("POST", screenPath, &reader2, map[string]any{"answers": answers}, nil); code != 403 {
		t.Errorf("screen with read-only permission: %d, want 403", code)
	}
	if code, body := do("POST", screenPath, &admin, map[string]any{"answers": map[string]any{"sensitive_data": "yes"}}, nil); code != 422 {
		t.Errorf("incomplete answers: %d %s, want 422", code, body)
	}
	code, body := do("POST", screenPath, &admin, map[string]any{"answers": answers}, nil)
	if code != 201 || !strings.Contains(body, `"screening_result":"required"`) {
		t.Fatalf("screen: %d %s", code, body)
	}
	var created dpiahttp.DpiaAssessment
	_ = json.Unmarshal([]byte(body), &created)

	item := "/admin/v1/dpia/assessments/" + created.Id.String()
	if code, body := do("GET", item, &reader2, nil, nil); code != 200 || !strings.Contains(body, `"round_no":1`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/dpia/assessments/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown assessment: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/dpia/assessments?activity_id="+activityID.String(), &reader2, nil, nil); code != 200 || !strings.Contains(body, created.Id.String()) {
		t.Errorf("list: %d %s", code, body)
	}
	if code, _ := do("POST", "/admin/v1/dpia/activities/"+uuid.New().String()+"/screen", &admin, map[string]any{"answers": answers}, nil); code != 422 {
		t.Errorf("unknown activity: %d, want 422", code)
	}
}
