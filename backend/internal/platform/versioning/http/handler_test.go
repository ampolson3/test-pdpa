package versioninghttp_test

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

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/versioning"
	versioninghttp "pdpa-platform/internal/platform/versioning/http"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the versioning endpoints behind the real OpenAPI validator and AuthZ.
func TestVersioningEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "versioninghttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{`DELETE FROM platform.approvals`, `DELETE FROM platform.record_versions`, `DELETE FROM platform.notifications`, `DELETE FROM platform.audit_log`, `DELETE FROM platform.tenant_keys`, `DELETE FROM iam.users WHERE email LIKE '%@verhttp.example'`} {
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
		return tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, 'dpo@verhttp.example', 'Dee', 'active') RETURNING id`, tenant.ID).Scan(&dpoUser)
	}); err != nil {
		t.Fatal(err)
	}
	client, _ := jobs.NewInsertClient(app)
	svc := &versioning.Service{Notify: &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client}, Audit: audit.New()}
	svc.Register("test_doc", versioning.Policy{ReadPermission: "x.doc.read", EditPermission: "x.doc.update", PublishPermission: "x.doc.publish",
		Steps: []versioning.Step{{Role: "DPO"}}})

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
	grants := map[string][]string{tenant.UserID.String(): {"x.doc.read", "x.doc.update", "x.doc.publish"}, other.String(): {"x.doc.read"}, dpoUser.String(): {}}
	roles := map[string][]string{tenant.UserID.String(): {"DPO"}, dpoUser.String(): {"DPO"}}
	cache := authz.NewCachedLoader(rdb, func(_ context.Context, tid, uid string) (authz.Grants, error) {
		return authz.Grants{TenantID: tid, UserID: uid, Permissions: grants[uid], Roles: roles[uid]}, nil
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
	strict := versioninghttp.NewStrictHandlerWithOptions(versioninghttp.NewStrict(svc),
		[]versioninghttp.StrictMiddlewareFunc{authz.StrictMiddleware[versioninghttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		versioninghttp.StrictHTTPServerOptions{
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
	versioninghttp.HandlerWithOptions(strict, versioninghttp.ChiServerOptions{BaseRouter: r})
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
	author, reader, approver := tenant.UserID, other, dpoUser
	record := uuid.New()
	var draft versioning.Version
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), author.String(), func(ctx context.Context) error {
		var err error
		draft, err = svc.SaveDraft(authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: author.String(), Permissions: grants[author.String()]}),
			"test_doc", record, map[string]any{"title": "ประกาศ"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	list := "/admin/v1/platform/records/test_doc/" + record.String() + "/versions"
	item := "/admin/v1/platform/record-versions/" + draft.ID.String()
	if code, _ := do("GET", list, nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	stranger := uuid.New()
	if code, _ := do("GET", list, &stranger, nil, nil); code != 404 {
		t.Errorf("no permission: %d, want 404", code)
	}
	if code, body := do("GET", list, &reader, nil, nil); code != 200 || !strings.Contains(body, `"status":"draft"`) {
		t.Errorf("list: %d %s", code, body)
	}
	if code, _ := do("POST", item+"/submit", &reader, nil, map[string]string{"If-Match": `"1"`}); code != 403 {
		t.Errorf("submit with read permission only: %d, want 403", code)
	}
	if code, _ := do("POST", item+"/submit", &author, nil, nil); code != 428 {
		t.Errorf("submit without If-Match: %d, want 428", code)
	}
	if code, body := do("POST", item+"/submit", &author, nil, map[string]string{"If-Match": `"1"`}); code != 200 || !strings.Contains(body, `"status":"in_review"`) {
		t.Fatalf("submit: %d %s", code, body)
	}
	if code, _ := do("POST", item+"/submit", &author, nil, map[string]string{"If-Match": `"2"`}); code != 409 {
		t.Errorf("submit twice: %d, want 409", code)
	}
	code, body := do("GET", "/admin/v1/platform/my-approvals", &approver, nil, nil)
	if code != 200 || !strings.Contains(body, "Alice") {
		t.Fatalf("approver inbox: %d %s", code, body)
	}
	var inbox struct {
		Data []versioninghttp.ApprovalInboxItem
	}
	_ = json.Unmarshal([]byte(body), &inbox)
	decide := "/admin/v1/platform/approvals/" + inbox.Data[0].Id.String() + "/decision"
	if code, body := do("GET", "/admin/v1/platform/my-approvals", &author, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("author's own work in her inbox: %s", body)
	}
	if code, body := do("POST", decide, &author, map[string]any{"decision": "approved"}, map[string]string{"If-Match": `"1"`}); code != 403 || !strings.Contains(body, "versioning.self_approval") {
		t.Errorf("maker approves: %d %s, want 403", code, body)
	}
	if code, _ := do("POST", decide, &approver, map[string]any{"decision": "maybe"}, map[string]string{"If-Match": `"1"`}); code != 400 {
		t.Errorf("bad decision: %d, want 400", code)
	}
	if code, _ := do("POST", decide, &approver, map[string]any{"decision": "returned"}, map[string]string{"If-Match": `"1"`}); code != 422 {
		t.Errorf("return without reason: %d, want 422", code)
	}
	if code, body := do("POST", decide, &approver, map[string]any{"decision": "approved"}, map[string]string{"If-Match": `"1"`}); code != 200 || !strings.Contains(body, `"status":"approved"`) {
		t.Fatalf("approve: %d %s", code, body)
	}
	if code, body := do("POST", item+"/publish", &author, nil, map[string]string{"If-Match": `"3"`}); code != 200 || !strings.Contains(body, `"status":"published"`) {
		t.Errorf("publish: %d %s", code, body)
	}
	if code, body := do("GET", item+"/compare?with="+draft.ID.String(), &reader, nil, nil); code != 200 || !strings.Contains(body, `"changes":[]`) {
		t.Errorf("compare with itself: %d %s", code, body)
	}
}
