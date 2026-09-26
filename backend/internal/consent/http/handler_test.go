package consenthttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	"pdpa-platform/internal/pkg/authz"
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
}
