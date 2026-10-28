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
	"pdpa-platform/internal/platform/forms"
	ropahttp "pdpa-platform/internal/ropa/http"
	ropaservice "pdpa-platform/internal/ropa/service"
	templatesservice "pdpa-platform/internal/ropa/templates"
)

// TestSmeExemptionEndpoints_Contract is ROPA-20's own HTTP contract test — kept as a separate,
// self-contained fixture (the same move RTG-08's own test already made) rather than extending
// TestAssetEndpoints_Contract, whose own unrelated assertion is pre-existing and already failing.
func TestSmeExemptionEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "ropa20http")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM ropa.sme_exemption_checks`, `DELETE FROM platform.form_submissions`,
				`DELETE FROM platform.form_versions`, `DELETE FROM platform.form_definitions`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})

	orgSvc := &orgservice.Service{Audit: audit.New()}
	formsSvc := &forms.Service{Audit: audit.New()}
	formsSvc.Register("sme_exemption", forms.Policy{Read: "ropa.activity.read", Create: "ropa.activity.create",
		Update: "ropa.activity.update", Publish: "ropa.activity.approve", Respond: "ropa.activity.create"})
	svc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Forms: formsSvc}
	templates := templatesservice.New()

	var legalEntityID uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"ropa.activity.read", "ropa.activity.create", "ropa.activity.update", "ropa.activity.approve"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntityID = le.ID
		yn := []forms.Option{{Value: "yes"}, {Value: "no"}}
		d := forms.Draft{Languages: []string{"th", "en"}, Schema: forms.Schema{Sections: []forms.Section{
			{Key: "controller", Title: forms.Text{"th": "ผู้ควบคุม"}, Questions: []forms.Question{
				{Key: "is_sme_or_community", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
				{Key: "not_large_scale", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
				{Key: "no_sensitive_data", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
				{Key: "no_high_risk_profiling", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
			}},
			{Key: "processor", Title: forms.Text{"th": "ผู้ประมวลผล"}, Questions: []forms.Question{
				{Key: "acts_only_as_processor", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
				{Key: "no_subcontracted_high_risk", Type: forms.TypeSingle, Label: forms.Text{"th": "x"}, Options: yn, Required: true},
			}},
		}}}
		form, err := formsSvc.CreateForm(ctx, "sme_exemption", "SME exemption screening", "sme_exemption", d)
		if err != nil {
			return err
		}
		_, err = formsSvc.Publish(ctx, form.ID, form.LatestVersion)
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
	viewer := uuid.New()
	grants := map[string][]string{
		tenant.UserID.String(): {"ropa.activity.read", "ropa.activity.create", "ropa.activity.update", "ropa.activity.approve"},
		viewer.String():        {"ropa.activity.read"},
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
	listPath := "/admin/v1/ropa/sme-exemption-checks?legal_entity_id=" + legalEntityID.String()

	if code, _ := do("GET", listPath, nil, nil); code != 401 {
		t.Errorf("list, no principal: %d, want 401", code)
	}
	if code, _ := do("POST", "/admin/v1/ropa/sme-exemption-checks", &viewer, map[string]any{
		"legal_entity_id": legalEntityID, "is_processor": false, "answers": map[string]string{"is_sme_or_community": "yes"},
	}); code != 403 {
		t.Errorf("assess with read-only permission: %d, want 403", code)
	}

	answers := map[string]string{"is_sme_or_community": "yes", "not_large_scale": "yes", "no_sensitive_data": "yes",
		"no_high_risk_profiling": "yes", "acts_only_as_processor": "yes", "no_subcontracted_high_risk": "yes"}
	code, body := do("POST", "/admin/v1/ropa/sme-exemption-checks", &admin, map[string]any{
		"legal_entity_id": legalEntityID, "is_processor": false, "answers": answers,
	})
	if code != 201 || !strings.Contains(body, `"exempt"`) {
		t.Fatalf("assess exemption: %d %s", code, body)
	}

	if code, body := do("GET", listPath, &admin, nil); code != 200 || !strings.Contains(body, `"exempt"`) {
		t.Errorf("list checks: %d %s", code, body)
	}

	if code, body := do("POST", "/admin/v1/ropa/sme-exemption-checks", &admin, map[string]any{
		"legal_entity_id": uuid.New(), "is_processor": false, "answers": answers,
	}); code != 422 {
		t.Errorf("assess with an unknown legal entity: %d %s, want 422", code, body)
	}
}
