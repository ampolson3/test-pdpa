package notifyhttp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	notifyhttp "pdpa-platform/internal/platform/notify/http"
)

type env struct {
	srv    *httptest.Server
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *notify.Service
}

// setup mounts the notify endpoints behind the real OpenAPI validator (for 400/428), AuthZ with a stub
// grants loader, and stand-ins for AuthN and the Tx middleware — the same order cmd/api uses.
func setup(t *testing.T, grants map[uuid.UUID][]string) *env {
	t.Helper()
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	owner := dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "notifyhttp")
	client, err := jobs.NewInsertClient(app)
	if err != nil {
		t.Fatal(err)
	}
	svc := &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), app, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.notifications`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.notification_templates WHERE tenant_id IS NOT NULL`)
			return err
		})
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM platform.tenant_keys`)
			return err
		})
		_, _ = app.Exec(context.Background(), `DELETE FROM river_job WHERE kind = 'notify.deliver' AND args->>'tenant_id' = $1`, tenant.ID.String())
	})

	spec, err := openapi3.NewLoader().LoadFromFile("../../../../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	validateMw, err := validate.Middleware(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	perms := map[string]string{}
	for _, item := range spec.Paths.Map() {
		for _, op := range item.Operations() {
			if code, ok := op.Extensions["x-permission"].(string); ok {
				perms[strings.ToUpper(op.OperationID[:1])+op.OperationID[1:]] = code
			}
		}
	}
	cache := authz.NewCachedLoader(rdb, func(_ context.Context, tenantID, userID string) (authz.Grants, error) {
		id, _ := uuid.Parse(userID)
		return authz.Grants{TenantID: tenantID, UserID: userID, Permissions: grants[id]}, nil
	})
	t.Cleanup(func() {
		for u := range grants {
			_ = cache.Invalidate(context.Background(), tenant.ID.String(), u.String())
		}
	})

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler { // AuthN stand-in
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if raw := req.Header.Get("X-Test-User"); raw != "" {
				req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{TenantID: tenant.ID.String(), UserID: raw, ActorType: "user"}))
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Use(validateMw)
	r.Method(http.MethodGet, "/admin/v1/platform/inbox/stream", &notifyhttp.Stream{Service: svc, Pool: app, Interval: 100 * time.Millisecond})
	r.Group(func(g chi.Router) {
		g.Use(func(next http.Handler) http.Handler { // Tx stand-in
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
		strict := notifyhttp.NewStrictHandlerWithOptions(notifyhttp.NewStrict(svc),
			[]notifyhttp.StrictMiddlewareFunc{authz.StrictMiddleware[notifyhttp.StrictHandlerFunc](cache, func(op string) (string, bool) {
				c, ok := perms[op]
				return c, ok
			})},
			notifyhttp.StrictHTTPServerOptions{
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
		notifyhttp.HandlerWithOptions(strict, notifyhttp.ChiServerOptions{BaseRouter: g})
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &env{srv: srv, app: app, tenant: tenant, svc: svc}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func (e *env) do(t *testing.T, method, path string, user *uuid.UUID, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, &buf)
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
	return res, out.Bytes()
}

func TestTemplateEndpoints_Contract(t *testing.T) {
	admin, marketer, nobody := uuid.New(), uuid.New(), uuid.New()
	e := setup(t, map[uuid.UUID][]string{
		admin:    {"admin.notification.create", "admin.notification.read", "admin.notification.update", "admin.notification.delete"},
		marketer: {"admin.notification.read"},
		nobody:   nil,
	})
	const base = "/admin/v1/platform/notification-templates"
	input := map[string]any{"code": "dsar.received", "channel": "email", "language": "th",
		"subject": "ได้รับคำขอ {{.request_no}}", "body": "เรียน {{.name}}", "variables": []string{"name", "request_no"}}

	if res, _ := e.do(t, "GET", base, nil, nil, nil); res.StatusCode != 401 {
		t.Errorf("list without principal: %d, want 401", res.StatusCode)
	}
	if res, _ := e.do(t, "GET", base, &nobody, nil, nil); res.StatusCode != 403 {
		t.Errorf("list without admin.notification.read: %d, want 403", res.StatusCode)
	}
	if res, _ := e.do(t, "POST", base, &marketer, input, nil); res.StatusCode != 403 {
		t.Errorf("create with read-only permission: %d, want 403", res.StatusCode)
	}
	res, body := e.do(t, "POST", base, &admin, input, nil)
	if res.StatusCode != 201 || res.Header.Get("ETag") != `"1"` {
		t.Fatalf("create: %d ETag %q %s", res.StatusCode, res.Header.Get("ETag"), body)
	}
	var created notifyhttp.NotificationTemplate
	_ = json.Unmarshal(body, &created)
	if res, _ := e.do(t, "POST", base, &admin, input, nil); res.StatusCode != 409 {
		t.Errorf("duplicate create: %d, want 409", res.StatusCode)
	}
	bad := map[string]any{"code": "x.bad", "channel": "sms", "language": "th", "body": "Hi {{.undeclared}}"}
	if res, _ := e.do(t, "POST", base, &admin, bad, nil); res.StatusCode != 422 {
		t.Errorf("template using an undeclared variable: %d, want 422", res.StatusCode)
	}

	item := base + "/" + created.Id.String()
	patch := map[string]any{"body": "เรียนคุณ {{.name}}", "variables": []string{"name", "request_no"}}
	if res, _ := e.do(t, "PATCH", item, &admin, patch, nil); res.StatusCode != 428 {
		t.Errorf("PATCH without If-Match: %d, want 428", res.StatusCode)
	}
	if res, _ := e.do(t, "PATCH", item, &admin, patch, map[string]string{"If-Match": `"7"`}); res.StatusCode != 412 {
		t.Errorf("PATCH with stale If-Match: %d, want 412", res.StatusCode)
	}
	if res, _ := e.do(t, "PATCH", item, &admin, patch, map[string]string{"If-Match": `"1"`}); res.StatusCode != 200 || res.Header.Get("ETag") != `"2"` {
		t.Errorf("PATCH: %d ETag %q, want 200 \"2\"", res.StatusCode, res.Header.Get("ETag"))
	}

	res, body = e.do(t, "POST", base+"/preview", &marketer, map[string]any{"subject": "S {{.a}}", "body": "B {{.a}} {{.b}}", "variables": []string{"a", "b"}, "values": map[string]string{"a": "1"}}, nil)
	if res.StatusCode != 200 || !strings.Contains(string(body), `"body":"B 1 {b}"`) {
		t.Errorf("preview: %d %s", res.StatusCode, body)
	}
	if res, _ := e.do(t, "DELETE", item, &admin, nil, map[string]string{"If-Match": `"2"`}); res.StatusCode != 204 {
		t.Errorf("DELETE: %d, want 204", res.StatusCode)
	}
}

func TestDeliveryLogAndInbox_Contract(t *testing.T) {
	admin, nobody := uuid.New(), uuid.New()
	e := setup(t, map[uuid.UUID][]string{admin: {"admin.notification.read", "admin.notification.create"}, nobody: nil})
	recipient := e.tenant.UserID
	ctx := context.Background()
	var msgID, inAppID uuid.UUID
	if err := pdb.WithTenantTx(ctx, e.app, e.tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active' WHERE id = $1`, recipient); err != nil {
			return err
		}
		for _, in := range []notify.TemplateInput{
			{Code: "t.sms", Channel: "sms", Language: "th", Body: "OTP {{.otp}}", Variables: []string{"otp"}},
			{Code: "t.app", Channel: "in_app", Language: "th", Subject: "งาน", Body: "คำขอ {{.n}}", Variables: []string{"n"}},
		} {
			if _, err := e.svc.CreateTemplate(ctx, in); err != nil {
				return err
			}
		}
		var err error
		if msgID, err = e.svc.Send(ctx, notify.Request{TemplateCode: "t.sms", Channel: "sms", RecipientAddress: "0812345678", Vars: map[string]any{"otp": "123456"}}); err != nil {
			return err
		}
		inAppID, err = e.svc.Send(ctx, notify.Request{TemplateCode: "t.app", Channel: "in_app", RecipientUserID: &recipient, Vars: map[string]any{"n": "D-1"}})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if res, _ := e.do(t, "GET", "/admin/v1/platform/notifications", &nobody, nil, nil); res.StatusCode != 403 {
		t.Errorf("delivery log without permission: %d, want 403", res.StatusCode)
	}
	res, body := e.do(t, "GET", "/admin/v1/platform/notifications?status=queued&channel=sms", &admin, nil, nil)
	if res.StatusCode != 200 || !strings.Contains(string(body), msgID.String()) || !strings.Contains(string(body), `"recipient_masked":"+66*******78"`) {
		t.Errorf("delivery log: %d %s", res.StatusCode, body)
	}
	if strings.Contains(string(body), "123456") || strings.Contains(string(body), "812345678") {
		t.Error("delivery log exposes the message variables or the full number")
	}
	if res, _ := e.do(t, "GET", "/admin/v1/platform/notifications?status=bogus", &admin, nil, nil); res.StatusCode != 400 {
		t.Errorf("unknown status filter: %d, want 400", res.StatusCode)
	}

	res, body = e.do(t, "GET", "/admin/v1/platform/inbox", &recipient, nil, nil)
	if res.StatusCode != 200 || !strings.Contains(string(body), `"unread":1`) || !strings.Contains(string(body), "คำขอ D-1") {
		t.Errorf("inbox: %d %s", res.StatusCode, body)
	}
	if res, _ := e.do(t, "POST", "/admin/v1/platform/inbox/"+inAppID.String()+"/read", &nobody, nil, nil); res.StatusCode != 404 {
		t.Errorf("someone else marking it read: %d, want 404", res.StatusCode)
	}
	if res, _ := e.do(t, "POST", "/admin/v1/platform/inbox/"+inAppID.String()+"/read", &recipient, nil, nil); res.StatusCode != 204 {
		t.Errorf("mark read: %d, want 204", res.StatusCode)
	}
}

