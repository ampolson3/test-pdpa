package noticehttp_test

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

	noticehttp "pdpa-platform/internal/notice/http"
	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/jobs"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the notice endpoints (PNG-01) behind the real OpenAPI validator and AuthZ.
func TestNoticeEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "noticehttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM notice.notice_activity_links`, `DELETE FROM notice.notices`,
				`DELETE FROM platform.document_versions`, `DELETE FROM platform.documents`,
				`DELETE FROM org.legal_entities`, `DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	client, err := jobs.NewInsertClient(app)
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := &orgservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc}
	versioningSvc := wiring.Versioning(nil, audit.New())
	docsSvc := wiring.Docs(versioningSvc, nil, client, audit.New(), nil)
	docsSvc.RegisterVersioning()
	svc := &noticeservice.Service{Audit: audit.New(), Org: orgSvc, Ropa: ropaSvc, Docs: docsSvc}

	var legalEntity uuid.UUID
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		e, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		legalEntity = e.ID
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
	other := uuid.New()
	grants := map[string][]string{tenant.UserID.String(): {"notice.document.read", "notice.document.create", "notice.document.update"}, other.String(): {"notice.document.read"}}
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
	strict := noticehttp.NewStrictHandlerWithOptions(noticehttp.NewStrict(svc),
		[]noticehttp.StrictMiddlewareFunc{authz.StrictMiddleware[noticehttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		noticehttp.StrictHTTPServerOptions{
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
	noticehttp.HandlerWithOptions(strict, noticehttp.ChiServerOptions{BaseRouter: r})
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
	admin, viewer := tenant.UserID, other
	wiz := map[string]any{"legal_entity_id": legalEntity, "notice_type": "privacy_notice", "title": "ประกาศทดสอบ", "slug": "test-notice"}

	if code, _ := do("GET", "/admin/v1/notices", nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, _ := do("POST", "/admin/v1/notices", &viewer, wiz); code != 403 {
		t.Errorf("create with read permission only: %d, want 403", code)
	}
	if code, _ := do("POST", "/admin/v1/notices", &admin, map[string]any{"legal_entity_id": legalEntity, "notice_type": "bogus", "title": "x", "slug": "x"}); code != 400 {
		t.Errorf("bad notice_type: %d, want 400 (schema)", code)
	}
	if code, body := do("POST", "/admin/v1/notices", &admin, map[string]any{"legal_entity_id": uuid.New(), "notice_type": "privacy_notice", "title": "x", "slug": "unknown-entity"}); code != 422 || !strings.Contains(body, "notice.invalid_input") {
		t.Errorf("unknown legal entity: %d %s, want 422", code, body)
	}
	code, body := do("POST", "/admin/v1/notices", &admin, wiz)
	if code != 201 || !strings.Contains(body, `"status":"draft"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created noticehttp.Notice
	_ = json.Unmarshal([]byte(body), &created)
	if created.DocumentId == uuid.Nil {
		t.Fatal("expected a document_id")
	}
	item := "/admin/v1/notices/" + created.Id.String()
	if code, body := do("GET", item, &viewer, nil); code != 200 || !strings.Contains(body, `"slug":"test-notice"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/notices/"+uuid.New().String(), &admin, nil); code != 404 {
		t.Errorf("unknown notice: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/notices?notice_type=privacy_notice", &viewer, nil); code != 200 || !strings.Contains(body, `"slug":"test-notice"`) {
		t.Errorf("list: %d %s", code, body)
	}
}
