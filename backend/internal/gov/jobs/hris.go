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

// HrisCheckArgs is gov.hris_sync_check for one tenant (DPX-13).
type HrisCheckArgs struct {
	jobs.TenantArgs
}

func (HrisCheckArgs) Kind() string { return "gov.hris_sync_check" }

func (HrisCheckArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// HrisChecker closes the accounts of employees the tenant's HR connectors report as having left.
type HrisChecker struct {
	river.WorkerDefaults[HrisCheckArgs]

	Gov *govservice.Service
}

func (w *HrisChecker) Work(ctx context.Context, _ *river.Job[HrisCheckArgs]) error {
	if _, err := w.Gov.SyncHRIS(ctx); err != nil {
		return fmt.Errorf("gov.hris_sync_check: %w", err)
	}
	return nil
}

// HrisSweepArgs is gov.hris_sync_sweep: platform-wide (jobs.GlobalKinds), no tenant transaction.
type HrisSweepArgs struct{}

func (HrisSweepArgs) Kind() string { return "gov.hris_sync_sweep" }

// HrisSweeper enqueues gov.hris_sync_check for every live tenant.
type HrisSweeper struct {
	river.WorkerDefaults[HrisSweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx] // nil means the client working this job
}

func (s *HrisSweeper) Work(ctx context.Context, _ *river.Job[HrisSweepArgs]) error {
	tenants, err := store.New(s.Pool).ListLiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("gov.hris_sync_sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: HrisCheckArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := s.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("gov.hris_sync_sweep: enqueue: %w", err)
	}
	return nil
}

// HrisSweepPeriodicJob runs the sweep daily on the leader.
func HrisSweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return HrisSweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "gov-hris-sync-sweep-daily"},
	)
}
