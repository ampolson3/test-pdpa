package jobshttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/jobs"
	jobshttp "pdpa-platform/internal/platform/jobs/http"
)

type tenantJob struct {
	jobs.TenantArgs
}

func (tenantJob) Kind() string { return "test.jobshttp.contract" }

// Contract test for GET /admin/v1/platform/jobs (CLAUDE.md rule 2): 401 without a principal, 403
// without admin.job.read, 200 with it — and the 200 lists only the caller's tenant's jobs.
func TestPlatformListJobs_Contract(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	platform := dbtest.PlatformPool(t)
	rdb := redisOrSkip(t)

	a := dbtest.SeedTenant(t, ctx, app, platform, "jobshttp-a")
	b := dbtest.SeedTenant(t, ctx, app, platform, "jobshttp-b")
	clean := func() {
		_, _ = app.Exec(context.Background(), `DELETE FROM river_job WHERE kind = 'test.jobshttp.contract'`)
	}
	clean()
	t.Cleanup(clean)

	client, err := jobs.NewInsertClient(app)
	if err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []uuid.UUID{a.ID, b.ID} {
		if _, err := client.Insert(ctx, tenantJob{jobs.TenantArgs{TenantID: tenant.String()}}, nil); err != nil {
			t.Fatal(err)
		}
	}

	// Stand-in for iam's loader: user A holds admin.job.read, the "plain" user does not.
	plainUser := uuid.NewString()
	loader := func(_ context.Context, tenantID, userID string) (authz.Grants, error) {
		g := authz.Grants{TenantID: tenantID, UserID: userID}
		if userID == a.UserID.String() {
			g.Permissions = []string{"admin.job.read"}
		}
		return g, nil
	}
	cache := authz.NewCachedLoader(rdb, loader)
	t.Cleanup(func() {
		_ = cache.Invalidate(context.Background(), a.ID.String(), a.UserID.String())
		_ = cache.Invalidate(context.Background(), a.ID.String(), plainUser)
	})
	srv := newServer(t, app, client, cache)

	if got := get(t, srv, nil).StatusCode; got != http.StatusUnauthorized {
		t.Errorf("no principal: status %d, want 401", got)
	}
	if got := get(t, srv, &httpx.Principal{TenantID: a.ID.String(), UserID: plainUser, ActorType: "user"}).StatusCode; got != http.StatusForbidden {
		t.Errorf("without admin.job.read: status %d, want 403", got)
	}

	res := get(t, srv, &httpx.Principal{TenantID: a.ID.String(), UserID: a.UserID.String(), ActorType: "user"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("with admin.job.read: status %d, want 200", res.StatusCode)
	}
	var body jobshttp.JobList
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	var mine int
	for _, j := range body.Data {
		if j.Kind == "test.jobshttp.contract" {
			mine++
		}
	}
	if mine != 1 {
		t.Errorf("tenant A sees %d contract-test jobs, want exactly its own 1 (not tenant B's)", mine)
	}
}

func newServer(t *testing.T, pool *pgxpool.Pool, client *river.Client[pgx.Tx], cache *authz.CachedLoader) *httptest.Server {
	t.Helper()
	perms := func(op string) (string, bool) {
		if op == "PlatformListJobs" {
			return "admin.job.read", true
		}
		return "", false
	}
	r := chi.NewRouter()
	// Stand-ins for AuthN (#7) and Tx (#11): the principal comes from the test, and the request
	// runs inside WithTenantTx exactly as cmd/api's audit TxMiddleware does (minus the audit row).
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			p, ok := httpx.PrincipalFromContext(req.Context())
			if !ok {
				next.ServeHTTP(w, req)
				return
			}
			_ = pdb.WithTenantTx(req.Context(), pool, p.TenantID, "", func(ctx context.Context) error {
				next.ServeHTTP(w, req.WithContext(ctx))
				return nil
			})
		})
	})
	strict := jobshttp.NewStrictHandler(jobshttp.NewStrict(client),
		[]jobshttp.StrictMiddlewareFunc{authz.StrictMiddleware[jobshttp.StrictHandlerFunc](cache, perms)})
	jobshttp.HandlerFromMux(strict, r)

	outer := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if raw := req.Header.Get("X-Test-Principal"); raw != "" {
			var p httpx.Principal
			_ = json.Unmarshal([]byte(raw), &p)
			req = req.WithContext(httpx.WithPrincipal(req.Context(), p))
		}
		r.ServeHTTP(w, req)
	})
	srv := httptest.NewServer(outer)
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, p *httpx.Principal) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/admin/v1/platform/jobs?limit=200", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p != nil {
		raw, _ := json.Marshal(p)
		req.Header.Set("X-Test-Principal", string(raw))
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func redisOrSkip(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("no reachable Valkey/Redis at %s (set TEST_REDIS_ADDR): %v", addr, err)
	}
	t.Cleanup(func() { rdb.Close() })
	return rdb
}
