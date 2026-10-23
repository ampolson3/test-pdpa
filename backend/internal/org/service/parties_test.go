package service_test

import (
	"context"
	"errors"
	"testing"

	orgservice "pdpa-platform/internal/org/service"
)

func TestExternalParties_CRUDAndValidation(t *testing.T) {
	e := setup(t, "orgparty")
	var created orgservice.ExternalParty
	e.in(t, func(ctx context.Context) error {
		var err error
		created, err = e.svc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "บริษัท ประมวลผล จำกัด",
			CountryCode: "th", Contact: orgservice.Contact{Email: "a@example.com"}}, 0)
		if err != nil {
			return err
		}
		if created.CountryCode != "TH" || created.Status != "active" || created.RowVersion != 1 {
			t.Errorf("create did not stick: %+v", created)
		}
		got, err := e.svc.GetExternalParty(ctx, created.ID)
		if err != nil {
			return err
		}
		if got.NameTh != created.NameTh || got.Contact.Email != "a@example.com" {
			t.Errorf("read back mismatch: %+v", got)
		}
		for name, in := range map[string]orgservice.ExternalParty{
			"no name":        {PartyType: "processor", CountryCode: "TH"},
			"bad party type": {PartyType: "bogus", NameTh: "x", CountryCode: "TH"},
			"bad country":    {PartyType: "processor", NameTh: "x", CountryCode: "T"},
			"bad email":      {PartyType: "processor", NameTh: "x", CountryCode: "TH", Contact: orgservice.Contact{Email: "not-an-email"}},
		} {
			if _, err := e.svc.SaveExternalParty(ctx, in, 0); !errors.Is(err, orgservice.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		return nil
	})

	// Update: stale version refused; a real update bumps row_version and sticks.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveExternalParty(ctx, orgservice.ExternalParty{ID: created.ID, PartyType: "processor", NameTh: "x", CountryCode: "TH"}, 0); !errors.Is(err, orgservice.ErrVersionMismatch) {
			t.Errorf("stale version: %v, want ErrVersionMismatch", err)
		}
		updated, err := e.svc.SaveExternalParty(ctx, orgservice.ExternalParty{ID: created.ID, PartyType: "recipient", NameTh: "บริษัท รับข้อมูล จำกัด", CountryCode: "TH"}, created.RowVersion)
		if err != nil {
			return err
		}
		if updated.PartyType != "recipient" || updated.RowVersion != 2 {
			t.Errorf("update did not stick: %+v", updated)
		}
		return nil
	})
}

func TestExternalParties_DuplicatesAndMerge(t *testing.T) {
	e := setup(t, "orgpartydup")
	var a, b, c orgservice.ExternalParty
	e.in(t, func(ctx context.Context) error {
		var err error
		if a, err = e.svc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "บริษัท เอ", CountryCode: "TH"}, 0); err != nil {
			return err
		}
		// Same normalized name + country (extra spaces) — a duplicate of a.
		if b, err = e.svc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "บริษัท  เอ ", CountryCode: "TH"}, 0); err != nil {
			return err
		}
		// A different country — not a duplicate.
		if c, err = e.svc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "บริษัท เอ", CountryCode: "US"}, 0); err != nil {
			return err
		}
		// An unrelated party — never appears as a duplicate.
		if _, err = e.svc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "recipient", NameTh: "บริษัท บี", CountryCode: "TH"}, 0); err != nil {
			return err
		}

		groups, err := e.svc.DuplicateExternalParties(ctx)
		if err != nil {
			return err
		}
		if len(groups) != 1 {
			t.Fatalf("want exactly one duplicate group, got %d: %+v", len(groups), groups)
		}
		for _, members := range groups {
			if len(members) != 2 {
				t.Errorf("want a and b together, got %+v", members)
			}
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		// Merging into itself is refused.
		if err := e.svc.MergeExternalParty(ctx, a.ID, a.ID, a.RowVersion); !errors.Is(err, orgservice.ErrInvalid) {
			t.Errorf("merge into self: %v, want ErrInvalid", err)
		}
		if err := e.svc.MergeExternalParty(ctx, b.ID, a.ID, b.RowVersion); err != nil {
			return err
		}
		merged, err := e.svc.GetExternalParty(ctx, b.ID)
		if err != nil {
			return err
		}
		if merged.Status != "inactive" || merged.MergedIntoID == nil || *merged.MergedIntoID != a.ID {
			t.Errorf("b should be merged into a: %+v", merged)
		}
		// The pair no longer shows as a duplicate (b is inactive/merged); c (different country) never did.
		groups, err := e.svc.DuplicateExternalParties(ctx)
		if err != nil {
			return err
		}
		if len(groups) != 0 {
			t.Errorf("no duplicates should remain: %+v", groups)
		}
		// A merged party can't be merged again, nor edited.
		if err := e.svc.MergeExternalParty(ctx, b.ID, c.ID, merged.RowVersion); !errors.Is(err, orgservice.ErrPartyMerged) {
			t.Errorf("re-merge: %v, want ErrPartyMerged", err)
		}
		if _, err := e.svc.SaveExternalParty(ctx, orgservice.ExternalParty{ID: b.ID, PartyType: "processor", NameTh: "x", CountryCode: "TH"}, merged.RowVersion); !errors.Is(err, orgservice.ErrPartyMerged) {
			t.Errorf("edit a merged party: %v, want ErrPartyMerged", err)
		}
		return nil
	})
}

func TestExternalParties_Isolation(t *testing.T) {
	a := setup(t, "orgpartya")
	b := setup(t, "orgpartyb")
	var created orgservice.ExternalParty
	a.in(t, func(ctx context.Context) error {
		var err error
		created, err = a.svc.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "เอ", CountryCode: "TH"}, 0)
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetExternalParty(ctx, created.ID); !errors.Is(err, orgservice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's party: %v", err)
		}
		list, _, err := b.svc.ListExternalParties(ctx, orgservice.PartyFilter{})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B's list should be empty, got %+v", list)
		}
		return nil
	})
}
