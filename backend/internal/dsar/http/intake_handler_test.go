package dsarhttp_test

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

	dsarhttp "pdpa-platform/internal/dsar/http"
	dsarservice "pdpa-platform/internal/dsar/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

// Contract test for DSAR-01's admin intake-form endpoints (list/create/get/publish/retire) behind the real
// OpenAPI validator and AuthZ — the same chain TestDsarEndpoints_Contract already exercises for requests.
func TestDsarIntakeFormEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "dsarintakehttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM platform.public_keys`, `DELETE FROM dsar.intake_forms`,
				`DELETE FROM org.legal_entities`, `DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc}
	versioningSvc := wiring.Versioning(nil, audit.New())
	docsSvc := wiring.Docs(versioningSvc, nil, nil, audit.New(), nil)
	docsSvc.RegisterVersioning()
	keyring := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	svc := &dsarservice.Service{Audit: audit.New(), Org: orgSvc, Docs: docsSvc, Ropa: ropaSvc, Keyring: keyring}

	var legalEntity uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		e, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท อินเทคHTTP จำกัด", IsController: true}, 0)
		legalEntity = e.ID
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
		tenant.UserID.String(): {"dsar.form.read", "dsar.form.create", "dsar.form.publish", "dsar.form.delete"},
		other.String():         {"dsar.form.read"},
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
	strict := dsarhttp.NewStrictHandlerWithOptions(dsarhttp.NewStrict(svc),
		[]dsarhttp.StrictMiddlewareFunc{authz.StrictMiddleware[dsarhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		dsarhttp.StrictHTTPServerOptions{
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
	dsarhttp.HandlerWithOptions(strict, dsarhttp.ChiServerOptions{BaseRouter: r})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	do := func(method, path string, user *uuid.UUID, body any, headers ...map[string]string) (int, string) {
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
		for _, hs := range headers {
			for k, v := range hs {
				req.Header.Set(k, v)
			}
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

	if code, _ := do("GET", "/admin/v1/dsar/forms", nil, nil); code != 401 {
		t.Errorf("list, no principal: %d, want 401", code)
	}
	if code, _ := do("POST", "/admin/v1/dsar/forms", &viewer, map[string]any{"legal_entity_id": legalEntity, "name": "F"}); code != 403 {
		t.Errorf("create with read only: %d, want 403", code)
	}
	code, body := do("POST", "/admin/v1/dsar/forms", &admin, map[string]any{"legal_entity_id": legalEntity, "name": "แบบฟอร์ม DSAR"})
	if code != 201 || !strings.Contains(body, `"status":"draft"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created dsarhttp.DsarIntakeForm
	_ = json.Unmarshal([]byte(body), &created)
	item := "/admin/v1/dsar/forms/" + created.Id.String()

	if code, body := do("GET", item, &viewer, nil); code != 200 || !strings.Contains(body, `"draft"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("POST", item+"/publish", &admin, nil); code != 428 {
		t.Errorf("publish without If-Match: %d, want 428", code)
	}
	code, body = do("POST", item+"/publish", &admin, nil, map[string]string{"If-Match": `"` + strconv.Itoa(created.RowVersion) + `"`})
	if code != 200 || !strings.Contains(body, `"status":"active"`) || !strings.Contains(body, `"public_key"`) {
		t.Fatalf("publish: %d %s", code, body)
	}
	var published dsarhttp.DsarIntakeForm
	_ = json.Unmarshal([]byte(body), &published)
	if published.PublicKey == nil || *published.PublicKey == "" {
		t.Fatalf("published form has no public key: %+v", published)
	}

	if code, body := do("GET", "/admin/v1/dsar/forms", &viewer, nil); code != 200 || !strings.Contains(body, created.Id.String()) {
		t.Errorf("list: %d %s", code, body)
	}

	// Stale If-Match → 412.
	if code, _ := do("POST", item+"/retire", &admin, nil, map[string]string{"If-Match": `"0"`}); code != 412 {
		t.Errorf("retire with stale If-Match: %d, want 412", code)
	}
	code, body = do("POST", item+"/retire", &admin, nil, map[string]string{"If-Match": `"` + strconv.Itoa(published.RowVersion) + `"`})
	if code != 200 || !strings.Contains(body, `"status":"retired"`) {
		t.Fatalf("retire: %d %s", code, body)
	}
}
