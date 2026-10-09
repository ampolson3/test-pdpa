package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	agreementstore "pdpa-platform/internal/agreement/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// VendorContractStatus is VEN-11's own acceptance criterion: whether a vendor who is a processor has at
// least one DPA on record (ม.40 requires a processor to have one). Computed live on every call — never
// persisted, so it can never go stale the way a stored flag would (the same rule DSAR-07's own SLAStatus
// and ROPA-08's own completeness item already use).
type VendorContractStatus struct {
	VendorID    uuid.UUID
	IsProcessor bool
	HasDPA      bool
}

// VendorContractStatus checks a single vendor — the module doc's own frontend note scopes this to one
// vendor's own detail page ("แท็บความเชื่อมโยงของคู่ค้า"), not a cross-vendor monitoring list.
func (s *Service) VendorContractStatus(ctx context.Context, vendorID uuid.UUID) (VendorContractStatus, error) {
	v, err := s.Vendor.GetVendor(ctx, vendorID)
	if err != nil {
		return VendorContractStatus{}, fmt.Errorf("%w: vendor_id", ErrInvalid)
	}
	count, err := agreementstore.New(pdb.MustTxFromContext(ctx)).CountAgreementsForVendorByType(ctx,
		agreementstore.CountAgreementsForVendorByTypeParams{VendorID: pgUUID(&vendorID), AgreementType: "dpa"})
	if err != nil {
		return VendorContractStatus{}, err
	}
	return VendorContractStatus{VendorID: vendorID, IsProcessor: v.IsProcessor, HasDPA: count > 0}, nil
}
