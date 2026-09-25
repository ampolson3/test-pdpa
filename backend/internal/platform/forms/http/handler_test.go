package formshttp_test

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

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	formshttp "pdpa-platform/internal/platform/forms/http"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/wiring"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the form endpoints behind the real OpenAPI validator and AuthZ.
func TestFormEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "formshttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{`DELETE FROM platform.form_section_assignments`, `DELETE FROM platform.form_submissions`, `UPDATE platform.form_definitions SET current_version_id = NULL WHERE tenant_id IS NOT NULL`, `DELETE FROM platform.form_versions WHERE tenant_id IS NOT NULL`, `DELETE FROM platform.form_definitions WHERE tenant_id IS NOT NULL`, `DELETE FROM platform.notifications`, `DELETE FROM platform.audit_log`, `DELETE FROM platform.tenant_keys`, `DELETE FROM iam.users WHERE email LIKE '%@formshttp.example'`} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
		_, _ = app.Exec(context.Background(), `DELETE FROM river_job WHERE args->>'tenant_id' = $1`, tenant.ID.String())
	})
	var dpoUser uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active', display_name = 'Alice' WHERE id = $1`, tenant.UserID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, 'dpo@formshttp.example', 'Bob', 'active') RETURNING id`, tenant.ID).Scan(&dpoUser)
	}); err != nil {
		t.Fatal(err)
	}
	client, _ := jobs.NewInsertClient(app)
	svc := wiring.Forms(&notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client}, audit.New())

	spec, err := openapi3.NewLoader().LoadFromFile("../../../../../api/openapi/openapi.yaml")
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
	grants := map[string][]string{tenant.UserID.String(): {"assessment.template.read", "assessment.template.create", "assessment.template.update", "assessment.template.publish", "assessment.dpia.create"},
		other.String(): {"dsar.form.read", "dsar.form.create"}, dpoUser.String(): {}}
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
	strict := formshttp.NewStrictHandlerWithOptions(formshttp.NewStrict(svc),
		[]formshttp.StrictMiddlewareFunc{authz.StrictMiddleware[formshttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		formshttp.StrictHTTPServerOptions{
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
	formshttp.HandlerWithOptions(strict, formshttp.ChiServerOptions{BaseRouter: r})
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
	alice, dsarUser, bob := tenant.UserID, other, dpoUser
	raw, err := os.ReadFile("../../../../../packages/form-renderer/src/fixtures/engine-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Schema  json.RawMessage `json:"schema"`
		Scoring json.RawMessage `json:"scoring"`
	}
	_ = json.Unmarshal(raw, &fx)
	create := map[string]any{"code": "dpia_screening", "name": "คัดกรอง DPIA", "form_type": "assessment", "draft": map[string]any{"schema": fx.Schema, "scoring": fx.Scoring}}

	if code, _ := do("GET", "/admin/v1/platform/forms", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/platform/form-types", &dsarUser, nil, nil); code != 200 || !strings.Contains(body, `{"can_create":true,"can_publish":false,"can_respond":false,"can_update":false,"form_type":"dsar"}`) {
		t.Errorf("form types: %d %s", code, body)
	}
	if code, _ := do("POST", "/admin/v1/platform/forms", &dsarUser, create, nil); code != 403 {
		t.Errorf("create another module's form: %d, want 403", code)
	}
	bad := map[string]any{"code": "x", "name": "x", "form_type": "assessment", "draft": map[string]any{"schema": map[string]any{"sections": []any{
		map[string]any{"key": "s", "title": map[string]any{"th": "ส"}, "questions": []any{map[string]any{"key": "q", "type": "slider", "label": map[string]any{"th": "ค"}}}}}}}}
	if code, _ := do("POST", "/admin/v1/platform/forms", &alice, bad, nil); code != 400 {
		t.Errorf("unknown question type: %d, want 400", code)
	}
	dup := map[string]any{"code": "x", "name": "x", "form_type": "assessment", "draft": map[string]any{"schema": map[string]any{"sections": []any{
		map[string]any{"key": "s", "title": map[string]any{"th": "ส"}, "questions": []any{map[string]any{"key": "q", "type": "text", "label": map[string]any{"th": "ค"}},
			map[string]any{"key": "q", "type": "text", "label": map[string]any{"th": "ค"}}}}}}}}
	if code, body := do("POST", "/admin/v1/platform/forms", &alice, dup, nil); code != 422 || !strings.Contains(body, "forms.invalid_schema") {
		t.Errorf("duplicate keys: %d %s, want 422", code, body)
	}
	if code, body := do("POST", "/admin/v1/platform/forms", &dsarUser, map[string]any{"code": "x", "name": "x", "form_type": "breach", "draft": map[string]any{"schema": fx.Schema}}, nil); code != 422 || !strings.Contains(body, "forms.unknown_type") {
		t.Errorf("unregistered type: %d %s, want 422", code, body)
	}
	code, body := do("POST", "/admin/v1/platform/forms", &alice, create, nil)
	if code != 201 {
		t.Fatalf("create: %d %s", code, body)
	}
	var form formshttp.Form
	_ = json.Unmarshal([]byte(body), &form)
	item := "/admin/v1/platform/forms/" + form.Id.String()
	if code, _ := do("GET", item, &dsarUser, nil, nil); code != 404 {
		t.Errorf("read another module's form: %d, want 404", code)
	}
	if code, _ := do("POST", item+"/publish", &alice, nil, nil); code != 428 {
		t.Errorf("publish without If-Match: %d, want 428", code)
	}
	if code, _ := do("POST", item+"/publish", &alice, nil, map[string]string{"If-Match": `"9"`}); code != 412 {
		t.Errorf("publish stale: %d, want 412", code)
	}
	draftETag := `"` + strconv.Itoa(form.Versions[0].RowVersion) + `"`
	if code, body := do("POST", item+"/publish", &alice, nil, map[string]string{"If-Match": draftETag}); code != 200 || !strings.Contains(body, `"status":"published"`) {
		t.Fatalf("publish: %d %s", code, body)
	}
	if code, _ := do("PUT", item+"/draft", &alice, map[string]any{"schema": fx.Schema}, map[string]string{"If-Match": `"1"`}); code != 409 && code != 412 {
		t.Errorf("save a draft that doesn't exist: %d", code)
	}
	if code, _ := do("POST", item+"/responses", &dsarUser, nil, nil); code != 404 {
		t.Errorf("respond without read: %d, want 404", code)
	}
	code, body = do("POST", item+"/responses", &alice, nil, nil)
	if code != 201 {
		t.Fatalf("start: %d %s", code, body)
	}
	var resp formshttp.FormResponse
	_ = json.Unmarshal([]byte(body), &resp)
	rItem := "/admin/v1/platform/form-responses/" + resp.Id.String()
	etagOf := func(v int) map[string]string { return map[string]string{"If-Match": `"` + strconv.Itoa(v) + `"`} }
	if code, body := do("PATCH", rItem+"/answers", &alice, map[string]any{"answers": map[string]any{"subjects": -1, "processes_sensitive": "maybe"}}, etagOf(resp.RowVersion)); code != 422 ||
		!strings.Contains(body, `"field":"subjects","code":"out_of_range"`) || !strings.Contains(body, `"field":"processes_sensitive","code":"invalid_option"`) {
		t.Errorf("invalid answers: %d %s", code, body)
	}
	if code, _ := do("GET", rItem, &bob, nil, nil); code != 404 {
		t.Errorf("stranger reads the response: %d, want 404", code)
	}
	code, body = do("PATCH", rItem+"/answers", &alice, map[string]any{"answers": map[string]any{"org_name": "ACME", "processes_sensitive": "yes", "sensitive_types": []string{"criminal"}, "subjects": 20000}}, etagOf(resp.RowVersion))
	if code != 200 {
		t.Fatalf("answers: %d %s", code, body)
	}
	_ = json.Unmarshal([]byte(body), &resp)
	code, body = do("PUT", rItem+"/assignments/scale", &alice, map[string]any{"assignee_user_id": bob}, etagOf(resp.RowVersion))
	if code != 200 || !strings.Contains(body, `"assignee_name":"Bob"`) {
		t.Fatalf("assign: %d %s", code, body)
	}
	_ = json.Unmarshal([]byte(body), &resp)
	if code, body := do("GET", "/admin/v1/platform/my-form-sections", &bob, nil, nil); code != 200 || !strings.Contains(body, `"section":"scale"`) {
		t.Errorf("Bob's sections: %d %s", code, body)
	}
	if code, body := do("POST", rItem+"/submit", &alice, nil, etagOf(resp.RowVersion)); code != 409 || !strings.Contains(body, "forms.invalid_state") {
		t.Errorf("submit with an open section: %d %s", code, body)
	}
	if code, body := do("POST", rItem+"/sections/scale/complete", &bob, nil, nil); code != 422 || !strings.Contains(body, `"field":"large_scale_reason","code":"required"`) {
		t.Errorf("complete unanswered: %d %s", code, body)
	}
	code, body = do("GET", rItem, &bob, nil, nil)
	_ = json.Unmarshal([]byte(body), &resp)
	if code, body := do("PATCH", rItem+"/answers", &bob, map[string]any{"answers": map[string]any{"large_scale_reason": "x"}}, etagOf(resp.RowVersion)); code != 200 {
		t.Fatalf("Bob answers: %d %s", code, body)
	}
	if code, body := do("POST", rItem+"/sections/scale/complete", &bob, nil, nil); code != 200 || !strings.Contains(body, `"status":"done"`) {
		t.Fatalf("complete: %d %s", code, body)
	}
	code, body = do("GET", rItem, &alice, nil, nil)
	_ = json.Unmarshal([]byte(body), &resp)
	// yes 5×2 + criminal 5 = 15 → high
	if code, body := do("POST", rItem+"/submit", &alice, nil, etagOf(resp.RowVersion)); code != 200 || !strings.Contains(body, `"band":"high"`) || !strings.Contains(body, `"score":15`) {
		t.Errorf("submit: %d %s", code, body)
	}
	if code, body := do("GET", item+"/responses", &alice, nil, nil); code != 200 || !strings.Contains(body, `"status":"submitted"`) {
		t.Errorf("responses: %d %s", code, body)
	}
}
