// PNG-15 (periodic review reminder): internal/cookie/service/schedule.go's own two-step "sweep lists
// tenants, per-tenant job reads its own rows" pattern (CLAUDE.md rule 1 — a global job never reads another
// tenant's RLS-protected rows), reused here for notice.notices' own review_cycle_months/next_review_at
// columns (already in the baseline schema, set for the first time by OnDocumentPublished in versions.go).
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	iamservice "pdpa-platform/internal/iam/service"
	noticestore "pdpa-platform/internal/notice/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// ReviewSweepArgs is notice.review_sweep: platform-wide (jobs.GlobalKinds), no tenant transaction — it only
// enqueues one notice.review_check per tenant, which does the real (RLS-scoped) read.
type ReviewSweepArgs struct{}

func (ReviewSweepArgs) Kind() string { return "notice.review_sweep" }

// ReviewSweeper enqueues notice.review_check for every trial/active tenant.
type ReviewSweeper struct {
	river.WorkerDefaults[ReviewSweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx]
}

func (w *ReviewSweeper) Work(ctx context.Context, _ *river.Job[ReviewSweepArgs]) error {
	tenants, err := noticestore.New(w.Pool).ListActiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("notice.review_sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: ReviewCheckArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := w.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("notice.review_sweep: enqueue: %w", err)
	}
	return nil
}

// ReviewSweepPeriodicJob runs notice.review_sweep once a day — a notice's own review cycle is never shorter
// than a month, so checking more often than this would only waste cycles.
func ReviewSweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return ReviewSweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "notice-review-sweep", RunOnStart: false},
	)
}

// ReviewCheckArgs is notice.review_check: one tenant's own turn to look at its notices.
type ReviewCheckArgs struct {
	jobs.TenantArgs
}

func (ReviewCheckArgs) Kind() string { return "notice.review_check" }

// ReviewChecker works notice.review_check inside the tenant's own transaction.
type ReviewChecker struct {
	river.WorkerDefaults[ReviewCheckArgs]
	Service *Service
}

func (w *ReviewChecker) Work(ctx context.Context, _ *river.Job[ReviewCheckArgs]) error {
	return w.Service.CheckDueReviews(ctx)
}

// CheckDueReviews is the acceptance criterion itself ("เจ้าของประกาศได้รับงานทบทวนตามรอบ"): every published
// notice whose next_review_at has arrived alerts its owner (falling back to role DPO, the same "default
// recipients until real per-record routing exists" pattern BRE-07/PNG-04/DSAR-07 already use when a notice
// has no owner_user_id set), then reschedules next_review_at one more cycle out — a stale tick (the worker
// retrying, or two sweeps landing the same day) is a harmless no-op the next time it fires, since the
// rescheduled date is always in the future relative to today.
func (s *Service) CheckDueReviews(ctx context.Context) error {
	if s.Notify == nil {
		return nil
	}
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	now := s.now()
	due, err := q.ListNoticesDueForReview(ctx, pgtype.Date{Time: now, Valid: true})
	if err != nil {
		return err
	}
	for _, n := range due {
		recipients, err := s.reviewRecipients(ctx, n.OwnerUserID)
		if err != nil {
			return err
		}
		vars := map[string]any{"title": n.Title, "cycle_months": n.ReviewCycleMonths}
		for _, uid := range recipients {
			uid := uid
			for _, ch := range []string{"in_app", "email"} {
				err := pdb.Savepoint(ctx, func(ctx context.Context) error {
					_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "notice.review_due", Channel: ch,
						RecipientUserID: &uid, Vars: vars, EntityType: "notice", EntityID: &n.ID, Urgent: false})
					return err
				})
				if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
					return err
				}
			}
		}
		next := now.AddDate(0, int(n.ReviewCycleMonths), 0)
		if err := q.SetNoticeNextReview(ctx, noticestore.SetNoticeNextReviewParams{
			ID: n.ID, NextReviewAt: pgtype.Date{Time: next, Valid: true},
		}); err != nil {
			return err
		}
	}
	return nil
}

// MarkNoticesForReviewByActivity is PNG-10's own acceptance criterion ("RoPA เปลี่ยนแล้วประกาศที่เกี่ยวข้อง
// ถูกทำเครื่องหมายให้ทบทวน"): every notice composed from activityID (notice.notice_activity_links, set at
// wizard time by PNG-01/PNG-03) gets its next_review_at brought forward to today, so PNG-15's own daily
// CheckDueReviews sweep notifies its owner on its very next run — no second reminder mechanism to build.
// MarkNoticeForReview is itself a no-op for a notice never published (no review clock to disturb yet) or
// already due sooner than today, so a redelivered event is harmless. Called from internal/wiring/events.go's
// own ropa.activity_approved subscriber — notice never reads the ropa schema directly (rule 9).
func (s *Service) MarkNoticesForReviewByActivity(ctx context.Context, activityID uuid.UUID) error {
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	ids, err := q.ListNoticeIDsByActivity(ctx, activityID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := q.MarkNoticeForReview(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) reviewRecipients(ctx context.Context, owner pgtype.UUID) ([]uuid.UUID, error) {
	if owner.Valid {
		return []uuid.UUID{uuid.UUID(owner.Bytes)}, nil
	}
	return iamservice.UsersWithRole(ctx, "DPO")
}
