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
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// Acceptance criterion (PNG-15): a published notice's own owner gets a review-reminder task once its
// next_review_at arrives; the clock then restarts from today, review_cycle_months out.
func TestCheckDueReviews_NotifiesOwnerAndReschedules(t *testing.T) {
	e := setup(t, "pngreview")
	keyring := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Notify = &notify.Service{Keyring: keyring, River: client, Quiet: notify.QuietHours{}}

	var legalEntity, ownerID uuid.UUID
	var notice noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntity = le.ID
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES (current_setting('app.tenant_id')::uuid, $1, 'Notice Owner', 'active') RETURNING id`,
			uuid.NewString()[:8]+"@owner.example").Scan(&ownerID); err != nil {
			return err
		}
		notice, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: legalEntity, NoticeType: "privacy_notice",
			Title: "ประกาศทดสอบทบทวน", Slug: "review-test"})
		return err
	})

	publishNotice(t, e, notice, completeContent())

	// Backdate next_review_at to "now" (it would otherwise be 12 months out from today's publish) and set
	// a real owner, both directly on the row — review_cycle_months stays the seeded default (12).
	e.in(t, func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx,
			`UPDATE notice.notices SET next_review_at = current_date, owner_user_id = $2 WHERE id = $1`, notice.ID, ownerID)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		return e.svc.CheckDueReviews(ctx)
	})

	e.in(t, func(ctx context.Context) error {
		var count int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*) FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id WHERE n.recipient_user_id = $1 AND n.channel = 'in_app' AND t.code = 'notice.review_due'`,
			ownerID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			t.Error("expected at least one platform.notifications row for the notice's owner")
		}
		var next pgtype.Date
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT next_review_at FROM notice.notices WHERE id = $1`, notice.ID).Scan(&next); err != nil {
			return err
		}
		if !next.Valid || !next.Time.After(time.Now()) {
			t.Errorf("next_review_at should be rescheduled into the future, got %v", next)
		}
		return nil
	})

	// A second, immediate check is a harmless no-op — the notice is no longer due.
	e.in(t, func(ctx context.Context) error {
		return e.svc.CheckDueReviews(ctx)
	})
	e.in(t, func(ctx context.Context) error {
		var count int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*) FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id WHERE n.recipient_user_id = $1 AND n.channel = 'in_app' AND t.code = 'notice.review_due'`,
			ownerID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("a not-yet-due notice should not be notified again, got %d notifications", count)
		}
		return nil
	})
}

// A notice with no owner falls back to role DPO (BRE-07/PNG-04's own "default recipients" pattern).
func TestCheckDueReviews_FallsBackToDPOWithNoOwner(t *testing.T) {
	e := setup(t, "pngreviewdpo")
	keyring := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Notify = &notify.Service{Keyring: keyring, River: client, Quiet: notify.QuietHours{}}

	var legalEntity uuid.UUID
	var notice noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntity = le.ID
		notice, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: legalEntity, NoticeType: "privacy_notice",
			Title: "ประกาศไม่มีเจ้าของ", Slug: "review-nodpo-test"})
		return err
	})

	publishNotice(t, e, notice, completeContent())

	e.in(t, func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE notice.notices SET next_review_at = current_date WHERE id = $1`, notice.ID)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		return e.svc.CheckDueReviews(ctx)
	})

	e.in(t, func(ctx context.Context) error {
		var count int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*) FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id WHERE n.recipient_user_id = $1 AND n.channel = 'in_app' AND t.code = 'notice.review_due'`,
			e.dpo).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			t.Error("expected the seeded DPO user to be notified when the notice has no owner")
		}
		return nil
	})
}

// Two-tenant isolation: CheckDueReviews only ever reads the calling tenant's own RLS-scoped rows.
func TestCheckDueReviews_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "pngreviewisoA")
	b := setup(t, "pngreviewisoB")
	for _, e := range []env{a, b} {
		client, err := jobs.NewInsertClient(e.app)
		if err != nil {
			t.Fatal(err)
		}
		e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}
	}

	var noticeA noticeservice.Notice
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "A", IsController: true}, 0)
		if err != nil {
			return err
		}
		noticeA, err = a.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "A", Slug: "review-iso-a"})
		return err
	})
	publishNotice(t, a, noticeA, completeContent())
	a.in(t, func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE notice.notices SET next_review_at = current_date WHERE id = $1`, noticeA.ID)
		return err
	})

	// Tenant B's own check never sees tenant A's due notice (RLS), so no notifications are written for it.
	b.in(t, func(ctx context.Context) error {
		return b.svc.CheckDueReviews(ctx)
	})
	b.in(t, func(ctx context.Context) error {
		var count int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM notice.notices WHERE id = $1`, noticeA.ID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Error("tenant B should never see tenant A's notice under RLS")
		}
		return nil
	})
}
