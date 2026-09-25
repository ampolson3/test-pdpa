// Command worker runs River background jobs: today just partition.maintain
// (internal/platform/jobs). Per-module jobs (outbox dispatch, notifications, exports, ...) get
// registered here as each module implements them — see CLAUDE.md's feature workflow step 4 and
// docs/architecture/code-structure.md's job wrapper rule (one db.WithTenantTx per job, from the
// tenant_id carried in the job's own args).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker: fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := envOr("DATABASE_URL", "postgres://pdpa_app:pdpa_app@localhost:5432/pdpa?sslmode=disable")
	pool, err := pdb.NewPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	workers := river.NewWorkers()
	river.AddWorker(workers, &jobs.PartitionMaintainWorker{Pool: pool})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}},
		Workers:      workers,
		PeriodicJobs: []*river.PeriodicJob{jobs.PeriodicJob()},
	})
	if err != nil {
		return err
	}

	if err := client.Start(ctx); err != nil {
		return err
	}
	slog.Info("worker: started")

	<-ctx.Done()
	slog.Info("worker: shutting down")
	return client.Stop(context.Background())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
