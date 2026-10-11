package govhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	dsarservice "pdpa-platform/internal/dsar/service"
	govhttp "pdpa-platform/internal/gov/http"
	govservice "pdpa-platform/internal/gov/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/files"
	ropaservice "pdpa-platform/internal/ropa/service"
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type govFakeFiles struct{ clean map[uuid.UUID]bool }

func (f *govFakeFiles) Get(ctx context.Context, id uuid.UUID) (files.File, error) {
	if !f.clean[id] {
		return files.File{}, os.ErrNotExist
	}
	return files.File{ID: id, AVStatus: "clean"}, nil
}
func (f *govFakeFiles) AttachSystem(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error {
	return nil
}

// TestGovHandler_Contract drives the retention-schedule + disposal-job endpoints through the real
// validator/AuthZ chain (401/403/200/201/422/428), mirroring ropa/http's own tia_test.go pattern.
func TestGovHandler_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "govhttp")

	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM gov.disposal_jobs`, `DELETE FROM gov.retention_schedules`, `DELETE FROM ropa.retention_rules`,
				`DELETE FROM ropa.processing_activities`, `DELETE FROM ropa.assets`, `DELETE FROM org.org_units`,
				`DELETE FROM org.legal_entities`, `DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})

	orgSvc := &orgservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc}
	dsarSvc := &dsarservice.Service{Audit: audit.New()}
	fFiles := &govFakeFiles{clean: map[uuid.UUID]bool{}}
	svc := &govservice.Service{Audit: audit.New(), Ropa: ropaSvc, Dsar: dsarSvc, Files: fFiles}

	var ruleID, assetID uuid.UUID
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"ropa.activity.create", "ropa.activity.update"}})
		le, err := orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := orgSvc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
		if err != nil {
			return err
		}
		asset, err := ropaSvc.SaveAsset(ctx, ropaservice.Asset{Name: "HR system", AssetType: "application", OrgUnitID: &unit.ID}, 0)
		if err != nil {
			return err
		}
		assetID = asset.ID
		activity, err := ropaSvc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-01", Name: "HR records", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		rule, err := ropaSvc.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: activity.ID, RetentionBasis: "ม.37(3)", TriggerEvent: "สิ้นสุดการจ้าง", DisposalMethod: "delete"})
		ruleID = rule.ID
		return err
	}); err != nil {
		t.Fatal(err)
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
	reader := uuid.New()
	grants := map[string][]string{
		tenant.UserID.String(): {"dpx.retention.read", "dpx.retention.create", "dpx.retention.update", "dpx.retention.approve", "dpx.retention.execute"},
		reader.String():        {"dpx.retention.read"},
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
	strict := govhttp.NewStrictHandlerWithOptions(govhttp.NewStrict(svc),
		[]govhttp.StrictMiddlewareFunc{authz.StrictMiddleware[govhttp.StrictHandlerFunc](cache, func(op string) (string, bool) { c, ok := perms[op]; return c, ok })},
		govhttp.StrictHTTPServerOptions{
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
	govhttp.HandlerWithOptions(strict, govhttp.ChiServerOptions{BaseRouter: r})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	do := func(method, path string, user *uuid.UUID, ifMatch string, body any) (int, string, http.Header) {
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
		if ifMatch != "" {
			req.Header.Set("If-Match", ifMatch)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out bytes.Buffer
		_, _ = out.ReadFrom(res.Body)
		return res.StatusCode, out.String(), res.Header
	}
	admin := tenant.UserID
	schedulesPath := "/admin/v1/governance/retention-schedules"

	if code, _, _ := do("GET", schedulesPath, nil, "", nil); code != 401 {
		t.Errorf("no principal: %d, want 401", code)
	}

	createBody := map[string]any{"retention_rule_id": ruleID, "asset_id": assetID, "next_due_at": "2020-01-01"}
	if code, _, _ := do("POST", schedulesPath, &reader, "", createBody); code != 403 {
		t.Errorf("create with read-only permission: %d, want 403", code)
	}
	code, body, headers := do("POST", schedulesPath, &admin, "", createBody)
	if code != 201 || !strings.Contains(body, `"active"`) {
		t.Fatalf("create schedule: %d %s", code, body)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &created)
	etag := headers.Get("ETag")

	if code, _, _ := do("GET", schedulesPath, &admin, "", nil); code != 200 {
		t.Errorf("list schedules: %d", code)
	}

	// Missing If-Match -> 428.
	if code, _, _ := do("PATCH", schedulesPath+"/"+created.ID, &admin, "", map[string]any{"status": "paused"}); code != 428 {
		t.Errorf("patch without If-Match: %d, want 428", code)
	}
	if code, _, _ := do("PATCH", schedulesPath+"/"+created.ID, &admin, etag, map[string]any{"status": "paused"}); code != 200 {
		t.Errorf("pause schedule: %d", code)
	}

	// Run the sweep directly (not an HTTP endpoint) to create a disposal job to drive through the lifecycle.
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"dpx.retention.read", "dpx.retention.update"}})
		// Re-activate so the sweep can pick it up.
		sch, err := svc.GetSchedule(ctx, uuid.MustParse(created.ID))
		if err != nil {
			return err
		}
		if _, err := svc.SetScheduleStatus(ctx, sch.ID, sch.RowVersion, "active"); err != nil {
			return err
		}
		_, err = svc.RunRetentionCheck(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	jobsPath := "/admin/v1/governance/disposal-jobs"
	code, body, _ = do("GET", jobsPath, &admin, "", nil)
	if code != 200 {
		t.Fatalf("list jobs: %d %s", code, body)
	}
	var jobsResp struct {
		Data []struct {
			ID         string `json:"id"`
			RowVersion int    `json:"row_version"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &jobsResp)
	if len(jobsResp.Data) != 1 {
		t.Fatalf("expected one disposal job, got %d", len(jobsResp.Data))
	}
	job := jobsResp.Data[0]
	jobEtag := `"` + strconv.Itoa(job.RowVersion) + `"`

	if code, _, _ := do("POST", jobsPath+"/"+job.ID+"/approve", &reader, jobEtag, nil); code != 403 {
		t.Errorf("approve with read-only permission: %d, want 403", code)
	}
	code, body, headers = do("POST", jobsPath+"/"+job.ID+"/approve", &admin, jobEtag, nil)
	if code != 200 || !strings.Contains(body, `"approved"`) {
		t.Fatalf("approve job: %d %s", code, body)
	}
	_ = headers
	var approved struct {
		RowVersion int `json:"row_version"`
	}
	_ = json.Unmarshal([]byte(body), &approved)
	startEtag := `"` + strconv.Itoa(approved.RowVersion) + `"`
	code, body, _ = do("POST", jobsPath+"/"+job.ID+"/start", &admin, startEtag, nil)
	if code != 200 {
		t.Fatalf("start job: %d %s", code, body)
	}
	var started struct {
		RowVersion int `json:"row_version"`
	}
	_ = json.Unmarshal([]byte(body), &started)
	completeEtag := `"` + strconv.Itoa(started.RowVersion) + `"`

	// Completing without evidence is refused (422) — the acceptance criterion's own gate.
	if code, body, _ := do("POST", jobsPath+"/"+job.ID+"/complete", &admin, completeEtag, map[string]any{}); code != 422 && code != 400 {
		t.Errorf("complete without evidence: %d %s, want 422/400", code, body)
	}

	evidenceID := uuid.New()
	fFiles.clean[evidenceID] = true
	code, body, _ = do("POST", jobsPath+"/"+job.ID+"/complete", &admin, completeEtag, map[string]any{"evidence_file_id": evidenceID})
	if code != 200 || !strings.Contains(body, `"done"`) {
		t.Fatalf("complete with evidence: %d %s", code, body)
	}
}
