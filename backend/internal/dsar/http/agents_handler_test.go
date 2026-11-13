package dsarhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	dsarhttp "pdpa-platform/internal/dsar/http"
	dsarservice "pdpa-platform/internal/dsar/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

// Contract test for DSAR-04's agent endpoints (list/record/verify) and its own Transition gate — a request
// made on the data subject's own behalf can't leave verifying for in_review until an agent record with its
// authority document exists — behind the real OpenAPI validator and AuthZ.
func TestDsarAgentEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "dsaragenthttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM dsar.agents`, `DELETE FROM dsar.requests`, `DELETE FROM platform.files`,
				`DELETE FROM org.legal_entities`, `DELETE FROM platform.audit_log`,
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

	var legalEntity, requestTypeID uuid.UUID
	var req dsarservice.Request
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		e, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอเจนต์HTTP จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntity = e.ID
		types, err := svc.ListRequestTypes(ctx)
		if err != nil || len(types) == 0 {
			t.Fatal(err, "expected seeded request types")
		}
		requestTypeID = types[0].ID
		req, err = svc.CreateRequest(ctx, dsarservice.CreateRequestInput{
			RequestTypeID: requestTypeID, LegalEntityID: legalEntity, Channel: "phone", OnBehalf: true,
			RequesterName: "เจ้าของข้อมูล", RequesterContact: "agent-http@example.com", ContactKind: crypto.KindEmail,
		})
		if err != nil {
			return err
		}
		_, _, err = svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "verifying"})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	insertCleanFile := func() uuid.UUID {
		id := uuid.New()
		if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
			_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `INSERT INTO platform.files (id, tenant_id, bucket, object_key, file_name, mime_type, size_bytes, sha256, av_status)
				VALUES ($1, current_setting('app.tenant_id')::uuid, 'test', $2, 'poa.pdf', 'application/pdf', 3, 'x', 'clean')`,
				id, id.String())
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}

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
	viewer := uuid.New()
	grants := map[string][]string{
		tenant.UserID.String(): {"dsar.request.read", "dsar.request.update"},
		viewer.String():        {"dsar.request.read"},
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
		hreq, _ := http.NewRequest(method, srv.URL+path, &buf)
		if body != nil {
			hreq.Header.Set("Content-Type", "application/json")
		}
		if user != nil {
			hreq.Header.Set("X-Test-User", user.String())
		}
		for _, hs := range headers {
			for k, v := range hs {
				hreq.Header.Set(k, v)
			}
		}
		res, err := http.DefaultClient.Do(hreq)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out bytes.Buffer
		_, _ = out.ReadFrom(res.Body)
		return res.StatusCode, out.String()
	}
	admin := tenant.UserID
	base := "/admin/v1/dsar/requests/" + req.ID.String()
	ifMatch := func(v int32) map[string]string {
		return map[string]string{"If-Match": `"` + strconv.Itoa(int(v)) + `"`}
	}

	if code, _ := do("GET", base+"/agents", nil, nil); code != 401 {
		t.Errorf("list, no principal: %d, want 401", code)
	}
	if code, body := do("GET", base+"/agents", &viewer, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("empty list: %d %s", code, body)
	}

	// The acceptance criterion's own precondition: without an agent record, transition is refused.
	code, body := do("POST", base+"/transition", &admin, map[string]any{"to": "in_review"}, ifMatch(req.RowVersion))
	if code != 422 || !strings.Contains(body, "dsar.agent_required") {
		t.Fatalf("transition without an agent document: %d %s, want 422 dsar.agent_required", code, body)
	}

	if code, _ := do("POST", base+"/agents", &viewer, map[string]any{"agent_name": "x", "authority_type": "parent", "authority_file_id": insertCleanFile()}); code != 403 {
		t.Errorf("record with read only: %d, want 403", code)
	}
	if code, body := do("POST", base+"/agents", &admin, map[string]any{"agent_name": "x", "authority_type": "bogus", "authority_file_id": insertCleanFile()}); code != 400 {
		t.Errorf("bad authority_type: %d %s, want 400 (schema)", code, body)
	}
	fileID := insertCleanFile()
	code, body = do("POST", base+"/agents", &admin, map[string]any{"agent_name": "สมศักดิ์ ผู้รับมอบอำนาจ", "authority_type": "power_of_attorney", "authority_file_id": fileID})
	if code != 201 || !strings.Contains(body, `"authority_type":"power_of_attorney"`) {
		t.Fatalf("record: %d %s", code, body)
	}
	var agent dsarhttp.DsarAgent
	_ = json.Unmarshal([]byte(body), &agent)

	if code, body := do("GET", base+"/agents", &viewer, nil); code != 200 || !strings.Contains(body, agent.Id.String()) {
		t.Errorf("list after record: %d %s", code, body)
	}

	code, body = do("POST", base+"/agents/"+agent.Id.String()+"/verify", &admin, nil)
	if code != 200 || !strings.Contains(body, `"verified_at"`) {
		t.Fatalf("verify: %d %s", code, body)
	}

	// Now that the authority document is on record, the same transition succeeds.
	code, body = do("POST", base+"/transition", &admin, map[string]any{"to": "in_review"}, ifMatch(req.RowVersion))
	if code != 200 || !strings.Contains(body, `"status":"in_review"`) {
		t.Fatalf("transition after recording the agent document: %d %s", code, body)
	}
}
