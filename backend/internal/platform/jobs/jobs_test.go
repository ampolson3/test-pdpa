package jobs_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/jobs"
)

// Every job kind used here starts with "test.jobs." and is deleted before and after each test, so
// leftovers from an interrupted run never reach another test's client.

type scopedArgs struct {
	jobs.TenantArgs
	OtherUserID string `json:"other_user_id"`
}

func (scopedArgs) Kind() string { return "test.jobs.scoped" }

type unscopedArgs struct {
	N int `json:"n"`
}

func (unscopedArgs) Kind() string { return "test.jobs.unscoped" }

type countArgs struct {
	jobs.TenantArgs
	N int `json:"n"`
}

func (countArgs) Kind() string { return "test.jobs.count" }

type failArgs struct {
	jobs.TenantArgs
}

func (failArgs) Kind() string { return "test.jobs.fail" }

type slowArgs struct {
	jobs.TenantArgs
}

func (slowArgs) Kind() string { return "test.jobs.slow" }

type periodicArgs struct {
	jobs.TenantArgs
}

func (periodicArgs) Kind() string { return "test.jobs.periodic" }

// immediateRetry makes a failed job available again right away instead of River's attempt^4 s.
type immediateRetry struct{}

func (immediateRetry) NextRetry(*rivertype.JobRow) time.Time { return time.Now() }

type fixture struct {
	pool   *pgxpool.Pool
	tenant dbtest.Tenant
	other  dbtest.Tenant
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	app := dbtest.Pool(t)
	platform := dbtest.PlatformPool(t)

	clean := func() {
		if _, err := app.Exec(context.Background(), `DELETE FROM river_job WHERE kind LIKE 'test.jobs.%'`); err != nil {
			t.Logf("cleanup river_job: %v", err)
		}
	}
	clean()
	t.Cleanup(clean)

	return fixture{
		pool:   app,
		tenant: dbtest.SeedTenant(t, ctx, app, platform, "jobs-a"),
		other:  dbtest.SeedTenant(t, ctx, app, platform, "jobs-b"),
	}
}

func startWorker(t *testing.T, pool *pgxpool.Pool, opts jobs.WorkerOptions) (*river.Client[pgx.Tx], context.CancelFunc) {
	t.Helper()
	client, err := jobs.NewWorkerClient(pool, opts)
	if err != nil {
		t.Fatalf("NewWorkerClient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		<-client.Stopped()
	})
	return client, cancel
}

type jobState struct {
	State   string
	Attempt int
}

func stateOf(t *testing.T, pool *pgxpool.Pool, id int64) jobState {
	t.Helper()
	var s jobState
	if err := pool.QueryRow(context.Background(), `SELECT state, attempt FROM river_job WHERE id = $1`, id).Scan(&s.State, &s.Attempt); err != nil {
		t.Fatalf("read job %d: %v", id, err)
	}
	return s
}

