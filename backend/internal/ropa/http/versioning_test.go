package ropahttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	templatesservice "pdpa-platform/internal/ropa/templates"

	ropahttp "pdpa-platform/internal/ropa/http"
)

// TestActivityApproval_Contract exercises ROPA-13's transition and version-history endpoints behind the
// real OpenAPI validator and AuthZ: 401/403 gating (ropa.activity.approve for approve/return,
// ropa.activity.update for request-review/confirm-no-change), the happy path, and a 409 on an invalid edge.
func TestActivityApproval_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "ropaverhttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.activity_versions`)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.activity_controls`)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.retention_rules`)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.activity_data`)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.activity_purposes`)
			_, _ = tx.Exec(ctx, `DELETE FROM ropa.processing_activities`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.org_units`)
			_, _ = tx.Exec(ctx, `DELETE FROM org.legal_entities`)
			_, err := tx.Exec(ctx, `DELETE FROM platform.audit_log`)
			return err
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	svc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc, Risk: riskservice.New(), Templates: templatesservice.New()}

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
	dpo, editor := tenant.UserID, uuid.New()
	grants := map[string][]string{
		dpo.String():    {"ropa.activity.read", "ropa.activity.create", "ropa.activity.update", "ropa.activity.approve", "ropa.risk.read", "ropa.risk.create"},
		editor.String(): {"ropa.activity.read", "ropa.activity.create", "ropa.activity.update", "ropa.risk.read", "ropa.risk.create"},
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
	strict := ropahttp.NewStrictHandlerWithOptions(ropahttp.NewStrict(svc, templatesservice.New()),
		[]ropahttp.StrictMiddlewareFunc{authz.StrictMiddleware[ropahttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		ropahttp.StrictHTTPServerOptions{
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
	ropahttp.HandlerWithOptions(strict, ropahttp.ChiServerOptions{BaseRouter: r})
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

	// Build a complete activity directly through the service (not the HTTP layer — several child-table
	// writes would otherwise be a lot of extra plumbing this contract test doesn't need).
	var legalEntityID, orgUnitID, controlID uuid.UUID
	months := 12
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntityID = le.ID
		unit, err := orgSvc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		orgUnitID = unit.ID
		controls, err := svc.ListControls(ctx)
		if err != nil {
			return err
		}
		controlID = controls[0].ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	actBase := "/admin/v1/ropa/activities"
	activity := map[string]any{"legal_entity_id": legalEntityID, "org_unit_id": orgUnitID, "code": "HR-APPROVE-01", "name": "การจ่ายเงินเดือน", "role": "controller", "rights_and_access": "ติดต่อ DPO"}
	code, body := do("POST", actBase, &dpo, activity, nil)
	if code != 201 {
		t.Fatalf("create activity: %d %s", code, body)
	}
	var act ropahttp.ProcessingActivity
	_ = json.Unmarshal([]byte(body), &act)
	actURL := actBase + "/" + act.Id.String()

	var categoryID, subjectTypeID uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT id FROM org.data_categories LIMIT 1`).Scan(&categoryID); err != nil {
			return err
		}
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT id FROM org.data_subject_types LIMIT 1`).Scan(&subjectTypeID)
	}); err != nil {
		t.Fatal(err)
	}
	var lawfulBasisCode string
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT code FROM org.lawful_bases WHERE NOT requires_consent LIMIT 1`).Scan(&lawfulBasisCode)
	}); err != nil {
		t.Fatal(err)
	}

	if code, body := do("POST", actURL+"/data", &dpo, map[string]any{"data_category_id": categoryID, "subject_type_id": subjectTypeID, "source": "direct"}, nil); code != 201 {
		t.Fatalf("add data: %d %s", code, body)
	}
	if code, body := do("POST", actURL+"/purposes", &dpo, map[string]any{"purpose_text": "ทดสอบ", "lawful_basis_code": lawfulBasisCode}, nil); code != 201 {
		t.Fatalf("add purpose: %d %s", code, body)
	}
	if code, body := do("POST", actURL+"/retention-rules", &dpo, map[string]any{"retention_basis": "กฎหมาย", "trigger_event": "สิ้นสุดสัญญา", "disposal_method": "delete", "retention_months": months}, nil); code != 201 {
		t.Fatalf("add retention rule: %d %s", code, body)
	}
	if code, body := do("POST", actURL+"/controls", &dpo, map[string]any{"control_id": controlID}, nil); code != 201 {
		t.Fatalf("add control: %d %s", code, body)
	}

	// 401/403 before trying the real transitions.
	if code, _ := do("POST", actURL+"/approve", nil, nil, map[string]string{"If-Match": `"1"`}); code != 401 {
		t.Errorf("approve, no principal: %d, want 401", code)
	}
	if code, _ := do("POST", actURL+"/approve", &editor, nil, map[string]string{"If-Match": `"1"`}); code != 403 {
		t.Errorf("approve without ropa.activity.approve: %d, want 403", code)
	}
	// Approving a draft (not yet submitted) is an invalid transition.
	if code, _ := do("POST", actURL+"/approve", &dpo, nil, map[string]string{"If-Match": `"1"`}); code != 409 {
		t.Errorf("approve a draft: %d, want 409", code)
	}

	if code, body := do("POST", actURL+"/submit", &dpo, nil, map[string]string{"If-Match": `"1"`}); code != 200 || !strings.Contains(body, `"status":"pending_approval"`) {
		t.Fatalf("submit: %d %s", code, body)
	}
	if code, body := do("POST", actURL+"/approve", &dpo, nil, map[string]string{"If-Match": `"2"`}); code != 200 || !strings.Contains(body, `"status":"active"`) {
		t.Fatalf("approve: %d %s", code, body)
	}

	// A version was frozen.
	if code, body := do("GET", actURL+"/versions", &dpo, nil, nil); code != 200 || !strings.Contains(body, `"version_no":1`) {
		t.Errorf("list versions: %d %s", code, body)
	}

	// Editing an active activity directly is refused.
	if code, _ := do("PATCH", actURL, &dpo, activity, map[string]string{"If-Match": `"3"`}); code != 409 {
		t.Errorf("direct edit of an active activity: %d, want 409", code)
	}

	// Reopen for review (editor's own ropa.activity.update is enough), then confirm no change.
	if code, body := do("POST", actURL+"/request-review", &editor, nil, map[string]string{"If-Match": `"3"`}); code != 200 || !strings.Contains(body, `"status":"under_review"`) {
		t.Fatalf("request review: %d %s", code, body)
	}
	if code, body := do("POST", actURL+"/confirm-no-change", &editor, nil, map[string]string{"If-Match": `"4"`}); code != 200 || !strings.Contains(body, `"status":"active"`) {
		t.Fatalf("confirm no change: %d %s", code, body)
	}

	// confirm-no-change took no new version.
	if code, body := do("GET", actURL+"/versions", &dpo, nil, nil); code != 200 || strings.Contains(body, `"version_no":2`) {
		t.Errorf("confirm-no-change should not add a version: %d %s", code, body)
	}
}
