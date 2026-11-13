// RTG-10 (template versioning + legal-change review): "เวอร์ชัน template และแจ้งเมื่อกฎหมายเปลี่ยน" —
// issuing a new version of a template (a new goose migration inserting a fresh ropa.activity_templates
// row for the same template_set_id+code, version_no+1 — the identical mechanism RTG-01/02/03/07's own
// seeded content already used, since this is reviewed legal/business content, rule 8, not a live admin
// action) is SUPER's own half; this file is the reactive half every activity's own tenant needs: find
// every activity still pointing at an older version of its own template and open one review task per
// activity — the acceptance criterion itself ("ประกาศ สคส. ใหม่ → กิจกรรมที่ได้รับผลกระทบได้รับงานทบทวน").
// Follows ROPA-15's own exact "sweep lists tenants, per-tenant job reads its own rows" pattern
// (CLAUDE.md rule 1).
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	ropastore "pdpa-platform/internal/ropa/store"
	templatesservice "pdpa-platform/internal/ropa/templates"
)

// TemplateReviewSweepArgs is ropa.template_review_sweep: platform-wide (jobs.GlobalKinds), no tenant
// transaction — it only enqueues one ropa.template_review_check per tenant, which does the real
// (RLS-scoped) read.
type TemplateReviewSweepArgs struct{}

func (TemplateReviewSweepArgs) Kind() string { return "ropa.template_review_sweep" }

// TemplateReviewSweeper enqueues ropa.template_review_check for every trial/active tenant.
type TemplateReviewSweeper struct {
	river.WorkerDefaults[TemplateReviewSweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx]
}

func (w *TemplateReviewSweeper) Work(ctx context.Context, _ *river.Job[TemplateReviewSweepArgs]) error {
	tenants, err := ropastore.New(w.Pool).ListActiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("ropa.template_review_sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: TemplateReviewCheckArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := w.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("ropa.template_review_sweep: enqueue: %w", err)
	}
	return nil
}

// TemplateReviewSweepPeriodicJob runs ropa.template_review_sweep once a day — a new template version is
// never issued more often than that, so checking more frequently would only waste cycles.
func TemplateReviewSweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return TemplateReviewSweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "ropa-template-review-sweep", RunOnStart: false},
	)
}

// TemplateReviewCheckArgs is ropa.template_review_check: one tenant's own turn to look at its activities.
type TemplateReviewCheckArgs struct {
	jobs.TenantArgs
}

func (TemplateReviewCheckArgs) Kind() string { return "ropa.template_review_check" }

// TemplateReviewChecker works ropa.template_review_check inside the tenant's own transaction.
type TemplateReviewChecker struct {
	river.WorkerDefaults[TemplateReviewCheckArgs]
	Service *Service
}

func (w *TemplateReviewChecker) Work(ctx context.Context, _ *river.Job[TemplateReviewCheckArgs]) error {
	return w.Service.CheckOutdatedTemplateActivities(ctx)
}

// OutdatedTemplateActivity is one activity whose own template has since been reissued at a newer
// version — the "รายการที่ต้องทบทวน" list the module doc's own frontend note calls for.
type OutdatedTemplateActivity struct {
	ActivityID     uuid.UUID
	ActivityName   string
	ActivityCode   string
	TemplateCode   string
	CurrentVersion int
	LatestVersion  int
}

// ListOutdatedTemplateActivities is the read-only half of RTG-10's own mechanism — the same comparison
// CheckOutdatedTemplateActivities uses to decide what to open a task for, exposed directly for the admin
// UI's own "รายการที่ต้องทบทวน" panel. Returns an empty list, never an error, when Templates isn't wired
// (a context that never needs this).
func (s *Service) ListOutdatedTemplateActivities(ctx context.Context) ([]OutdatedTemplateActivity, error) {
	if s.Templates == nil {
		return nil, nil
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListActivitiesWithTemplate(ctx)
	if err != nil {
		return nil, err
	}
	var out []OutdatedTemplateActivity
	for _, a := range rows {
		if !a.TemplateID.Valid {
			continue
		}
		templateID := uuid.UUID(a.TemplateID.Bytes)
		tpl, err := s.Templates.GetActivityTemplate(ctx, templateID)
		if errors.Is(err, templatesservice.ErrNotFound) {
			continue // the template itself was removed; nothing to compare the activity against
		}
		if err != nil {
			return nil, err
		}
		latest, err := s.Templates.LatestVersion(ctx, tpl.TemplateSetID, tpl.Code)
		if err != nil {
			return nil, err
		}
		if latest.VersionNo <= tpl.VersionNo {
			continue
		}
		out = append(out, OutdatedTemplateActivity{
			ActivityID: a.ID, ActivityName: a.Name, ActivityCode: a.Code, TemplateCode: tpl.Code,
			CurrentVersion: tpl.VersionNo, LatestVersion: latest.VersionNo,
		})
	}
	return out, nil
}

// CheckOutdatedTemplateActivities is the acceptance criterion itself: every activity created from a
// template now opens a review task once its own template has been reissued at a newer version — deduped
// via Dpo.HasTaskForActivity, so a repeated tick after the task is already open is a harmless no-op.
// A nil Dpo (a context that never needs this, e.g. a lightweight test fixture) is also a no-op, the same
// convention every other optional cross-cutting field on this Service already follows.
func (s *Service) CheckOutdatedTemplateActivities(ctx context.Context) error {
	if s.Dpo == nil {
		return nil
	}
	outdated, err := s.ListOutdatedTemplateActivities(ctx)
	if err != nil {
		return err
	}
	for _, o := range outdated {
		has, err := s.Dpo.HasTaskForActivity(ctx, o.ActivityID)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		title := fmt.Sprintf("ทบทวนกิจกรรม %q ตาม template เวอร์ชันใหม่", o.ActivityName)
		description := fmt.Sprintf(
			"Template %q ของกิจกรรมนี้มีเวอร์ชันใหม่ (v%d → v%d) จากการปรับปรุงเนื้อหา โปรดทบทวนว่ากิจกรรมนี้ยังสอดคล้องกับแนวทางล่าสุดหรือไม่",
			o.TemplateCode, o.CurrentVersion, o.LatestVersion,
		)
		if _, err := s.Dpo.OpenTemplateReviewTask(ctx, o.ActivityID, title, description); err != nil {
			return err
		}
	}
	return nil
}
