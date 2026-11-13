// Package jobs is the platform's background-job framework on River (PLT-10, ADR-08): the worker
// client (NewWorkerClient) with one tenant transaction per job (TenantTxMiddleware) and failure
// alerting (AlertHandler), in-transaction enqueue (Enqueue), and the few job kinds that are
// platform-wide rather than per tenant (GlobalKinds). Module jobs live in their module's jobs/
// package, embed TenantArgs, and are registered in cmd/worker.
package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// PartitionMaintainArgs triggers platform.ensure_monthly_partitions, a SECURITY DEFINER function
// (decisions.md D-19) that pre-creates the next few months of partitions for every partitioned
// table. It touches no tenant data, so it runs without a tenant transaction.
type PartitionMaintainArgs struct {
	MonthsAhead int `json:"months_ahead"`
}

func (PartitionMaintainArgs) Kind() string { return "partition.maintain" }

type PartitionMaintainWorker struct {
	river.WorkerDefaults[PartitionMaintainArgs]
	Pool *pgxpool.Pool
}

func (w *PartitionMaintainWorker) Work(ctx context.Context, job *river.Job[PartitionMaintainArgs]) error {
	monthsAhead := job.Args.MonthsAhead
	if monthsAhead <= 0 {
		monthsAhead = 3
	}
	if _, err := w.Pool.Exec(ctx, `SELECT platform.ensure_monthly_partitions($1)`, monthsAhead); err != nil {
		return fmt.Errorf("partition.maintain: %w", err)
	}
	return nil
}

// PeriodicJob runs PartitionMaintainArgs daily, per docs/architecture/deployment.md: "partition
// รายเดือนสร้างล่วงหน้า 3 เดือนด้วย job partition.maintain ... ทุกวัน".
func PeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			// Unique per day as well as leader-only, so a leadership handover mid-period can't
			// enqueue a second run.
			return PartitionMaintainArgs{MonthsAhead: 3}, &river.InsertOpts{
				UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour},
			}
		},
		&river.PeriodicJobOpts{ID: "partition-maintain-daily", RunOnStart: true},
	)
}
