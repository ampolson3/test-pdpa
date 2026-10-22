package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestMarkNoticesForReviewByActivity is PNG-10's own consumer-side half, and its literal acceptance
// criterion: a published notice composed from a RoPA activity gets its own review clock brought forward to
// today the moment that activity is marked changed — internal/wiring/events.go's own ropa.activity_approved
// subscriber calls this directly, so this test drives the same method the real subscriber would.
func TestMarkNoticesForReviewByActivity(t *testing.T) {
	e := setup(t, "pngmarkreview")
	var legalEntity, activityID uuid.UUID
	var notice noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntity = le.ID
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "PNG10-ACT", Name: "กิจกรรมทดสอบ", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		notice, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: legalEntity, NoticeType: "privacy_notice",
			Title: "ประกาศทดสอบ PNG-10", Slug: "png10-test", ActivityIDs: []uuid.UUID{activityID}})
		return err
	})

	publishNotice(t, e, notice, completeContent())

	// Push next_review_at a year out — the published notice must not already be due today.
	e.in(t, func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx,
			`UPDATE notice.notices SET next_review_at = $2 WHERE id = $1`, notice.ID, pgtype.Date{Time: time.Now().AddDate(1, 0, 0), Valid: true})
		return err
	})

	// The acceptance criterion itself: marking the linked activity brings the clock forward to today.
	e.in(t, func(ctx context.Context) error {
		return e.svc.MarkNoticesForReviewByActivity(ctx, activityID)
	})
	e.in(t, func(ctx context.Context) error {
		var next pgtype.Date
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT next_review_at FROM notice.notices WHERE id = $1`, notice.ID).Scan(&next); err != nil {
			return err
		}
		if !next.Valid || next.Time.After(time.Now()) {
			t.Errorf("expected next_review_at brought forward to today, got %v", next)
		}
		return nil
	})

	// An unrelated activity (not linked to any notice) is a harmless no-op.
	e.in(t, func(ctx context.Context) error {
		return e.svc.MarkNoticesForReviewByActivity(ctx, uuid.New())
	})
}

// TestMarkNoticesForReviewByActivity_UnpublishedNoticeUntouched: a notice still in draft (no
// current_version_id yet) has no review clock to disturb — marking its activity is a no-op for it, not an
// error, and not a fabricated due date for a notice that was never published.
func TestMarkNoticesForReviewByActivity_UnpublishedNoticeUntouched(t *testing.T) {
	e := setup(t, "pngmarkreviewdraft")
	var activityID uuid.UUID
	var notice noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "PNG10-DRAFT", Name: "กิจกรรมทดสอบ", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		notice, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice",
			Title: "ประกาศร่าง", Slug: "png10-draft-test", ActivityIDs: []uuid.UUID{activityID}})
		return err
	})

	e.in(t, func(ctx context.Context) error { return e.svc.MarkNoticesForReviewByActivity(ctx, activityID) })
	e.in(t, func(ctx context.Context) error {
		var next pgtype.Date
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT next_review_at FROM notice.notices WHERE id = $1`, notice.ID).Scan(&next); err != nil {
			return err
		}
		if next.Valid {
			t.Errorf("an unpublished notice should get no review date, got %v", next)
		}
		return nil
	})
}

// TestMarkNoticesForReviewByActivity_TwoTenantIsolation: marking an activity id from another tenant never
// touches this tenant's own notices (RLS on notice.notice_activity_links already scopes the lookup).
func TestMarkNoticesForReviewByActivity_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "pngmarkreviewisoA")
	b := setup(t, "pngmarkreviewisoB")

	var activityA uuid.UUID
	var noticeA noticeservice.Notice
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "A", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "A", NameTh: "A", UnitType: "department"})
		if err != nil {
			return err
		}
		act, err := a.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "ISO-A", Name: "A", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityA = act.ID
		noticeA, err = a.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice",
			Title: "A", Slug: "iso-a-notice", ActivityIDs: []uuid.UUID{activityA}})
		return err
	})
	publishNotice(t, a, noticeA, completeContent())
	a.in(t, func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx,
			`UPDATE notice.notices SET next_review_at = $2 WHERE id = $1`, noticeA.ID, pgtype.Date{Time: time.Now().AddDate(1, 0, 0), Valid: true})
		return err
	})

	// Tenant B marking tenant A's own activity id sees nothing under its own RLS.
	b.in(t, func(ctx context.Context) error { return b.svc.MarkNoticesForReviewByActivity(ctx, activityA) })

	a.in(t, func(ctx context.Context) error {
		var next pgtype.Date
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT next_review_at FROM notice.notices WHERE id = $1`, noticeA.ID).Scan(&next); err != nil {
			return err
		}
		if !next.Valid || !next.Time.After(time.Now()) {
			t.Error("tenant B's call should never have touched tenant A's notice")
		}
		return nil
	})
}
