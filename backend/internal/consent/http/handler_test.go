package consenthttp_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"pdpa-platform/internal/consent/consenttest"
	consenthttp "pdpa-platform/internal/consent/http"
	consentpublichttp "pdpa-platform/internal/consent/publichttp"
	consent "pdpa-platform/internal/consent/service"
	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
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

// Contract test of the admin and public consent endpoints behind the real OpenAPI validator, AuthZ, the public-key
// middleware, Idempotency and the Tx + audit middleware — the same chains as cmd/api.
func TestConsentEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	f := consenttest.Setup(t)
	guardianNotifier := &fakeIamNotifier{}
	f.Svc.Verification = &iamservice.Service{Keyring: f.Svc.Keyring, Notify: guardianNotifier} // CON-11: guardian OTP (IAM-05)

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
	reader := uuid.New() // may read purposes, nothing else
	grants := map[string][]string{f.Alice.String(): consenttest.Maker, reader.String(): {"consent.purpose.read"}}
	cache := authz.NewCachedLoader(rdb, func(_ context.Context, tid, uid string) (authz.Grants, error) {
		return authz.Grants{TenantID: tid, UserID: uid, Permissions: grants[uid]}, nil
	})
	t.Cleanup(func() {
		for uid := range grants {
			_ = cache.Invalidate(context.Background(), f.A.ID.String(), uid)
		}
	})
	requestError := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteProblem(w, r, httpx.RequestInvalid(err.Error()))
	}
	responseError := func(w http.ResponseWriter, r *http.Request, err error) {
		if p, ok := err.(httpx.Problem); ok {
			httpx.WriteProblem(w, r, p)
			return
		}
		t.Logf("handler error: %v", err)
		httpx.WriteProblem(w, r, httpx.Internal())
	}
	auditSvc := audit.New()
	idem := idempotency.New(rdb)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if u := req.Header.Get("X-Test-User"); u != "" {
				req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{TenantID: f.A.ID.String(), UserID: u, ActorType: "user"}))
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Use(validateMw)
	r.Group(func(g chi.Router) {
		g.Use(publickeys.Middleware(f.App), idem.Handler, auditSvc.TxMiddleware(f.App))
		strict := consentpublichttp.NewStrictHandlerWithOptions(consentpublichttp.NewStrict(f.Svc),
			[]consentpublichttp.StrictMiddlewareFunc{consentpublichttp.RequestInfo},
			consentpublichttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		consentpublichttp.HandlerWithOptions(strict, consentpublichttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})
	})
	r.Group(func(g chi.Router) {
		g.Use(idem.Handler, auditSvc.TxMiddleware(f.App))
		strict := consenthttp.NewStrictHandlerWithOptions(consenthttp.NewStrict(f.Svc),
			[]consenthttp.StrictMiddlewareFunc{authz.StrictMiddleware[consenthttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
			consenthttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		consenthttp.HandlerWithOptions(strict, consenthttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	type resp struct {
		code int
		body map[string]any
		hdr  http.Header
	}
	do := func(method, path string, user *uuid.UUID, body any, headers map[string]string) resp {
		t.Helper()
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
		b, _ := io.ReadAll(res.Body)
		out := resp{code: res.StatusCode, hdr: res.Header}
		_ = json.Unmarshal(b, &out.body)
		return out
	}
	errCodes := func(b map[string]any) []string {
		var out []string
		es, _ := b["errors"].([]any)
		for _, e := range es {
			m, _ := e.(map[string]any)
			out = append(out, m["field"].(string)+":"+m["code"].(string))
		}
		return out
	}
	alice, dpoLess := f.Alice, reader

	// ---- admin ----
	if res := do("GET", "/admin/v1/consent/purposes", nil, nil, nil); res.code != 401 {
		t.Errorf("no principal: %d, want 401", res.code)
	}
	if res := do("GET", "/admin/v1/consent/purposes", &dpoLess, nil, nil); res.code != 200 {
		t.Errorf("reader lists purposes: %d", res.code)
	}
	newPurpose := map[string]any{"code": "NEWS", "legal_entity_id": f.EntityA,
		"content": map[string]any{"name": map[string]string{"th": "ข่าวสาร"}, "consent_text": map[string]string{"th": "ยินยอมรับข่าวสาร"}}}
	if res := do("POST", "/admin/v1/consent/purposes", &dpoLess, newPurpose, nil); res.code != 403 {
		t.Errorf("reader creates a purpose: %d, want 403", res.code)
	}
	if res := do("POST", "/admin/v1/consent/purposes", &alice, map[string]any{"code": "X"}, nil); res.code != 400 {
		t.Errorf("invalid body: %d, want 400", res.code)
	}
	if res := do("POST", "/admin/v1/consent/purposes", &alice, newPurpose, nil); res.code != 201 || res.body["code"] != "NEWS" || res.body["id"] == nil {
		t.Fatalf("create purpose: %d %v", res.code, res.body)
	}

	news := f.LivePurpose(t, "NEWSLETTER", consenttest.Content("จดหมายข่าว", "ยินยอมรับจดหมายข่าว"))
	cp := f.LiveCP(t, "SIGNUP", consent.CPPurposeInput{PurposeID: news.ID})
	cpPath := "/admin/v1/consent/collection-points/" + cp.ID.String()
	if res := do("GET", "/admin/v1/consent/purposes", &alice, nil, nil); res.code == 200 {
		found := false
		for _, it := range res.body["data"].([]any) {
			m := it.(map[string]any)
			if m["code"] == "NEWSLETTER" {
				found = true
				live := m["live"].(map[string]any)
				if live["consent_text"].(map[string]any)["th"] != "ยินยอมรับจดหมายข่าว" || len(m["versions"].([]any)) != 1 {
					t.Errorf("listed purpose without its live text or versions: %v", m)
				}
			}
		}
		if !found {
			t.Error("published purpose not listed")
		}
	}
	if res := do("GET", "/admin/v1/consent/collection-points", &alice, nil, nil); res.code != 200 {
		t.Errorf("list collection points: %d", res.code)
	} else {
		items, _ := res.body["data"].([]any)
		ps, _ := items[0].(map[string]any)["purposes"].([]any)
		if len(ps) != 1 || ps[0].(map[string]any)["current_version"] != float64(1) {
			t.Errorf("listed collection point without its purposes' versions: %v", items[0])
		}
	}
	update := map[string]any{"name": "สมัครสมาชิก", "channel": "web", "legal_entity_id": f.EntityA, "purposes": []map[string]any{{"purpose_id": news.ID, "required": false}}}
	if res := do("PUT", cpPath, &alice, update, nil); res.code != 428 {
		t.Errorf("update without If-Match: %d, want 428", res.code)
	}
	if res := do("PUT", cpPath, &alice, update, map[string]string{"If-Match": `"99"`}); res.code != 412 {
		t.Errorf("stale If-Match: %d, want 412", res.code)
	}
	if res := do("GET", "/admin/v1/consent/collection-points/"+uuid.NewString(), &alice, nil, nil); res.code != 404 {
		t.Errorf("unknown collection point: %d, want 404", res.code)
	}
	draft := do("POST", "/admin/v1/consent/collection-points", &alice, map[string]any{"code": "EMPTY", "name": "ว่าง", "channel": "web", "legal_entity_id": f.EntityA, "purposes": []any{}}, nil)
	if draft.code != 201 {
		t.Fatalf("create collection point: %d %v", draft.code, draft.body)
	}
	pub := do("POST", "/admin/v1/consent/collection-points/"+draft.body["id"].(string)+"/publish", &alice,
		map[string]bool{"separate_text": true, "not_bundled": true, "plain_language": true, "withdrawal_info": false}, map[string]string{"If-Match": draft.hdr.Get("ETag")})
	if pub.code != 422 || pub.body["code"] != "consent.publish_checks" || !strings.Contains(strings.Join(errCodes(pub.body), ","), "checks:no_purposes") {
		t.Errorf("publish an empty form: %d %v", pub.code, pub.body)
	}

	// ---- public ----
	formPath := "/public/v1/collection-points/" + cp.PublicKey
	if res := do("GET", "/public/v1/collection-points/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", nil, nil, nil); res.code != 404 {
		t.Errorf("unknown key: %d, want 404", res.code)
	}
	form := do("GET", formPath, nil, nil, map[string]string{"Accept-Language": "en"})
	if form.code != 200 || form.body["code"] != "SIGNUP" {
		t.Fatalf("form: %d %v", form.code, form.body)
	}
	if ps, _ := form.body["purposes"].([]any); len(ps) != 1 || ps[0].(map[string]any)["text"] != "ยินยอมรับจดหมายข่าว (en)" {
		t.Errorf("form purposes (en): %v", form.body["purposes"])
	}
	submit := map[string]any{"subject": map[string]any{"identifiers": []map[string]string{{"type": "email", "value": "Somchai@Example.com"}}},
		"decisions": []map[string]any{{"purpose_code": "NEWSLETTER", "purpose_version_no": 1, "decision": "CONSENTED"}}}
	keyHdr := func(idem string) map[string]string {
		return map[string]string{"X-Public-Key": cp.PublicKey, "Idempotency-Key": idem}
	}
	if res := do("POST", "/public/v1/consents", nil, submit, map[string]string{"X-Public-Key": cp.PublicKey}); res.code != 428 {
		t.Errorf("no Idempotency-Key: %d, want 428", res.code)
	}
	if res := do("POST", "/public/v1/consents", nil, submit, map[string]string{"X-Public-Key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "Idempotency-Key": uuid.NewString()}); res.code != 404 {
		t.Errorf("unknown key: %d, want 404", res.code)
	}
	idemKey := uuid.NewString()
	first := do("POST", "/public/v1/consents", nil, submit, keyHdr(idemKey))
	if first.code != 201 || first.body["receipt_no"] == nil {
		t.Fatalf("submit: %d %v", first.code, first.body)
	}
	replay := do("POST", "/public/v1/consents", nil, submit, keyHdr(idemKey))
	if replay.code != 201 || replay.hdr.Get("Idempotent-Replayed") != "true" || replay.body["receipt_no"] != first.body["receipt_no"] {
		t.Errorf("replay: %d %v %v", replay.code, replay.hdr, replay.body)
	}
	stale := map[string]any{"subject": submit["subject"], "decisions": []map[string]any{{"purpose_code": "NEWSLETTER", "purpose_version_no": 7, "decision": "CONSENTED"}}}
	if res := do("POST", "/public/v1/consents", nil, stale, keyHdr(uuid.NewString())); res.code != 422 || res.body["code"] != "consent.invalid_decisions" ||
		!strings.Contains(strings.Join(errCodes(res.body), ","), "stale_version") {
		t.Errorf("stale version: %d %v", res.code, res.body)
	}
	withdraw := map[string]any{"subject": submit["subject"], "decisions": []map[string]any{{"purpose_code": "NEWSLETTER", "purpose_version_no": 1, "decision": "WITHDRAWN"}}}
	if res := do("POST", "/public/v1/consents", nil, withdraw, keyHdr(uuid.NewString())); res.code != 422 {
		t.Errorf("public withdrawal: %d, want 422", res.code)
	}

	// Allowed origins: another site's browser can't use the key.
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		cur, err := f.Svc.GetCollectionPoint(ctx, cp.ID)
		if err != nil {
			return err
		}
		_, err = f.Svc.UpdateCollectionPoint(ctx, cp.ID, cur.RowVersion, consent.CollectionPointInput{Name: cur.Name, Channel: cur.Channel, LegalEntityID: cur.LegalEntityID,
			AllowedOrigins: []string{"https://shop.example"}, Purposes: []consent.CPPurposeInput{{PurposeID: news.ID}}})
		return err
	})
	evil := keyHdr(uuid.NewString())
	evil["Origin"] = "https://evil.example"
	if res := do("POST", "/public/v1/consents", nil, submit, evil); res.code != 403 {
		t.Errorf("foreign origin: %d, want 403", res.code)
	}
	good := keyHdr(uuid.NewString())
	good["Origin"] = "https://shop.example"
	if res := do("POST", "/public/v1/consents", nil, submit, good); res.code != 201 {
		t.Errorf("allowed origin: %d %v", res.code, res.body)
	}

	// Staff record on behalf, visible on the profile.
	staff := do("POST", "/admin/v1/consent/records", &alice, map[string]any{"collection_point_id": cp.ID,
		"identifiers": []map[string]string{{"type": "email", "value": "somchai@example.com"}},
		"decisions":   []map[string]any{{"purpose_code": "NEWSLETTER", "purpose_version_no": 1, "decision": "WITHDRAWN", "reason_code": "too_many_messages"}}}, nil)
	if staff.code != 201 || staff.body["subject_ref"] != first.body["subject_ref"] {
		t.Fatalf("record on behalf: %d %v", staff.code, staff.body)
	}
	if res := do("POST", "/admin/v1/consent/records", &dpoLess, map[string]any{"collection_point_id": cp.ID, "subject_id": first.body["subject_ref"],
		"decisions": []map[string]any{{"purpose_code": "NEWSLETTER", "purpose_version_no": 1, "decision": "CONSENTED"}}}, nil); res.code != 403 {
		t.Errorf("reader records on behalf: %d, want 403", res.code)
	}
	prof := do("GET", "/admin/v1/consent/subjects/"+first.body["subject_ref"].(string), &alice, nil, nil)
	if prof.code != 200 {
		t.Fatalf("profile: %d %v", prof.code, prof.body)
	}
	if b, _ := json.Marshal(prof.body); strings.Contains(string(b), "somchai@example.com") {
		t.Errorf("profile shows an unmasked identifier: %s", b)
	}
	if res := do("POST", "/admin/v1/consent/subjects/"+first.body["subject_ref"].(string)+"/verify", &alice, nil, nil); res.code != 200 || res.body["ok"] != true {
		t.Errorf("verify: %d %v", res.code, res.body)
	}

	// ---- CON-17: where consent data is stored and how identifiers are protected ----
	if res := do("GET", "/admin/v1/consent/settings", nil, nil, nil); res.code != 401 {
		t.Errorf("settings, no principal: %d, want 401", res.code)
	}
	if res := do("GET", "/admin/v1/consent/settings", &dpoLess, nil, nil); res.code != 403 {
		t.Errorf("settings, reader without consent.record.read: %d, want 403", res.code)
	}
	if res := do("GET", "/admin/v1/consent/settings", &alice, nil, nil); res.code != 200 || res.body["data_region"] != "TH" || res.body["identifier_encryption"] != "aes-256-gcm" {
		t.Errorf("settings: %d %v, want data_region TH and identifier_encryption aes-256-gcm", res.code, res.body)
	}

	// ---- CON-20: consent expiry and renewal — the near-expiry list ----
	lifespan := 10
	nearExpiryContent := consenttest.Content("NEAR-EXP", "ยินยอมที่มีอายุ")
	nearExpiryContent.LifespanDays = &lifespan
	nearExpiryPurpose := f.LivePurpose(t, "NEAR-EXP", nearExpiryContent)
	nearExpiryCP := f.LiveCP(t, "NEAR-EXP-CP", consent.CPPurposeInput{PurposeID: nearExpiryPurpose.ID})
	nearExpirySubmit := map[string]any{
		"subject":   map[string]any{"identifiers": []map[string]string{{"type": "email", "value": "near-expiry@example.com"}}},
		"decisions": []map[string]any{{"purpose_code": "NEAR-EXP", "purpose_version_no": 1, "decision": "CONSENTED"}},
	}
	nearExpiryHdr := map[string]string{"X-Public-Key": nearExpiryCP.PublicKey, "Idempotency-Key": uuid.NewString()}
	if res := do("POST", "/public/v1/consents", nil, nearExpirySubmit, nearExpiryHdr); res.code != 201 {
		t.Fatalf("near-expiry submission: %d %v", res.code, res.body)
	}
	if res := do("GET", "/admin/v1/consent/near-expiry", nil, nil, nil); res.code != 401 {
		t.Errorf("near-expiry, no principal: %d, want 401", res.code)
	}
	if res := do("GET", "/admin/v1/consent/near-expiry", &dpoLess, nil, nil); res.code != 403 {
		t.Errorf("near-expiry, reader without consent.record.read: %d, want 403", res.code)
	}
	res := do("GET", "/admin/v1/consent/near-expiry?within_days=30", &alice, nil, nil)
	if res.code != 200 {
		t.Fatalf("near-expiry: %d %v", res.code, res.body)
	}
	nearExpiryData, _ := res.body["data"].([]any)
	foundNearExpiry := false
	for _, it := range nearExpiryData {
		m, _ := it.(map[string]any)
		if m["purpose_code"] == "NEAR-EXP" {
			foundNearExpiry = true
		}
	}
	if !foundNearExpiry {
		t.Errorf("near-expiry: expected NEAR-EXP in %v", nearExpiryData)
	}

	// ---- CON-11: guardian consent (ม.20) ----
	minorContent := consenttest.Content("MKT-MINOR", "ยินยอมรับการตลาด")
	minAge := 13
	minorContent.MinAge = &minAge
	minorPurpose := f.LivePurpose(t, "MKT-MINOR", minorContent)
	minorCP := f.LiveCP(t, "MINOR-SIGNUP", consent.CPPurposeInput{PurposeID: minorPurpose.ID})
	minorSubmit := map[string]any{
		"subject": map[string]any{
			"identifiers": []map[string]string{{"type": "email", "value": "minor@example.com"}},
			"is_minor":    true,
			"guardian": map[string]any{
				"identifiers":  []map[string]string{{"type": "email", "value": "parent@example.com"}},
				"relationship": "parent",
			},
		},
		"decisions": []map[string]any{{"purpose_code": "MKT-MINOR", "purpose_version_no": 1, "decision": "CONSENTED"}},
	}
	minorKeyHdr := map[string]string{"X-Public-Key": minorCP.PublicKey, "Idempotency-Key": uuid.NewString()}
	minorRes := do("POST", "/public/v1/consents", nil, minorSubmit, minorKeyHdr)
	if minorRes.code != 201 {
		t.Fatalf("guardian submission: %d %v", minorRes.code, minorRes.body)
	}
	txs, _ := minorRes.body["transactions"].([]any)
	if len(txs) != 1 || txs[0].(map[string]any)["status"] != "PENDING" {
		t.Fatalf("expected a PENDING transaction: %v", txs)
	}
	gas, _ := minorRes.body["guardian_approvals"].([]any)
	if len(gas) != 1 {
		t.Fatalf("expected one guardian_approvals entry: %v", minorRes.body)
	}
	ga := gas[0].(map[string]any)
	code := guardianNotifier.lastCode()
	if code == "" {
		t.Fatal("expected an OTP to have been sent to the guardian")
	}
	verifyPath := "/public/v1/consents/guardian-approvals/" + ga["id"].(string) + "/verify"
	if res := do("POST", verifyPath, nil, map[string]any{"verification_id": ga["verification_id"], "code": "000000"}, minorKeyHdr); res.code == 200 {
		t.Errorf("wrong code should not confirm: %d %v", res.code, res.body)
	}
	verify := do("POST", verifyPath, nil, map[string]any{"verification_id": ga["verification_id"], "code": code}, map[string]string{"X-Public-Key": minorCP.PublicKey, "Idempotency-Key": uuid.NewString()})
	if verify.code != 200 || verify.body["status"] != "approved" {
		t.Fatalf("guardian verify: %d %v", verify.code, verify.body)
	}
	confirmed, _ := verify.body["confirmed_purposes"].([]any)
	if len(confirmed) != 1 || confirmed[0] != "MKT-MINOR" {
		t.Errorf("confirmed_purposes = %v, want [MKT-MINOR]", confirmed)
	}

	// ---- CON-19: identity verification required before recording ----
	otpPurpose := f.LivePurpose(t, "MKT-OTP", consenttest.Content("MKT-OTP", "ยินยอมรับข่าวสาร"))
	otpDraft := do("POST", "/admin/v1/consent/collection-points", &alice, map[string]any{"code": "OTP-SIGNUP", "name": "สมัครยืนยันตัวตน", "channel": "web",
		"legal_entity_id": f.EntityA, "verification_method": "otp", "purposes": []any{map[string]any{"purpose_id": otpPurpose.ID, "required": false}}}, nil)
	if otpDraft.code != 201 || otpDraft.body["verification_method"] != "otp" {
		t.Fatalf("create otp collection point: %d %v", otpDraft.code, otpDraft.body)
	}
	otpPub := do("POST", "/admin/v1/consent/collection-points/"+otpDraft.body["id"].(string)+"/publish", &alice,
		map[string]any{"separate_text": true, "not_bundled": true, "plain_language": true, "withdrawal_info": true}, map[string]string{"If-Match": otpDraft.hdr.Get("ETag")})
	if otpPub.code != 200 {
		t.Fatalf("publish otp collection point: %d %v", otpPub.code, otpPub.body)
	}
	otpKey := otpPub.body["public_key"].(string)
	otpSubmit := map[string]any{
		"subject":   map[string]any{"identifiers": []map[string]string{{"type": "email", "value": "otpuser@example.com"}}},
		"decisions": []map[string]any{{"purpose_code": "MKT-OTP", "purpose_version_no": 1, "decision": "CONSENTED"}},
	}
	otpKeyHdr := map[string]string{"X-Public-Key": otpKey, "Idempotency-Key": uuid.NewString()}

	// Recording without verification is refused.
	if res := do("POST", "/public/v1/consents", nil, otpSubmit, otpKeyHdr); res.code != 422 {
		t.Errorf("otp submission with no verification: %d %v, want 422", res.code, res.body)
	}

	// Start verification, then record with the right code: accepted.
	startRes := do("POST", "/public/v1/consents/start-verification", nil, map[string]any{"identifier": map[string]string{"type": "email", "value": "otpuser@example.com"}},
		map[string]string{"X-Public-Key": otpKey, "Idempotency-Key": uuid.NewString()})
	if startRes.code != 201 || startRes.body["channel"] != "email" {
		t.Fatalf("start verification: %d %v", startRes.code, startRes.body)
	}
	otpCode := guardianNotifier.lastCode()
	if otpCode == "" {
		t.Fatal("expected an OTP to have been sent for the consent verification")
	}
	otpSubmit["verification"] = map[string]any{"verification_id": startRes.body["verification_id"], "code": otpCode}
	if res := do("POST", "/public/v1/consents", nil, otpSubmit, map[string]string{"X-Public-Key": otpKey, "Idempotency-Key": uuid.NewString()}); res.code != 201 {
		t.Errorf("otp submission with verification: %d %v", res.code, res.body)
	}

	// An unknown public key refuses to start a verification (404), not leaking whether a collection point exists.
	if res := do("POST", "/public/v1/consents/start-verification", nil, map[string]any{"identifier": map[string]string{"type": "email", "value": "x@example.com"}},
		map[string]string{"X-Public-Key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "Idempotency-Key": uuid.NewString()}); res.code != 404 {
		t.Errorf("start verification, unknown key: %d, want 404", res.code)
	}

	// ---- CON-18: self-service preference centre ----
	pcPath := "/admin/v1/consent/preference-centers/" + f.EntityA.String()
	if res := do("GET", pcPath, &alice, nil, nil); res.code != 200 || res.body["public_key"] != nil {
		t.Fatalf("preference centre link before publish: %d %v, want public_key: null", res.code, res.body)
	}
	if res := do("GET", pcPath, nil, nil, nil); res.code != 401 {
		t.Errorf("no principal: %d, want 401", res.code)
	}
	if res := do("POST", pcPath+"/publish", &dpoLess, nil, nil); res.code != 403 {
		t.Errorf("reader publishes preference centre: %d, want 403", res.code)
	}
	pub1 := do("POST", pcPath+"/publish", &alice, nil, nil)
	if pub1.code != 200 || pub1.body["public_key"] == nil {
		t.Fatalf("publish preference centre: %d %v", pub1.code, pub1.body)
	}
	pcKey, _ := pub1.body["public_key"].(string)
	pub2 := do("POST", pcPath+"/publish", &alice, nil, nil)
	if pub2.code != 200 || pub2.body["public_key"] != pcKey {
		t.Fatalf("re-publish should keep the same key: %v vs %v", pub2.body["public_key"], pcKey)
	}

	pcPurpose := f.LivePurpose(t, "MKT-PC", consenttest.Content("MKT-PC", "ยินยอมรับข่าวสาร"))
	f.LiveCP(t, "PC-SIGNUP", consent.CPPurposeInput{PurposeID: pcPurpose.ID})

	startPath := "/public/v1/preference-centers/" + pcKey + "/start-verification"
	if res := do("POST", "/public/v1/preference-centers/unknown-key/start-verification",
		nil, map[string]any{"identifier": map[string]string{"type": "email", "value": "pc@example.com"}}, map[string]string{"Idempotency-Key": uuid.NewString()}); res.code != 404 {
		t.Errorf("start-verification with an unknown key: %d, want 404", res.code)
	}
	startRes2 := do("POST", startPath, nil, map[string]any{"identifier": map[string]string{"type": "email", "value": "pc@example.com"}}, map[string]string{"Idempotency-Key": uuid.NewString()})
	if startRes2.code != 201 || startRes2.body["channel"] != "email" {
		t.Fatalf("preference-centre start-verification: %d %v", startRes2.code, startRes2.body)
	}
	pcCode := guardianNotifier.lastCode()
	if pcCode == "" {
		t.Fatal("expected an OTP to have been sent for the preference centre")
	}

	viewBody := map[string]any{"verification_id": startRes2.body["verification_id"], "code": pcCode, "identifier": map[string]string{"type": "email", "value": "pc@example.com"}}
	viewPath := "/public/v1/preference-centers/" + pcKey + "/view"
	viewRes := do("POST", viewPath, nil, viewBody, map[string]string{"Idempotency-Key": uuid.NewString()})
	if viewRes.code != 200 {
		t.Fatalf("view preference centre: %d %v", viewRes.code, viewRes.body)
	}
	purposes, _ := viewRes.body["purposes"].([]any)
	found := false
	for _, p := range purposes {
		m, _ := p.(map[string]any)
		if m["code"] == "MKT-PC" {
			found = true
			if m["decision"] != nil {
				t.Errorf("fresh purpose already has a decision: %v", m["decision"])
			}
		}
	}
	if !found {
		t.Fatalf("expected MKT-PC among the preference centre's purposes, got %v", purposes)
	}

	// The same OTP code already verified covers the update too — no need to resend it.
	updatePath := "/public/v1/preference-centers/" + pcKey + "/update"
	grantBody := map[string]any{"verification_id": startRes2.body["verification_id"], "identifier": map[string]string{"type": "email", "value": "pc@example.com"},
		"decisions": []map[string]any{{"purpose_code": "MKT-PC", "purpose_version_no": 1, "decision": "CONSENTED"}}}
	grantRes := do("POST", updatePath, nil, grantBody, map[string]string{"Idempotency-Key": uuid.NewString()})
	if grantRes.code != 201 || grantRes.body["receipt_no"] == nil {
		t.Fatalf("grant via preference centre: %d %v", grantRes.code, grantRes.body)
	}

	// Withdrawal, unlike a collection-point submission, is allowed from the preference centre.
	withdrawBody := map[string]any{"verification_id": startRes2.body["verification_id"], "identifier": map[string]string{"type": "email", "value": "pc@example.com"},
		"decisions": []map[string]any{{"purpose_code": "MKT-PC", "purpose_version_no": 1, "decision": "WITHDRAWN"}}}
	withdrawRes := do("POST", updatePath, nil, withdrawBody, map[string]string{"Idempotency-Key": uuid.NewString()})
	if withdrawRes.code != 201 {
		t.Fatalf("withdraw via preference centre: %d %v", withdrawRes.code, withdrawRes.body)
	}

	// Retire (needs the delete permission, not already in alice's Maker set for this fixture).
	grants[alice.String()] = append(append([]string{}, consenttest.Maker...), "consent.collectionpoint.delete")
	if err := cache.Invalidate(ctx, f.A.ID.String(), alice.String()); err != nil {
		t.Fatal(err)
	}
	if res := do("POST", pcPath+"/retire", &alice, nil, nil); res.code != 204 {
		t.Fatalf("retire preference centre: %d %v", res.code, res.body)
	}
	if res := do("GET", pcPath, &alice, nil, nil); res.code != 200 || res.body["public_key"] != nil {
		t.Fatalf("preference centre link after retire: %d %v, want public_key: null", res.code, res.body)
	}
	if res := do("POST", startPath, nil, map[string]any{"identifier": map[string]string{"type": "email", "value": "pc@example.com"}}, map[string]string{"Idempotency-Key": uuid.NewString()}); res.code != 404 {
		t.Errorf("start-verification with a retired key: %d, want 404", res.code)
	}

	// ---- CON-23: double opt-in ----
	doiPurpose := f.LivePurpose(t, "MKT-DOI-C", consenttest.Content("MKT-DOI-C", "ยินยอมรับข่าวสาร"))
	doiDraft := do("POST", "/admin/v1/consent/collection-points", &alice, map[string]any{"code": "DOI-C-SIGNUP", "name": "สมัครแบบยืนยันซ้ำ", "channel": "web",
		"legal_entity_id": f.EntityA, "double_opt_in": true, "purposes": []any{map[string]any{"purpose_id": doiPurpose.ID, "required": false}}}, nil)
	if doiDraft.code != 201 || doiDraft.body["double_opt_in"] != true {
		t.Fatalf("create double opt-in collection point: %d %v", doiDraft.code, doiDraft.body)
	}
	doiPub := do("POST", "/admin/v1/consent/collection-points/"+doiDraft.body["id"].(string)+"/publish", &alice,
		map[string]any{"separate_text": true, "not_bundled": true, "plain_language": true, "withdrawal_info": true}, map[string]string{"If-Match": doiDraft.hdr.Get("ETag")})
	if doiPub.code != 200 {
		t.Fatalf("publish double opt-in collection point: %d %v", doiPub.code, doiPub.body)
	}
	doiKey := doiPub.body["public_key"].(string)
	doiSubmit := do("POST", "/public/v1/consents", nil, map[string]any{
		"subject":   map[string]any{"identifiers": []map[string]string{{"type": "email", "value": "doi-http@example.com"}}},
		"decisions": []map[string]any{{"purpose_code": "MKT-DOI-C", "purpose_version_no": 1, "decision": "CONSENTED"}},
	}, map[string]string{"X-Public-Key": doiKey, "Idempotency-Key": uuid.NewString()})
	if doiSubmit.code != 201 {
		t.Fatalf("double opt-in submission: %d %v", doiSubmit.code, doiSubmit.body)
	}
	doiTxs, _ := doiSubmit.body["transactions"].([]any)
	if len(doiTxs) != 1 || doiTxs[0].(map[string]any)["status"] != "PENDING" {
		t.Fatalf("expected a PENDING transaction, got %v", doiTxs)
	}
	doiReqs, _ := doiSubmit.body["double_opt_in_requests"].([]any)
	if len(doiReqs) != 1 {
		t.Fatalf("expected one double_opt_in_requests entry, got %v", doiSubmit.body)
	}

	doiToken := lastDoubleOptInToken(t, f)
	if doiToken == "" {
		t.Fatal("expected a double opt-in token to have been sent")
	}
	confirmPath := "/public/v1/consents/double-opt-in/confirm"
	if res := do("POST", confirmPath, nil, map[string]any{"token": "wrong-token-wrong-token-wrong-token"}, map[string]string{"X-Public-Key": doiKey, "Idempotency-Key": uuid.NewString()}); res.code != 404 {
		t.Errorf("wrong token: %d, want 404", res.code)
	}
	doiConfirm := do("POST", confirmPath, nil, map[string]any{"token": doiToken}, map[string]string{"X-Public-Key": doiKey, "Idempotency-Key": uuid.NewString()})
	if doiConfirm.code != 200 || doiConfirm.body["status"] != "confirmed" || doiConfirm.body["purpose_code"] != "MKT-DOI-C" {
		t.Fatalf("confirm double opt-in: %d %v", doiConfirm.code, doiConfirm.body)
	}
	// A redelivered confirm with the same token is a harmless no-op, not a 200 again.
	if res := do("POST", confirmPath, nil, map[string]any{"token": doiToken}, map[string]string{"X-Public-Key": doiKey, "Idempotency-Key": uuid.NewString()}); res.code == 200 {
		t.Errorf("second confirm with the same token should not succeed again: %d %v", res.code, res.body)
	}
}

// lastDoubleOptInToken decrypts the most recently queued consent.double_opt_in notification's own "link"
// variable and returns its token query parameter — the same white-box technique the service-level test
// uses, since the token is never otherwise exposed by any API response (by design).
func lastDoubleOptInToken(t *testing.T, f *consenttest.Fixture) string {
	t.Helper()
	var token string
	err := pdb.WithTenantTx(context.Background(), f.App, f.A.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		var payloadJSON []byte
		if err := tx.QueryRow(ctx, `SELECT n.payload FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id
			WHERE t.code = 'consent.double_opt_in' ORDER BY n.created_at DESC LIMIT 1`).Scan(&payloadJSON); err != nil {
			return err
		}
		var p struct {
			VarsEnc string `json:"vars_enc"`
		}
		if err := json.Unmarshal(payloadJSON, &p); err != nil {
			return err
		}
		varsEnc, err := base64.StdEncoding.DecodeString(p.VarsEnc)
		if err != nil {
			return err
		}
		plain, err := f.Svc.Keyring.Decrypt(ctx, "notification", "platform.notifications.payload.vars_enc", varsEnc)
		if err != nil {
			return err
		}
		var vars map[string]any
		if err := json.Unmarshal(plain, &vars); err != nil {
			return err
		}
		link, _ := vars["link"].(string)
		u, err := url.Parse(link)
		if err != nil {
			return err
		}
		token = u.Query().Get("token")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// fakeIamNotifier captures the OTP a StartVerification call would have sent (PLT-04), so the contract test can
// read it without a real SMTP/SMS sender.
type fakeIamNotifier struct{ sent []iamservice.NotifyRequest }

func (f *fakeIamNotifier) Send(ctx context.Context, req iamservice.NotifyRequest) (uuid.UUID, error) {
	f.sent = append(f.sent, req)
	return uuid.New(), nil
}

func (f *fakeIamNotifier) lastCode() string {
	if len(f.sent) == 0 {
		return ""
	}
	code, _ := f.sent[len(f.sent)-1].Vars["code"].(string)
	return code
}
