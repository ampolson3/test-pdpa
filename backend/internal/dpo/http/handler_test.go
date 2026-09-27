package dpohttp_test

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

	dpohttp "pdpa-platform/internal/dpo/http"
	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/wiring"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the DPO appointment endpoints (DPO-01) behind the real OpenAPI validator and AuthZ.
func TestAppointmentEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "dpohttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM dpo.security_assessments`)
			_, _ = tx.Exec(ctx, `DELETE FROM dpo.tasks`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.form_submissions`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.form_versions`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.form_definitions`)
			_, _ = tx.Exec(ctx, `DELETE FROM dpo.appointments`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.legal_entities`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	formsSvc := wiring.Forms(nil, audit.New())
	svc := &dposervice.Service{Audit: audit.New(), Org: orgSvc, Forms: formsSvc}

	var legalEntity, formID uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"org.structure.create", "dpo.risk.read", "dpo.risk.create", "dpo.risk.approve"}})
		e, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		legalEntity = e.ID
		if err != nil {
			return err
		}
		score := func(v float64) *float64 { return &v }
		d := forms.Draft{Languages: []string{"th"}, Schema: forms.Schema{Sections: []forms.Section{{Key: "controls", Title: forms.Text{"th": "s"},
			Questions: []forms.Question{{Key: "access_control", Type: forms.TypeYesNo, Label: forms.Text{"th": "q"}, Required: true,
				Options: []forms.Option{{Value: "yes", Score: score(1)}, {Value: "no", Score: score(0)}}}}}}},
			Scoring: &forms.Scoring{Bands: []forms.Band{{Key: "fail", Label: forms.Text{"th": "ไม่ผ่าน"}, Min: 0, Max: score(0)},
				{Key: "pass", Label: forms.Text{"th": "ผ่าน"}, Min: 1}}}}
		form, err := formsSvc.CreateForm(ctx, "sec_http_"+uuid.NewString()[:8], "checklist", "security", d)
		if err != nil {
			return err
		}
		form, err = formsSvc.Publish(ctx, form.ID, form.LatestVersion)
		formID = form.ID
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
	other := uuid.New()
	grants := map[string][]string{
		tenant.UserID.String(): {"dpo.profile.read", "dpo.profile.create", "dpo.profile.update", "dpo.risk.read", "dpo.risk.create"},
		other.String():         {"dpo.profile.read", "dpo.risk.read"},
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
	strict := dpohttp.NewStrictHandlerWithOptions(dpohttp.NewStrict(svc),
		[]dpohttp.StrictMiddlewareFunc{authz.StrictMiddleware[dpohttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		dpohttp.StrictHTTPServerOptions{
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
	dpohttp.HandlerWithOptions(strict, dpohttp.ChiServerOptions{BaseRouter: r})
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
	admin, viewer := tenant.UserID, other
	appt := map[string]any{"legal_entity_id": legalEntity, "dpo_type": "external", "external_name": "สมชาย ใจดี",
		"contact_email": "dpo@example.com", "appointed_at": "2026-01-01"}

	if code, _ := do("GET", "/admin/v1/dpo/appointments", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, _ := do("POST", "/admin/v1/dpo/appointments", &viewer, appt, nil); code != 403 {
		t.Errorf("create with read permission only: %d, want 403", code)
	}
	if code, _ := do("POST", "/admin/v1/dpo/appointments", &admin, map[string]any{"legal_entity_id": legalEntity, "dpo_type": "bogus",
		"contact_email": "x@example.com", "appointed_at": "2026-01-01"}, nil); code != 400 {
		t.Errorf("bad dpo_type: %d, want 400 (schema)", code)
	}
	if code, body := do("POST", "/admin/v1/dpo/appointments", &admin, map[string]any{"legal_entity_id": uuid.New(), "dpo_type": "external",
		"external_name": "x", "contact_email": "x@example.com", "appointed_at": "2026-01-01"}, nil); code != 422 || !strings.Contains(body, "dpo.invalid_input") {
		t.Errorf("unknown legal entity: %d %s, want 422", code, body)
	}
	code, body := do("POST", "/admin/v1/dpo/appointments", &admin, appt, nil)
	if code != 201 || !strings.Contains(body, `"dpo_type":"external"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created dpohttp.DpoAppointment
	_ = json.Unmarshal([]byte(body), &created)
	item := "/admin/v1/dpo/appointments/" + created.Id.String()
	if code, _ := do("PATCH", item, &admin, appt, nil); code != 428 {
		t.Errorf("update without If-Match: %d, want 428", code)
	}
	if code, _ := do("PATCH", item, &admin, appt, map[string]string{"If-Match": `"9"`}); code != 412 {
		t.Errorf("stale If-Match: %d, want 412", code)
	}
	if code, body := do("GET", item, &viewer, nil, nil); code != 200 || !strings.Contains(body, `"contact_email":"dpo@example.com"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/dpo/appointments/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown appointment: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/dpo/appointments?legal_entity_id="+legalEntity.String(), &viewer, nil, nil); code != 200 || !strings.Contains(body, created.Id.String()) {
		t.Errorf("list: %d %s", code, body)
	}

	// DPO-09 security assessments.
	if code, _ := do("GET", "/admin/v1/dpo/security-assessments", nil, nil, nil); code != 401 {
		t.Errorf("assessments, no principal: %d, want 401", code)
	}
	assess := map[string]any{"legal_entity_id": legalEntity, "form_id": formID, "answers": map[string]any{"access_control": "no"}}
	if code, _ := do("POST", "/admin/v1/dpo/security-assessments", &viewer, assess, nil); code != 403 {
		t.Errorf("record with read permission only: %d, want 403", code)
	}
	code, body = do("POST", "/admin/v1/dpo/security-assessments", &admin, assess, nil)
	if code != 201 || !strings.Contains(body, `"result":"fail"`) || !strings.Contains(body, `"tasks"`) {
		t.Fatalf("record: %d %s", code, body)
	}
	var recorded dpohttp.DpoSecurityAssessment
	_ = json.Unmarshal([]byte(body), &recorded)
	if code, body := do("GET", "/admin/v1/dpo/security-assessments/"+recorded.Id.String(), &viewer, nil, nil); code != 200 || !strings.Contains(body, `"result":"fail"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/dpo/security-assessments/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown assessment: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/dpo/security-assessments?legal_entity_id="+legalEntity.String(), &viewer, nil, nil); code != 200 || !strings.Contains(body, recorded.Id.String()) {
		t.Errorf("list: %d %s", code, body)
	}
	if code, body := do("POST", "/admin/v1/dpo/security-assessments", &admin, map[string]any{"legal_entity_id": uuid.New(), "form_id": formID,
		"answers": map[string]any{"access_control": "yes"}}, nil); code != 422 || !strings.Contains(body, "dpo.invalid_input") {
		t.Errorf("unknown legal entity: %d %s, want 422", code, body)
	}
	if code, body := do("POST", "/admin/v1/dpo/security-assessments", &admin, map[string]any{"legal_entity_id": legalEntity, "form_id": uuid.New(),
		"answers": map[string]any{"access_control": "yes"}}, nil); code != 422 || !strings.Contains(body, "dpo.bad_form") {
		t.Errorf("unknown form: %d %s, want 422", code, body)
	}
}
