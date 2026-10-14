package dsarpublichttp_test

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

	dsarpublichttp "pdpa-platform/internal/dsar/publichttp"
	dsarservice "pdpa-platform/internal/dsar/service"
	iamservice "pdpa-platform/internal/iam/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/idempotency"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/publickeys"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

type fakeNotifier struct{ sent []iamservice.NotifyRequest }

func (f *fakeNotifier) Send(ctx context.Context, req iamservice.NotifyRequest) (uuid.UUID, error) {
	f.sent = append(f.sent, req)
	return uuid.New(), nil
}

// Contract test of DSAR-01's public intake-form endpoints behind the real OpenAPI validator, the
// public-key middleware and the Idempotency + Tx + audit chain — the same chain cmd/api mounts them on.
func TestDsarPublicEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })

	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "dsarpublichttp")
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM dsar.verifications`, `DELETE FROM iam.subject_verifications`,
				`DELETE FROM platform.public_keys`, `DELETE FROM dsar.intake_forms`,
				`DELETE FROM dsar.requests`, `DELETE FROM org.legal_entities`, `DELETE FROM platform.audit_log`,
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
	notifier := &fakeNotifier{}
	svc.Verification = wiring.IamVerification(keyring, nil, nil)
	svc.Verification.Notify = notifier

	var publicKey string
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท พับลิกHTTP จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		form, err := svc.CreateIntakeForm(ctx, dsarservice.CreateIntakeFormInput{LegalEntityID: le.ID, Name: "F"})
		if err != nil {
			return err
		}
		form, err = svc.PublishIntakeForm(ctx, form.ID, form.RowVersion)
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
		strict := dsarpublichttp.NewStrictHandlerWithOptions(dsarpublichttp.NewStrict(svc), nil,
			dsarpublichttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		dsarpublichttp.HandlerWithOptions(strict, dsarpublichttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})
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

	if code, _ := do("GET", "/public/v1/dsar-forms/not-a-real-key-not-a-real-key", "", nil); code != 404 {
		t.Errorf("unknown key: %d, want 404", code)
	}
	if code, body := do("GET", "/public/v1/dsar-forms/"+publicKey, "", nil); code != 200 || !strings.Contains(body, `"access"`) {
		t.Errorf("get form: %d %s", code, body)
	}

	submitBody := map[string]any{"request_type_code": "access", "requester_name": "สมหญิง ดีใจ", "requester_email": "somying-http@example.com"}
	idemKey := uuid.NewString()
	code, body := do("POST", "/public/v1/dsar-forms/"+publicKey+"/requests", idemKey, submitBody)
	if code != 201 || !strings.Contains(body, `"request_no"`) || !strings.Contains(body, `"verification"`) {
		t.Fatalf("submit: %d %s", code, body)
	}
	var result dsarpublichttp.DsarPublicSubmissionResult
	_ = json.Unmarshal([]byte(body), &result)
	if result.RequestNo == "" {
		t.Fatal("expected a non-empty request_no immediately (the acceptance criterion)")
	}
	if result.Verification == nil || result.Verification.Status != "pending" {
		t.Fatalf("expected a pending verification, got %+v", result.Verification)
	}

	// Idempotency-Key replay: same body, same key → the original 201 result, not a second request.
	code2, body2 := do("POST", "/public/v1/dsar-forms/"+publicKey+"/requests", idemKey, submitBody)
	if code2 != 201 || body2 != body {
		t.Errorf("idempotent replay: %d %s, want the original %d %s", code2, body2, code, body)
	}

	if len(notifier.sent) != 1 {
		t.Fatalf("expected one OTP sent, got %d", len(notifier.sent))
	}
	otpCode, _ := notifier.sent[0].Vars["code"].(string)
	if otpCode == "" {
		t.Fatal("no OTP code captured")
	}

	verifyPath := "/public/v1/dsar-forms/" + publicKey + "/requests/" + result.Verification.RequestId.String() + "/verify"
	if code, _ := do("POST", verifyPath, uuid.NewString(), map[string]any{"verification_id": result.Verification.Id, "code": "000000"}); code != 422 {
		t.Errorf("wrong otp code: %d, want 422", code)
	}
	code, body = do("POST", verifyPath, uuid.NewString(), map[string]any{"verification_id": result.Verification.Id, "code": otpCode})
	if code != 200 || !strings.Contains(body, `"passed"`) {
		t.Fatalf("confirm otp: %d %s", code, body)
	}
}
