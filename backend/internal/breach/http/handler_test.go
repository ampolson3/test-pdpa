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

	// BRE-16: a drill incident never appears in the default register, but does when asked for.
	drill := map[string]any{}
	for k, v := range incident {
		drill[k] = v
	}
	drill["title"], drill["is_drill"] = "ซ้อมรับมือเหตุละเมิดประจำปี", true
	createdDrill := do("POST", "/admin/v1/breach/incidents", &sec, drill, nil)
	if createdDrill.code != 201 || createdDrill.body["is_drill"] != true {
		t.Fatalf("create drill: %d %v", createdDrill.code, createdDrill.body)
	}
	if res := do("GET", "/admin/v1/breach/incidents?q=ซ้อมรับมือ", &exec, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 0 {
		t.Errorf("drill in default register: %d %v, want empty", res.code, res.body)
	}
	if res := do("GET", "/admin/v1/breach/incidents?q=ซ้อมรับมือ&include_drills=true", &exec, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 1 {
		t.Errorf("drill with include_drills=true: %d %v, want 1", res.code, res.body)
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

	// PDPC filing rounds (BRE-08/09): recorded against a published pdpc_form document (PLT-16), maker-checker
	// through the API, late without a reason refused.
	docVersion := f.PDPCForm(t, "แบบแจ้ง สคส. สำหรับทดสอบ")
	if res := do("POST", path+"/pdpc-notifications", &sec, map[string]any{"notification_type": "initial", "document_version_id": docVersion.String(),
		"submitted_at": time.Now().UTC().Format(time.RFC3339)}, nil); res.code != 403 {
		t.Errorf("SEC recording a PDPC round: %d", res.code)
	}
	awareAt, err := time.Parse(time.RFC3339, incident["aware_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// The late/reason rule itself (submitted_at within the 72-hour window vs. after it, with/without a reason) is
	// covered by the service-level tests with an incident aged enough to make both "on time" and "late" reachable
	// without landing in the future; this contract test only needs a valid, on-time submission to exercise the
	// wire format and maker-checker permissions.
	rec := do("POST", path+"/pdpc-notifications", &dpo, map[string]any{"notification_type": "initial", "document_version_id": docVersion.String(),
		"submitted_at": awareAt.Add(time.Hour).Format(time.RFC3339), "submission_ref": "PDPC-2026-0001"}, nil)
	if rec.code != 201 || rec.body["is_late"] != false || rec.body["sequence_no"] != float64(1) {
		t.Fatalf("record: %d %v", rec.code, rec.body)
	}
	pdpcPath := "/admin/v1/breach/pdpc-notifications/" + rec.body["id"].(string)
	if res := do("POST", pdpcPath+"/confirm", &dpo, nil, map[string]string{"If-Match": rec.hdr.Get("ETag")}); res.code != 403 || res.body["code"] != "breach.self_approval" {
		t.Errorf("self confirmation: %d %v", res.code, res.body)
	}
	if res := do("POST", pdpcPath+"/confirm", &dpo2, nil, map[string]string{"If-Match": rec.hdr.Get("ETag")}); res.code != 200 || res.body["approved_by"] == nil {
		t.Errorf("confirm: %d %v", res.code, res.body)
	}
	if res := do("GET", path+"/pdpc-notifications", &dpo, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 1 {
		t.Errorf("list: %d %v", res.code, res.body)
	}

	// Root cause & corrective actions (BRE-15).
	rcPath := path + "/root-causes"
	if res := do("GET", rcPath, nil, nil, nil); res.code != 401 {
		t.Errorf("root-causes no principal: %d", res.code)
	}
	if res := do("POST", rcPath, &exec, map[string]any{"cause_category": "process_gap", "description": "x"}, nil); res.code != 403 {
		t.Errorf("exec adding root cause: %d, want 403", res.code)
	}
	if res := do("POST", rcPath, &sec, map[string]any{"cause_category": "not_a_category", "description": "x"}, nil); res.code != 400 {
		t.Errorf("bad cause_category: %d, want 400", res.code)
	}
	addRC := do("POST", rcPath, &sec, map[string]any{
		"cause_category": "process_gap", "description": "ไม่มีการตรวจสอบสิทธิ์ก่อนส่งออกข้อมูล",
		"corrective_actions": []map[string]any{{"description": "เพิ่มการอนุมัติสองชั้น"}, {"description": "อบรมพนักงาน"}},
	}, nil)
	if addRC.code != 201 || addRC.hdr.Get("ETag") == "" || len(addRC.body["corrective_actions"].([]any)) != 2 || addRC.body["all_actions_done"] != false {
		t.Fatalf("add root cause: %d %v", addRC.code, addRC.body)
	}
	rcID := addRC.body["id"].(string)
	actions := addRC.body["corrective_actions"].([]any)
	action1 := actions[0].(map[string]any)["id"].(string)
	action2 := actions[1].(map[string]any)["id"].(string)
	if res := do("GET", rcPath, &sec, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 1 {
		t.Errorf("list root causes: %d %v", res.code, res.body)
	}
	oneRCPath := "/admin/v1/breach/root-causes/" + rcID
	if res := do("POST", oneRCPath+"/corrective-actions/"+action1+"/complete", &sec, nil, nil); res.code != 428 {
		t.Errorf("complete action no If-Match: %d, want 428", res.code)
	}
	comp := do("POST", oneRCPath+"/corrective-actions/"+action1+"/complete", &sec, nil, map[string]string{"If-Match": addRC.hdr.Get("ETag")})
	if comp.code != 200 || comp.body["all_actions_done"] != false {
		t.Fatalf("complete first action: %d %v", comp.code, comp.body)
	}
	comp2 := do("POST", oneRCPath+"/corrective-actions/"+action2+"/complete", &sec, nil, map[string]string{"If-Match": comp.hdr.Get("ETag")})
	if comp2.code != 200 || comp2.body["all_actions_done"] != true {
		t.Fatalf("complete second action: %d %v", comp2.code, comp2.body)
	}
	// Re-completing an already-done action is a harmless no-op, not an error.
	comp3 := do("POST", oneRCPath+"/corrective-actions/"+action2+"/complete", &sec, nil, map[string]string{"If-Match": comp2.hdr.Get("ETag")})
	if comp3.code != 200 || comp3.body["all_actions_done"] != true {
		t.Fatalf("re-complete second action: %d %v", comp3.code, comp3.body)
	}
	if res := do("PUT", oneRCPath, &sec, map[string]any{"cause_category": "third_party", "description": "ผู้ให้บริการภายนอก"}, map[string]string{"If-Match": `"99"`}); res.code != 412 {
		t.Errorf("update stale If-Match: %d, want 412", res.code)
	}
	upd := do("PUT", oneRCPath, &sec, map[string]any{"cause_category": "third_party", "description": "ผู้ให้บริการภายนอก"}, map[string]string{"If-Match": comp3.hdr.Get("ETag")})
	if upd.code != 200 || upd.body["cause_category"] != "third_party" {
		t.Fatalf("update root cause: %d %v", upd.code, upd.body)
	}
	if res := do("DELETE", oneRCPath+"?incident_id="+id, &exec, nil, nil); res.code != 403 {
		t.Errorf("exec deleting root cause: %d, want 403", res.code)
	}
	if res := do("DELETE", oneRCPath+"?incident_id="+id, &sec, nil, nil); res.code != 204 {
		t.Errorf("delete root cause: %d, want 204", res.code)
	}
	if res := do("GET", rcPath, &sec, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 0 {
		t.Errorf("list after delete: %d %v", res.code, res.body)
	}
}
