package agreementhttp_test

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

	agreementhttp "pdpa-platform/internal/agreement/http"
	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
	"pdpa-platform/internal/wiring"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Contract test for the agreement endpoints (DPA-02) behind the real OpenAPI validator and AuthZ.
func TestAgreementEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "agreementhttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM agreement.clauses`, `DELETE FROM agreement.agreement_activities`, `DELETE FROM agreement.parties`, `DELETE FROM agreement.agreements`,
				`DELETE FROM platform.document_versions`, `DELETE FROM platform.documents`,
				`DELETE FROM vendor.vendors`, `UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM org.external_parties`, `DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	vendorSvc := &vendorservice.Service{Audit: audit.New(), Org: orgSvc}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: &riskservice.Service{Audit: audit.New()}}
	versioningSvc := wiring.Versioning(nil, audit.New())
	docsSvc := wiring.Docs(versioningSvc, nil, nil, audit.New(), nil)
	docsSvc.RegisterVersioning()
	svc := &agreementservice.Service{Docs: docsSvc, Org: orgSvc, Vendor: vendorSvc, Ropa: ropaSvc, Audit: audit.New()}

	var legalEntityID, vendorID uuid.UUID
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntityID = le.ID
		party, err := orgSvc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการ", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		v, err := vendorSvc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ประมวลผล", IsProcessor: true}, 0)
		vendorID = v.ID
		return err
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
	reader := uuid.New()
	grants := map[string][]string{
		tenant.UserID.String(): {"agreement.dpa.read", "agreement.dpa.create", "agreement.dpa.update"},
		reader.String():        {"agreement.dpa.read"},
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
	strict := agreementhttp.NewStrictHandlerWithOptions(agreementhttp.NewStrict(svc),
		[]agreementhttp.StrictMiddlewareFunc{authz.StrictMiddleware[agreementhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		agreementhttp.StrictHTTPServerOptions{
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
	agreementhttp.HandlerWithOptions(strict, agreementhttp.ChiServerOptions{BaseRouter: r})
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
	admin := tenant.UserID

	if code, _ := do("GET", "/admin/v1/agreements", nil, nil, nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}
	if code, body := do("GET", "/admin/v1/agreements", &reader, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("list (none yet): %d %s", code, body)
	}

	newAgreement := map[string]any{
		"agreement_type": "dpa", "our_role": "controller", "vendor_id": vendorID, "legal_entity_id": legalEntityID,
		"title": "DPA กับผู้ให้บริการ",
	}
	if code, _ := do("POST", "/admin/v1/agreements", &reader, newAgreement, nil); code != 403 {
		t.Errorf("create with read only: %d, want 403", code)
	}
	if code, body := do("POST", "/admin/v1/agreements", &admin, map[string]any{"agreement_type": "dpa"}, nil); code != 400 {
		t.Errorf("missing required fields: %d %s, want 400 (schema)", code, body)
	}
	if code, body := do("POST", "/admin/v1/agreements", &admin, map[string]any{
		"agreement_type": "dsa", "our_role": "controller", "vendor_id": vendorID, "legal_entity_id": legalEntityID, "title": "x",
	}, nil); code != 422 {
		t.Errorf("unsupported agreement_type dsa: %d %s, want 422", code, body)
	}
	code, body := do("POST", "/admin/v1/agreements", &admin, newAgreement, nil)
	if code != 201 || !strings.Contains(body, `"agreement_type":"dpa"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var created agreementhttp.Agreement
	_ = json.Unmarshal([]byte(body), &created)
	item := "/admin/v1/agreements/" + created.Id.String()

	if code, body := do("GET", item, &reader, nil, nil); code != 200 || !strings.Contains(body, `"document_id"`) {
		t.Errorf("get: %d %s", code, body)
	}
	if code, _ := do("GET", "/admin/v1/agreements/"+uuid.New().String(), &admin, nil, nil); code != 404 {
		t.Errorf("unknown agreement: %d, want 404", code)
	}
	if code, body := do("GET", "/admin/v1/agreements", &reader, nil, nil); code != 200 || !strings.Contains(body, created.Id.String()) {
		t.Errorf("list: %d %s", code, body)
	}
	if code, body := do("GET", "/admin/v1/agreements?agreement_type=dpa", &reader, nil, nil); code != 200 || !strings.Contains(body, created.Id.String()) {
		t.Errorf("list filtered by type: %d %s", code, body)
	}

	// DPA-03: mandatory-clause panel.
	if code, body := do("GET", item+"/missing-clauses", &reader, nil, nil); code != 200 || !strings.Contains(body, "dpa.confidentiality") {
		t.Errorf("missing-clauses: %d %s, want dpa.confidentiality listed", code, body)
	}
	var clauseID uuid.UUID
	_ = pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT id FROM platform.clause_library WHERE code = 'dpa.confidentiality' AND tenant_id IS NULL`).Scan(&clauseID)
	})
	if code, _ := do("POST", item+"/clauses", &reader, map[string]any{"clause_id": clauseID}, nil); code != 403 {
		t.Errorf("add clause with read only: %d, want 403", code)
	}
	code, body = do("POST", item+"/clauses", &admin, map[string]any{"clause_id": clauseID}, nil)
	if code != 201 || !strings.Contains(body, `"clause_code":"dpa.confidentiality"`) {
		t.Fatalf("add clause: %d %s", code, body)
	}
	var addedClause agreementhttp.AgreementClause
	_ = json.Unmarshal([]byte(body), &addedClause)
	if code, body := do("GET", item+"/clauses", &reader, nil, nil); code != 200 || !strings.Contains(body, `"clause_code":"dpa.confidentiality"`) {
		t.Errorf("list clauses: %d %s", code, body)
	}
	if code, body := do("GET", item+"/missing-clauses", &reader, nil, nil); code != 200 || strings.Contains(body, "dpa.confidentiality") {
		t.Errorf("missing-clauses after attach: %d %s, want dpa.confidentiality no longer listed", code, body)
	}
	if code, _ := do("POST", item+"/clauses", &admin, map[string]any{"clause_id": uuid.New()}, nil); code != 422 {
		t.Errorf("add unknown clause: %d, want 422", code)
	}
	if code, _ := do("DELETE", item+"/clauses/"+addedClause.Id.String(), &reader, nil, nil); code != 403 {
		t.Errorf("remove clause with read only: %d, want 403", code)
	}
	if code, _ := do("DELETE", item+"/clauses/"+addedClause.Id.String(), &admin, nil, nil); code != 204 {
		t.Errorf("remove clause: %d", code)
	}
	if code, body := do("GET", item+"/clauses", &reader, nil, nil); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Errorf("list clauses after remove: %d %s", code, body)
	}

	// DPA-04: the processing-schedule annex — empty here since newAgreement linked no activities.
	if code, body := do("GET", item+"/processing-schedule", &reader, nil, nil); code != 200 || !strings.Contains(body, `"activities":[]`) {
		t.Errorf("processing-schedule: %d %s, want an empty activities list", code, body)
	}
	if code, _ := do("GET", "/admin/v1/agreements/"+uuid.New().String()+"/processing-schedule", &admin, nil, nil); code != 404 {
		t.Errorf("processing-schedule unknown agreement: %d, want 404", code)
	}
}
