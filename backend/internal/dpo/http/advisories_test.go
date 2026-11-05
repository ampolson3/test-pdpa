package dpohttp_test

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

	dposervice "pdpa-platform/internal/dpo/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"

	dpohttp "pdpa-platform/internal/dpo/http"
)

// TestAdvisoryEndpoints_Contract is DPO-07's own acceptance criterion through the real validator + AuthZ
// chain: submit -> answer, and the answer is immediately found by the knowledge-base search.
func TestAdvisoryEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "dpo07http")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM dpo.advisories`)
			_, _ = tx.Exec(ctx, `DELETE FROM dpo.kb_articles`)
			_, _ = tx.Exec(ctx, `DELETE FROM iam.users WHERE id != $1`, tenant.UserID)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	svc := &dposervice.Service{Audit: audit.New()}

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
	dpo, emp := tenant.UserID, uuid.New()
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx,
			`INSERT INTO iam.users (id, tenant_id, email, display_name, status) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, 'Employee', 'active')`,
			emp, emp.String()+"@dbtest.example")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	grants := map[string][]string{
		dpo.String(): {"dpo.advisory.read", "dpo.advisory.update", "dpo.kb.read", "dpo.kb.create"},
		emp.String(): {"dpo.advisory.create", "dpo.advisory.read", "dpo.kb.read"},
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

	// 401 with no principal.
	if code, _ := do("GET", "/admin/v1/dpo/advisories", nil, nil, nil); code != 401 {
		t.Errorf("list, no principal: %d, want 401", code)
	}

	// Submit, as the employee.
	code, body := do("POST", "/admin/v1/dpo/advisories", &emp, map[string]any{"subject": "VPN access abroad", "question": "Can staff access systems from overseas?"}, nil)
	if code != 201 {
		t.Fatalf("submit: %d %s", code, body)
	}
	var submitted struct {
		ID         uuid.UUID `json:"id"`
		Status     string    `json:"status"`
		RowVersion int       `json:"row_version"`
	}
	_ = json.Unmarshal([]byte(body), &submitted)
	if submitted.Status != "open" {
		t.Errorf("expected status open, got %+v", submitted)
	}

	// A DPO-only caller can't submit one (no dpo.advisory.create).
	if code, _ := do("POST", "/admin/v1/dpo/advisories", &dpo, map[string]any{"subject": "x", "question": "y"}, nil); code != 403 {
		t.Errorf("submit without dpo.advisory.create: %d, want 403", code)
	}

	// Answer, as the DPO — 428 missing If-Match, then the real answer.
	answerBody := map[string]any{"answer": "Yes, through the corporate VPN with MFA."}
	if code, _ := do("POST", "/admin/v1/dpo/advisories/"+submitted.ID.String()+"/answer", &dpo, answerBody, nil); code != 428 {
		t.Errorf("answer without If-Match: %d, want 428", code)
	}
	code, body = do("POST", "/admin/v1/dpo/advisories/"+submitted.ID.String()+"/answer", &dpo, answerBody,
		map[string]string{"If-Match": `"` + strconv.Itoa(submitted.RowVersion) + `"`})
	if code != 200 {
		t.Fatalf("answer: %d %s", code, body)
	}
	var answered struct {
		Status      string     `json:"status"`
		KbArticleID *uuid.UUID `json:"kb_article_id"`
	}
	_ = json.Unmarshal([]byte(body), &answered)
	if answered.Status != "answered" || answered.KbArticleID == nil {
		t.Errorf("expected answered with a kb_article_id, got %+v", answered)
	}

	// The acceptance criterion itself: search the knowledge base and find it immediately.
	code, body = do("GET", "/admin/v1/dpo/kb-articles?q=VPN", &emp, nil, nil)
	if code != 200 || !strings.Contains(body, "corporate VPN") {
		t.Errorf("kb search: %d %s", code, body)
	}
}
