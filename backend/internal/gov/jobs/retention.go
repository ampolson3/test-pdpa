// Package jobs is DPX-05's daily automatic disposal-job creation: a daily gov.retention_sweep
// enqueues one gov.retention_check per tenant (mirroring PLT-12's own audit.verify_sweep/audit.verify
// pair exactly), which runs RunRetentionCheck inside that tenant's own transaction.
package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	govservice "pdpa-platform/internal/gov/service"
	store "pdpa-platform/internal/platform/audit/store"
	"pdpa-platform/internal/platform/jobs"
)

// CheckArgs is gov.retention_check for one tenant (unique while queued or running).
type CheckArgs struct {
	jobs.TenantArgs
}

func (CheckArgs) Kind() string { return "gov.retention_check" }

func (CheckArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// Checker works gov.retention_check: find every due schedule with no disposal job still open and
// create one (RunRetentionCheck), inside the tenant transaction TenantTxMiddleware already opened.
type Checker struct {
	river.WorkerDefaults[CheckArgs]

	Gov *govservice.Service
}

func (w *Checker) Work(ctx context.Context, _ *river.Job[CheckArgs]) error {
	if _, err := w.Gov.RunRetentionCheck(ctx); err != nil {
		return fmt.Errorf("gov.retention_check: %w", err)
	}
	return nil
}

// SweepArgs is gov.retention_sweep: platform-wide (jobs.GlobalKinds), no tenant transaction.
type SweepArgs struct{}

func (SweepArgs) Kind() string { return "gov.retention_sweep" }

// Sweeper enqueues gov.retention_check for every tenant with retention schedules to check.
type Sweeper struct {
	river.WorkerDefaults[SweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx] // nil means the client working this job
}

func (s *Sweeper) Work(ctx context.Context, _ *river.Job[SweepArgs]) error {
	tenants, err := store.New(s.Pool).ListLiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("gov.retention_sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: CheckArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := s.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("gov.retention_sweep: enqueue: %w", err)
	}
	return nil
}

// SweepPeriodicJob runs the sweep daily on the leader.
func SweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return SweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "gov-retention-sweep-daily"},
	)
}