func waitForState(t *testing.T, pool *pgxpool.Pool, id int64, want string) jobState {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		s := stateOf(t, pool, id)
		if s.State == want {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %d: state %q after 30s, want %q", id, s.State, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A tenant job runs inside its tenant's transaction: app.tenant_id is set, and RLS hides the other
// tenant's rows from it (CLAUDE.md rule 1).
func TestTenantTxMiddleware_ScopesJobToItsTenant(t *testing.T) {
	f := setup(t)

	type seen struct {
		tenantSetting  string
		otherUserFound bool
		err            error
	}
	results := make(chan seen, 1)

	workers := river.NewWorkers()
	river.AddWorker(workers, river.WorkFunc(func(ctx context.Context, job *river.Job[scopedArgs]) error {
		tx := pdb.MustTxFromContext(ctx)
		var r seen
		if r.err = tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&r.tenantSetting); r.err == nil {
			var id uuid.UUID
			err := tx.QueryRow(ctx, `SELECT id FROM iam.users WHERE id = $1`, job.Args.OtherUserID).Scan(&id)
			r.otherUserFound = err == nil
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				r.err = err
			}
		}
		results <- r
		return nil
	}))
	client, _ := startWorker(t, f.pool, jobs.WorkerOptions{Workers: workers})

	res, err := client.Insert(context.Background(), scopedArgs{
		TenantArgs:  jobs.TenantArgs{TenantID: f.tenant.ID.String()},
		OtherUserID: f.other.UserID.String(),
	}, nil)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	select {
	case r := <-results:
		if r.err != nil {
			t.Fatalf("worker query: %v", r.err)
		}
		if r.tenantSetting != f.tenant.ID.String() {
			t.Errorf("app.tenant_id = %q, want %q", r.tenantSetting, f.tenant.ID)
		}
		if r.otherUserFound {
			t.Error("job for tenant A read tenant B's iam.users row — RLS not applied")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("job never ran")
	}
	waitForState(t, f.pool, res.Job.ID, "completed")
}

// A job with no tenant_id whose kind isn't registered as global is cancelled, never run unscoped.
func TestTenantTxMiddleware_CancelsJobWithoutTenant(t *testing.T) {
	f := setup(t)

	var ran atomic.Bool
	workers := river.NewWorkers()
	river.AddWorker(workers, river.WorkFunc(func(ctx context.Context, job *river.Job[unscopedArgs]) error {
		ran.Store(true)
		return nil
	}))
	client, _ := startWorker(t, f.pool, jobs.WorkerOptions{Workers: workers})

	res, err := client.Insert(context.Background(), unscopedArgs{N: 1}, nil)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	waitForState(t, f.pool, res.Job.ID, "cancelled")
	if ran.Load() {
		t.Error("worker ran for a job without tenant_id")
	}
}

// Enqueue uses the caller's transaction: the job exists only if that transaction commits.
func TestEnqueue_FollowsTheCallersTransaction(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	insert, err := jobs.NewInsertClient(f.pool)
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}
	args := countArgs{TenantArgs: jobs.TenantArgs{TenantID: f.tenant.ID.String()}}

	var committedID, rolledBackID int64
	if err := pdb.WithTenantTx(ctx, f.pool, f.tenant.ID.String(), "", func(ctx context.Context) error {
		res, err := jobs.Enqueue(ctx, insert, args, nil)
		if err == nil {
			committedID = res.Job.ID
		}
		return err
	}); err != nil {
		t.Fatalf("commit path: %v", err)
	}
	errBusiness := errors.New("business rule failed")
	if err := pdb.WithTenantTx(ctx, f.pool, f.tenant.ID.String(), "", func(ctx context.Context) error {
		res, err := jobs.Enqueue(ctx, insert, args, nil)
		if err != nil {
			return err
		}
		rolledBackID = res.Job.ID
		return errBusiness
	}); !errors.Is(err, errBusiness) {
		t.Fatalf("rollback path: got %v", err)
	}

	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE id = $1`, committedID).Scan(&n); err != nil || n != 1 {
		t.Errorf("committed job: count=%d err=%v, want 1", n, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE id = $1`, rolledBackID).Scan(&n); err != nil || n != 0 {
		t.Errorf("rolled-back job: count=%d err=%v, want 0", n, err)
	}
}

func TestEnqueue_RejectsAnotherTenantsJob(t *testing.T) {
	f := setup(t)
	insert, err := jobs.NewInsertClient(f.pool)
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}
	err = pdb.WithTenantTx(context.Background(), f.pool, f.tenant.ID.String(), "", func(ctx context.Context) error {
		_, err := jobs.Enqueue(ctx, insert, countArgs{TenantArgs: jobs.TenantArgs{TenantID: f.other.ID.String()}}, nil)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("enqueue for another tenant: got %v, want tenant mismatch error", err)
	}
}

