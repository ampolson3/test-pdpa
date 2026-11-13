// VEN-10 (periodic re-assessment, ม.37(1)): the same two-step "sweep lists tenants, per-tenant job reads
// its own rows" pattern PNG-15/ROPA-15/CON-04 already established (CLAUDE.md rule 1), reused here for
// vendor.vendors' own tier/next_assessment_at columns — both already set by VEN-02's own RecordIntake
// (intake.go), unread by anything until now.
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
	vendorstore "pdpa-platform/internal/vendormgmt/store"
)

// ReassessmentSweepArgs is vendor.reassessment_sweep: platform-wide (jobs.GlobalKinds), no tenant
// transaction — it only enqueues one vendor.reassessment_check per tenant, which does the real
// (RLS-scoped) read.
type ReassessmentSweepArgs struct{}

func (ReassessmentSweepArgs) Kind() string { return "vendor.reassessment_sweep" }

// ReassessmentSweeper enqueues vendor.reassessment_check for every trial/active tenant.
type ReassessmentSweeper struct {
	river.WorkerDefaults[ReassessmentSweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx]
}

func (w *ReassessmentSweeper) Work(ctx context.Context, _ *river.Job[ReassessmentSweepArgs]) error {
	tenants, err := vendorstore.New(w.Pool).ListActiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("vendor.reassessment_sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: ReassessmentCheckArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := w.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("vendor.reassessment_sweep: enqueue: %w", err)
	}
	return nil
}

// ReassessmentSweepPeriodicJob runs vendor.reassessment_sweep once a day — the shortest re-assessment
// interval (critical tier, 6 months) is far longer than that, so a daily check never misses one.
func ReassessmentSweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return ReassessmentSweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "vendor-reassessment-sweep", RunOnStart: false},
	)
}

// ReassessmentCheckArgs is vendor.reassessment_check: one tenant's own turn to look at its vendors.
type ReassessmentCheckArgs struct {
	jobs.TenantArgs
}

func (ReassessmentCheckArgs) Kind() string { return "vendor.reassessment_check" }

// ReassessmentChecker works vendor.reassessment_check inside the tenant's own transaction.
type ReassessmentChecker struct {
	river.WorkerDefaults[ReassessmentCheckArgs]
	Service *Service
}

func (w *ReassessmentChecker) Work(ctx context.Context, _ *river.Job[ReassessmentCheckArgs]) error {
	return w.Service.CheckDueReassessments(ctx)
}

// CheckDueReassessments is the acceptance criterion itself ("คู่ค้าถูกส่งแบบประเมินซ้ำตามรอบโดยไม่ต้องสั่งเอง"):
// every approved/conditional vendor whose next_assessment_at has arrived alerts its relationship owner
// (falling back to role PROC — the module doc's own actor for VEN-02's intake, the same "default recipients
// until real per-record routing exists" pattern BRE-07/PNG-04/PNG-15/ROPA-15 already use when a record has
// no owner set), then reschedules next_assessment_at one more tier interval out — the same
// reassessmentInterval(tier) function VEN-02's own RecordIntake already uses, so a redue date always lands
// on the same cadence a fresh intake would. A stale tick afterward is a harmless no-op, since the
// rescheduled date is always in the future relative to today.
func (s *Service) CheckDueReassessments(ctx context.Context) error {
	if s.Notify == nil {
		return nil
	}
	q := vendorstore.New(pdb.MustTxFromContext(ctx))
	now := s.now()
	due, err := q.ListVendorsDueForReassessment(ctx, pgtype.Date{Time: now, Valid: true})
	if err != nil {
		return err
	}
	for _, v := range due {
		tier := ""
		if v.Tier != nil {
			tier = *v.Tier
		}
		recipients, err := s.reassessmentRecipients(ctx, v.RelationshipOwnerID)
		if err != nil {
			return err
		}
		vars := map[string]any{"tier": tier}
		for _, uid := range recipients {
			uid := uid
			for _, ch := range []string{"in_app", "email"} {
				err := pdb.Savepoint(ctx, func(ctx context.Context) error {
					_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "vendor.reassessment_due", Channel: ch,
						RecipientUserID: &uid, Vars: vars, EntityType: VendorEntityType, EntityID: &v.ID, Urgent: false})
					return err
				})
				if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
					return err
				}
			}
		}
		next := now.Add(reassessmentInterval(tier))
		if err := q.SetVendorNextAssessment(ctx, vendorstore.SetVendorNextAssessmentParams{
			ID: v.ID, NextAssessmentAt: pgtype.Date{Time: next, Valid: true},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) reassessmentRecipients(ctx context.Context, owner pgtype.UUID) ([]uuid.UUID, error) {
	if owner.Valid {
		return []uuid.UUID{uuid.UUID(owner.Bytes)}, nil
	}
	return iamservice.UsersWithRole(ctx, "PROC")
}
