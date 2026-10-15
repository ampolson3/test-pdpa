package cookiepublichttp_test

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

	cookiepublichttp "pdpa-platform/internal/cookie/publichttp"
	cookieservice "pdpa-platform/internal/cookie/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/idempotency"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/publickeys"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test of CON-01's own /public/v1 cookie-banner endpoints — the SDK's own fetch and consent
// submission — behind the real OpenAPI validator, the public-key middleware and the Idempotency + Tx +
// audit chain, the same chain cmd/api mounts them on.
func TestCookiePublicEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })

	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "cookiepublichttp")
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM cookie.consent_records`, `DELETE FROM cookie.banner_configs`, `DELETE FROM cookie.domains`,
				`DELETE FROM cookie.categories`, `DELETE FROM platform.public_keys`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})

	orgSvc := &orgservice.Service{Audit: audit.New()}
	svc := &cookieservice.Service{Audit: audit.New(), Org: orgSvc}

	var publicKey string
	var bannerConfigID, domainID uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"cookie.banner.create", "cookie.banner.read", "cookie.banner.update", "cookie.banner.publish"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท พับลิกคุกกี้ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		d, err := svc.RegisterDomain(ctx, "public.example", le.ID)
		if err != nil {
			return err
		}
		domainID = d.ID
		if _, err := svc.CreateCategory(ctx, cookieservice.CreateCategoryInput{Code: "necessary", NameTh: "จำเป็น", IsRequired: true}); err != nil {
			return err
		}
		bc, err := svc.CreateBannerConfig(ctx, d.ID, cookieservice.CreateBannerConfigInput{Layout: "bar", Texts: cookieservice.BannerTexts{
			TitleTh: "เราใช้คุกกี้", AcceptAllTh: "ยอมรับทั้งหมด", RejectAllTh: "ปฏิเสธทั้งหมด", SettingsTh: "ตั้งค่า",
		}})
		if err != nil {
			return err
		}
		bannerConfigID = bc.ID
		published, err := svc.PublishBannerConfig(ctx, bc.ID, bc.RowVersion)
		if err != nil {
			return err
		}
		key, ok, err := publickeys.Get(ctx, publickeys.EntityCookieDomain, d.ID)
		if err != nil || !ok {
			t.Fatalf("expected a public key after publish: %v %v", ok, err)
		}
		publicKey = key
		_ = published
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	spec, err := openapi3.NewLoader().LoadFromFile("../../../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	validateMw, _ := validate.Middleware(spec, nil)
	requestError := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteProblem(w, r, httpx.RequestInvalid(err.Error()))
	}
	responseError := func(w http.ResponseWriter, r *http.Request, err error) {
		if p, ok := err.(httpx.Problem); ok {
			httpx.WriteProblem(w, r, p)
			return
		}
		httpx.WriteProblem(w, r, httpx.Internal())
	}
	idem := idempotency.New(rdb)
	auditSvc := audit.New()

	r := chi.NewRouter()
	r.Use(validateMw)
	r.Group(func(g chi.Router) {
		g.Use(publickeys.Middleware(app), idem.Handler, auditSvc.TxMiddleware(app))
		strict := cookiepublichttp.NewStrictHandlerWithOptions(cookiepublichttp.NewStrict(svc),
			[]cookiepublichttp.StrictMiddlewareFunc{cookiepublichttp.RequestInfo},
			cookiepublichttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		cookiepublichttp.HandlerWithOptions(strict, cookiepublichttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	do := func(method, path, idemKey string, body any) (int, string) {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, &buf)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idemKey != "" {
			req.Header.Set("Idempotency-Key", idemKey)
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

	if code, _ := do("GET", "/public/v1/cookie-banners/not-a-real-key-not-a-real-key", "", nil); code != 404 {
		t.Errorf("unknown key: %d, want 404", code)
	}
	code, body := do("GET", "/public/v1/cookie-banners/"+publicKey, "", nil)
	if code != 200 || !strings.Contains(body, `"accept_all_th"`) || !strings.Contains(body, `"necessary"`) {
		t.Fatalf("get banner: %d %s", code, body)
	}

	// 428: Idempotency-Key is required on every /public/v1 POST (rule 10).
	submit := map[string]any{"domain_id": domainID, "visitor_id": "visitor-contract-1", "banner_config_id": bannerConfigID, "action": "reject_all"}
	if code, _ := do("POST", "/public/v1/cookie-banners/"+publicKey+"/consents", "", submit); code != 428 {
		t.Errorf("missing Idempotency-Key: %d, want 428", code)
	}

	idemKey := uuid.NewString()
	code, body = do("POST", "/public/v1/cookie-banners/"+publicKey+"/consents", idemKey, submit)
	if code != 201 || !strings.Contains(body, `"recorded":true`) {
		t.Fatalf("submit consent: %d %s", code, body)
	}

	// Idempotency-Key replay: same body, same key → the original 201 result, not a second write.
	code2, body2 := do("POST", "/public/v1/cookie-banners/"+publicKey+"/consents", idemKey, submit)
	if code2 != 201 || body2 != body {
		t.Errorf("idempotent replay: %d %s, want the original %d %s", code2, body2, code, body)
	}

	// Unknown banner_config_id is refused.
	bad := map[string]any{"domain_id": domainID, "visitor_id": "visitor-contract-2", "banner_config_id": uuid.New(), "action": "accept_all"}
	if code, _ := do("POST", "/public/v1/cookie-banners/"+publicKey+"/consents", uuid.NewString(), bad); code != 422 {
		t.Errorf("unknown banner_config_id: %d, want 422", code)
	}
}
