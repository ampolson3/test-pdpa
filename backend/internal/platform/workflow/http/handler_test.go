package workflowhttp_test

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

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/workflow"
	workflowhttp "pdpa-platform/internal/platform/workflow/http"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the workflow endpoints behind the real OpenAPI validator and AuthZ.
func TestWorkflowEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "workflowhttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{`DELETE FROM platform.sla_timers`, `DELETE FROM platform.workflow_tasks`, `DELETE FROM platform.workflow_instances`,
				`DELETE FROM platform.workflow_definitions WHERE tenant_id IS NOT NULL`, `DELETE FROM platform.notifications`, `DELETE FROM platform.audit_log`,
				`DELETE FROM platform.tenant_keys`, `DELETE FROM iam.groups`} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
		_, _ = app.Exec(context.Background(), `DELETE FROM river_job WHERE args->>'tenant_id' = $1`, tenant.ID.String())
	})
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active', display_name = 'Alice' WHERE id = $1`, tenant.UserID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO iam.groups (tenant_id, name) VALUES ($1, 'Privacy team')`, tenant.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	client, _ := jobs.NewInsertClient(app)
	svc := &workflow.Service{Calendars: &orgservice.Service{}, Notify: &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client},
		River: client, Audit: audit.New()}

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
	other := uuid.New()
	grants := map[string][]string{tenant.UserID.String(): {"admin.workflow.read", "admin.workflow.create", "admin.workflow.update"}, other.String(): {"admin.workflow.read"}}
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
	strict := workflowhttp.NewStrictHandlerWithOptions(workflowhttp.NewStrict(svc),
		[]workflowhttp.StrictMiddlewareFunc{authz.StrictMiddleware[workflowhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		workflowhttp.StrictHTTPServerOptions{
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
	workflowhttp.HandlerWithOptions(strict, workflowhttp.ChiServerOptions{BaseRouter: r})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	do := func(method, path string, user *uuid.UUID, body any, headers map[string]string) (int, string) {
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
		var out bytes.Buffer
		_, _ = out.ReadFrom(res.Body)
		return res.StatusCode, out.String()
	}
	admin, dpo := tenant.UserID, other
	def := map[string]any{"code": "test_flow", "name": "ทดสอบ", "entity_type": "test_record", "definition": map[string]any{
		"initial": "review",
		"states": []any{
			map[string]any{"key": "review", "label": map[string]any{"th": "ตรวจ"}, "task": map[string]any{"title": map[string]any{"th": "ตรวจคำขอ"}, "assignee_user_id": admin}},
			map[string]any{"key": "done", "label": map[string]any{"th": "เสร็จ"}, "terminal": true},
		},
		"transitions": []any{map[string]any{"from": "review", "to": "done"}},
		"sla":         map[string]any{"code": "response", "mode": "calendar_days", "amount": 30, "remind_before": []int{10, 5}},
	}}
	base := "/admin/v1/platform/workflow-definitions"

	if code, _ := do("GET", base, nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, _ := do("POST", base, &dpo, def, nil); code != 403 {
		t.Errorf("create with read permission only: %d, want 403", code)
	}
	bad := map[string]any{"code": "x", "name": "x", "entity_type": "x", "definition": map[string]any{"initial": "a", "states": []any{map[string]any{"key": "a", "label": map[string]any{"th": "a"}}}, "transitions": []any{}}}
	if code, body := do("POST", base, &admin, bad, nil); code != 422 || !strings.Contains(body, "workflow.invalid_definition") {
		t.Errorf("dead-end definition: %d %s, want 422", code, body)
	}
	if code, _ := do("POST", base, &admin, map[string]any{"code": "Bad Code", "name": "x", "entity_type": "x", "definition": def["definition"]}, nil); code != 400 {
		t.Errorf("bad code pattern: %d, want 400", code)
	}
	code, body := do("POST", base, &admin, def, nil)
	if code != 201 || !strings.Contains(body, `"version":1`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created workflowhttp.WorkflowDefinition
	_ = json.Unmarshal([]byte(body), &created)
	versions := base + "/" + created.Id.String() + "/versions"
	if code, _ := do("POST", versions, &admin, def, nil); code != 428 {
		t.Errorf("version without If-Match: %d, want 428", code)
	}
	if code, _ := do("POST", versions, &admin, def, map[string]string{"If-Match": `"7"`}); code != 412 {
		t.Errorf("stale If-Match: %d, want 412", code)
	}
	if code, _ := do("POST", versions, &dpo, def, map[string]string{"If-Match": `"1"`}); code != 403 {
		t.Errorf("version with read permission only: %d, want 403", code)
	}
	if code, body := do("POST", versions, &admin, def, map[string]string{"If-Match": `"1"`}); code != 201 || !strings.Contains(body, `"version":2`) {
		t.Errorf("new version: %d %s", code, body)
	}
	if code, body := do("GET", base, &dpo, nil, nil); code != 200 || strings.Count(body, `"code":"test_flow"`) != 1 {
		t.Errorf("list: %d %s", code, body)
	}
	if code, body := do("GET", "/admin/v1/platform/assignable-groups?q=Priv", &dpo, nil, nil); code != 200 || !strings.Contains(body, "Privacy team") {
		t.Errorf("group search: %d %s", code, body)
	}

	var inst workflow.Instance
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), admin.String(), func(ctx context.Context) error {
		var err error
		inst, err = svc.Start(authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: admin.String()}),
			workflow.StartInput{DefinitionCode: "test_flow", EntityType: "test_record", EntityID: uuid.New()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	item := "/admin/v1/platform/workflow-instances/" + inst.ID.String()
	if code, _ := do("GET", item, &dpo, nil, nil); code != 404 {
		t.Errorf("uninvolved user: %d, want 404", code)
	}
	code, body = do("GET", item, &admin, nil, nil)
	if code != 200 || !strings.Contains(body, `"transitions":[{"from":"review","to":"done"`) || !strings.Contains(body, `"sla_status":"on_track"`) {
		t.Fatalf("assignee view: %d %s", code, body)
	}
	code, body = do("GET", "/admin/v1/platform/my-tasks", &admin, nil, nil)
	if code != 200 || !strings.Contains(body, "ตรวจคำขอ") {
		t.Fatalf("my tasks: %d %s", code, body)
	}
	var mine struct{ Data []workflowhttp.MyTask }
	_ = json.Unmarshal([]byte(body), &mine)
	taskURL := "/admin/v1/platform/workflow-tasks/" + mine.Data[0].Id.String()
	if code, _ := do("PATCH", taskURL, &admin, map[string]any{"assignee_user_id": dpo}, map[string]string{"If-Match": `"1"`}); code != 403 {
		t.Errorf("reassign without the record's write permission: %d, want 403", code)
	}
	if code, body := do("PATCH", taskURL, &admin, map[string]any{"status": "in_progress"}, map[string]string{"If-Match": `"1"`}); code != 200 || !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("start own task: %d %s", code, body)
	}
	tr := item + "/transitions"
	if code, _ := do("POST", tr, &admin, map[string]any{"to": "done"}, nil); code != 428 {
		t.Errorf("transition without If-Match: %d, want 428", code)
	}
	if code, body := do("POST", tr, &admin, map[string]any{"to": "review"}, map[string]string{"If-Match": `"1"`}); code != 409 || !strings.Contains(body, "workflow.invalid_transition") {
		t.Errorf("invalid transition: %d %s, want 409", code, body)
	}
	if code, _ := do("POST", tr, &admin, map[string]any{"to": "done"}, map[string]string{"If-Match": `"9"`}); code != 412 {
		t.Errorf("stale instance: %d, want 412", code)
	}
	if code, body := do("POST", tr, &admin, map[string]any{"to": "done", "comment": "เรียบร้อย"}, map[string]string{"If-Match": `"1"`}); code != 200 || !strings.Contains(body, `"state":"done"`) || !strings.Contains(body, `"status":"met"`) {
		t.Errorf("complete: %d %s", code, body)
	}
}
