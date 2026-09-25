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
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/importer"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/wiring"
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

	store, err := files.NewS3Store(files.S3ConfigFromEnv())
	if err != nil {
		return err
	}
	river.AddWorker(workers, &files.Scanner{Store: store, AV: &files.Clamd{Addr: envOr("CLAMD_ADDR", "localhost:3310")}, Audit: auditservice.New(), Logger: slog.Default()})
	river.AddWorker(workers, &files.Expirer{Store: store})

	kek, err := crypto.KEKFromEnv()
	if err != nil {
		return err
	}
	// The worker enqueues its own retries (notify.deliver) with an insert-only client of its pool.
	inserter, err := jobs.NewInsertClient(pool)
	if err != nil {
		return err
	}
	importSvc := &importer.Service{Types: wiring.ImportTypes(), Files: fileStore(store, inserter), River: inserter, Audit: auditservice.New()}
	river.AddWorker(workers, &importer.Preparer{Service: importSvc})
	river.AddWorker(workers, &importer.Validator{Service: importSvc})
	river.AddWorker(workers, &importer.Applier{Service: importSvc})

	river.AddWorker(workers, &notify.Deliverer{
		Service: &notify.Service{Keyring: &crypto.Keyring{KEK: kek}, River: inserter, Quiet: notify.DefaultQuietHours()},
		Senders: notifySenders(),
		Audit:   auditservice.New(),
		Logger:  slog.Default(),
	})

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

// notifySenders: SMTP for e-mail when SMTP_ADDR is set; SMS and LINE use mocks until their providers are
// chosen (decisions.md Q-03) — so does e-mail without SMTP_ADDR (development).
func notifySenders() map[string]notify.Sender {
	senders := map[string]notify.Sender{
		notify.ChannelSMS:   &notify.MockSender{Channel: notify.ChannelSMS, Logger: slog.Default()},
		notify.ChannelLine:  &notify.MockSender{Channel: notify.ChannelLine, Logger: slog.Default()},
		notify.ChannelEmail: &notify.MockSender{Channel: notify.ChannelEmail, Logger: slog.Default()},
	}
	if addr := os.Getenv("SMTP_ADDR"); addr != "" {
		senders[notify.ChannelEmail] = &notify.SMTPSender{
			Addr: addr, From: envOr("SMTP_FROM", "noreply@pdpa.local"),
			Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD"),
		}
	}
	return senders
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
