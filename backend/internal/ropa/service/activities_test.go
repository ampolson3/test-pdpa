package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// lawfulBasisCode returns a code that does NOT require consent evidence (ROPA-06), so callers that
// don't care about that rule can use it without also stubbing a consent purpose.
func lawfulBasisCode(t *testing.T, ctx context.Context, org *orgservice.Service) string {
	t.Helper()
	items, err := org.ListMaster(ctx, "lawful_bases")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if !it.RequiresConsent {
			return it.Code
		}
	}
	t.Fatal("expected at least one non-consent lawful basis to be seeded (ORG-07)")
	return ""
}

// consentLawfulBasisCode returns a code that DOES require consent evidence (ROPA-06's own acceptance
// criterion), i.e. org.lawful_bases.requires_consent — the seeded 'CONSENT' code.
func consentLawfulBasisCode(t *testing.T, ctx context.Context, org *orgservice.Service) string {
	t.Helper()
	items, err := org.ListMaster(ctx, "lawful_bases")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.RequiresConsent {
			return it.Code
		}
	}
	t.Fatal("expected at least one consent-requiring lawful basis to be seeded (ORG-07)")
	return ""
}

// insertConsentPurpose stubs a minimal consent.purposes row (bypassing the consent service's own
// draft/approve/publish flow, which this test doesn't need) so ActivityPurpose.consent_purpose_id has
// something real to point at (rule 1: FK visibility).
func insertConsentPurpose(t *testing.T, ctx context.Context, legalEntity uuid.UUID, basisCode string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
		`INSERT INTO consent.purposes (tenant_id, code, name_th, legal_entity_id, lawful_basis_code)
		 VALUES (current_setting('app.tenant_id')::uuid, 'test-purpose', 'ทดสอบ', $1, $2) RETURNING id`,
		legalEntity, basisCode).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestActivities_CRUDAndValidation(t *testing.T) {
	e := setup(t, "ropaact")
	var le orgservice.LegalEntity
	var unit orgservice.OrgUnit
	var party orgservice.ExternalParty
	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"}); err != nil {
			return err
		}
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการคลาวด์", CountryCode: "US"}, 0)
		return err
	})

	var created ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		created, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-01",
			Name: "การจ่ายเงินเดือน", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		if created.Status != "draft" || created.RowVersion != 1 {
			t.Errorf("create did not stick: %+v", created)
		}
		got, err := e.svc.GetActivity(ctx, created.ID)
		if err != nil {
			return err
		}
		if got.Name != created.Name {
			t.Errorf("read back mismatch: %+v", got)
		}
		bogus := uuid.New()
		for name, in := range map[string]ropaservice.Activity{
			"no code":              {LegalEntityID: le.ID, OrgUnitID: unit.ID, Name: "x", Role: "controller"},
			"no name":              {LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "X-01", Role: "controller"},
			"bad role":             {LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "X-02", Name: "x", Role: "bogus"},
			"unknown legal entity": {LegalEntityID: bogus, OrgUnitID: unit.ID, Code: "X-03", Name: "x", Role: "controller"},
			"unknown org unit":     {LegalEntityID: le.ID, OrgUnitID: bogus, Code: "X-04", Name: "x", Role: "controller"},
			"unknown controller":   {LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "X-05", Name: "x", Role: "processor", ControllerPartyID: &bogus},
			"unknown owner":        {LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "X-06", Name: "x", Role: "controller", OwnerUserID: &bogus},
		} {
			if _, err := e.svc.SaveActivity(ctx, in, 0); !errors.Is(err, ropaservice.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		return nil
	})

	// Duplicate code within the tenant is refused — its own transaction, since the underlying
	// unique-constraint violation aborts whatever transaction it happens in.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-01",
			Name: "อื่น", Role: "controller"}, 0); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("duplicate code: %v, want ErrInvalid", err)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveActivity(ctx, ropaservice.Activity{ID: created.ID, LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: created.Code,
			Name: "x", Role: "controller"}, 0); !errors.Is(err, ropaservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		updated, err := e.svc.SaveActivity(ctx, ropaservice.Activity{ID: created.ID, LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: created.Code,
			Name: "การจ่ายเงินเดือนพนักงาน", Role: "processor", ControllerPartyID: &party.ID, RightsAndAccess: "ติดต่อ DPO"}, created.RowVersion)
		if err != nil {
			return err
		}
		if updated.Role != "processor" || updated.RowVersion != 2 || updated.RightsAndAccess != "ติดต่อ DPO" {
			t.Errorf("update did not stick: %+v", updated)
		}
		return nil
	})
}

