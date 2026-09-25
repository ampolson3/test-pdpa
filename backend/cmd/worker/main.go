// Command worker runs River background jobs through internal/platform/jobs, which wires in the
// platform's job rules: one db.WithTenantTx per job from the tenant_id in its args, failure
// alerting, leader-only periodic jobs and a graceful shutdown. Module jobs get registered here as
// each module implements them (docs/architecture/integration.md § Background jobs).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	auditjobs "pdpa-platform/internal/platform/audit/jobs"
	auditservice "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/events"
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
	// Not ctx: the pool must outlive the signal so running jobs can finish during the soft stop.
	pool, err := pdb.NewPool(context.Background(), dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	softStop, err := time.ParseDuration(envOr("WORKER_SOFT_STOP_TIMEOUT", "25s"))
	if err != nil {
		return err
	}

	// In-process event subscribers: modules register theirs here as they are built (PLT-11).
	subscribers := events.NewRegistry()

	workers := river.NewWorkers()
	river.AddWorker(workers, &jobs.PartitionMaintainWorker{Pool: pool})
	river.AddWorker(workers, &events.Dispatcher{Subscribers: subscribers, Logger: slog.Default()})
	river.AddWorker(workers, &events.Sweeper{Pool: pool})
	river.AddWorker(workers, &auditjobs.Verifier{Audit: auditservice.New(), Logger: slog.Default()})
	river.AddWorker(workers, &auditjobs.VerifySweeper{Pool: pool})

	client, err := jobs.NewWorkerClient(pool, jobs.WorkerOptions{
		Logger:          slog.Default(),
		Workers:         workers,
		PeriodicJobs:    []*river.PeriodicJob{jobs.PeriodicJob(), events.SweepPeriodicJob(), auditjobs.VerifySweepPeriodicJob()},
		SoftStopTimeout: softStop,
	})
	if err != nil {
		return err
	}

	// Cancelling ctx (SIGINT/SIGTERM) starts River's soft stop: no new jobs are fetched, running
	// ones get SoftStopTimeout to finish, then their contexts are cancelled. An interrupted job's
	// tenant transaction rolls back and River retries it (it is still "running", so the rescuer
	// picks it up after RescueStuckJobsAfter).
	if err := client.Start(ctx); err != nil {
		return err
	}
	slog.Info("worker: started")

	<-client.Stopped()
	slog.Info("worker: stopped")
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
