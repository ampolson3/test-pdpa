package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	consentservice "pdpa-platform/internal/consent/service"
	dposervice "pdpa-platform/internal/dpo/service"
	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

func insertConsentPurpose(t *testing.T, ctx context.Context, legalEntity uuid.UUID, basisCode string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`INSERT INTO consent.purposes (tenant_id, code, name_th, legal_entity_id, lawful_basis_code)
		 VALUES (current_setting('app.tenant_id')::uuid, 'test-reconsent-purpose', 'ส่งอีเมลการตลาด', $1, $2) RETURNING id`,
		legalEntity, basisCode).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// PNG-07's own acceptance criterion: a publish marked "changes a purpose" opens a dpo.tasks job per consent
// purpose this notice's linked RoPA activities reference, automatically.
func TestOnDocumentPublished_ChangesPurposeOpensConsentTask(t *testing.T) {
	e := setup(t, "pngreconsent")
	e.svc.Dpo = &dposervice.Service{Audit: audit.New()}
	e.svc.Consent = &consentservice.Service{}
	e.ropa.Consent = &consentservice.Service{} // ROPA-06: AddActivityPurpose validates consent_purpose_id through this

	var legalEntity, activityID, purposeID uuid.UUID
	var notice noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntity = le.ID
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: legalEntity, Code: "MKT", NameTh: "MKT", UnitType: "department"})
		if err != nil {
			return err
		}
		basis := lawfulBasisCode(t, ctx, e.org)
		purposeID = insertConsentPurpose(t, ctx, legalEntity, basis)
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: legalEntity, OrgUnitID: unit.ID, Code: "MKT-01", Name: "การตลาด", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		_, err = e.ropa.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: a.ID, PurposeText: "ส่งอีเมลการตลาด", LawfulBasisCode: basis, ConsentPurposeID: &purposeID})
		if err != nil {
			return err
		}
		notice, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: legalEntity, NoticeType: "privacy_notice",
			Title: "ประกาศทดสอบ", Slug: "reconsent-test", ActivityIDs: []uuid.UUID{activityID}})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		staged, err := e.svc.SetPublishIntent(ctx, notice.ID, notice.RowVersion, false, true)
		if err != nil {
			return err
		}
		notice = staged
		return nil
	})

	publishNotice(t, e, notice, completeContent())

	e.in(t, func(ctx context.Context) error {
		var n int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*) FROM dpo.tasks WHERE source_type = 'consent' AND source_id = $1`, purposeID).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("dpo.tasks for purpose: got %d, want 1", n)
		}
		versions, err := e.svc.ListNoticeVersions(ctx, notice.ID)
		if err != nil {
			return err
		}
		if len(versions) != 1 || !versions[0].ChangesPurpose || versions[0].IsMaterialChange {
			t.Errorf("published version flags = %+v, want changes_purpose=true, is_material_change=false", versions)
		}
		return nil
	})
}

// Mismatched row_version on SetPublishIntent is refused, same ETag discipline as every other update, and the
// staged flags round-trip until the next publish consumes them.
func TestSetPublishIntent_VersionMismatchAndRoundTrip(t *testing.T) {
	e := setup(t, "pngreconsentetag")
	var legalEntity uuid.UUID
	var notice noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		legalEntity = le.ID
		notice, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: legalEntity, NoticeType: "privacy_notice", Title: "ประกาศทดสอบ", Slug: "reconsent-etag"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SetPublishIntent(ctx, notice.ID, 999, true, false); !errors.Is(err, noticeservice.ErrVersionMismatch) {
			t.Errorf("stale row_version: got %v, want ErrVersionMismatch", err)
		}
		out, err := e.svc.SetPublishIntent(ctx, notice.ID, notice.RowVersion, true, false)
		if err != nil {
			t.Fatal(err)
		}
		if !out.PendingIsMaterialChange || out.PendingChangesPurpose {
			t.Errorf("staged flags = %v/%v, want true/false", out.PendingIsMaterialChange, out.PendingChangesPurpose)
		}
		return nil
	})
}
