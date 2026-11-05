package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

// approveVendor marks a vendor "approved" directly (VEN-02/07's own transition isn't built here, and this
// package's own fixture() never writes vendor.status — new rows default to "prospect").
func approveVendor(t *testing.T, ctx context.Context, id uuid.UUID) {
	t.Helper()
	if _, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE vendor.vendors SET status = 'approved' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

// TestListVendorsMissingDPA_FindsEveryOne is DPA-12's own acceptance criterion directly ("คู่ค้าที่ขาด DPA
// ถูกพบทุกราย"): an approved processor vendor with no active DPA is found; one with an active DPA is not;
// a vendor still at "prospect" (never approved) is not either, since it isn't yet a real business
// relationship ม.40 applies to.
func TestListVendorsMissingDPA_FindsEveryOne(t *testing.T) {
	e := setup(t, "dpa12find")
	var leID, missingVendorID, coveredVendorID, prospectVendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, missingVendorID, activityID = fixture(t, ctx, e)
		approveVendor(t, ctx, missingVendorID)

		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการที่มี DPA แล้ว", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		v, err := e.vendor.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "มี DPA ที่มีผลแล้ว", IsProcessor: true}, 0)
		if err != nil {
			return err
		}
		coveredVendorID = v.ID
		approveVendor(t, ctx, coveredVendorID)

		partyP, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ยังไม่ผ่านการอนุมัติ", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		vp, err := e.vendor.SaveVendor(ctx, vendorservice.Vendor{PartyID: partyP.ID, ServiceDescription: "prospect เฉยๆ", IsProcessor: true}, 0)
		if err != nil {
			return err
		}
		prospectVendorID = vp.ID

		// coveredVendorID gets a real DPA that reaches "active" via the real ST-04 lifecycle (OnDocumentTransition).
		a, err := e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", VendorID: coveredVendorID, LegalEntityID: leID,
			ActivityIDs: []uuid.UUID{activityID}, Title: "DPA ที่มีผลแล้ว",
		})
		if err != nil {
			return err
		}
		if _, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE agreement.agreements SET status = 'active' WHERE id = $1`, a.ID); err != nil {
			return err
		}
		return nil
	})

	var missing []agreementservice.MissingDPAVendor
	e.in(t, func(ctx context.Context) error {
		var err error
		missing, err = e.svc.ListVendorsMissingDPA(ctx)
		return err
	})

	foundMissing, foundCovered, foundProspect := false, false, false
	for _, v := range missing {
		switch v.VendorID {
		case missingVendorID:
			foundMissing = true
		case coveredVendorID:
			foundCovered = true
		case prospectVendorID:
			foundProspect = true
		}
	}
	if !foundMissing {
		t.Error("expected the approved vendor with no DPA to be found missing")
	}
	if foundCovered {
		t.Error("a vendor with an active DPA should not be flagged missing")
	}
	if foundProspect {
		t.Error("a prospect vendor (never approved) should not be flagged missing")
	}
}

// TestCheckMissingDPA_OpensOneTaskPerVendorAndDedupes is the other half of the acceptance criterion: the
// daily sweep opens exactly one dpo.tasks job per vendor newly found missing, and a repeat sweep against
// the same still-missing vendor is a harmless no-op rather than a second task.
func TestCheckMissingDPA_OpensOneTaskPerVendorAndDedupes(t *testing.T) {
	e := setup(t, "dpa12task")
	var vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		_, vendorID, _ = fixture(t, ctx, e)
		approveVendor(t, ctx, vendorID)
		return nil
	})

	dpo := &fakeDpo{}
	e.svc.Dpo = dpo

	e.in(t, func(ctx context.Context) error {
		opened, err := e.svc.CheckMissingDPA(ctx)
		if err != nil {
			return err
		}
		if opened != 1 {
			t.Errorf("opened = %d, want 1", opened)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		opened, err := e.svc.CheckMissingDPA(ctx)
		if err != nil {
			return err
		}
		if opened != 0 {
			t.Errorf("second sweep opened = %d, want 0 (dedup)", opened)
		}
		return nil
	})
	if len(dpo.opened) != 1 {
		t.Fatalf("len(opened tasks) = %d, want 1", len(dpo.opened))
	}
	if dpo.opened[0] != vendorID {
		t.Errorf("task opened for vendor %v, want %v", dpo.opened[0], vendorID)
	}
}

// TestListVendorsMissingDPA_TwoTenantIsolation proves one tenant's own approved-but-uncovered vendor never
// leaks into another tenant's own missing-DPA list.
func TestListVendorsMissingDPA_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpa12isoA")
	b := setup(t, "dpa12isoB")

	var vendorID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		_, vendorID, _ = fixture(t, ctx, a)
		approveVendor(t, ctx, vendorID)
		return nil
	})

	var bMissing []agreementservice.MissingDPAVendor
	b.in(t, func(ctx context.Context) error {
		var err error
		bMissing, err = b.svc.ListVendorsMissingDPA(ctx)
		return err
	})
	for _, v := range bMissing {
		if v.VendorID == vendorID {
			t.Fatal("tenant B's missing-DPA list leaked tenant A's own vendor")
		}
	}
}

type fakeDpo struct {
	opened []uuid.UUID
}

func (f *fakeDpo) HasTaskForVendorAgreement(ctx context.Context, vendorID uuid.UUID) (bool, error) {
	for _, id := range f.opened {
		if id == vendorID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeDpo) OpenMissingDPATask(ctx context.Context, vendorID uuid.UUID, title, description string) (uuid.UUID, error) {
	f.opened = append(f.opened, vendorID)
	return uuid.New(), nil
}
