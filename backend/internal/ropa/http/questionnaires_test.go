package ropahttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	templatesservice "pdpa-platform/internal/ropa/templates"

	ropahttp "pdpa-platform/internal/ropa/http"
)

// TestQuestionnaireEndpoints_Contract is ROPA-11's own acceptance criterion through the real validator +
// AuthZ chain: send -> answer -> approve, and the approved round carries the newly created activity_id.
func TestQuestionnaireEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "ropaqhttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM ropa.questionnaires`, `DELETE FROM platform.form_submissions`,
				`DELETE FROM platform.form_versions`, `DELETE FROM platform.form_definitions`,
				`DELETE FROM ropa.activity_purposes`, `DELETE FROM ropa.processing_activities`,
				`DELETE FROM org.org_units`, `DELETE FROM org.legal_entities`,
				`DELETE FROM iam.users WHERE id != $1`,
			} {
				if strings.Contains(q, "$1") {
					_, _ = tx.Exec(ctx, q, tenant.UserID)
					continue
				}
				_, _ = tx.Exec(ctx, q)
			}
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	formsSvc := &forms.Service{Audit: audit.New()}
	formsSvc.Register("questionnaire", forms.Policy{Read: "ropa.template.read", Create: "ropa.template.create",
		Update: "ropa.template.update", Publish: "ropa.template.publish", Respond: "ropa.activity.update"})
	svc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskservice.New(), Templates: templatesservice.New(), Forms: formsSvc}

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
	dpo, respondent, reader := tenant.UserID, uuid.New(), uuid.New()
	grants := map[string][]string{
		dpo.String():        {"ropa.activity.read", "ropa.activity.create", "ropa.activity.update", "ropa.activity.approve", "ropa.template.read", "ropa.template.create", "ropa.template.update", "ropa.template.publish"},
		respondent.String(): {"ropa.activity.read", "ropa.activity.update"},
		reader.String():     {"ropa.activity.read"},
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
	strict := ropahttp.NewStrictHandlerWithOptions(ropahttp.NewStrict(svc, templatesservice.New()),
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

	var legalEntityID, orgUnitID uuid.UUID
	var formVersionID uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: grants[dpo.String()]})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntityID = le.ID
		unit, err := orgSvc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		orgUnitID = unit.ID
		if _, err := pdb.MustTxFromContext(ctx).Exec(ctx,
			`INSERT INTO iam.users (id, tenant_id, email, display_name, status) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, 'Respondent', 'active')`,
			respondent, respondent.String()+"@dbtest.example"); err != nil {
			return err
		}
		d := forms.Draft{Languages: []string{"th"}, Schema: forms.Schema{Sections: []forms.Section{{Key: "mapping",
			Title: forms.Text{"th": "ข้อมูล"}, Questions: []forms.Question{
				{Key: "activity_name", Type: forms.TypeText, Label: forms.Text{"th": "ชื่อกิจกรรม"}, Required: true},
			}}}}}
		form, err := formsSvc.CreateForm(ctx, "ropa_questionnaire_http", "Data mapping questionnaire", "questionnaire", d)
		if err != nil {
			return err
		}
		form, err = formsSvc.Publish(ctx, form.ID, form.LatestVersion)
		if err != nil {
			return err
		}
		formVersionID = form.Versions[len(form.Versions)-1].ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// 401 with no principal.
	if code, _ := do("GET", "/admin/v1/ropa/questionnaires", nil, nil, nil); code != 401 {
		t.Errorf("list, no principal: %d, want 401", code)
	}

	// Send (DPO only — ropa.activity.create).
	sendBody := map[string]any{"org_unit_id": orgUnitID, "respondent_user_id": respondent}
	code, body := do("POST", "/admin/v1/ropa/questionnaires", &dpo, sendBody, nil)
	if code != 201 {
		t.Fatalf("send: %d %s", code, body)
	}
	var sent struct {
		ID         uuid.UUID `json:"id"`
		Status     string    `json:"status"`
		RowVersion int       `json:"row_version"`
	}
	_ = json.Unmarshal([]byte(body), &sent)
	if sent.Status != "sent" {
		t.Errorf("expected status sent, got %+v", sent)
	}

	// A plain reader can't send one.
	if code, _ := do("POST", "/admin/v1/ropa/questionnaires", &reader, sendBody, nil); code != 403 {
		t.Errorf("send without ropa.activity.create: %d, want 403", code)
	}

	// Answer, as the respondent.
	answerBody := map[string]any{"form_version_id": formVersionID, "answers": map[string]any{"activity_name": "ประกาศรับสมัครงาน"}}
	code, body = do("POST", "/admin/v1/ropa/questionnaires/"+sent.ID.String()+"/answer", &respondent, answerBody, nil)
	if code != 200 {
		t.Fatalf("answer: %d %s", code, body)
	}
	var answered struct {
		Status     string `json:"status"`
		RowVersion int    `json:"row_version"`
	}
	_ = json.Unmarshal([]byte(body), &answered)
	if answered.Status != "answered" {
		t.Errorf("expected status answered, got %+v", answered)
	}

	// Approve, as the DPO — 428 missing If-Match, then the real approve carrying activity_id.
	approveBody := map[string]any{"legal_entity_id": legalEntityID, "code": "HR-Q-HTTP-01"}
	if code, _ := do("POST", "/admin/v1/ropa/questionnaires/"+sent.ID.String()+"/approve", &dpo, approveBody, nil); code != 428 {
		t.Errorf("approve without If-Match: %d, want 428", code)
	}
	code, body = do("POST", "/admin/v1/ropa/questionnaires/"+sent.ID.String()+"/approve", &dpo, approveBody,
		map[string]string{"If-Match": `"` + strconv.Itoa(answered.RowVersion) + `"`})
	if code != 200 {
		t.Fatalf("approve: %d %s", code, body)
	}
	var approved struct {
		Status     string     `json:"status"`
		ActivityID *uuid.UUID `json:"activity_id"`
	}
	_ = json.Unmarshal([]byte(body), &approved)
	if approved.Status != "approved" || approved.ActivityID == nil {
		t.Errorf("expected status approved with an activity_id, got %+v", approved)
	}
}
