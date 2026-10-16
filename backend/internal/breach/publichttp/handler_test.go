package breachpublichttp_test

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

	breachpublichttp "pdpa-platform/internal/breach/publichttp"
	breachservice "pdpa-platform/internal/breach/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/idempotency"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/publickeys"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test of BRE-01's public report-form endpoints behind the real OpenAPI validator, the public-key
// middleware and the Idempotency + Tx + audit chain — the same chain cmd/api mounts them on.
func TestBreachPublicEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })

	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "breachpublichttp")
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM platform.public_keys`, `DELETE FROM breach.report_forms`,
				`DELETE FROM breach.timeline_events`, `DELETE FROM breach.incidents`,
				`DELETE FROM org.legal_entities`, `DELETE FROM platform.audit_log`, `DELETE FROM platform.notifications`,
				`DELETE FROM platform.outbox_events`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
		_, _ = app.Exec(context.Background(), `DELETE FROM river_job WHERE args->>'tenant_id' = $1`, tenant.ID.String())
	})

	orgSvc := &orgservice.Service{Audit: audit.New()}
	keyring := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	svc := &breachservice.Service{Audit: audit.New(), Org: orgSvc, Keyring: keyring}

	var publicKey string
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท พับลิกHTTP จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		form, err := svc.CreateReportForm(ctx, breachservice.CreateReportFormInput{LegalEntityID: le.ID, Name: "F"})
		if err != nil {
			return err
		}
		form, err = svc.PublishReportForm(ctx, form.ID, form.RowVersion)
		if err != nil {
			return err
		}
		publicKey = *form.PublicKey
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
		strict := breachpublichttp.NewStrictHandlerWithOptions(breachpublichttp.NewStrict(svc), nil,
			breachpublichttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		breachpublichttp.HandlerWithOptions(strict, breachpublichttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	do := func(method, path string, idemKey string, body any) (int, string) {
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

	if code, _ := do("GET", "/public/v1/breach-report-forms/not-a-real-key-not-a-real-key", "", nil); code != 404 {
		t.Errorf("unknown key: %d, want 404", code)
	}
	if code, body := do("GET", "/public/v1/breach-report-forms/"+publicKey, "", nil); code != 200 || !strings.Contains(body, `"breach_types"`) {
		t.Errorf("get form: %d %s", code, body)
	}

	submitBody := map[string]any{"title": "พบไฟล์หลุด", "description": "พบไฟล์ข้อมูลลูกค้าเปิดเผยบนเว็บไซต์สาธารณะ", "breach_types": []string{"confidentiality"}}
	idemKey := uuid.NewString()
	code, body := do("POST", "/public/v1/breach-report-forms/"+publicKey+"/incidents", idemKey, submitBody)
	if code != 201 || !strings.Contains(body, `"incident_no"`) {
		t.Fatalf("submit: %d %s", code, body)
	}
	var result breachpublichttp.BreachPublicReportResult
	_ = json.Unmarshal([]byte(body), &result)
	if result.IncidentNo == "" {
		t.Fatal("expected a non-empty incident_no immediately (the acceptance criterion)")
	}

	// Idempotency-Key replay: same body, same key → the original 201 result, not a second request.
	code2, body2 := do("POST", "/public/v1/breach-report-forms/"+publicKey+"/incidents", idemKey, submitBody)
	if code2 != 201 || body2 != body {
		t.Errorf("idempotent replay: %d %s, want the original %d %s", code2, body2, code, body)
	}

	// Missing breach_types is refused by the schema.
	if code, _ := do("POST", "/public/v1/breach-report-forms/"+publicKey+"/incidents", uuid.NewString(),
		map[string]any{"title": "x", "description": "y", "breach_types": []string{}}); code != 400 {
		t.Errorf("empty breach_types: %d, want 400 (schema minItems)", code)
	}
}
