// DPA-07 ตรวจทานและอนุมัติ (review & approval): keeps agreement.agreements.status (ST-04) in step with
// the underlying PLT-08 version's own review lifecycle, and gates the one action this feature's literal
// acceptance criterion names — sending for signature — on that status already being "approved". The
// actual e-signature exchange (ESIGN provider, signature_requests, the vendor-facing guest link) is
// DPA-09's own job, a separate, un-depended-upon backlog row; this feature only owns the status column
// and the one gate, the same "build only the minimal slice this feature needs" reasoning this codebase
// uses throughout.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	agreementstore "pdpa-platform/internal/agreement/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// OnDocumentTransition is docs.Service's own SetOnTransition hook for "dpa"/"dsa" (wired in cmd/api's own
// docsSvc.SetOnTransition calls, once both docs.Service and this service exist — the same sequencing
// SetSubmitValidate/SetValidate already established): status is versioning.Policy's own vocabulary
// ("in_review" on Submit, "draft" on Decide: rejected/returned, "approved" on Decide: the last step
// approved) and maps onto ST-04's identically-named states one for one — rule 5's own requirement, that
// only the owning module's service method ever writes agreement.agreements.status, is satisfied here:
// docs/versioning never touch the column themselves, they only report what happened on the version they
// do own. A document_id that isn't an agreement (shouldn't happen for "dpa"/"dsa", but the hook is generic
// across every PLT-16 type) is a harmless no-op, not an error.
func (s *Service) OnDocumentTransition(ctx context.Context, documentID uuid.UUID, status string) error {
	switch status {
	case "in_review", "draft", "approved":
	default:
		return nil // out_for_signature/active/expired/terminated are this module's own transitions, never the hook's
	}
	n, err := agreementstore.New(pdb.MustTxFromContext(ctx)).SetAgreementStatusByDocumentID(ctx,
		agreementstore.SetAgreementStatusByDocumentIDParams{DocumentID: documentID, Status: status})
	if err != nil || n == 0 {
		return err
	}
	return s.audit(ctx, "agreement.status.transition", documentID, nil, map[string]any{"status": status})
}

// SendForSignature is DPA-07's own acceptance criterion, literally: "ส่งลงนามได้หลังอนุมัติเท่านั้น" — the
// agreement can move from approved to out_for_signature only from approved, enforced by the guarded UPDATE
// itself (WHERE status = 'approved'), not by a separate read-then-check that a concurrent change could race.
func (s *Service) SendForSignature(ctx context.Context, id uuid.UUID, rowVersion int32) (Agreement, error) {
	before, err := s.GetAgreement(ctx, id)
	if err != nil {
		return Agreement{}, err
	}
	if before.RowVersion != rowVersion {
		return Agreement{}, ErrVersionMismatch
	}
	if before.Status != "approved" {
		return Agreement{}, fmt.Errorf("%w: agreement is %q, not approved", ErrInvalidTransition, before.Status)
	}
	row, err := agreementstore.New(pdb.MustTxFromContext(ctx)).SendAgreementForSignature(ctx,
		agreementstore.SendAgreementForSignatureParams{ID: id, RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return Agreement{}, ErrVersionMismatch // approved a moment ago, but row_version (or status) moved under us
	}
	if err != nil {
		return Agreement{}, err
	}
	after := toAgreement(row, before.ActivityIDs)
	if err := s.audit(ctx, "agreement.status.send_for_signature", id,
		map[string]any{"status": before.Status}, map[string]any{"status": after.Status}); err != nil {
		return Agreement{}, err
	}
	return after, nil
}
