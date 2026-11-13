package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
)

// TestAddParty_SupportsMoreThanTwoParties is DSA-02's own acceptance criterion directly: an agreement
// starts with one party (the counterparty CreateWizard wrote) and can grow past two — an external party
// plus our own legal entity named as a joint controller.
func TestAddParty_SupportsMoreThanTwoParties(t *testing.T) {
	e := setup(t, "dsa02")
	var leID, vendorID, activityID, thirdPartyID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, []uuid.UUID{activityID})

	e.in(t, func(ctx context.Context) error {
		parties, err := e.svc.ListParties(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(parties) != 1 {
			t.Fatalf("parties after create = %d, want 1 (the counterparty)", len(parties))
		}
		return nil
	})

	// A third party: another external party (receiving the data alongside the original counterparty).
	e.in(t, func(ctx context.Context) error {
		_, thirdPartyID, _ = fixture(t, ctx, e)
		_, err := e.svc.AddParty(ctx, a.ID, agreementservice.AddPartyInput{PartyID: &thirdPartyID, PartyRole: "receiving"})
		return err
	})

	// A fourth party: our own legal entity, named as a joint controller.
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.AddParty(ctx, a.ID, agreementservice.AddPartyInput{LegalEntityID: &leID, PartyRole: "joint_controller"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		parties, err := e.svc.ListParties(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(parties) != 3 {
			t.Fatalf("parties after adding two more = %d, want 3", len(parties))
		}
		return nil
	})
}

func TestAddParty_Validation(t *testing.T) {
	e := setup(t, "dsa02validate")
	var leID, vendorID, activityID, partyID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)
		_, partyID, _ = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, []uuid.UUID{activityID})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddParty(ctx, a.ID, agreementservice.AddPartyInput{PartyID: &partyID, LegalEntityID: &leID, PartyRole: "receiving"}); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Errorf("both party_id and legal_entity_id set: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.AddParty(ctx, a.ID, agreementservice.AddPartyInput{PartyRole: "receiving"}); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Errorf("neither set: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.AddParty(ctx, a.ID, agreementservice.AddPartyInput{PartyID: &partyID, PartyRole: "not_a_role"}); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Errorf("unknown party_role: %v, want ErrInvalid", err)
		}
		unknown := uuid.New()
		if _, err := e.svc.AddParty(ctx, a.ID, agreementservice.AddPartyInput{PartyID: &unknown, PartyRole: "receiving"}); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Errorf("unknown party_id: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.AddParty(ctx, uuid.New(), agreementservice.AddPartyInput{PartyID: &partyID, PartyRole: "receiving"}); !errors.Is(err, agreementservice.ErrNotFound) {
			t.Errorf("unknown agreement: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func TestRemoveParty(t *testing.T) {
	e := setup(t, "dsa02remove")
	var leID, vendorID, activityID, partyID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)
		_, partyID, _ = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, []uuid.UUID{activityID})

	var added agreementservice.Party
	e.in(t, func(ctx context.Context) error {
		var err error
		added, err = e.svc.AddParty(ctx, a.ID, agreementservice.AddPartyInput{PartyID: &partyID, PartyRole: "receiving"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		if err := e.svc.RemoveParty(ctx, a.ID, added.ID); err != nil {
			return err
		}
		parties, err := e.svc.ListParties(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(parties) != 1 {
			t.Fatalf("parties after remove = %d, want 1 (back to just the counterparty)", len(parties))
		}
		if err := e.svc.RemoveParty(ctx, a.ID, added.ID); !errors.Is(err, agreementservice.ErrNotFound) {
			t.Errorf("remove again: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestListParties_TwoTenantIsolation proves tenant B cannot list or add parties on tenant A's agreement.
func TestListParties_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dsa02a")
	b := setup(t, "dsa02b")
	var leID, vendorID, activityID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, a)
		return nil
	})
	agr := createAgreement(t, a, leID, vendorID, []uuid.UUID{activityID})

	var bPartyID uuid.UUID
	b.in(t, func(ctx context.Context) error {
		_, bPartyID, _ = fixture(t, ctx, b)
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.ListParties(ctx, agr.ID); !errors.Is(err, agreementservice.ErrNotFound) {
			t.Errorf("tenant B listing tenant A's agreement parties: %v, want ErrNotFound", err)
		}
		if _, err := b.svc.AddParty(ctx, agr.ID, agreementservice.AddPartyInput{PartyID: &bPartyID, PartyRole: "receiving"}); !errors.Is(err, agreementservice.ErrNotFound) {
			t.Errorf("tenant B adding a party to tenant A's agreement: %v, want ErrNotFound", err)
		}
		return nil
	})
}
