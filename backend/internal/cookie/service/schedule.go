package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	cookiestore "pdpa-platform/internal/cookie/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
)

// scheduleInterval maps cookie.domains.scan_schedule's own free-text value to a re-scan interval. Unknown
// or empty values never trigger (a domain with no recognized schedule only ever scans on demand) — a
// tunable default, not a legal/contract question, so no docs/decisions.md entry is needed.
func scheduleInterval(schedule string) (time.Duration, bool) {
	switch schedule {
	case "daily":
		return 24 * time.Hour, true
	case "weekly":
		return 7 * 24 * time.Hour, true
	case "monthly":
		return 30 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

// scanDue is the pure decision CON-04's own "สแกนตามรอบ" needs: a domain with no prior scan is always due;
// otherwise due once its own interval has elapsed since the last one finished.
func scanDue(schedule string, lastFinishedAt *time.Time, now time.Time) bool {
	interval, ok := scheduleInterval(schedule)
	if !ok {
		return false
	}
	if lastFinishedAt == nil {
		return true
	}
	return !now.Before(lastFinishedAt.Add(interval))
}

// ScheduleSweepArgs is cookie.scan_schedule_sweep: platform-wide (jobs.GlobalKinds), no tenant transaction
// — it only enqueues one cookie.scan_schedule_check per tenant, which does the real (RLS-scoped) read.
type ScheduleSweepArgs struct{}

func (ScheduleSweepArgs) Kind() string { return "cookie.scan_schedule_sweep" }

// ScheduleSweeper enqueues cookie.scan_schedule_check for every trial/active tenant.
type ScheduleSweeper struct {
	river.WorkerDefaults[ScheduleSweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx]
}

func (w *ScheduleSweeper) Work(ctx context.Context, _ *river.Job[ScheduleSweepArgs]) error {
	tenants, err := cookiestore.New(w.Pool).ListActiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("cookie.scan_schedule_sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: ScheduleCheckArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := w.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("cookie.scan_schedule_sweep: enqueue: %w", err)
	}
	return nil
}

// SweepPeriodicJob runs cookie.scan_schedule_sweep once a day — a domain's own interval is never shorter
// than that, so checking more often than this would only waste cycles.
func SweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return ScheduleSweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "cookie-scan-schedule-sweep", RunOnStart: false},
	)
}

// ScheduleCheckArgs is cookie.scan_schedule_check: one tenant's own turn to look at its domains.
type ScheduleCheckArgs struct {
	jobs.TenantArgs
}

func (ScheduleCheckArgs) Kind() string { return "cookie.scan_schedule_check" }

// ScheduleChecker works cookie.scan_schedule_check inside the tenant's own transaction.
type ScheduleChecker struct {
	river.WorkerDefaults[ScheduleCheckArgs]
	Service *Service
}

func (w *ScheduleChecker) Work(ctx context.Context, _ *river.Job[ScheduleCheckArgs]) error {
	return w.Service.CheckScheduledScans(ctx)
}

// CheckScheduledScans starts a scan for every one of the tenant's own domains whose scan_schedule is due.
func (s *Service) CheckScheduledScans(ctx context.Context) error {
	q := cookiestore.New(pdb.MustTxFromContext(ctx))
	domains, err := q.ListDomainsWithSchedule(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for _, d := range domains {
		if d.ScanSchedule == nil {
			continue
		}
		last, err := q.LastDoneScanByDomain(ctx, d.ID)
		var lastFinishedAt *time.Time
		if err == nil && last.FinishedAt.Valid {
			t := last.FinishedAt.Time
			lastFinishedAt = &t
		} else if err != nil && !pgErrNoRows(err) {
			return err
		}
		if !scanDue(*d.ScanSchedule, lastFinishedAt, now) {
			continue
		}
		if _, err := s.startScan(ctx, d.ID, "schedule"); err != nil {
			return err
		}
	}
	return nil
}
