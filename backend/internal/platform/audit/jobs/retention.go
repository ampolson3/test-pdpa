package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// DefaultRetentionMonths is how long platform.audit_log is kept (decisions.md D-22: 5 years). The purge
// function refuses anything shorter, so AUDIT_RETENTION_MONTHS can only lengthen it.
const DefaultRetentionMonths = 60

// RetentionArgs is audit.retention: drop the audit_log months older than KeepMonths. Platform-wide
// (jobs.GlobalKinds): the SECURITY DEFINER function reads each tenant's last purged row under that
// tenant's own RLS setting to record the chain anchor.
type RetentionArgs struct {
	KeepMonths int `json:"keep_months"`
}

func (RetentionArgs) Kind() string { return "audit.retention" }

// Retention works audit.retention through platform.drop_expired_audit_partitions (migration 00033).
type Retention struct {
	river.WorkerDefaults[RetentionArgs]

	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

func (w *Retention) Work(ctx context.Context, job *river.Job[RetentionArgs]) error {
	keep := job.Args.KeepMonths
	if keep == 0 {
		keep = DefaultRetentionMonths
	}
	rows, err := w.Pool.Query(ctx, `SELECT partition_name, rows_dropped FROM platform.drop_expired_audit_partitions($1)`, keep)
	if err != nil {
		return fmt.Errorf("audit.retention: %w", err)
	}
	defer rows.Close()
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}
	for rows.Next() {
		var name string
		var n int64
		if err := rows.Scan(&name, &n); err != nil {
			return fmt.Errorf("audit.retention: %w", err)
		}
		logger.InfoContext(ctx, "audit partition dropped after retention", "partition", name, "rows", n, "keep_months", keep)
	}
	return rows.Err()
}

// RetentionPeriodicJob runs audit.retention daily; a month becomes droppable once, so most runs do nothing.
func RetentionPeriodicJob(keepMonths int) *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return RetentionArgs{KeepMonths: keepMonths}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "audit-retention-daily"},
	)
}
