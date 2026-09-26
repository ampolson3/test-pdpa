package breachhttp_test

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
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"pdpa-platform/internal/breach/breachtest"
	breachhttp "pdpa-platform/internal/breach/http"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test of the breach endpoints behind the real OpenAPI validator, AuthZ and a tenant transaction.
func TestBreachEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	f := breachtest.Setup(t)
	form := f.AssessmentForm(t)

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
	grants := map[string][]string{f.Sec.String(): breachtest.SEC, f.DPO.String(): breachtest.DPO, f.DPO2.String(): breachtest.DPO,
		f.Employee.String(): breachtest.Employee, f.Exec.String(): {"breach.incident.read"}}
	cache := authz.NewCachedLoader(rdb, func(_ context.Context, tid, uid string) (authz.Grants, error) {
		return authz.Grants{TenantID: tid, UserID: uid, Permissions: grants[uid]}, nil
	})
	t.Cleanup(func() {
		for uid := range grants {
			_ = cache.Invalidate(context.Background(), f.A.ID.String(), uid)
		}
	})
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
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			p, ok := httpx.PrincipalFromContext(req.Context())
			if !ok {
				next.ServeHTTP(w, req)
				return
			}
			_ = pdb.WithTenantTx(req.Context(), f.App, p.TenantID, p.UserID, func(ctx context.Context) error {
				next.ServeHTTP(w, req.WithContext(ctx))
				return nil
			})
		})
	})
	strict := breachhttp.NewStrictHandlerWithOptions(breachhttp.NewStrict(f.Svc),
		[]breachhttp.StrictMiddlewareFunc{authz.StrictMiddleware[breachhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		breachhttp.StrictHTTPServerOptions{
			RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				httpx.WriteProblem(w, r, httpx.RequestInvalid(err.Error()))
			},
			ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				if p, ok := err.(httpx.Problem); ok {
					httpx.WriteProblem(w, r, p)
					return
				}
				t.Logf("handler error: %v", err)
				httpx.WriteProblem(w, r, httpx.Internal())
			},
		})
	breachhttp.HandlerWithOptions(strict, breachhttp.ChiServerOptions{BaseRouter: r})
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
	sec, dpo, dpo2, emp, exec := f.Sec, f.DPO, f.DPO2, f.Employee, f.Exec
	incident := map[string]any{"legal_entity_id": f.EntityA, "reported_via": "email", "title": "ไฟล์ลูกค้ารั่ว", "description": "พบไฟล์บนเว็บสาธารณะ",
		"breach_types": []string{"confidentiality"}, "aware_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339), "affected_subjects": 120}

	if res := do("GET", "/admin/v1/breach/incidents", nil, nil, nil); res.code != 401 {
		t.Errorf("no principal: %d", res.code)
	}
	if res := do("POST", "/admin/v1/breach/incidents", &exec, incident, nil); res.code != 403 {
		t.Errorf("executive creating: %d, want 403", res.code)
	}
	if res := do("POST", "/admin/v1/breach/incidents", &sec, map[string]any{"title": "x"}, nil); res.code != 400 {
		t.Errorf("invalid body: %d, want 400", res.code)
	}
	bad := map[string]any{}
	for k, v := range incident {
		bad[k] = v
	}
	bad["aware_at"] = time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	if res := do("POST", "/admin/v1/breach/incidents", &sec, bad, nil); res.code != 422 || res.body["code"] != "breach.invalid" {
		t.Errorf("future awareness: %d %v", res.code, res.body)
	}
	created := do("POST", "/admin/v1/breach/incidents", &sec, incident, nil)
	if created.code != 201 || created.hdr.Get("ETag") == "" || created.body["status"] != "reported" || created.body["clock"].(map[string]any)["state"] != "on_track" {
		t.Fatalf("create: %d %v", created.code, created.body)
	}
	id := created.body["id"].(string)
	path := "/admin/v1/breach/incidents/" + id
	if res := do("GET", "/admin/v1/breach/incidents/"+uuid.NewString(), &sec, nil, nil); res.code != 404 {
		t.Errorf("unknown: %d", res.code)
	}
	if res := do("GET", path, &emp, nil, nil); res.code != 404 {
		t.Errorf("reporter-only reading another's incident: %d, want 404", res.code)
	}
	if res := do("GET", "/admin/v1/breach/incidents?q=ลูกค้า&status=reported", &exec, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 1 {
		t.Errorf("search: %d %v", res.code, res.body)
	}
	if res := do("POST", path+"/transitions", &sec, map[string]any{"to": "triage"}, nil); res.code != 428 {
		t.Errorf("no If-Match: %d, want 428", res.code)
	}
	if res := do("POST", path+"/transitions", &sec, map[string]any{"to": "triage"}, map[string]string{"If-Match": `"99"`}); res.code != 412 {
		t.Errorf("stale If-Match: %d, want 412", res.code)
	}
	if res := do("POST", path+"/transitions", &sec, map[string]any{"to": "notifying"}, map[string]string{"If-Match": created.hdr.Get("ETag")}); res.code != 409 || res.body["code"] != "breach.invalid_transition" {
		t.Errorf("reported → notifying: %d %v", res.code, res.body)
	}
	tr := do("POST", path+"/transitions", &sec, map[string]any{"to": "triage"}, map[string]string{"If-Match": created.hdr.Get("ETag")})
	tr = do("POST", path+"/transitions", &sec, map[string]any{"to": "assessing"}, map[string]string{"If-Match": tr.hdr.Get("ETag")})
	if tr.code != 200 || tr.body["status"] != "assessing" {
		t.Fatalf("to assessing: %d %v", tr.code, tr.body)
	}
	as := do("POST", path+"/assessments", &sec, map[string]any{"form_id": form, "answers": map[string]any{"sensitive": "no", "volume": "many", "encrypted": "no"}}, nil)
	if as.code != 201 || as.body["risk_level"] != "low" || len(as.body["factors"].([]any)) != 3 {
		t.Fatalf("assess: %d %v", as.code, as.body)
	}
	cur := do("GET", path, &dpo, nil, nil)
	if res := do("POST", path+"/decision", &sec, map[string]any{"decision": "notify_pdpc", "reason": "x"}, map[string]string{"If-Match": cur.hdr.Get("ETag")}); res.code != 403 {
		t.Errorf("SEC deciding: %d, want 403", res.code)
	}
	if res := do("POST", path+"/decision", &dpo, map[string]any{"decision": "no_notification", "reason": "x"}, map[string]string{"If-Match": cur.hdr.Get("ETag")}); res.code != 422 || res.body["code"] != "breach.decision_too_weak" {
		t.Errorf("weaker decision: %d %v", res.code, res.body)
	}
	dec := do("POST", path+"/decision", &dpo, map[string]any{"decision": "notify_pdpc_and_subjects", "reason": "แจ้งเจ้าของข้อมูลด้วยเพื่อความโปร่งใส"}, map[string]string{"If-Match": cur.hdr.Get("ETag")})
	if dec.code != 200 || dec.body["status"] != "notifying" || dec.body["decision"] != "notify_pdpc_and_subjects" {
		t.Fatalf("decide: %d %v", dec.code, dec.body)
	}
	if res := do("POST", path+"/timeline", &sec, map[string]any{"text": "ปิดช่องโหว่แล้ว"}, nil); res.code != 204 {
		t.Errorf("note: %d", res.code)
	}
	tl := do("GET", path+"/timeline", &exec, nil, nil)
	if tl.code != 200 || len(tl.body["data"].([]any)) < 6 {
		t.Errorf("timeline: %d %v", tl.code, tl.body)
	}
	file := f.Upload(t, sec, "evidence.txt", []byte("access log excerpt"))
	if res := do("POST", path+"/evidence", &sec, map[string]any{"file_id": file, "description": "log"}, nil); res.code != 201 || len(res.body["sha256"].(string)) != 64 {
		t.Errorf("evidence: %d %v", res.code, res.body)
	}
	// Notices: maker-checker through the API.
	vars := map[string]any{"organization": "บริษัท ทดสอบ", "summary": "ข้อมูลของท่านอาจรั่วไหล", "remedy": "เปลี่ยนรหัสผ่าน", "contact": "dpo@test.example"}
	if res := do("POST", path+"/notices", &sec, map[string]any{"channel": "email", "variables": vars}, nil); res.code != 403 {
		t.Errorf("SEC drafting a notice: %d", res.code)
	}
	n := do("POST", path+"/notices", &dpo, map[string]any{"channel": "email", "variables": vars}, nil)
	if n.code != 201 || n.body["status"] != "draft" {
		t.Fatalf("notice: %d %v", n.code, n.body)
	}
	np := "/admin/v1/breach/notices/" + n.body["id"].(string)
	csv := f.Upload(t, dpo, "people.csv", []byte("email\na@example.com\nb@example.com\n"))
	ld := do("POST", np+"/recipients", &dpo, map[string]any{"file_id": csv}, map[string]string{"If-Match": n.hdr.Get("ETag")})
	if ld.code != 200 || ld.body["total"] != float64(2) {
		t.Fatalf("recipients: %d %v", ld.code, ld.body)
	}
	if res := do("POST", np+"/send", &dpo, nil, map[string]string{"If-Match": ld.hdr.Get("ETag")}); res.code != 403 || res.body["code"] != "breach.self_approval" {
		t.Errorf("self approval: %d %v", res.code, res.body)
	}
	if res := do("POST", np+"/send", &dpo2, nil, map[string]string{"If-Match": ld.hdr.Get("ETag")}); res.code != 200 || res.body["status"] != "sending" {
		t.Errorf("send: %d %v", res.code, res.body)
	}
	if res := do("GET", np+"/recipients", &dpo, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 2 ||
		strings.Contains(res.body["data"].([]any)[0].(map[string]any)["masked"].(string), "a@example.com") {
		t.Errorf("recipients list: %d %v", res.code, res.body)
	}
	if res := do("GET", path+"/notices", &exec, nil, nil); res.code != 403 {
		t.Errorf("notices without a notification permission: %d", res.code)
	}
}
