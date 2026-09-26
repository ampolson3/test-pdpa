package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestActivityTransfers_ForeignRecipientNeedsTransfer is the acceptance criterion: a recipient
// outside Thailand is a cross-border transfer, and one without a logged mechanism is warned
// (transfer_basis in the missing-item list) until a transfer with a basis is recorded for it.
func TestActivityTransfers_ForeignRecipientNeedsTransfer(t *testing.T) {
	e := setup(t, "ropatransfer")
	var le orgservice.LegalEntity
	var unit orgservice.OrgUnit
	var thaiParty, foreignParty orgservice.ExternalParty
	var activity ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"}); err != nil {
			return err
		}
		if thaiParty, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการไทย", CountryCode: "TH"}, 0); err != nil {
			return err
		}
		if foreignParty, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการต่างประเทศ", CountryCode: "US"}, 0); err != nil {
			return err
		}
		activity, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "IT-01", Name: "ระบบคลาวด์", Role: "controller"}, 0)
		return err
	})

	var thaiRecipient, foreignRecipient ropaservice.ActivityRecipient
	e.in(t, func(ctx context.Context) error {
		var err error
		thaiRecipient, err = e.svc.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: activity.ID, PartyID: thaiParty.ID, RecipientRole: "processor", DisclosureBasis: "สัญญา"})
		if err != nil {
			return err
		}
		foreignRecipient, err = e.svc.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: activity.ID, PartyID: foreignParty.ID, RecipientRole: "processor", DisclosureBasis: "สัญญา"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range got.MissingItems {
			if m == "transfer_basis" {
				found = true
			}
		}
		if !found {
			t.Errorf("a foreign recipient with no logged transfer should flag transfer_basis, got %v", got.MissingItems)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		bogus := uuid.New()
		for name, tr := range map[string]ropaservice.ActivityTransfer{
			"bad country code":   {ActivityID: activity.ID, CountryCode: "USA", TransferBasis: "standard_clauses"},
			"unknown country":    {ActivityID: activity.ID, CountryCode: "ZZ", TransferBasis: "standard_clauses"},
			"bad transfer basis": {ActivityID: activity.ID, CountryCode: "US", TransferBasis: "bogus"},
			"unknown recipient":  {ActivityID: activity.ID, CountryCode: "US", TransferBasis: "standard_clauses", RecipientID: &bogus},
		} {
			if _, err := e.svc.AddActivityTransfer(ctx, tr); !errors.Is(err, ropaservice.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		// A recipient that legitimately belongs to this activity is accepted (proven by the real add below).
		_ = thaiRecipient
		return nil
	})

	var transfer ropaservice.ActivityTransfer
	e.in(t, func(ctx context.Context) error {
		var err error
		transfer, err = e.svc.AddActivityTransfer(ctx, ropaservice.ActivityTransfer{ActivityID: activity.ID, RecipientID: &foreignRecipient.ID,
			CountryCode: "us", TransferBasis: "standard_clauses", Safeguards: "SCC 2021"})
		if err != nil {
			return err
		}
		if transfer.CountryCode != "US" {
			t.Errorf("country code should be normalized upper-case: %+v", transfer)
		}
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		for _, m := range got.MissingItems {
			if m == "transfer_basis" {
				t.Errorf("transfer_basis should clear once the foreign recipient has a logged transfer: %v", got.MissingItems)
			}
		}
		list, err := e.svc.ListActivityTransfers(ctx, activity.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Errorf("expected one transfer, got %+v", list)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if err := e.svc.DeleteActivityTransfer(ctx, activity.ID, transfer.ID); err != nil {
			return err
		}
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range got.MissingItems {
			if m == "transfer_basis" {
				found = true
			}
		}
		if !found {
			t.Errorf("deleting the only transfer should bring transfer_basis back: %v", got.MissingItems)
		}
		return nil
	})
}

func TestActivityTransfers_Isolation(t *testing.T) {
	a := setup(t, "ropatransfera")
	b := setup(t, "ropatransferb")
	var activity ropaservice.Activity
	var transfer ropaservice.ActivityTransfer
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "เอ", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "U", NameTh: "U", UnitType: "department"})
		if err != nil {
			return err
		}
		activity, err = a.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "A-01", Name: "เอ", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		transfer, err = a.svc.AddActivityTransfer(ctx, ropaservice.ActivityTransfer{ActivityID: activity.ID, CountryCode: "US", TransferBasis: "standard_clauses"})
		return err
	})
	b.in(t, func(ctx context.Context) error {
		list, err := b.svc.ListActivityTransfers(ctx, activity.ID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B should not see tenant A's transfer, got %+v", list)
		}
		if err := b.svc.DeleteActivityTransfer(ctx, activity.ID, transfer.ID); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("tenant B deleting tenant A's transfer: %v, want ErrNotFound", err)
		}
		return nil
	})
}