// Acceptance criterion: "job ที่ล้มเหลวมี retry และแจ้งเตือน" — a failing job is retried up to
// MaxAttempts, each failure is reported, and the final one raises the job_discarded alert.
func TestFailingJob_IsRetriedThenAlerted(t *testing.T) {
	f := setup(t)

	var attempts atomic.Int32
	workers := river.NewWorkers()
	river.AddWorker(workers, river.WorkFunc(func(ctx context.Context, job *river.Job[failArgs]) error {
		attempts.Add(1)
		return errors.New("downstream unavailable")
	}))
	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client, _ := startWorker(t, f.pool, jobs.WorkerOptions{Workers: workers, Logger: logger, RetryPolicy: immediateRetry{}})

	res, err := client.Insert(context.Background(), failArgs{TenantArgs: jobs.TenantArgs{TenantID: f.tenant.ID.String()}},
		&river.InsertOpts{MaxAttempts: 3})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	s := waitForState(t, f.pool, res.Job.ID, "discarded")
	if s.Attempt != 3 || attempts.Load() != 3 {
		t.Errorf("attempts: row=%d worker=%d, want 3", s.Attempt, attempts.Load())
	}
	out := logs.String()
	if got := strings.Count(out, `"msg":"job attempt failed; will retry"`); got != 2 {
		t.Errorf("retry warnings = %d, want 2\n%s", got, out)
	}
	if got := strings.Count(out, `"alert":"job_discarded"`); got != 1 {
		t.Errorf("discard alerts = %d, want 1\n%s", got, out)
	}
	if !strings.Contains(out, `"tenant_id":"`+f.tenant.ID.String()+`"`) {
		t.Errorf("alert does not name the tenant\n%s", out)
	}
}

func TestFailingJob_SucceedsOnRetry(t *testing.T) {
	f := setup(t)

	var attempts atomic.Int32
	workers := river.NewWorkers()
	river.AddWorker(workers, river.WorkFunc(func(ctx context.Context, job *river.Job[failArgs]) error {
		if attempts.Add(1) == 1 {
			return errors.New("transient")
		}
		return nil
	}))
	client, _ := startWorker(t, f.pool, jobs.WorkerOptions{Workers: workers, RetryPolicy: immediateRetry{}})

	res, err := client.Insert(context.Background(), failArgs{TenantArgs: jobs.TenantArgs{TenantID: f.tenant.ID.String()}}, nil)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if s := waitForState(t, f.pool, res.Job.ID, "completed"); s.Attempt != 2 {
		t.Errorf("completed on attempt %d, want 2", s.Attempt)
	}
}