// The SSE stream sends the unread count on connect and again when it changes.
func TestInboxStream(t *testing.T) {
	e := setup(t, nil)
	recipient := e.tenant.UserID
	ctx := context.Background()
	if err := pdb.WithTenantTx(ctx, e.app, e.tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active' WHERE id = $1`, recipient); err != nil {
			return err
		}
		_, err := e.svc.CreateTemplate(ctx, notify.TemplateInput{Code: "t.app", Channel: "in_app", Language: "th", Body: "hi"})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if res, _ := e.do(t, "GET", "/admin/v1/platform/inbox/stream", nil, nil, nil); res.StatusCode != 401 {
		t.Errorf("stream without principal: %d, want 401", res.StatusCode)
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(reqCtx, "GET", e.srv.URL+"/admin/v1/platform/inbox/stream", nil)
	req.Header.Set("X-Test-User", recipient.String())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q", ct)
	}
	lines := bufio.NewScanner(res.Body)
	next := func() string {
		for lines.Scan() {
			if l := lines.Text(); strings.HasPrefix(l, "data: ") {
				return l
			}
		}
		t.Fatalf("stream ended: %v", lines.Err())
		return ""
	}
	if got := next(); got != `data: {"unread":0}` {
		t.Fatalf("first event %q", got)
	}
	if err := pdb.WithTenantTx(ctx, e.app, e.tenant.ID.String(), "", func(ctx context.Context) error {
		_, err := e.svc.Send(ctx, notify.Request{TemplateCode: "t.app", Channel: "in_app", RecipientUserID: &recipient})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := next(); got != `data: {"unread":1}` {
		t.Errorf("after a new message %q", got)
	}
}
