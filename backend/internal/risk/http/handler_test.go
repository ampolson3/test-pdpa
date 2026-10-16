package riskhttp_test

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

	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	riskhttp "pdpa-platform/internal/risk/http"
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

// Contract test for the risk matrix endpoints (RRA-02) behind the real OpenAPI validator and AuthZ.
func TestRiskMatrixEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "riskhttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM risk.risk_matrices`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	svc := &riskservice.Service{Audit: audit.New()}

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
		tenant.UserID.String(): {"ropa.risk.read", "ropa.risk.create", "ropa.risk.update", "ropa.risk.delete"},
		reader.String():        {"ropa.risk.read"},
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
	strict := riskhttp.NewStrictHandlerWithOptions(riskhttp.NewStrict(svc),
		[]riskhttp.StrictMiddlewareFunc{authz.StrictMiddleware[riskhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		riskhttp.StrictHTTPServerOptions{
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
	riskhttp.HandlerWithOptions(strict, riskhttp.ChiServerOptions{BaseRouter: r})
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

	if code, _ := do("GET", "/admin/v1/risk/matrices", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/risk/matrices", &reader, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("list (none yet): %d %s", code, body)
	}
	newMatrix := map[string]any{
		"name": "3x3", "likelihood_levels": []string{"low", "medium", "high"}, "impact_levels": []string{"low", "medium", "high"},
		"thresholds": []map[string]any{{"level": "low", "min_score": 1}, {"level": "medium", "min_score": 4}, {"level": "high", "min_score": 7}},
	}
	if code, _ := do("POST", "/admin/v1/risk/matrices", &reader, newMatrix, nil); code != 403 {
		t.Errorf("create with read only: %d, want 403", code)
	}
	if code, body := do("POST", "/admin/v1/risk/matrices", &admin, map[string]any{"name": "x"}, nil); code != 400 {
		t.Errorf("missing required fields: %d %s, want 400 (schema)", code, body)
	}
	if code, body := do("POST", "/admin/v1/risk/matrices", &admin, map[string]any{
		"name": "bad", "likelihood_levels": []string{"a", "b"}, "impact_levels": []string{"a", "b"},
		"thresholds": []map[string]any{{"level": "high", "min_score": 2}}, // no threshold covers score 1
	}, nil); code != 422 {
		t.Errorf("thresholds not covering every score: %d %s, want 422", code, body)
	}
	code, body := do("POST", "/admin/v1/risk/matrices", &admin, newMatrix, nil)
	if code != 201 || !strings.Contains(body, `"name":"3x3"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created riskhttp.RiskMatrix
	_ = json.Unmarshal([]byte(body), &created)
	item := "/admin/v1/risk/matrices/" + created.Id.String()

	if code, body := do("GET", item, &reader, nil, nil); code != 200 || !strings.Contains(body, `"thresholds"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/risk/matrices/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown matrix: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/risk/matrices", &reader, nil, nil); code != 200 || !strings.Contains(body, created.Id.String()) {
		t.Errorf("list: %d %s", code, body)
	}

	update := item
	if code, _ := do("PUT", update, &admin, newMatrix, nil); code != 428 {
		t.Errorf("update, no If-Match: %d, want 428", code)
	}
	if code, _ := do("PUT", update, &admin, newMatrix, map[string]string{"If-Match": `"99"`}); code != 412 {
		t.Errorf("update, stale version: %d, want 412", code)
	}
	if code, body := do("PUT", update, &reader, newMatrix, map[string]string{"If-Match": etagOf(int(created.RowVersion))}); code != 403 {
		t.Errorf("update without update permission: %d %s, want 403", code, body)
	}
	renamed := map[string]any{}
	for k, v := range newMatrix {
		renamed[k] = v
	}
	renamed["name"] = "renamed"
	code, body = do("PUT", update, &admin, renamed, map[string]string{"If-Match": etagOf(int(created.RowVersion))})
	if code != 200 || !strings.Contains(body, `"name":"renamed"`) {
		t.Fatalf("update: %d %s", code, body)
	}
	var updated riskhttp.RiskMatrix
	_ = json.Unmarshal([]byte(body), &updated)

	if code, _ := do("DELETE", item, &reader, nil, map[string]string{"If-Match": etagOf(int(updated.RowVersion))}); code != 403 {
		t.Errorf("delete without delete permission: %d, want 403", code)
	}
	if code, _ := do("DELETE", item, &admin, nil, map[string]string{"If-Match": `"99"`}); code != 412 {
		t.Errorf("delete, stale version: %d, want 412", code)
	}
	if code, _ := do("DELETE", item, &admin, nil, map[string]string{"If-Match": etagOf(int(updated.RowVersion))}); code != 204 {
		t.Errorf("delete: %d, want 204", code)
	}
	if code, _ := do("GET", item, &admin, nil, nil); code != 404 {
		t.Errorf("get after delete: %d, want 404", code)
	}
}

func etagOf(v int) string { return `"` + itoa(v) + `"` }

func itoa(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Contract test for the activity risk-score endpoints (RRA-01) behind the real OpenAPI validator and AuthZ.
func TestActivityRiskScoreEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "riskscorehttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM risk.activity_scores`, `DELETE FROM risk.risk_matrices`,
				`DELETE FROM ropa.processing_activities`, `DELETE FROM org.org_units`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	riskSvc := &riskservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskSvc}
	riskSvc.Ropa = wiring.RiskRopa{Ropa: ropaSvc, Org: orgSvc}

	var activityID uuid.UUID
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"ropa.risk.read", "ropa.risk.create", "ropa.risk.update", "org.structure.read", "org.structure.update", "ropa.activity.read", "ropa.activity.create"}})
		if _, err := riskSvc.SaveMatrix(ctx, riskservice.RiskMatrix{
			Name: "default", LikelihoodLevels: []string{"low", "medium", "high"}, ImpactLevels: []string{"low", "medium", "high"},
			Thresholds: []riskservice.Threshold{{Level: "low", MinScore: 1}, {Level: "medium", MinScore: 4}, {Level: "high", MinScore: 7}},
			IsDefault:  true,
		}, 0); err != nil {
			return err
		}
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := orgSvc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := ropaSvc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-SCOREHTTP", Name: "ทดสอบคะแนนความเสี่ยง", Role: "controller"}, 0)
		activityID = a.ID
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
		tenant.UserID.String(): {"ropa.risk.read", "ropa.risk.create"},
		reader.String():        {"ropa.risk.read"},
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
	strict := riskhttp.NewStrictHandlerWithOptions(riskhttp.NewStrict(riskSvc),
		[]riskhttp.StrictMiddlewareFunc{authz.StrictMiddleware[riskhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		riskhttp.StrictHTTPServerOptions{
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
	riskhttp.HandlerWithOptions(strict, riskhttp.ChiServerOptions{BaseRouter: r})
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
	scorePath := "/admin/v1/ropa/activities/" + activityID.String() + "/risk-score"

	if code, _ := do("GET", scorePath, nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, _ := do("GET", scorePath, &reader, nil, nil); code != 404 {
		t.Errorf("get before any score: %d, want 404", code)
	}
	if code, _ := do("POST", scorePath, &reader, nil, nil); code != 403 {
		t.Errorf("score with read only: %d, want 403", code)
	}
	code, body := do("POST", scorePath, &admin, nil, nil)
	if code != 201 || !strings.Contains(body, `"factors"`) {
		t.Fatalf("score: %d %s", code, body)
	}
	if code, body := do("GET", scorePath, &reader, nil, nil); code != 200 || !strings.Contains(body, `"level"`) {
		t.Errorf("get after scoring: %d %s", code, body)
	}
	if code, _ := do("POST", "/admin/v1/ropa/activities/"+uuid.New().String()+"/risk-score", &admin, nil, nil); code != 404 {
		t.Errorf("score unknown activity: %d, want 404", code)
	}
}

// Contract test for the gap-analysis endpoints (RRA-04) behind the real OpenAPI validator and AuthZ.
func TestGapAnalysisEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "riskgaphttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM risk.gap_findings`, `DELETE FROM ropa.processing_activities`, `DELETE FROM org.org_units`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	riskSvc := &riskservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskSvc}
	riskSvc.Ropa = wiring.RiskRopa{Ropa: ropaSvc, Org: orgSvc}

	var activityID uuid.UUID
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"org.structure.read", "org.structure.update", "ropa.activity.create"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := orgSvc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := ropaSvc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-GAPHTTP", Name: "ทดสอบช่องว่าง", Role: "controller"}, 0)
		activityID = a.ID
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
		tenant.UserID.String(): {"ropa.risk.read", "ropa.risk.create"},
		reader.String():        {"ropa.risk.read"},
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
	strict := riskhttp.NewStrictHandlerWithOptions(riskhttp.NewStrict(riskSvc),
		[]riskhttp.StrictMiddlewareFunc{authz.StrictMiddleware[riskhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		riskhttp.StrictHTTPServerOptions{
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
	riskhttp.HandlerWithOptions(strict, riskhttp.ChiServerOptions{BaseRouter: r})
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
	analyzePath := "/admin/v1/ropa/activities/" + activityID.String() + "/gap-analysis"
	findingsPath := "/admin/v1/ropa/activities/" + activityID.String() + "/gap-findings"

	if code, _ := do("GET", "/admin/v1/risk/gap-rules", nil, nil, nil); code != 401 {
		t.Errorf("gap-rules, no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/risk/gap-rules", &reader, nil, nil); code != 200 || !strings.Contains(body, `"no_lawful_basis"`) {
		t.Errorf("gap-rules: %d %s", code, body)
	}
	if code, _ := do("POST", analyzePath, &reader, nil, nil); code != 403 {
		t.Errorf("analyze with read only: %d, want 403", code)
	}
	code, body := do("POST", analyzePath, &admin, nil, nil)
	if code != 200 || !strings.Contains(body, `"no_lawful_basis"`) {
		t.Fatalf("analyze (no purpose set, should find no_lawful_basis): %d %s", code, body)
	}
	if code, body := do("GET", findingsPath, &reader, nil, nil); code != 200 || !strings.Contains(body, `"status":"open"`) {
		t.Errorf("activity findings: %d %s", code, body)
	}
	if code, body := do("GET", "/admin/v1/risk/gap-findings", &reader, nil, nil); code != 200 || !strings.Contains(body, activityID.String()) {
		t.Errorf("tenant-wide open findings: %d %s", code, body)
	}
	if code, _ := do("POST", "/admin/v1/ropa/activities/"+uuid.New().String()+"/gap-analysis", &admin, nil, nil); code != 404 {
		t.Errorf("analyze unknown activity: %d, want 404", code)
	}
}

// TestRemediateGapFindingEndpoint_Contract is RRA-07's own HTTP contract test: remediating an open finding
// through the real validator/AuthZ chain opens a real linked task.
func TestRemediateGapFindingEndpoint_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "riskremediatehttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM risk.gap_findings`, `DELETE FROM dpo.tasks`, `DELETE FROM ropa.processing_activities`,
				`DELETE FROM org.org_units`, `UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
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
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskSvc}
	riskSvc.Ropa = wiring.RiskRopa{Ropa: ropaSvc, Org: orgSvc}
	riskSvc.Dpo = &dposervice.Service{Audit: audit.New()} // RRA-07: opens the linked remediation task

	var activityID, findingID uuid.UUID
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"org.structure.read", "org.structure.update", "ropa.activity.create", "ropa.risk.create"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := orgSvc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := ropaSvc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-REMEDIATEHTTP", Name: "ทดสอบแก้ไขช่องว่าง", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		findings, err := riskSvc.AnalyzeActivity(ctx, activityID)
		if err != nil {
			return err
		}
		for _, f := range findings {
			if f.RuleCode == "no_lawful_basis" {
				findingID = f.ID
			}
		}
		return nil
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
		tenant.UserID.String(): {"ropa.risk.read", "ropa.risk.create"},
		reader.String():        {"ropa.risk.read"},
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
	strict := riskhttp.NewStrictHandlerWithOptions(riskhttp.NewStrict(riskSvc),
		[]riskhttp.StrictMiddlewareFunc{authz.StrictMiddleware[riskhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		riskhttp.StrictHTTPServerOptions{
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
	riskhttp.HandlerWithOptions(strict, riskhttp.ChiServerOptions{BaseRouter: r})
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
	remediatePath := "/admin/v1/risk/gap-findings/" + findingID.String() + "/remediate"

	if code, _ := do("POST", remediatePath, &reader, map[string]string{"priority": "high"}); code != 403 {
		t.Errorf("remediate with read only: %d, want 403", code)
	}
	code, body := do("POST", remediatePath, &admin, map[string]string{"priority": "high"})
	if code != 200 || !strings.Contains(body, `"task_id"`) {
		t.Fatalf("remediate: %d %s", code, body)
	}
	if code, _ := do("POST", "/admin/v1/risk/gap-findings/"+uuid.New().String()+"/remediate", &admin, map[string]string{"priority": "high"}); code != 404 {
		t.Errorf("remediate unknown finding: %d, want 404", code)
	}
}
