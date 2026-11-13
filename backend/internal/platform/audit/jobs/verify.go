// Package jobs verifies each tenant's audit hash chain in the background (PLT-12): a daily
// audit.verify_sweep enqueues one audit.verify per tenant, which replays that tenant's chain inside
// its own tenant transaction and raises alert=audit_chain_broken if any row was altered, removed or
// inserted out of band.
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"pdpa-platform/internal/platform/audit/service"
	store "pdpa-platform/internal/platform/audit/store"
	"pdpa-platform/internal/platform/jobs"
)

// VerifyArgs is audit.verify for one tenant (unique while queued or running).
type VerifyArgs struct {
	jobs.TenantArgs
}

func (VerifyArgs) Kind() string { return "audit.verify" }

func (VerifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// Verifier works audit.verify. A broken chain is reported, not retried: the job succeeds (the
// finding is the result) and the alert carries the first broken row's id for investigation.
type Verifier struct {
	river.WorkerDefaults[VerifyArgs]

	Audit  *service.Service
	Logger *slog.Logger
}

func (w *Verifier) Work(ctx context.Context, job *river.Job[VerifyArgs]) error {
	tenantID, err := uuid.Parse(job.Args.TenantID)
	if err != nil {
		return river.JobCancel(err)
	}
	res, err := w.Audit.Verify(ctx, tenantID)
	if err != nil {
		return err
	}
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if !res.OK {
		logger.ErrorContext(ctx, "audit hash chain broken", "alert", "audit_chain_broken",
			"tenant_id", tenantID.String(), "row_id", res.BrokenAt, "reason", res.Reason, "rows_checked", res.Checked)
		return nil
	}
	logger.InfoContext(ctx, "audit hash chain verified", "tenant_id", tenantID.String(), "rows_checked", res.Checked)
	return nil
}

// VerifySweepArgs is audit.verify_sweep: platform-wide (jobs.GlobalKinds), no tenant transaction.
type VerifySweepArgs struct{}

func (VerifySweepArgs) Kind() string { return "audit.verify_sweep" }

// VerifySweeper enqueues audit.verify for every tenant that still has an audit trail to protect.
type VerifySweeper struct {
	river.WorkerDefaults[VerifySweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx] // nil means the client working this job
}

func (s *VerifySweeper) Work(ctx context.Context, _ *river.Job[VerifySweepArgs]) error {
	tenants, err := store.New(s.Pool).ListLiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("audit.verify_sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: VerifyArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := s.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("audit.verify_sweep: enqueue: %w", err)
	}
	return nil
}

// VerifySweepPeriodicJob runs the sweep daily on the leader.
func VerifySweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return VerifySweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "audit-verify-sweep-daily"},
	)
}
