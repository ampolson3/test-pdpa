// ROPA-15 (periodic review reminder): internal/cookie/service/schedule.go's own two-step "sweep lists
// tenants, per-tenant job reads its own rows" pattern (CLAUDE.md rule 1), reused here for
// ropa.processing_activities' own review_cycle_months/next_review_at columns (migration 00072 added the
// cycle column; next_review_at was already in the baseline schema and is set for the first time by
// ApproveActivity in versioning.go).
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
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	ropastore "pdpa-platform/internal/ropa/store"
)

// ReviewSweepArgs is ropa.review_sweep: platform-wide (jobs.GlobalKinds), no tenant transaction — it only
// enqueues one ropa.review_check per tenant, which does the real (RLS-scoped) read.
type ReviewSweepArgs struct{}

func (ReviewSweepArgs) Kind() string { return "ropa.review_sweep" }

// ReviewSweeper enqueues ropa.review_check for every trial/active tenant.
type ReviewSweeper struct {
	river.WorkerDefaults[ReviewSweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx]
}

func (w *ReviewSweeper) Work(ctx context.Context, _ *river.Job[ReviewSweepArgs]) error {
	tenants, err := ropastore.New(w.Pool).ListActiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("ropa.review_sweep: list tenants: %w", err)
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
		return fmt.Errorf("ropa.review_sweep: enqueue: %w", err)
	}
	return nil
}

// ReviewSweepPeriodicJob runs ropa.review_sweep once a day — an activity's own review cycle is never
// shorter than a month, so checking more often than this would only waste cycles.
func ReviewSweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return ReviewSweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "ropa-review-sweep", RunOnStart: false},
	)
}

// ReviewCheckArgs is ropa.review_check: one tenant's own turn to look at its activities.
type ReviewCheckArgs struct {
	jobs.TenantArgs
}

func (ReviewCheckArgs) Kind() string { return "ropa.review_check" }

// ReviewChecker works ropa.review_check inside the tenant's own transaction.
type ReviewChecker struct {
	river.WorkerDefaults[ReviewCheckArgs]
	Service *Service
}

func (w *ReviewChecker) Work(ctx context.Context, _ *river.Job[ReviewCheckArgs]) error {
	return w.Service.CheckDueReviews(ctx)
}

// CheckDueReviews is the acceptance criterion itself ("เจ้าของกิจกรรมได้รับงานทบทวนตามรอบ"): every active
// activity whose next_review_at has arrived alerts its owner (falling back to role DPO, the same "default
// recipients until real per-record routing exists" pattern BRE-07/PNG-04/PNG-15 already use when an
// activity has no owner_user_id set), then reschedules next_review_at one more cycle out — a stale tick is
// a harmless no-op the next time it fires, since the rescheduled date is always in the future relative to
// today.
func (s *Service) CheckDueReviews(ctx context.Context) error {
	if s.Notify == nil {
		return nil
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	now := s.now()
	due, err := q.ListActivitiesDueForReview(ctx, pgtype.Date{Time: now, Valid: true})
	if err != nil {
		return err
	}
	for _, a := range due {
		recipients, err := s.reviewRecipients(ctx, a.OwnerUserID)
		if err != nil {
			return err
		}
		vars := map[string]any{"name": a.Name, "code": a.Code, "cycle_months": a.ReviewCycleMonths}
		for _, uid := range recipients {
			uid := uid
			for _, ch := range []string{"in_app", "email"} {
				err := pdb.Savepoint(ctx, func(ctx context.Context) error {
					_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "ropa.review_due", Channel: ch,
						RecipientUserID: &uid, Vars: vars, EntityType: "ropa_activity", EntityID: &a.ID, Urgent: false})
					return err
				})
				if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
					return err
				}
			}
		}
		next := now.AddDate(0, int(a.ReviewCycleMonths), 0)
		if err := q.SetActivityNextReview(ctx, ropastore.SetActivityNextReviewParams{
			ID: a.ID, NextReviewAt: pgtype.Date{Time: next, Valid: true},
		}); err != nil {
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
