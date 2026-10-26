package riskhttp_test

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
	"pdpa-platform/internal/platform/forms"
	riskhttp "pdpa-platform/internal/risk/http"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

func checklistScorePtr(v float64) *float64 { return &v }

// Contract test for RRA-05's checklist endpoints behind the real OpenAPI validator and AuthZ.
func TestActivityChecklistEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rra05http")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM risk.activity_checklists`, `DELETE FROM platform.form_submissions`,
				`DELETE FROM platform.form_versions`, `DELETE FROM platform.form_definitions`,
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
	riskSvc.Forms = &forms.Service{Audit: audit.New()}
	riskSvc.Forms.Register("checklist", forms.Policy{Read: "ropa.risk.read", Create: "ropa.risk.create",
		Update: "ropa.risk.approve", Publish: "ropa.risk.approve", Respond: "ropa.risk.update"})

	var activityID, formID uuid.UUID
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"ropa.risk.read", "ropa.risk.create", "ropa.risk.update", "ropa.risk.approve",
				"org.structure.read", "org.structure.update", "ropa.activity.read", "ropa.activity.create"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := orgSvc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := ropaSvc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-CHECKLISTHTTP", Name: "ทดสอบ checklist", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID

		d := forms.Draft{Languages: []string{"th", "en"}, Schema: forms.Schema{Sections: []forms.Section{{Key: "controls",
			Title: forms.Text{"th": "คำถาม", "en": "Questions"}, Questions: []forms.Question{
				{Key: "has_dpia", Type: forms.TypeYesNo, Label: forms.Text{"th": "ทำ DPIA แล้วหรือยัง", "en": "DPIA done?"}, Required: true,
					Options: []forms.Option{{Value: "yes", Score: checklistScorePtr(1)}, {Value: "no", Score: checklistScorePtr(0)}}},
			}}}},
			Scoring: &forms.Scoring{Bands: []forms.Band{{Key: "incomplete", Label: forms.Text{"th": "ยังไม่ครบ"}, Min: 0, Max: checklistScorePtr(0)},
				{Key: "complete", Label: forms.Text{"th": "ครบ"}, Min: 1}}}}
		form, err := riskSvc.Forms.CreateForm(ctx, "checklist_http_"+uuid.NewString()[:8], "Activity checklist", "checklist", d)
		if err != nil {
			return err
		}
		form, err = riskSvc.Forms.Publish(ctx, form.ID, form.LatestVersion)
		formID = form.ID
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
		tenant.UserID.String(): {"ropa.risk.read", "ropa.risk.update"},
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
	listPath := "/admin/v1/ropa/activities/" + activityID.String() + "/checklists"

	if code, _ := do("GET", listPath, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, body := do("GET", listPath, &reader, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("list before any run: %d %s", code, body)
	}
	if code, _ := do("POST", listPath, &reader, map[string]any{"form_id": formID, "answers": map[string]any{"has_dpia": "yes"}}); code != 403 {
		t.Errorf("answer with read only: %d, want 403", code)
	}
	code, body := do("POST", listPath, &admin, map[string]any{"form_id": formID, "answers": map[string]any{"has_dpia": "yes"}})
	if code != 201 || !strings.Contains(body, `"result":"complete"`) {
		t.Fatalf("answer: %d %s", code, body)
	}
	var run riskhttp.RiskActivityChecklist
	_ = json.Unmarshal([]byte(body), &run)

	if code, body := do("GET", listPath, &reader, nil); code != 200 || !strings.Contains(body, run.Id.String()) {
		t.Errorf("list after answering: %d %s", code, body)
	}
	if code, body := do("GET", "/admin/v1/risk/activity-checklists/"+run.Id.String(), &reader, nil); code != 200 || !strings.Contains(body, `"complete"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/risk/activity-checklists/"+uuid.New().String(), &reader, nil); code != 404 {
		t.Errorf("get unknown: %d, want 404", code)
	}
	if code, _ := do("POST", "/admin/v1/ropa/activities/"+uuid.New().String()+"/checklists", &admin,
		map[string]any{"form_id": formID, "answers": map[string]any{"has_dpia": "yes"}}); code != 422 {
		t.Errorf("answer unknown activity: %d, want 422", code)
	}
}
