package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// wizardLegalEntityAndUnit seeds a legal entity + org unit, the same minimal setup every ROPA test uses.
func wizardLegalEntityAndUnit(t *testing.T, ctx context.Context, e env) orgservice.OrgUnit {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	return unit
}

func wizardOneCategory(t *testing.T, ctx context.Context, e env) uuid.UUID {
	t.Helper()
	items, err := e.org.ListMaster(ctx, "data_categories")
	if err != nil || len(items) == 0 || items[0].ID == nil {
		t.Fatalf("expected at least one seeded data category, got %v err=%v", items, err)
	}
	return *items[0].ID
}

func wizardOneSubjectType(t *testing.T, ctx context.Context, e env) uuid.UUID {
	t.Helper()
	items, err := e.org.ListMaster(ctx, "data_subject_types")
	if err != nil || len(items) == 0 || items[0].ID == nil {
		t.Fatalf("expected at least one seeded data subject type, got %v err=%v", items, err)
	}
	return *items[0].ID
}

func wizardBaseInput(t *testing.T, ctx context.Context, e env, unit orgservice.OrgUnit, code string) ropaservice.WizardInput {
	t.Helper()
	return ropaservice.WizardInput{
		LegalEntityID: unit.LegalEntityID, OrgUnitID: unit.ID, Code: code, Name: "ระบบติดตามผลงานพนักงาน",
		Purpose: "ใช้ประเมินผลงานประจำปี", LegalReason: ropaservice.WizardReasonContract,
		SubjectTypeID: wizardOneSubjectType(t, ctx, e), DataCategoryIDs: []uuid.UUID{wizardOneCategory(t, ctx, e)},
		Retention: ropaservice.WizardRetentionContractPlus1Y,
	}
}

