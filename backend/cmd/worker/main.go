// Command worker runs River background jobs through internal/platform/jobs, which wires in the
// platform's job rules: one db.WithTenantTx per job from the tenant_id in its args, failure
// alerting, leader-only periodic jobs and a graceful shutdown. Module jobs get registered here as
// each module implements them (docs/architecture/integration.md § Background jobs).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/riverqueue/river"

	agreementservice "pdpa-platform/internal/agreement/service"
	breach "pdpa-platform/internal/breach/service"
	consentservice "pdpa-platform/internal/consent/service"
	cookiescan "pdpa-platform/internal/cookie/scan"
	cookieservice "pdpa-platform/internal/cookie/service"
	dpiaservice "pdpa-platform/internal/dpia/service"
	dposervice "pdpa-platform/internal/dpo/service"
	dsarservice "pdpa-platform/internal/dsar/service"
	govjobs "pdpa-platform/internal/gov/jobs"
	govservice "pdpa-platform/internal/gov/service"
	notice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	auditjobs "pdpa-platform/internal/platform/audit/jobs"
	auditservice "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/importer"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/workflow"
	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
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
	river.AddWorker(workers, &auditjobs.Retention{Pool: pool, Logger: slog.Default()})
	retentionMonths, err := strconv.Atoi(envOr("AUDIT_RETENTION_MONTHS", strconv.Itoa(auditjobs.DefaultRetentionMonths)))
	if err != nil || retentionMonths < auditjobs.DefaultRetentionMonths {
		return fmt.Errorf("AUDIT_RETENTION_MONTHS must be a whole number of at least %d", auditjobs.DefaultRetentionMonths)
	}

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
	importSvc := &importer.Service{Types: wiring.ImportTypes(&crypto.Keyring{KEK: kek}, inserter), Files: fileStore(store, inserter), River: inserter, Audit: auditservice.New()}
	river.AddWorker(workers, &importer.Preparer{Service: importSvc})
	river.AddWorker(workers, &importer.Validator{Service: importSvc})
	river.AddWorker(workers, &importer.Applier{Service: importSvc})

	notifySvc := &notify.Service{Keyring: &crypto.Keyring{KEK: kek}, River: inserter, Quiet: notify.DefaultQuietHours()}
	orgSvc := &orgservice.Service{Audit: auditservice.New()}
	// CON-04: the scanner worker. Scanner is the one real cookiescan.Crawler implementation — cmd/api
	// never sets it, since the API process only ever enqueues a scan, it doesn't crawl.
	cookieSvc := &cookieservice.Service{Audit: auditservice.New(), Org: orgSvc, Notify: notifySvc, River: inserter, Scanner: cookiescan.ChromedpCrawler{}}
	river.AddWorker(workers, &cookieservice.ScanWorker{Service: cookieSvc})
	river.AddWorker(workers, &cookieservice.ScheduleSweeper{Pool: pool, River: inserter})
	river.AddWorker(workers, &cookieservice.ScheduleChecker{Service: cookieSvc})
	consentSvc := &consentservice.Service{Audit: auditservice.New(), Org: orgSvc, Notify: notifySvc, Keyring: notifySvc.Keyring,
		PortalBaseURL: envOr("PORTAL_PUBLIC_URL", "")} // CON-23: double opt-in sweep (expires unconfirmed links)
	river.AddWorker(workers, &consentservice.DoubleOptInSweeper{Pool: pool, River: inserter})
	river.AddWorker(workers, &consentservice.DoubleOptInChecker{Service: consentSvc})
	// CON-20: daily expiry sweep (ACTIVE consents past their own expires_at move to EXPIRED)
	river.AddWorker(workers, &consentservice.ExpirySweeper{Pool: pool, River: inserter})
	river.AddWorker(workers, &consentservice.ExpiryChecker{Service: consentSvc})
	ropaSvc := &ropaservice.Service{Audit: auditservice.New(), Org: orgSvc, Notify: notifySvc, River: inserter} // ROPA-15: periodic review reminder
	dpiaSvc := &dpiaservice.Service{Audit: auditservice.New()}                                                  // DPIA-13: breach.activity_linked marks the activity's approved round needs_review
	river.AddWorker(workers, &ropaservice.ReviewSweeper{Pool: pool, River: inserter})
	river.AddWorker(workers, &ropaservice.ReviewChecker{Service: ropaSvc})
	vendorSvc := &vendorservice.Service{Audit: auditservice.New(), Org: orgSvc, Notify: notifySvc, River: inserter} // VEN-10: periodic re-assessment reminder
	river.AddWorker(workers, &vendorservice.ReassessmentSweeper{Pool: pool, River: inserter})
	river.AddWorker(workers, &vendorservice.ReassessmentChecker{Service: vendorSvc})
	river.AddWorker(workers, &vendorservice.CertificateReminderWorker{Service: vendorSvc})         // VEN-06: certificate expiry reminder
	riskSvc := &riskservice.Service{Audit: auditservice.New(), Notify: notifySvc, River: inserter} // RRA-08: acceptance-review reminder at expires_at
	riskSvc.Ropa = wiring.RiskRopa{Ropa: ropaSvc, Org: orgSvc}                                     // RRA-12: rescore an activity when its own ropa.activity_approved event fires
	riskSvc.DpiaTrigger = dpiaSvc                                                                  // RRA-03: a high/very_high rescore opens a DPIA round here too
	river.AddWorker(workers, &riskservice.AcceptanceReviewWorker{Service: riskSvc})
	river.AddWorker(workers, &workflow.Ticker{Service: wiring.Workflow(notifySvc, inserter, auditservice.New())})
	breachSvc := wiring.Breach(notifySvc, fileStore(store, inserter), inserter, auditservice.New(), notifySvc.Keyring, nil, ropaSvc) // BRE-09 recording is admin-only
	river.AddWorker(workers, &breach.TimerWorker{Service: breachSvc})
	river.AddWorker(workers, &breach.NoticeWorker{Service: breachSvc})
	noticeSvc := &notice.Service{Notify: notifySvc, River: inserter} // PNG-04: 30-day indirect-collection reminders
	wiring.Events(subscribers, ropaSvc, dpiaSvc, noticeSvc, riskSvc) // ROPA-10: logs DSAR rejections; PNG-10: marks notices for review; RRA-12: rescores the activity when it changes
	river.AddWorker(workers, &notice.DueWorker{Service: noticeSvc})
	river.AddWorker(workers, &notice.ReviewSweeper{Pool: pool, River: inserter}) // PNG-15: periodic review reminder
	river.AddWorker(workers, &notice.ReviewChecker{Service: noticeSvc})
	dsarSvc := &dsarservice.Service{Audit: auditservice.New(), Notify: notifySvc, River: inserter, Files: fileStore(store, inserter)} // DSAR-07: SLA reminder; DSAR-14: secure delivery purge
	river.AddWorker(workers, &dsarservice.ReminderWorker{Service: dsarSvc})
	river.AddWorker(workers, &dsarservice.PackagePurgeWorker{Service: dsarSvc})
	// DPX-05: the daily retention/disposal sweep (gov.retention_sweep -> gov.retention_check per tenant).
	govSvc := &govservice.Service{Audit: auditservice.New(), Ropa: ropaSvc, Dsar: dsarSvc, Files: fileStore(store, inserter), River: inserter}
	river.AddWorker(workers, &govjobs.Checker{Gov: govSvc})
	river.AddWorker(workers, &govjobs.Sweeper{Pool: pool})
	dpoSvc := &dposervice.Service{Audit: auditservice.New()} // DPA-12: missing-DPA task creation only — no other dpo.tasks consumer runs in this worker
	ropaSvc.Dpo = dpoSvc                                     // ROPA-19: open a review task per linked consent purpose when the activity changes (the ropa.activity_approved subscriber above)
	agreementSvc := &agreementservice.Service{Audit: auditservice.New(), Vendor: vendorSvc, Dpo: dpoSvc,
		Notify: notifySvc, River: inserter} // DPA-10: expiry reminder; DPA-12: missing-DPA sweep
	river.AddWorker(workers, &agreementservice.ReminderWorker{Service: agreementSvc})
	river.AddWorker(workers, &agreementservice.MissingDPASweeper{Pool: pool, River: inserter})
	river.AddWorker(workers, &agreementservice.MissingDPAChecker{Service: agreementSvc})
	// PLT-16: PDF / Word of published documents (Gotenberg via GOTENBERG_URL, or a local Chromium via CHROMIUM_PATH).
	river.AddWorker(workers, &docs.Renderer{Service: wiring.Docs(nil, fileStore(store, inserter), inserter, auditservice.New(), render.FromEnv()), Logger: slog.Default()})

	river.AddWorker(workers, &notify.Deliverer{
		Service: notifySvc,
		Senders: notifySenders(),
		Audit:   auditservice.New(),
		Logger:  slog.Default(),
	})

	client, err := jobs.NewWorkerClient(pool, jobs.WorkerOptions{
		Logger:  slog.Default(),
		Workers: workers,
		PeriodicJobs: []*river.PeriodicJob{jobs.PeriodicJob(), events.SweepPeriodicJob(), auditjobs.VerifySweepPeriodicJob(),
			auditjobs.RetentionPeriodicJob(retentionMonths), govjobs.SweepPeriodicJob(), cookieservice.SweepPeriodicJob(),
			notice.ReviewSweepPeriodicJob(), ropaservice.ReviewSweepPeriodicJob(), vendorservice.ReassessmentSweepPeriodicJob(),
			consentservice.DoubleOptInSweepPeriodicJob(), consentservice.ExpirySweepPeriodicJob(), agreementservice.MissingDPASweepPeriodicJob()},
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
