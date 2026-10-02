package dsarhttp_test

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

	dsarhttp "pdpa-platform/internal/dsar/http"
	dsarservice "pdpa-platform/internal/dsar/service"
	iamservice "pdpa-platform/internal/iam/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/wiring"
)

// fakeNotifier captures DSAR-06's OTP so the contract test can read the code back (never logged — rule 3),
// the same test double IAM-05/dsar's own service-level tests already use.
type fakeNotifier struct{ sent []iamservice.NotifyRequest }

func (f *fakeNotifier) Send(ctx context.Context, req iamservice.NotifyRequest) (uuid.UUID, error) {
	f.sent = append(f.sent, req)
	return uuid.New(), nil
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the dsar endpoints (DSAR-13 + its minimal intake/transition slice) behind the real
// OpenAPI validator and AuthZ.
func TestDsarEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "dsarhttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM dsar.verifications`, `DELETE FROM iam.subject_verifications`,
				`DELETE FROM dsar.requests`, `DELETE FROM platform.document_versions`, `DELETE FROM platform.documents`,
				`DELETE FROM org.legal_entities`, `DELETE FROM platform.audit_log`,
				`DELETE FROM iam.users WHERE email = 'dsarhttp-assignee@dbtest.example'`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	versioningSvc := wiring.Versioning(nil, audit.New())
	docsSvc := wiring.Docs(versioningSvc, nil, nil, audit.New(), nil)
	docsSvc.RegisterVersioning()
	keyring := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	svc := &dsarservice.Service{Audit: audit.New(), Org: orgSvc, Docs: docsSvc, Keyring: keyring}
	notifier := &fakeNotifier{}
	svc.Verification = wiring.IamVerification(keyring, nil, nil)
	svc.Verification.Notify = notifier

	var legalEntity uuid.UUID
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		e, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		legalEntity = e.ID
		return err
	})
	var requestTypeID uuid.UUID
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		types, err := svc.ListRequestTypes(ctx)
		if err != nil || len(types) == 0 {
			t.Fatal(err, "expected seeded request types")
		}
		requestTypeID = types[0].ID
		return nil
	})
	var assignee uuid.UUID
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, 'Assignee', 'active') RETURNING id`,
			"dsarhttp-assignee@dbtest.example").Scan(&assignee)
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
	grants := map[string][]string{
		tenant.UserID.String(): {"dsar.request.read", "dsar.request.create", "dsar.request.execute", "dsar.request.update", "dsar.request.approve"},
		other.String():         {"dsar.request.read"},
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

	if code, _ := do("GET", "/admin/v1/dsar/request-types", nil, nil); code != 401 {
		t.Errorf("request-types, no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/dsar/request-types", &viewer, nil); code != 200 || !strings.Contains(body, `"access"`) {
		t.Errorf("request-types: %d %s", code, body)
	}

	create := map[string]any{"request_type_id": requestTypeID, "legal_entity_id": legalEntity, "channel": "web",
		"requester_name": "สมชาย ใจดี", "requester_contact": "somchai@example.com", "contact_kind": "email"}
	if code, _ := do("POST", "/admin/v1/dsar/requests", &viewer, create); code != 403 {
		t.Errorf("create with read only: %d, want 403", code)
	}
	if code, _ := do("POST", "/admin/v1/dsar/requests", &admin, map[string]any{"request_type_id": requestTypeID, "legal_entity_id": legalEntity, "channel": "bogus", "requester_name": "x", "requester_contact": "y@example.com", "contact_kind": "email"}); code != 400 {
		t.Errorf("bad channel: %d, want 400 (schema)", code)
	}
	code, body := do("POST", "/admin/v1/dsar/requests", &admin, create)
	if code != 201 || !strings.Contains(body, `"status":"received"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created dsarhttp.DsarRequest
	_ = json.Unmarshal([]byte(body), &created)
	item := "/admin/v1/dsar/requests/" + created.Id.String()

	if code, body := do("GET", item, &viewer, nil); code != 200 || !strings.Contains(body, `"status":"received"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/dsar/requests/"+uuid.New().String(), &admin, nil); code != 404 {
		t.Errorf("unknown request: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/dsar/requests", &viewer, nil); code != 200 || !strings.Contains(body, created.RequestNo) {
		t.Errorf("list: %d %s", code, body)
	}
	// DSAR-17: history search by request number (substring) or e-mail (blind index).
	if code, body := do("GET", "/admin/v1/dsar/requests?search="+created.RequestNo[len(created.RequestNo)-6:], &viewer, nil); code != 200 || !strings.Contains(body, created.RequestNo) {
		t.Errorf("search by request_no: %d %s", code, body)
	}
	if code, body := do("GET", "/admin/v1/dsar/requests?search=somchai@example.com", &viewer, nil); code != 200 || !strings.Contains(body, created.RequestNo) {
		t.Errorf("search by email: %d %s", code, body)
	}
	if code, body := do("GET", "/admin/v1/dsar/requests?search=nobody@example.com", &viewer, nil); code != 200 || strings.Contains(body, created.RequestNo) {
		t.Errorf("search by unknown email: %d %s", code, body)
	}
	if code, body := do("GET", item, &viewer, nil); code != 200 || !strings.Contains(body, `"sla_status"`) {
		t.Errorf("expected sla_status on the request: %d %s", code, body)
	}

	assign := item + "/assign"
	if code, _ := do("POST", assign, nil, map[string]any{"assignee_user_id": assignee}, map[string]string{"If-Match": etagOf(int(created.RowVersion))}); code != 401 {
		t.Errorf("assign, no principal: %d, want 401", code)
	}
	if code, _ := do("POST", assign, &admin, map[string]any{"assignee_user_id": assignee}); code != 428 {
		t.Errorf("assign, no If-Match: %d, want 428", code)
	}
	if code, body := do("POST", assign, &admin, map[string]any{"assignee_user_id": assignee}, map[string]string{"If-Match": etagOf(int(created.RowVersion))}); code != 200 || !strings.Contains(body, assignee.String()) {
		t.Errorf("assign: %d %s", code, body)
	}
	if code, body := do("POST", assign, &admin, map[string]any{"assignee_user_id": uuid.New()}, map[string]string{"If-Match": etagOf(int(created.RowVersion) + 1)}); code != 422 {
		t.Errorf("assign unknown user: %d %s, want 422", code, body)
	}
	if code, body := do("POST", assign, &admin, map[string]any{}, map[string]string{"If-Match": etagOf(int(created.RowVersion) + 1)}); code != 200 || strings.Contains(body, assignee.String()) {
		t.Errorf("unassign: %d %s", code, body)
	}

	transition := item + "/transition"
	if code, _ := do("POST", transition, nil, map[string]any{"to": "verifying"}, map[string]string{"If-Match": `"1"`}); code != 401 {
		t.Errorf("transition, no principal: %d, want 401", code)
	}
	if code, _ := do("POST", transition, &admin, map[string]any{"to": "verifying"}); code != 428 {
		t.Errorf("transition, no If-Match: %d, want 428", code)
	}
	if code, _ := do("POST", transition, &admin, map[string]any{"to": "verifying"}, map[string]string{"If-Match": `"99"`}); code != 412 {
		t.Errorf("transition, stale version: %d, want 412", code)
	}
	if code, body := do("POST", transition, &admin, map[string]any{"to": "verifying"}, map[string]string{"If-Match": etagOf(int(created.RowVersion) + 2)}); code != 200 || !strings.Contains(body, `"status":"verifying"`) {
		t.Errorf("transition to verifying: %d %s", code, body)
	}
	if code, body := do("POST", transition, &admin, map[string]any{"to": "in_review"}, map[string]string{"If-Match": etagOf(int(created.RowVersion) + 3)}); code != 200 || !strings.Contains(body, `"status":"in_review"`) {
		t.Errorf("transition to in_review: %d %s", code, body)
	}
	if code, body := do("POST", transition, &admin, map[string]any{"to": "rejected"}, map[string]string{"If-Match": etagOf(int(created.RowVersion) + 4)}); code != 422 || !strings.Contains(body, "dsar.invalid_input") {
		t.Errorf("reject without reason: %d %s, want 422", code, body)
	}
	code, body = do("POST", transition, &admin, map[string]any{"to": "rejected", "rejection_reason_code": "ไม่พบข้อมูล"}, map[string]string{"If-Match": etagOf(int(created.RowVersion) + 4)})
	if code != 200 || !strings.Contains(body, `"status":"rejected"`) || !strings.Contains(body, `"document_id"`) {
		t.Errorf("reject with reason: %d %s", code, body)
	}
	if code, body := do("POST", transition, &admin, map[string]any{"to": "completed"}, map[string]string{"If-Match": etagOf(int(created.RowVersion) + 5)}); code != 409 || !strings.Contains(body, "dsar.invalid_transition") {
		t.Errorf("transition from terminal rejected: %d %s, want 409", code, body)
	}

	// DSAR-06 identity verification: a fresh request so it starts "received" (the first one above is already
	// terminal).
	code, body = do("POST", "/admin/v1/dsar/requests", &admin, create)
	var created2 dsarhttp.DsarRequest
	_ = json.Unmarshal([]byte(body), &created2)
	item2 := "/admin/v1/dsar/requests/" + created2.Id.String()
	verifications := item2 + "/verifications"
	if code, _ := do("GET", verifications, nil, nil); code != 401 {
		t.Errorf("verifications, no principal: %d, want 401", code)
	}
	if code, body := do("GET", verifications, &viewer, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("verifications (none yet): %d %s", code, body)
	}

	startOtp := item2 + "/verifications/otp"
	if code, _ := do("POST", startOtp, &viewer, map[string]any{"method": "otp_email"}); code != 403 {
		t.Errorf("start otp with read only: %d, want 403", code)
	}
	if code, body := do("POST", startOtp, &admin, map[string]any{"method": "bogus"}); code != 400 && code != 422 {
		t.Errorf("start otp, bad method: %d %s, want 400/422", code, body)
	}
	if code, _ := do("POST", "/admin/v1/dsar/requests/"+uuid.New().String()+"/verifications/otp", &admin, map[string]any{"method": "otp_email"}); code != 404 {
		t.Errorf("start otp, unknown request: %d, want 404", code)
	}
	code, body = do("POST", startOtp, &admin, map[string]any{"method": "otp_email"})
	if code != 201 || !strings.Contains(body, `"method":"otp_email"`) || !strings.Contains(body, `"status":"pending"`) {
		t.Errorf("start otp: %d %s", code, body)
	}
	var startedV dsarhttp.DsarVerification
	_ = json.Unmarshal([]byte(body), &startedV)
	if len(notifier.sent) == 0 {
		t.Fatal("expected an OTP to be sent")
	}
	otpCode, _ := notifier.sent[len(notifier.sent)-1].Vars["code"].(string)

	if code, body := do("GET", item2, &viewer, nil); code != 200 || !strings.Contains(body, `"status":"verifying"`) {
		t.Errorf("request after start otp: %d %s, want status verifying", code, body)
	}

	confirmOtp := verifications + "/" + startedV.Id.String() + "/confirm-otp"
	if code, _ := do("POST", confirmOtp, &admin, map[string]any{"code": "000000"}); code != 400 && code != 422 {
		t.Errorf("confirm otp, wrong code: %d, want 400/422", code)
	}
	if code, body := do("POST", confirmOtp, &admin, map[string]any{"code": otpCode}); code != 200 || !strings.Contains(body, `"status":"passed"`) {
		t.Errorf("confirm otp: %d %s", code, body)
	}
	if code, body := do("GET", item2, &viewer, nil); code != 200 || !strings.Contains(body, `"status":"in_review"`) || !strings.Contains(body, `"verified_at"`) {
		t.Errorf("request after confirm otp: %d %s, want in_review + verified_at", code, body)
	}
	if code, body := do("GET", verifications, &viewer, nil); code != 200 || !strings.Contains(body, `"status":"passed"`) {
		t.Errorf("verifications after confirm: %d %s", code, body)
	}
}

func etagOf(v int) string { return `"` + strconv.Itoa(v) + `"` }