// TestActivities_CompletenessAndMissingItems is the acceptance criterion: an activity missing
// mandatory ม.39 items shows an incomplete status with the specific list of what's missing, and
// filling each one in clears it from the list until the activity can be submitted.
func TestActivities_CompletenessAndMissingItems(t *testing.T) {
	e := setup(t, "ropaactcomplete")
	var le orgservice.LegalEntity
	var unit orgservice.OrgUnit
	var plain orgservice.MasterItem
	var subjectType orgservice.MasterItem
	var basis string
	var activity ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"}); err != nil {
			return err
		}
		_, plain = twoCategories(t, ctx, e.org)
		subjects, err := e.org.ListMaster(ctx, "data_subject_types")
		if err != nil {
			return err
		}
		if len(subjects) == 0 {
			t.Fatal("expected data subject types to be seeded (ORG-07)")
		}
		subjectType = subjects[0]
		basis = lawfulBasisCode(t, ctx, e.org)
		activity, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-02",
			Name: "การรับสมัครงาน", Role: "controller"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		want := map[string]bool{"data": true, "purpose": true, "retention": true, "rights_access": true}
		for _, m := range got.MissingItems {
			if m == "controller" {
				t.Errorf("role=controller should never require controller_party_id, got missing item %q", m)
			}
			delete(want, m)
		}
		if len(want) != 0 {
			t.Errorf("expected all of %v to be missing initially, got %v", want, got.MissingItems)
		}
		if got.Completeness == 100 {
			t.Errorf("brand new activity should not read as 100%% complete")
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: activity.ID, DataCategoryID: *plain.ID, SubjectTypeID: *subjectType.ID, Source: "direct"}); err != nil {
			return err
		}
		if _, err := e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activity.ID, PurposeText: "จ่ายเงินเดือน", LawfulBasisCode: basis}); err != nil {
			return err
		}
		if _, err := e.svc.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: activity.ID, RetentionBasis: "กฎหมายแรงงาน", TriggerEvent: "สิ้นสุดการจ้าง", DisposalMethod: "destroy"}); err != nil {
			return err
		}
		// Each Add* above recomputes and persists completeness (a real column, protected by the same
		// row_version as every other field), so row_version has moved on since the create above.
		beforeCore, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		if _, err := e.svc.SaveActivity(ctx, ropaservice.Activity{ID: activity.ID, LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: activity.Code,
			Name: activity.Name, Role: "controller", RightsAndAccess: "ติดต่อ HR ที่ hr@example.com"}, beforeCore.RowVersion); err != nil {
			return err
		}
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		if len(got.MissingItems) != 0 || got.Completeness != 100 {
			t.Errorf("expected a fully complete activity, got %d%% missing %v", got.Completeness, got.MissingItems)
		}
		submitted, err := e.svc.SubmitActivity(ctx, activity.ID, got.RowVersion)
		if err != nil {
			return err
		}
		if submitted.Status != "pending_approval" {
			t.Errorf("submit did not transition: %+v", submitted)
		}
		return nil
	})
}

func TestActivities_ProcessorRequiresController(t *testing.T) {
	e := setup(t, "ropaactprocessor")
	var le orgservice.LegalEntity
	var unit orgservice.OrgUnit
	var party orgservice.ExternalParty
	var activity ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"}); err != nil {
			return err
		}
		if party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "ลูกค้า", CountryCode: "TH"}, 0); err != nil {
			return err
		}
		activity, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "IT-01", Name: "ประมวลผลแทนลูกค้า", Role: "processor"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range got.MissingItems {
			if m == "controller" {
				found = true
			}
		}
		if !found {
			t.Errorf("processor without a controller_party_id should be missing 'controller', got %v", got.MissingItems)
		}
		updated, err := e.svc.SaveActivity(ctx, ropaservice.Activity{ID: activity.ID, LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: activity.Code,
			Name: activity.Name, Role: "processor", ControllerPartyID: &party.ID}, got.RowVersion)
		if err != nil {
			return err
		}
		if updated.ControllerPartyID == nil {
			t.Fatal("controller_party_id did not stick")
		}
		for _, m := range updated.MissingItems {
			if m == "controller" {
				t.Errorf("controller should no longer be missing once controller_party_id is set: %v", updated.MissingItems)
			}
		}
		return nil
	})
}

