package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
)

// TestVendorContractStatus_FlagsProcessorWithoutDPA is VEN-11's own acceptance criterion directly: a
// processor vendor with no DPA on record reads has_dpa=false; creating one flips it to true without any
// change to is_processor.
func TestVendorContractStatus_FlagsProcessorWithoutDPA(t *testing.T) {
	e := setup(t, "ven11")
	var legalEntityID, vendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		legalEntityID, vendorID, activityID = fixture(t, ctx, e)
		return nil
	})
	_ = legalEntityID
	_ = activityID

	e.in(t, func(ctx context.Context) error {
		st, err := e.svc.VendorContractStatus(ctx, vendorID)
		if err != nil {
			return err
		}
		if !st.IsProcessor {
			t.Errorf("is_processor = false, want true (fixture creates a processor vendor)")
		}
		if st.HasDPA {
			t.Errorf("has_dpa = true before any agreement exists, want false")
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: legalEntityID,
			ActivityIDs: []uuid.UUID{activityID}, Title: "ทดสอบ VEN-11",
		})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		st, err := e.svc.VendorContractStatus(ctx, vendorID)
		if err != nil {
			return err
		}
		if !st.HasDPA {
			t.Error("has_dpa = false after creating a DPA for this vendor, want true")
		}
		return nil
	})
}

func TestVendorContractStatus_UnknownVendorRefused(t *testing.T) {
	e := setup(t, "ven11unknown")
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.VendorContractStatus(ctx, uuid.New()); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Errorf("unknown vendor: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestVendorContractStatus_TwoTenantIsolation proves a DPA created for tenant A's vendor is never visible
// when checking tenant B's own differently-created vendor.
func TestVendorContractStatus_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "ven11a")
	b := setup(t, "ven11b")
	var aLegalEntityID, aVendorID, aActivityID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		aLegalEntityID, aVendorID, aActivityID = fixture(t, ctx, a)
		return nil
	})
	a.in(t, func(ctx context.Context) error {
		_, err := a.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", VendorID: aVendorID, LegalEntityID: aLegalEntityID,
			ActivityIDs: []uuid.UUID{aActivityID}, Title: "เอ",
		})
		return err
	})

	var bVendorID uuid.UUID
	b.in(t, func(ctx context.Context) error {
		_, bVendorID, _ = fixture(t, ctx, b)
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		st, err := b.svc.VendorContractStatus(ctx, bVendorID)
		if err != nil {
			return err
		}
		if st.HasDPA {
			t.Error("tenant B's vendor should not see tenant A's DPA: has_dpa = true, want false")
		}
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.VendorContractStatus(ctx, aVendorID); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Errorf("tenant B checking tenant A's vendor id: %v, want ErrInvalid (not visible under RLS)", err)
		}
		return nil
	})
}