// TestCreateActivityFromWizard_HappyPath is RTG-05's own acceptance criterion: answering the
// plain-language questions creates a real activity with a purpose, data and retention rule already in
// place — no legal jargon required from the caller.
func TestCreateActivityFromWizard_HappyPath(t *testing.T) {
	e := setup(t, "ropawizard")
	e.in(t, func(ctx context.Context) error {
		unit := wizardLegalEntityAndUnit(t, ctx, e)
		in := wizardBaseInput(t, ctx, e, unit, "WZ-01")

		a, err := e.svc.CreateActivityFromWizard(ctx, in)
		if err != nil {
			t.Fatalf("CreateActivityFromWizard: %v", err)
		}
		if a.Code != "WZ-01" || a.Role != "controller" || a.RightsAndAccess == "" {
			t.Errorf("unexpected activity: %+v", a)
		}

		purposes, err := e.svc.ListActivityPurposes(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(purposes) != 1 || purposes[0].LawfulBasisCode != "CONTRACT" || purposes[0].PurposeText != in.Purpose {
			t.Errorf("purpose not mapped from plain language: %+v", purposes)
		}

		data, err := e.svc.ListActivityData(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(data) != 1 || data[0].DataCategoryID != in.DataCategoryIDs[0] || data[0].SubjectTypeID != in.SubjectTypeID {
			t.Errorf("data not mapped from plain language: %+v", data)
		}

		rules, err := e.svc.ListRetentionRules(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(rules) != 1 || rules[0].DisposalMethod != "delete" {
			t.Errorf("retention rule not mapped from plain language: %+v", rules)
		}
		return nil
	})
}

// TestCreateActivityFromWizard_ConsentRequiresPurpose is ROPA-06's own rule surfacing through the
// wizard: picking "the person agreed" without choosing a real consent purpose is refused before
// anything is written.
func TestCreateActivityFromWizard_ConsentRequiresPurpose(t *testing.T) {
	e := setup(t, "ropawizardconsent")
	e.in(t, func(ctx context.Context) error {
		unit := wizardLegalEntityAndUnit(t, ctx, e)
		in := wizardBaseInput(t, ctx, e, unit, "WZ-02")
		in.LegalReason = ropaservice.WizardReasonConsent

		if _, err := e.svc.CreateActivityFromWizard(ctx, in); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("consent reason without a consent purpose: %v, want ErrInvalid", err)
		}

		consentPurpose := insertConsentPurpose(t, ctx, unit.LegalEntityID, "CONSENT")
		in.ConsentPurposeID = &consentPurpose
		in.Code = "WZ-02b"
		a, err := e.svc.CreateActivityFromWizard(ctx, in)
		if err != nil {
			t.Fatalf("CreateActivityFromWizard with a real consent purpose: %v", err)
		}
		purposes, err := e.svc.ListActivityPurposes(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(purposes) != 1 || purposes[0].LawfulBasisCode != "CONSENT" || purposes[0].ConsentPurposeID == nil || *purposes[0].ConsentPurposeID != consentPurpose {
			t.Errorf("consent purpose link not carried through: %+v", purposes)
		}
		return nil
	})
}

// TestCreateActivityFromWizard_RecipientOptional covers the optional "who else sees it" question, and
// that leaving it out creates no recipient row at all.
func TestCreateActivityFromWizard_RecipientOptional(t *testing.T) {
	e := setup(t, "ropawizardrecipient")
	e.in(t, func(ctx context.Context) error {
		unit := wizardLegalEntityAndUnit(t, ctx, e)

		withoutRecipient := wizardBaseInput(t, ctx, e, unit, "WZ-03")
		a, err := e.svc.CreateActivityFromWizard(ctx, withoutRecipient)
		if err != nil {
			return err
		}
		recipients, err := e.svc.ListActivityRecipients(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(recipients) != 0 {
			t.Errorf("no recipient question answered: expected no recipient rows, got %+v", recipients)
		}

		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการคลาวด์", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		withRecipient := wizardBaseInput(t, ctx, e, unit, "WZ-04")
		withRecipient.RecipientKind = ropaservice.WizardRecipientProcessor
		withRecipient.RecipientPartyID = &party.ID
		withRecipient.RecipientNote = "เก็บไฟล์บนคลาวด์"
		b, err := e.svc.CreateActivityFromWizard(ctx, withRecipient)
		if err != nil {
			t.Fatalf("CreateActivityFromWizard with a recipient: %v", err)
		}
		recipients, err = e.svc.ListActivityRecipients(ctx, b.ID)
		if err != nil {
			return err
		}
		if len(recipients) != 1 || recipients[0].RecipientRole != "processor" || recipients[0].PartyID != party.ID {
			t.Errorf("recipient not mapped from plain language: %+v", recipients)
		}
		return nil
	})
}

// TestCreateActivityFromWizard_Validation covers the rest of the wizard's own fixed validation.
func TestCreateActivityFromWizard_Validation(t *testing.T) {
	e := setup(t, "ropawizardvalid")
	e.in(t, func(ctx context.Context) error {
		unit := wizardLegalEntityAndUnit(t, ctx, e)
		base := wizardBaseInput(t, ctx, e, unit, "WZ-05")

		noPurpose := base
		noPurpose.Purpose = "  "
		if _, err := e.svc.CreateActivityFromWizard(ctx, noPurpose); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("empty purpose: %v, want ErrInvalid", err)
		}

		badReason := base
		badReason.LegalReason = "because_we_felt_like_it"
		if _, err := e.svc.CreateActivityFromWizard(ctx, badReason); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("unknown legal_reason: %v, want ErrInvalid", err)
		}

		badRetention := base
		badRetention.Retention = "forever"
		if _, err := e.svc.CreateActivityFromWizard(ctx, badRetention); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("unknown retention: %v, want ErrInvalid", err)
		}

		noData := base
		noData.DataCategoryIDs = nil
		if _, err := e.svc.CreateActivityFromWizard(ctx, noData); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("no data categories: %v, want ErrInvalid", err)
		}

		recipientWithoutParty := base
		recipientWithoutParty.RecipientKind = ropaservice.WizardRecipientGovt
		if _, err := e.svc.CreateActivityFromWizard(ctx, recipientWithoutParty); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("recipient_kind without a party: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestCreateActivityFromWizard_TwoTenantIsolation mirrors the rest of this codebase's own isolation tests.
func TestCreateActivityFromWizard_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "ropawizardiso1")
	b := setup(t, "ropawizardiso2")
	var unitA orgservice.OrgUnit
	a.in(t, func(ctx context.Context) error {
		unitA = wizardLegalEntityAndUnit(t, ctx, a)
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		in := wizardBaseInput(t, ctx, b, unitA, "WZ-06")
		if _, err := b.svc.CreateActivityFromWizard(ctx, in); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("tenant B using tenant A's legal entity/org unit: %v, want ErrInvalid", err)
		}
		return nil
	})
}