// Acceptance criterion: "ไม่มี job ซ้ำเมื่อรันหลาย instance" — two worker instances share the queue
// and each job is worked exactly once.
func TestTwoInstances_WorkEachJobOnce(t *testing.T) {
	f := setup(t)

	var mu sync.Mutex
	runs := map[int]int{}
	newWorkers := func() *river.Workers {
		w := river.NewWorkers()
		river.AddWorker(w, river.WorkFunc(func(ctx context.Context, job *river.Job[countArgs]) error {
			mu.Lock()
			runs[job.Args.N]++
			mu.Unlock()
			return nil
		}))
		return w
	}
	a, _ := startWorker(t, f.pool, jobs.WorkerOptions{Workers: newWorkers()})
	startWorker(t, f.pool, jobs.WorkerOptions{Workers: newWorkers()})

	const n = 40
	params := make([]river.InsertManyParams, n)
	for i := range params {
		params[i] = river.InsertManyParams{Args: countArgs{TenantArgs: jobs.TenantArgs{TenantID: f.tenant.ID.String()}, N: i}}
	}
	if _, err := a.InsertMany(context.Background(), params); err != nil {
		t.Fatalf("insert: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		var done int
		if err := f.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM river_job WHERE kind = 'test.jobs.count' AND state = 'completed'`).Scan(&done); err != nil {
			t.Fatal(err)
		}
		if done == n {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d/%d jobs completed after 30s", done, n)
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < n; i++ {
		if runs[i] != 1 {
			t.Errorf("job %d worked %d times, want 1", i, runs[i])
		}
	}
}

// Periodic jobs are enqueued by the elected leader only, so two instances fire a cron job once.
func TestTwoInstances_RunPeriodicJobOnce(t *testing.T) {
	f := setup(t)

	var runs atomic.Int32
	newOpts := func() jobs.WorkerOptions {
		w := river.NewWorkers()
		river.AddWorker(w, river.WorkFunc(func(ctx context.Context, job *river.Job[periodicArgs]) error {
			runs.Add(1)
			return nil
		}))
		return jobs.WorkerOptions{Workers: w, PeriodicJobs: []*river.PeriodicJob{river.NewPeriodicJob(
			river.PeriodicInterval(time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return periodicArgs{TenantArgs: jobs.TenantArgs{TenantID: f.tenant.ID.String()}}, nil
			},
			&river.PeriodicJobOpts{ID: "test-jobs-periodic", RunOnStart: true},
		)}}
	}
	startWorker(t, f.pool, newOpts())
	startWorker(t, f.pool, newOpts())

	deadline := time.Now().Add(30 * time.Second)
	for runs.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("periodic job never ran")
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(3 * time.Second) // give a hypothetical second instance time to fire as well
	if got := runs.Load(); got != 1 {
		t.Errorf("periodic job ran %d times across two instances, want 1", got)
	}
}

// Unique jobs: a duplicate within the same tenant is skipped; the same job for another tenant is not
// a duplicate, because tenant_id is part of the args.
func TestUniqueJob_IsPerTenant(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	insert, err := jobs.NewInsertClient(f.pool)
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}
	opts := &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
	enqueue := func(tenant uuid.UUID) *rivertype.JobInsertResult {
		var res *rivertype.JobInsertResult
		if err := pdb.WithTenantTx(ctx, f.pool, tenant.String(), "", func(ctx context.Context) error {
			var err error
			res, err = jobs.Enqueue(ctx, insert, countArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, N: 7}, opts)
			return err
		}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		return res
	}

	first := enqueue(f.tenant.ID)
	if dup := enqueue(f.tenant.ID); !dup.UniqueSkippedAsDuplicate || dup.Job.ID != first.Job.ID {
		t.Errorf("same tenant: skipped=%v id=%d, want duplicate of %d", dup.UniqueSkippedAsDuplicate, dup.Job.ID, first.Job.ID)
	}
	if other := enqueue(f.other.ID); other.UniqueSkippedAsDuplicate {
		t.Error("other tenant's identical job was treated as a duplicate")
	}
}

// Graceful shutdown: on stop, a running job gets SoftStopTimeout to finish and completes normally.
func TestShutdown_LetsRunningJobFinish(t *testing.T) {
	f := setup(t)

	started := make(chan struct{})
	workers := river.NewWorkers()
	river.AddWorker(workers, river.WorkFunc(func(ctx context.Context, job *river.Job[slowArgs]) error {
		close(started)
		select {
		case <-time.After(time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}))
	client, stop := startWorker(t, f.pool, jobs.WorkerOptions{Workers: workers, SoftStopTimeout: 10 * time.Second})

	res, err := client.Insert(context.Background(), slowArgs{TenantArgs: jobs.TenantArgs{TenantID: f.tenant.ID.String()}}, nil)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		t.Fatal("job never started")
	}
	stop() // what SIGTERM does in cmd/worker
	select {
	case <-client.Stopped():
	case <-time.After(15 * time.Second):
		t.Fatal("client did not stop")
	}
	if s := stateOf(t, f.pool, res.Job.ID); s.State != "completed" {
		t.Errorf("job state after graceful stop = %q, want completed", s.State)
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
