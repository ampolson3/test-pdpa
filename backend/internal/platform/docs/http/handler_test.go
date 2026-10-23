package docshttp_test

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

	"pdpa-platform/internal/platform/docs/docstest"
	docshttp "pdpa-platform/internal/platform/docs/http"
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

// Contract test of the document composer endpoints behind the real OpenAPI validator, AuthZ and a tenant transaction.
func TestDocsEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	f := docstest.Setup(t)

	spec, err := openapi3.NewLoader().LoadFromFile("../../../../../api/openapi/openapi.yaml")
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
	grants := map[string][]string{f.Priv.String(): docstest.Privacy, f.DPOUser.String(): docstest.DPO, f.Law.String(): docstest.Legal}
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
	strict := docshttp.NewStrictHandlerWithOptions(docshttp.NewStrict(f.Svc),
		[]docshttp.StrictMiddlewareFunc{authz.StrictMiddleware[docshttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		docshttp.StrictHTTPServerOptions{
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
	docshttp.HandlerWithOptions(strict, docshttp.ChiServerOptions{BaseRouter: r})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	type resp struct {
		code int
		body map[string]any
		hdr  http.Header
		raw  []byte
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
		out := resp{code: res.StatusCode, hdr: res.Header, raw: b}
		_ = json.Unmarshal(b, &out.body)
		return out
	}
	priv, dpo, law := f.Priv, f.DPOUser, f.Law
	para := func(text string) map[string]any {
		return map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}
	}
	doc := func(blocks ...any) map[string]any { return map[string]any{"type": "doc", "content": blocks} }

	if res := do("GET", "/admin/v1/platform/documents", nil, nil, nil); res.code != 401 {
		t.Errorf("no principal: %d", res.code)
	}
	if res := do("POST", "/admin/v1/platform/documents", &priv, map[string]any{"doc_type": "memo", "title": "x"}, nil); res.code != 400 {
		t.Errorf("unknown doc type enum: %d", res.code)
	}
	if res := do("POST", "/admin/v1/platform/documents", &priv, map[string]any{"doc_type": "report", "title": "x"}, nil); res.code != 422 || res.body["code"] != "docs.unknown_type" {
		t.Errorf("unoffered type: %d %v", res.code, res.body)
	}
	if res := do("POST", "/admin/v1/platform/documents", &priv, map[string]any{"doc_type": "dpa", "title": "x"}, nil); res.code != 403 {
		t.Errorf("dpa without its create permission: %d", res.code)
	}
	types := do("GET", "/admin/v1/platform/documents/types", &priv, nil, nil)
	if types.code != 200 || len(types.body["types"].([]any)) != 2 || len(types.body["fields"].([]any)) == 0 {
		t.Errorf("types: %d %v", types.code, types.body)
	}
	created := do("POST", "/admin/v1/platform/documents", &priv, map[string]any{"doc_type": "notice", "title": "ประกาศ", "legal_entity_id": f.EntityA}, nil)
	if created.code != 201 || created.hdr.Get("ETag") == "" || created.body["entity_type"] != "document_notice" || created.body["latest"] == nil {
		t.Fatalf("create: %d %v", created.code, created.body)
	}
	id := created.body["id"].(string)
	path := "/admin/v1/platform/documents/" + id
	if res := do("GET", path, &law, nil, nil); res.code != 200 {
		t.Errorf("legal reads notices: %d", res.code)
	}
	if res := do("GET", "/admin/v1/platform/documents/"+uuid.NewString(), &priv, nil, nil); res.code != 404 {
		t.Errorf("unknown: %d", res.code)
	}
	draft := map[string]any{"title": "ประกาศความเป็นส่วนตัว", "legal_entity_id": f.EntityA,
		"content": map[string]any{"th": doc(para("ข้อความภาษาไทย"), map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "mergeField", "attrs": map[string]any{"key": "org_name_th"}}}})}}
	if res := do("PUT", path+"/draft", &priv, draft, nil); res.code != 428 {
		t.Errorf("no If-Match: %d", res.code)
	}
	if res := do("PUT", path+"/draft", &priv, draft, map[string]string{"If-Match": `"99"`}); res.code != 412 {
		t.Errorf("stale If-Match: %d", res.code)
	}
	if res := do("PUT", path+"/draft", &law, draft, map[string]string{"If-Match": created.hdr.Get("ETag")}); res.code != 403 {
		t.Errorf("legal edits a notice: %d", res.code)
	}
	bad := map[string]any{"title": "x", "content": map[string]any{"th": doc(map[string]any{"type": "iframe"})}}
	if res := do("PUT", path+"/draft", &priv, bad, map[string]string{"If-Match": created.hdr.Get("ETag")}); res.code != 422 || res.body["code"] != "docs.invalid" {
		t.Errorf("unknown node: %d %v", res.code, res.body)
	}
	saved := do("PUT", path+"/draft", &priv, draft, map[string]string{"If-Match": created.hdr.Get("ETag")})
	if saved.code != 200 || saved.hdr.Get("ETag") == created.hdr.Get("ETag") || saved.body["missing"] != nil {
		t.Fatalf("save: %d %v", saved.code, saved.body)
	}
	exp := do("GET", path+"/export?language=th&format=docx", &priv, nil, nil)
	if exp.code != 200 || !strings.HasPrefix(exp.hdr.Get("Content-Type"), "application/vnd.openxmlformats") ||
		!strings.Contains(exp.hdr.Get("Content-Disposition"), "attachment") || !bytes.HasPrefix(exp.raw, []byte("PK")) {
		t.Errorf("export: %d %v %q", exp.code, exp.hdr, exp.raw[:min(len(exp.raw), 40)])
	}
	if res := do("GET", path+"/export?language=fr&format=docx", &priv, nil, nil); res.code != 400 {
		t.Errorf("bad language: %d", res.code)
	}
	latest := saved.body["latest"].(map[string]any)["id"].(string)
	cmp := do("GET", path+"/compare?from="+latest+"&to="+latest+"&language=th", &priv, nil, nil)
	if cmp.code != 200 || cmp.body["summary"].(map[string]any)["equal"] != float64(2) {
		t.Errorf("compare: %d %v", cmp.code, cmp.body)
	}
	if res := do("GET", path+"/compare?from="+uuid.NewString()+"&to="+latest+"&language=th", &priv, nil, nil); res.code != 404 {
		t.Errorf("compare with a foreign version: %d", res.code)
	}
	if res := do("GET", path+"/published", &priv, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 0 {
		t.Errorf("published: %d %v", res.code, res.body)
	}
	do("POST", "/admin/v1/platform/documents", &priv, map[string]any{"doc_type": "notice", "title": "second"}, nil)
	page := do("GET", "/admin/v1/platform/documents?limit=1", &priv, nil, nil)
	if page.code != 200 || len(page.body["data"].([]any)) != 1 || page.body["next_cursor"] == nil {
		t.Fatalf("page 1: %d %v", page.code, page.body)
	}
	if res := do("GET", "/admin/v1/platform/documents?limit=1&cursor="+page.body["next_cursor"].(string), &priv, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 1 || res.body["next_cursor"] != nil {
		t.Errorf("page 2: %d %v", res.code, res.body)
	}

	// Clause library.
	clause := map[string]any{"code": "security.basic", "category": "security", "applies_to": []string{"dpa", "notice"},
		"body": map[string]any{"th": map[string]any{"title": "มาตรการพื้นฐาน", "doc": doc(para("เข้ารหัสข้อมูล"))}}}
	if res := do("POST", "/admin/v1/platform/document-clauses", &dpo, clause, nil); res.code != 403 {
		t.Errorf("DPO creating a clause: %d", res.code)
	}
	cl := do("POST", "/admin/v1/platform/document-clauses", &law, clause, nil)
	if cl.code != 201 || cl.body["status"] != "draft" || cl.body["version"] != float64(1) {
		t.Fatalf("clause: %d %v", cl.code, cl.body)
	}
	if res := do("POST", "/admin/v1/platform/document-clauses", &law, clause, nil); res.code != 409 || res.body["code"] != "docs.code_taken" {
		t.Errorf("duplicate code: %d %v", res.code, res.body)
	}
	cpath := "/admin/v1/platform/document-clauses/" + cl.body["id"].(string)
	if res := do("POST", cpath+"/publish", &law, nil, map[string]string{"If-Match": `"99"`}); res.code != 412 {
		t.Errorf("stale publish: %d", res.code)
	}
	pub := do("POST", cpath+"/publish", &law, nil, map[string]string{"If-Match": cl.hdr.Get("ETag")})
	if pub.code != 200 || pub.body["status"] != "published" {
		t.Fatalf("publish clause: %d %v", pub.code, pub.body)
	}
	if res := do("GET", "/admin/v1/platform/document-clauses?published_only=true", &priv, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 1 {
		t.Errorf("editor lists clauses: %d %v", res.code, res.body)
	}
	if res := do("GET", cpath, &dpo, nil, nil); res.code != 200 || len(res.body["versions"].([]any)) != 1 {
		t.Errorf("clause detail: %d %v", res.code, res.body)
	}

	// Templates.
	tpl := do("POST", "/admin/v1/platform/document-templates", &dpo, map[string]any{"doc_type": "notice", "code": "notice.basic", "name": "พื้นฐาน",
		"content": map[string]any{"th": doc(para("หัวข้อ"))}}, nil)
	if tpl.code != 201 || tpl.body["status"] != "draft" {
		t.Fatalf("template: %d %v", tpl.code, tpl.body)
	}
	tpath := "/admin/v1/platform/document-templates/" + tpl.body["id"].(string)
	if res := do("POST", tpath+"/publish", &priv, nil, map[string]string{"If-Match": tpl.hdr.Get("ETag")}); res.code != 403 {
		t.Errorf("privacy publishing a template: %d", res.code)
	}
	if res := do("POST", tpath+"/publish", &dpo, nil, map[string]string{"If-Match": tpl.hdr.Get("ETag")}); res.code != 200 || res.body["status"] != "published" {
		t.Errorf("publish template: %d %v", res.code, res.body)
	}
	if res := do("GET", "/admin/v1/platform/document-templates?doc_type=notice&published_only=true", &priv, nil, nil); res.code != 200 || len(res.body["data"].([]any)) != 1 {
		t.Errorf("templates: %d %v", res.code, res.body)
	}
}
