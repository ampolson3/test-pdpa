package collabhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"pdpa-platform/internal/platform/collab"
	collabhttp "pdpa-platform/internal/platform/collab/http"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the collaboration endpoints behind the real OpenAPI validator and AuthZ.
func TestCollabEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "collabhttp")
	record := uuid.New()
	var bob uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active', display_name = 'Alice' WHERE id = $1`, tenant.UserID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, 'bob@collabhttp.example', 'Bobby', 'active') RETURNING id`, tenant.ID).Scan(&bob)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), app, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.comments`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.notifications`)
			_, err := tx.Exec(ctx, `DELETE FROM iam.users WHERE id = $1`, bob)
			return err
		})
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.tenant_keys`)
			return err
		})
	})

	client, _ := jobs.NewInsertClient(app)
	svc := &collab.Service{Notify: &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client}, Audit: audit.New()}
	svc.Register("test_record", collab.Policy{ReadPermission: "x.record.read", WritePermission: "x.record.update",
		Exists: func(_ context.Context, id uuid.UUID) (bool, error) { return id == record, nil }})

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
	reader := uuid.New()
	grants := map[string][]string{tenant.UserID.String(): {"x.record.read", "x.record.update"}, bob.String(): {"x.record.read", "x.record.update"}, reader.String(): {"x.record.read"}}
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
	strict := collabhttp.NewStrictHandlerWithOptions(collabhttp.NewStrict(svc),
		[]collabhttp.StrictMiddlewareFunc{authz.StrictMiddleware[collabhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		collabhttp.StrictHTTPServerOptions{
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
	collabhttp.HandlerWithOptions(strict, collabhttp.ChiServerOptions{BaseRouter: r})
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
	alice := tenant.UserID
	base := "/admin/v1/platform/records/test_record/" + record.String()

	if code, _ := do("GET", base+"/comments", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, _ := do("GET", "/admin/v1/platform/records/Bad-Type/"+record.String()+"/comments", &alice, nil, nil); code != 400 {
		t.Errorf("malformed entity type: %d, want 400", code)
	}
	if code, _ := do("GET", "/admin/v1/platform/records/unregistered/"+record.String()+"/comments", &alice, nil, nil); code != 404 {
		t.Errorf("unregistered type: %d, want 404", code)
	}
	if code, _ := do("POST", base+"/comments", &reader, map[string]any{"body": "hi"}, nil); code != 403 {
		t.Errorf("comment with read permission only: %d, want 403", code)
	}
	code, body := do("GET", "/admin/v1/platform/mentionable-users?q=bob", &alice, nil, nil)
	if code != 200 || !strings.Contains(body, "Bobby") || strings.Contains(body, "@collabhttp.example") {
		t.Errorf("mention search: %d %s (names only, no e-mail)", code, body)
	}
	code, body = do("POST", base+"/comments", &alice, map[string]any{"body": fmt.Sprintf("ดู @[Bobby](%s)", bob)}, nil)
	if code != 201 || !strings.Contains(body, bob.String()) {
		t.Fatalf("create: %d %s", code, body)
	}
	var root collabhttp.Comment
	_ = json.Unmarshal([]byte(body), &root)
	if code, _ := do("POST", base+"/comments", &bob, map[string]any{"body": "ok", "parent_id": root.Id}, nil); code != 201 {
		t.Errorf("reply: %d", code)
	}
	if code, body := do("GET", base+"/comments", &reader, nil, nil); code != 200 || strings.Count(body, `"id"`) < 2 {
		t.Errorf("list: %d %s", code, body)
	}
	item := "/admin/v1/platform/comments/" + root.Id.String()
	if code, _ := do("PATCH", item, &alice, map[string]any{"body": "edited"}, nil); code != 428 {
		t.Errorf("PATCH without If-Match: %d, want 428", code)
	}
	if code, _ := do("PATCH", item, &alice, map[string]any{"body": "edited"}, map[string]string{"If-Match": `"9"`}); code != 412 {
		t.Errorf("stale If-Match: %d, want 412", code)
	}
	if code, _ := do("PATCH", item, &bob, map[string]any{"body": "hijack"}, map[string]string{"If-Match": `"1"`}); code != 403 {
		t.Errorf("editing someone else's comment: %d, want 403", code)
	}
	if code, _ := do("DELETE", item, &alice, nil, map[string]string{"If-Match": `"1"`}); code != 409 {
		t.Errorf("delete with replies: %d, want 409", code)
	}
	if code, body := do("POST", item+"/resolve", &bob, map[string]any{"resolved": true}, nil); code != 200 || !strings.Contains(body, `"resolved":true`) {
		t.Errorf("resolve: %d %s", code, body)
	}
	if code, body := do("GET", base+"/activity", &reader, nil, nil); code != 200 || !strings.Contains(body, "platform.comment.create") || !strings.Contains(body, "platform.comment.resolve") {
		t.Errorf("activity: %d %s", code, body)
	}
}