func TestActivities_SensitiveDataNeedsConsentEvidence(t *testing.T) {
	e := setup(t, "ropaactsensitive")
	var le orgservice.LegalEntity
	var unit orgservice.OrgUnit
	var sensitive orgservice.MasterItem
	var subjectType orgservice.MasterItem
	var basis string
	var activity ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"}); err != nil {
			return err
		}
		sensitive, _ = twoCategories(t, ctx, e.org)
		subjects, err := e.org.ListMaster(ctx, "data_subject_types")
		if err != nil {
			return err
		}
		subjectType = subjects[0]
		basis = lawfulBasisCode(t, ctx, e.org)
		activity, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-03", Name: "ตรวจสุขภาพพนักงาน", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		_, err = e.svc.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: activity.ID, DataCategoryID: *sensitive.ID, SubjectTypeID: *subjectType.ID, Source: "direct"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		// No consent evidence yet, and a purpose without one is refused up front.
		bogus := uuid.New()
		if _, err := e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activity.ID, PurposeText: "ตรวจสุขภาพ", LawfulBasisCode: basis, ConsentPurposeID: &bogus}); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("unknown consent purpose: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activity.ID, PurposeText: "ตรวจสุขภาพ", LawfulBasisCode: basis}); err != nil {
			return err
		}
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range got.MissingItems {
			if m == "sensitive_consent" {
				found = true
			}
		}
		if !found {
			t.Errorf("sensitive data without an explicit-consent purpose should flag sensitive_consent, got %v", got.MissingItems)
		}

		consentPurpose := insertConsentPurpose(t, ctx, le.ID, basis)
		if _, err := e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activity.ID, PurposeText: "ตรวจสุขภาพ (ยินยอม)", LawfulBasisCode: basis, ConsentPurposeID: &consentPurpose}); err != nil {
			return err
		}
		got, err = e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		for _, m := range got.MissingItems {
			if m == "sensitive_consent" {
				t.Errorf("sensitive_consent should clear once a purpose carries consent evidence: %v", got.MissingItems)
			}
		}
		return nil
	})
}

// TestActivities_ConsentLawfulBasisNeedsPurpose is ROPA-06's acceptance criterion: a purpose that
// relies on the consent lawful basis must already point at a real Purpose in the consent module
// before it can be saved at all — regardless of whether the underlying data is sensitive.
func TestActivities_ConsentLawfulBasisNeedsPurpose(t *testing.T) {
	e := setup(t, "ropaactconsentbasis")
	var le orgservice.LegalEntity
	var unit orgservice.OrgUnit
	var consentBasis string
	var activity ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "MKT", NameTh: "MKT", UnitType: "department"}); err != nil {
			return err
		}
		consentBasis = consentLawfulBasisCode(t, ctx, e.org)
		activity, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "MKT-01", Name: "การตลาดทางอีเมล", Role: "controller"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activity.ID, PurposeText: "ส่งอีเมลการตลาด", LawfulBasisCode: consentBasis}); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("consent-basis purpose without a Purpose link: %v, want ErrInvalid", err)
		}
		consentPurpose := insertConsentPurpose(t, ctx, le.ID, consentBasis)
		created, err := e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activity.ID, PurposeText: "ส่งอีเมลการตลาด", LawfulBasisCode: consentBasis, ConsentPurposeID: &consentPurpose})
		if err != nil {
			return err
		}
		if created.ConsentPurposeID == nil || *created.ConsentPurposeID != consentPurpose {
			t.Errorf("consent purpose link did not stick: %+v", created)
		}
		return nil
	})
}

func TestActivities_SubmitBlocked(t *testing.T) {
	e := setup(t, "ropaactsubmit")
	var le orgservice.LegalEntity
	var unit orgservice.OrgUnit
	var activity ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"}); err != nil {
			return err
		}
		activity, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-04", Name: "ยังไม่ครบ", Role: "controller"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		var inc *ropaservice.IncompleteError
		if _, err := e.svc.SubmitActivity(ctx, activity.ID, activity.RowVersion); !errors.As(err, &inc) {
			t.Errorf("submit while incomplete: %v, want an IncompleteError", err)
		} else if len(inc.Missing) == 0 {
			t.Error("IncompleteError should list what's missing")
		}
		if _, err := e.svc.SubmitActivity(ctx, activity.ID, 99); !errors.Is(err, ropaservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

func TestActivities_Isolation(t *testing.T) {
	a := setup(t, "ropaacta")
	b := setup(t, "ropaactb")
	var created ropaservice.Activity
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "เอ", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "U", NameTh: "U", UnitType: "department"})
		if err != nil {
			return err
		}
		created, err = a.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "A-01", Name: "เอ", Role: "controller"}, 0)
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetActivity(ctx, created.ID); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's activity: %v", err)
		}
		list, _, err := b.svc.ListActivities(ctx, ropaservice.ActivityFilter{})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B's list should be empty, got %+v", list)
		}
		return nil
	})
}
