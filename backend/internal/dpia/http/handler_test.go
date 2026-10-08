package dpiahttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
			_, _ = tx.Exec(ctx, `DELETE FROM assess.assessment_risks`)
			_, _ = tx.Exec(ctx, `DELETE FROM risk.risks`)
			_, _ = tx.Exec(ctx, `DELETE FROM risk.risk_matrices`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.dpo_opinions`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.answers`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.assessments`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.screening_rules`)
			_, _ = tx.Exec(ctx, `DELETE FROM assess.templates WHERE tenant_id IS NOT NULL`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.form_versions WHERE form_id IN (SELECT id FROM platform.form_definitions WHERE tenant_id IS NOT NULL)`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.form_definitions WHERE tenant_id IS NOT NULL`)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.processing_activities`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.org_units`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.legal_entities`)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.users WHERE email = 'limited-dpia@dbtest.example'`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	riskSvc := &riskservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskSvc}
	riskSvc.Ropa = wiring.RiskRopa{Ropa: ropaSvc, Org: orgSvc}
	formsSvc := wiring.Forms(nil, audit.New())
	svc := &dpiaservice.Service{Audit: audit.New(), Forms: formsSvc, Ropa: ropaSvc, Org: orgSvc, Risk: riskSvc}

	var activityID, legalEntityID, orgUnitID uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"org.structure.create", "ropa.activity.create"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntityID = le.ID
		unit, err := orgSvc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
		if err != nil {
			return err
		}
		orgUnitID = unit.ID
		a, err := ropaSvc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HTTP-01", Name: "กิจกรรมทดสอบ", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		_, err = riskSvc.SaveMatrix(ctx, riskservice.RiskMatrix{
			Name: "default", LikelihoodLevels: []string{"low", "medium", "high"}, ImpactLevels: []string{"low", "medium", "high"},
			Thresholds: []riskservice.Threshold{{Level: "low", MinScore: 1}, {Level: "medium", MinScore: 4}, {Level: "high", MinScore: 7}},
			IsDefault:  true,
		}, 0)
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
	limited := uuid.New() // DPIA-10: assessment.dpia.update but not .approve — a real iam.users row (below),
	// since assess.dpo_opinions.dpo_user_id is a real FK to iam.users.
	grants := map[string][]string{
		tenant.UserID.String(): {"assessment.dpia.read", "assessment.dpia.create", "assessment.dpia.update", "assessment.dpia.approve",
			"assessment.template.read", "assessment.template.update", "assessment.template.create", "assessment.template.publish", "assessment.template.delete"},
		reader.String():  {"assessment.dpia.read", "assessment.template.read"},
		limited.String(): {"assessment.dpia.read", "assessment.dpia.update"},
	}
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx,
			`INSERT INTO iam.users (id, tenant_id, email, display_name, status) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, 'Limited', 'active')`,
			limited, "limited-dpia@dbtest.example")
		return err
	}); err != nil {
		t.Fatal(err)
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

	// DPIA-04: the description composed live from the same RoPA activity.
	if code, _ := do("GET", item+"/description", nil, nil, nil); code != 401 {
		t.Errorf("description no principal: %d, want 401", code)
	}
	if code, body := do("GET", item+"/description", &reader2, nil, nil); code != 200 || !strings.Contains(body, `"activity_code":"HTTP-01"`) {
		t.Errorf("description: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/dpia/assessments/"+uuid.New().String()+"/description", &admin, nil, nil); code != 404 {
		t.Errorf("description unknown assessment: %d, want 404", code)
	}

	// DPIA-14: diff against the previous round — none yet for this first round.
	if code, _ := do("GET", item+"/diff", nil, nil, nil); code != 401 {
		t.Errorf("diff no principal: %d, want 401", code)
	}
	if code, body := do("GET", item+"/diff", &reader2, nil, nil); code != 200 || !strings.Contains(body, `"changes":null`) {
		t.Errorf("diff first round: %d %s, want empty changes", code, body)
	}
	if code, _ := do("GET", "/admin/v1/dpia/assessments/"+uuid.New().String()+"/diff", &admin, nil, nil); code != 404 {
		t.Errorf("diff unknown assessment: %d, want 404", code)
	}
	changedAnswers := map[string]any{}
	for k, v := range answers {
		changedAnswers[k] = v
	}
	changedAnswers["monitoring"] = "yes"
	code, body = do("POST", screenPath, &admin, map[string]any{"answers": changedAnswers}, nil)
	if code != 201 {
		t.Fatalf("re-screen: %d %s", code, body)
	}
	var second dpiahttp.DpiaAssessment
	_ = json.Unmarshal([]byte(body), &second)
	if code, body := do("GET", "/admin/v1/dpia/assessments/"+second.Id.String()+"/diff", &reader2, nil, nil); code != 200 ||
		!strings.Contains(body, `"question":"monitoring"`) {
		t.Errorf("diff second round: %d %s, want a change on monitoring", code, body)
	}

	// DPIA-05: the necessity/proportionality checklist on the same assessment.
	necessityAnswers := map[string]any{"minimal_data": "yes", "purpose_specific": "yes", "lawful_basis_appropriate": "yes", "less_invasive_considered": "yes"}
	if code, _ := do("GET", item+"/necessity", &reader2, nil, nil); code != 404 {
		t.Errorf("necessity before answering: %d, want 404", code)
	}
	if code, _ := do("POST", item+"/necessity", &reader2, map[string]any{"answers": necessityAnswers}, nil); code != 403 {
		t.Errorf("assess necessity with read-only permission: %d, want 403", code)
	}
	if code, body := do("POST", item+"/necessity", &admin, map[string]any{"answers": necessityAnswers}, nil); code != 200 || !strings.Contains(body, `"result":"necessary"`) {
		t.Errorf("assess necessity: %d %s", code, body)
	}
	if code, body := do("GET", item+"/necessity", &reader2, nil, nil); code != 200 || !strings.Contains(body, `"result":"necessary"`) {
		t.Errorf("get necessity: %d %s", code, body)
	}
	flagged := map[string]any{"minimal_data": "no", "purpose_specific": "yes", "lawful_basis_appropriate": "yes", "less_invasive_considered": "yes"}
	if code, body := do("POST", item+"/necessity", &admin, map[string]any{"answers": flagged}, nil); code != 200 || !strings.Contains(body, `"result":"needs_review"`) || !strings.Contains(body, `"minimal_data"`) {
		t.Errorf("re-assess necessity: %d %s", code, body)
	}

	// DPIA-06: identify and score a risk against the tenant's own matrix, while this round is still
	// in_progress (set up above, before the DPIA-10 section below moves it on to closed).
	risksPath := item + "/risks"
	if code, _ := do("GET", "/admin/v1/dpia/risk-catalog", nil, nil, nil); code != 401 {
		t.Errorf("risk catalog no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/dpia/risk-catalog", &reader2, nil, nil); code != 200 || !strings.Contains(body, `"code"`) {
		t.Errorf("risk catalog: %d %s", code, body)
	}
	if code, _ := do("POST", risksPath, &reader2, map[string]any{"title": "x", "likelihood": 1, "impact": 1}, nil); code != 403 {
		t.Errorf("identify risk with read-only permission: %d, want 403", code)
	}
	code, body = do("POST", risksPath, &admin, map[string]any{"title": "เข้าถึงข้อมูลโดยไม่ได้รับอนุญาต", "likelihood": 2, "impact": 3}, nil)
	if code != 201 || !strings.Contains(body, `"level":"medium"`) || !strings.Contains(body, `"inherent_score":6`) {
		t.Fatalf("identify risk: %d %s", code, body)
	}
	var risk dpiahttp.DpiaRisk
	_ = json.Unmarshal([]byte(body), &risk)
	riskPath := risksPath + "/" + risk.Id.String()

	if code, body := do("GET", risksPath, &reader2, nil, nil); code != 200 || !strings.Contains(body, risk.Id.String()) {
		t.Errorf("list assessment risks: %d %s", code, body)
	}
	if code, _ := do("PUT", riskPath, &admin, map[string]any{"title": "a", "likelihood": 3, "impact": 3}, nil); code != 428 {
		t.Errorf("update risk without If-Match: %d, want 428", code)
	}
	code, body = do("PUT", riskPath, &admin, map[string]any{"title": "a (revised)", "likelihood": 3, "impact": 3, "treatment": "mitigate"},
		map[string]string{"If-Match": `"1"`})
	if code != 200 || !strings.Contains(body, `"level":"high"`) || !strings.Contains(body, `"treatment":"mitigate"`) {
		t.Errorf("update risk: %d %s", code, body)
	}
	if code, _ := do("PUT", "/admin/v1/dpia/assessments/"+second.Id.String()+"/risks/"+risk.Id.String(), &admin,
		map[string]any{"title": "a", "likelihood": 1, "impact": 1}, map[string]string{"If-Match": `"2"`}); code != 404 {
		t.Errorf("update risk from a round it isn't linked to: %d, want 404", code)
	}
	if code, _ := do("DELETE", riskPath, &reader2, nil, nil); code != 403 {
		t.Errorf("remove risk with read-only permission: %d, want 403", code)
	}
	if code, _ := do("DELETE", riskPath, &admin, nil, nil); code != 204 {
		t.Errorf("remove risk: %d, want 204", code)
	}
	if code, body := do("GET", risksPath, &reader2, nil, nil); code != 200 || strings.Contains(body, risk.Id.String()) {
		t.Errorf("list assessment risks after remove: %d %s, want empty", code, body)
	}

	// DPIA-03 template library.
	draft := map[string]any{"schema": map[string]any{"sections": []map[string]any{
		{"key": "s1", "title": map[string]any{"th": "หัวข้อ"}, "questions": []map[string]any{
			{"key": "q1", "type": "yes_no", "label": map[string]any{"th": "คำถาม"}},
		}},
	}}}
	createBody := map[string]any{"assessment_type": "pia", "code": "http_tpl", "name": "HTTP template", "draft": draft}

	if code, _ := do("POST", "/admin/v1/dpia/templates", nil, createBody, nil); code != 401 {
		t.Errorf("create no principal: %d, want 401", code)
	}
	if code, _ := do("POST", "/admin/v1/dpia/templates", &reader2, createBody, nil); code != 403 {
		t.Errorf("create with read-only permission: %d, want 403", code)
	}
	code, body = do("POST", "/admin/v1/dpia/templates", &admin, createBody, nil)
	if code != 201 || !strings.Contains(body, `"status":"draft"`) {
		t.Fatalf("create template: %d %s", code, body)
	}
	var tpl dpiahttp.DpiaTemplate
	_ = json.Unmarshal([]byte(body), &tpl)

	if code, body = do("GET", "/admin/v1/dpia/templates?assessment_type=pia", &reader2, nil, nil); code != 200 || !strings.Contains(body, tpl.Id.String()) {
		t.Errorf("list by type: %d %s", code, body)
	}
	if code, body = do("GET", "/admin/v1/dpia/templates/"+tpl.Id.String(), &reader2, nil, nil); code != 200 || !strings.Contains(body, `"code":"http_tpl"`) {
		t.Errorf("get template: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/dpia/templates/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown template: %d, want 404", code)
	}

	cloneBody := map[string]any{"code": "http_tpl_clone", "name": "HTTP template clone"}
	if code, _ := do("POST", "/admin/v1/dpia/templates/"+tpl.Id.String()+"/clone", &reader2, cloneBody, nil); code != 403 {
		t.Errorf("clone with read-only permission: %d, want 403", code)
	}
	code, body = do("POST", "/admin/v1/dpia/templates/"+tpl.Id.String()+"/clone", &admin, cloneBody, nil)
	if code != 201 || !strings.Contains(body, `"code":"http_tpl_clone"`) {
		t.Fatalf("clone template: %d %s", code, body)
	}
	var clone dpiahttp.DpiaTemplate
	_ = json.Unmarshal([]byte(body), &clone)
	if clone.FormId == tpl.FormId {
		t.Errorf("clone shares the source's form id")
	}

	if code, _ := do("POST", "/admin/v1/dpia/templates/"+tpl.Id.String()+"/publish", &admin, nil, nil); code != 428 {
		t.Errorf("publish without If-Match: %d, want 428", code)
	}
	if code, _ := do("POST", "/admin/v1/dpia/templates/"+tpl.Id.String()+"/publish", &admin, nil, map[string]string{"If-Match": `"999"`}); code != 412 {
		t.Errorf("publish stale If-Match: %d, want 412", code)
	}
	if code, body = do("POST", "/admin/v1/dpia/templates/"+tpl.Id.String()+"/publish", &admin, nil, map[string]string{"If-Match": `"1"`}); code != 200 || !strings.Contains(body, `"status":"published"`) {
		t.Errorf("publish: %d %s", code, body)
	}

	if code, _ := do("POST", "/admin/v1/dpia/templates/"+clone.Id.String()+"/retire", &reader2, nil, map[string]string{"If-Match": `"1"`}); code != 403 {
		t.Errorf("retire with read-only permission: %d, want 403", code)
	}
	if code, body = do("POST", "/admin/v1/dpia/templates/"+clone.Id.String()+"/retire", &admin, nil, map[string]string{"If-Match": `"1"`}); code != 200 || !strings.Contains(body, `"status":"retired"`) {
		t.Errorf("retire: %d %s", code, body)
	}

	// DPIA-10: transition (ST-05#2) + DPO opinions, on the first screening round ("created", still in_progress).
	transitionPath := item + "/transition"
	opinionsPath := item + "/opinions"
	if code, _ := do("POST", transitionPath, &admin, map[string]any{"to": "approved"}, map[string]string{"If-Match": `"1"`}); code != 409 {
		t.Errorf("in_progress -> approved (skips in_review): %d, want 409", code)
	}
	if code, _ := do("POST", transitionPath, &admin, map[string]any{"to": "in_review"}, nil); code != 428 {
		t.Errorf("transition without If-Match: %d, want 428", code)
	}
	code, body = do("POST", transitionPath, &limited, map[string]any{"to": "in_review"}, map[string]string{"If-Match": `"1"`})
	if code != 200 || !strings.Contains(body, `"status":"in_review"`) {
		t.Fatalf("submit for review (update only, no approve needed): %d %s", code, body)
	}

	if code, _ := do("GET", opinionsPath, &reader2, nil, nil); code != 200 {
		t.Errorf("list opinions (none yet): %d", code)
	}
	if code, _ := do("POST", opinionsPath, &reader2, map[string]any{"opinion": "x", "recommendation": "proceed"}, nil); code != 403 {
		t.Errorf("record opinion with read-only permission: %d, want 403", code)
	}
	code, body = do("POST", opinionsPath, &limited, map[string]any{"opinion": "เห็นควรดำเนินการ", "recommendation": "proceed"}, nil)
	if code != 201 || !strings.Contains(body, `"recommendation":"proceed"`) {
		t.Fatalf("record opinion: %d %s", code, body)
	}
	if code, body := do("GET", opinionsPath, &reader2, nil, nil); code != 200 || !strings.Contains(body, `"opinion":"เห็นควรดำเนินการ"`) {
		t.Errorf("list opinions: %d %s", code, body)
	}

	if code, _ := do("POST", transitionPath, &limited, map[string]any{"to": "approved"}, map[string]string{"If-Match": `"2"`}); code != 403 {
		t.Errorf("decide without .approve: %d, want 403", code)
	}
	code, body = do("POST", transitionPath, &admin, map[string]any{"to": "approved"}, map[string]string{"If-Match": `"2"`})
	if code != 200 || !strings.Contains(body, `"status":"approved"`) {
		t.Fatalf("decide approved: %d %s", code, body)
	}
	if code, _ := do("POST", transitionPath, &admin, map[string]any{"to": "closed"}, map[string]string{"If-Match": `"3"`}); code != 200 {
		t.Errorf("close with an opinion on record: %d, want 200", code)
	}

	// DPIA-12: the registry report — one row per activity, always the latest round. round 1 ("created") was
	// transitioned to closed above, but round 2 ("second", from the DPIA-14 re-screen) is the activity's
	// actual latest round and was never transitioned past in_progress — the registry must show that live
	// current state, not round 1's stale closed status (the acceptance criterion: status matches reality).
	if code, _ := do("GET", "/admin/v1/dpia/registry", nil, nil, nil); code != 401 {
		t.Errorf("registry no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/dpia/registry", &reader2, nil, nil); code != 200 ||
		!strings.Contains(body, `"activity_code":"HTTP-01"`) || !strings.Contains(body, `"round_no":2`) || !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("registry: %d %s", code, body)
	}
	if code, body := do("GET", "/admin/v1/dpia/registry?org_unit_id="+orgUnitID.String(), &reader2, nil, nil); code != 200 || !strings.Contains(body, activityID.String()) {
		t.Errorf("registry org_unit filter: %d %s", code, body)
	}
	if code, body := do("GET", "/admin/v1/dpia/registry?legal_entity_id="+legalEntityID.String(), &reader2, nil, nil); code != 200 || !strings.Contains(body, activityID.String()) {
		t.Errorf("registry legal_entity filter: %d %s", code, body)
	}
	if code, body := do("GET", "/admin/v1/dpia/registry?org_unit_id="+uuid.New().String(), &reader2, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("registry unknown org_unit filter: %d %s, want empty data", code, body)
	}

	// DPIA-15: the PDF/Word report — language/format both required by the schema, the body is binary
	// (checked via a raw request so the Content-Disposition/Content-Type headers are visible, unlike the
	// plain do() helper above which only returns status + body text).
	reportPath := item + "/report"
	if code, _ := do("GET", reportPath+"?language=th&format=docx", nil, nil, nil); code != 401 {
		t.Errorf("report no principal: %d, want 401", code)
	}
	if code, _ := do("GET", reportPath+"?language=th&format=docx", &reader2, nil, nil); code != 200 {
		t.Errorf("report (read-only permission is enough): %d, want 200", code)
	}
	if code, _ := do("GET", "/admin/v1/dpia/assessments/"+uuid.New().String()+"/report?language=th&format=docx", &admin, nil, nil); code != 404 {
		t.Errorf("report unknown assessment: %d, want 404", code)
	}
	req, _ := http.NewRequest("GET", srv.URL+reportPath+"?language=en&format=docx", nil)
	req.Header.Set("X-Test-User", reader2.String())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/vnd.openxmlformats") ||
		!strings.Contains(res.Header.Get("Content-Disposition"), "attachment") || !bytes.HasPrefix(raw, []byte("PK")) {
		t.Errorf("report docx: %d %v %q", res.StatusCode, res.Header, raw[:min(len(raw), 40)])
	}
}
