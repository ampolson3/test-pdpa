package jobs

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

// GlobalKinds are the only job kinds allowed to run without a tenant transaction. Each touches no
// tenant data (see TenantTxMiddleware); add a kind here only with that justification.
var GlobalKinds = map[string]bool{
	PartitionMaintainArgs{}.Kind(): true,
	// events.SweepArgs (internal/platform/events imports this package, so the kind is spelled out):
	// reads only platform.tenants and enqueues one outbox.dispatch per tenant.
	"outbox.sweep": true,
	// audit/jobs.VerifySweepArgs: reads only platform.tenants and enqueues one audit.verify per tenant.
	"audit.verify_sweep": true,
	// audit/jobs.RetentionArgs: calls platform.drop_expired_audit_partitions(), which switches to each tenant's
	// RLS setting itself to record the chain anchors before dropping whole expired months.
	"audit.retention": true,
	// cookie/service.ScheduleSweepArgs (CON-04): reads only platform.tenants and enqueues one
	// cookie.scan_schedule_check per tenant. cookie.scan_schedule_check and cookie.scan themselves are
	// tenant-scoped jobs and must NOT be added here — they need the normal per-tenant transaction.
	"cookie.scan_schedule_sweep": true,
	// agreement/service.MissingDPASweepArgs (DPA-12): reads only platform.tenants and enqueues one
	// agreement.missing_dpa_check per tenant. agreement.missing_dpa_check itself is tenant-scoped and
	// must NOT be added here.
	"agreement.missing_dpa_sweep": true,
	// ropa/service.TemplateReviewSweepArgs (RTG-10): reads only platform.tenants and enqueues one
	// ropa.template_review_check per tenant. ropa.template_review_check itself is tenant-scoped and
	// must NOT be added here.
	"ropa.template_review_sweep": true,
}

// Known gap (not this feature's to fix — see docs/known-issues.md#jobs-01): vendor.reassessment_sweep
// (VEN-10), notice.review_sweep (PNG-15), ropa.review_sweep (ROPA-15), consent.double_opt_in_sweep,
// consent.expiry_sweep (CON-23/CON-20) and gov.retention_sweep (DPX-05) are the identical "sweep lists
// tenants, carries no tenant_id" shape as this package's own entries above — each one's own source
// comment even says "platform-wide (jobs.GlobalKinds)" — but none were ever actually added to this map.
// Every one of them is cancelled with ErrMissingTenant the moment it actually runs. Each was only ever
// disclosed as "not verified live"; add them here once that's addressed.

// WorkerOptions configures NewWorkerClient. Zero values take the documented defaults.
type WorkerOptions struct {
	Logger  *slog.Logger
	Workers *river.Workers
	// PeriodicJobs run only on the elected leader among all worker instances, so a cron job fires
	// once per period however many replicas are running (PLT-10: "ไม่มี job ซ้ำเมื่อรันหลาย instance").
	PeriodicJobs []*river.PeriodicJob
	// MaxWorkers per process on the default queue. Default 10.
	MaxWorkers int
	// SoftStopTimeout is how long a SIGTERM waits for running jobs to finish before their contexts
	// are cancelled. Default 25s, inside Kubernetes' default 30s termination grace period.
	SoftStopTimeout time.Duration
	// RetryPolicy overrides River's default (exponential, attempt^4 seconds, up to MaxAttempts=25
	// attempts ≈ 3 weeks). Tests use it to retry immediately.
	RetryPolicy river.ClientRetryPolicy
}

// NewWorkerClient builds cmd/worker's River client with the platform's job rules wired in: one
// tenant transaction per job (TenantTxMiddleware) and failure alerting (AlertHandler).
func NewWorkerClient(pool *pgxpool.Pool, opts WorkerOptions) (*river.Client[pgx.Tx], error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	maxWorkers := opts.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 10
	}
	softStop := opts.SoftStopTimeout
	if softStop <= 0 {
		softStop = 25 * time.Second
	}

	alerts, err := NewAlertHandler(logger)
	if err != nil {
		return nil, err
	}

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:          logger,
		Queues:          map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: maxWorkers}},
		Workers:         opts.Workers,
		PeriodicJobs:    opts.PeriodicJobs,
		ErrorHandler:    alerts,
		Middleware:      []rivertype.Middleware{&TenantTxMiddleware{Pool: pool, GlobalKinds: GlobalKinds}},
		SoftStopTimeout: softStop,
		RetryPolicy:     opts.RetryPolicy,
	})
}

// NewInsertClient builds the insert-only River client cmd/api uses with Enqueue: it works no jobs
// and runs no maintenance, so API replicas never compete with workers.
func NewInsertClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), &river.Config{})
}
