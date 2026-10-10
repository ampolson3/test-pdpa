package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
)

// TestCreateWizard_CreatesDsaFromCounterpartyAndActivities is DSA-04's own acceptance criterion: a "dsa"
// agreement drafts from an ORG-06 counterparty (not a VEN-01 vendor — DSA-04's counterparty is simply
// another controller, a government agency or a researcher) and its linked RoPA activities in one call, and
// its processing-schedule annex (DPA-04's own, agreement_type-agnostic) pulls that activity's purpose,
// lawful basis, data category and retention correctly — the exact same live annex DPA already uses, proving
// nothing dsa-specific had to be built there.
func TestCreateWizard_CreatesDsaFromCounterpartyAndActivities(t *testing.T) {
	e := setup(t, "dsaWizard")
	var leID, activityID, counterpartyID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		_, _, activityID = fixture(t, ctx, e)
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด 2", IsController: true}, 0)
		if err != nil {
			return err
		}
		leID = le.ID
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "หน่วยงานพันธมิตร", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		counterpartyID = party.ID
		return nil
	})

	var a agreementservice.Agreement
	e.in(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dsa", OurRole: "controller", CounterpartyPartyID: counterpartyID, CounterpartyRole: "receiving",
			LegalEntityID: leID, ActivityIDs: []uuid.UUID{activityID}, Title: "ข้อตกลงแบ่งปันข้อมูลกับหน่วยงานพันธมิตร",
		})
		return err
	})
	if a.AgreementType != "dsa" {
		t.Errorf("agreement_type = %q, want dsa", a.AgreementType)
	}
	if len(a.ActivityIDs) != 1 || a.ActivityIDs[0] != activityID {
		t.Errorf("activity_ids = %v, want [%s]", a.ActivityIDs, activityID)
	}

	e.in(t, func(ctx context.Context) error {
		parties, err := e.svc.ListParties(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(parties) != 1 || parties[0].PartyRole != "receiving" || parties[0].PartyID == nil || *parties[0].PartyID != counterpartyID {
			t.Errorf("parties = %+v, want one receiving party = %s", parties, counterpartyID)
		}
		return nil
	})

	// DSA-04's own acceptance criterion: the annex reflects the linked activity's own RoPA data — proven
	// here only that it resolves without error on a dsa agreement (the content-correctness assertions
	// already live in TestProcessingSchedule_MatchesRoPA, which is agreement_type-agnostic).
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.ProcessingSchedule(ctx, a.ID)
		return err
	})
}

func TestCreateWizard_DsaRequiresCounterpartyRole(t *testing.T) {
	e := setup(t, "dsaWizardValidate")
	var leID, activityID, counterpartyID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _, activityID = fixture(t, ctx, e)
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "หน่วยงานพันธมิตร", CountryCode: "TH"}, 0)
		counterpartyID = party.ID
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dsa", OurRole: "controller", CounterpartyPartyID: counterpartyID, LegalEntityID: leID,
			ActivityIDs: []uuid.UUID{activityID}, Title: "no role",
		}); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Errorf("missing counterparty_role: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dsa", OurRole: "controller", CounterpartyPartyID: counterpartyID, CounterpartyRole: "not_a_role", LegalEntityID: leID,
			ActivityIDs: []uuid.UUID{activityID}, Title: "bad role",
		}); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Errorf("invalid counterparty_role: %v, want ErrInvalid", err)
		}
		return nil
	})
}
