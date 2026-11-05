package service

import (
	"context"
	"strings"

	dsarstore "pdpa-platform/internal/dsar/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
)

// DSAR-21 (ม.19, ม.32): fulfilling a "withdraw_consent" request updates the Consent module automatically —
// every purpose the requester currently holds ACTIVE consent for is withdrawn, through consent's own
// Service.WithdrawAllActive (rule 9: dsar never writes to the consent schema directly), and the resulting
// receipt number is attached to the request itself (same disclosed jsonb-column pattern DSAR-03's own
// DataSource already uses, no new table). Scoped to "withdraw_consent" only, not "objection" (the module
// doc's own "คัดค้านการตลาด" half): nothing in this schema distinguishes a marketing purpose from any other
// one, so there is no reliable way to pick just the marketing purposes out of a subject's own list — add
// that once a real "is_marketing" flag exists on a purpose to filter by, the same "no consumer yet"
// deferral this codebase uses throughout. A requester with no matching consent subject, or nothing
// currently active, is a harmless no-op — most DSAR requests are never also a consent decision.
// It returns the receipt number it wrote (or nil when nothing applied), so the caller can reflect it on
// the Request value it already has in hand rather than re-reading the row back from the database.
func (s *Service) withdrawConsentIfApplicable(ctx context.Context, req Request, rt RequestType) (*string, error) {
	if s.Consent == nil || rt.Code != "withdraw_consent" {
		return nil, nil
	}
	contact, err := s.decryptRequesterContact(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(contact) == "" {
		return nil, nil
	}
	kind := string(crypto.KindPhone)
	if strings.Contains(contact, "@") {
		kind = string(crypto.KindEmail)
	}
	subjects, err := s.Consent.Subjects(ctx, kind, contact)
	if err != nil {
		return nil, err
	}
	if len(subjects) == 0 {
		return nil, nil
	}
	receipt, err := s.Consent.WithdrawAllActive(ctx, subjects[0].ID, "other")
	if err != nil {
		return nil, err
	}
	if receipt.No == "" {
		return nil, nil // nothing was active to withdraw
	}
	if err := dsarstore.New(pdb.MustTxFromContext(ctx)).SetRequestConsentReceipt(ctx, dsarstore.SetRequestConsentReceiptParams{
		ID: req.ID, ReceiptNo: receipt.No,
	}); err != nil {
		return nil, err
	}
	no := receipt.No
	return &no, nil
}
