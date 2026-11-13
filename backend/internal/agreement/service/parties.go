// DSA-02 (ข้อมูลคู่สัญญาและบทบาท): CRUD on agreement.parties beyond the single counterparty row
// CreateWizard already writes — the acceptance criterion is that an agreement can carry more than two
// parties (several external parties, our own legal entity named as a joint controller, or both), each
// with its own role and signatory.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	agreementstore "pdpa-platform/internal/agreement/store"
	pdb "pdpa-platform/internal/pkg/db"
)

var partyRoles = map[string]bool{"disclosing": true, "receiving": true, "joint_controller": true, "controller": true, "processor": true}

// Party is one row on agreement.parties — either an org.external_parties counterparty (PartyID) or one of
// our own org.legal_entities named as a party in its own right (LegalEntityID, the joint-controller case),
// never both.
type Party struct {
	ID             uuid.UUID
	AgreementID    uuid.UUID
	PartyID        *uuid.UUID
	LegalEntityID  *uuid.UUID
	PartyRole      string
	SignatoryName  *string
	SignatoryEmail *string
	RowVersion     int32
	CreatedAt      time.Time
}

// AddPartyInput adds one more party to an existing agreement. Exactly one of PartyID/LegalEntityID must
// be set.
type AddPartyInput struct {
	PartyID        *uuid.UUID
	LegalEntityID  *uuid.UUID
	PartyRole      string
	SignatoryName  *string
	SignatoryEmail *string
}

func toParty(r agreementstore.AgreementParty) Party {
	return Party{
		ID: r.ID, AgreementID: r.AgreementID, PartyID: uuidPtr(r.PartyID), LegalEntityID: uuidPtr(r.LegalEntityID),
		PartyRole: r.PartyRole, SignatoryName: r.SignatoryName, SignatoryEmail: r.SignatoryEmail,
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time,
	}
}

// ListParties returns every party on the agreement, oldest first — the counterparty CreateWizard wrote
// plus any added since.
func (s *Service) ListParties(ctx context.Context, agreementID uuid.UUID) ([]Party, error) {
	if _, err := s.GetAgreement(ctx, agreementID); err != nil {
		return nil, err
	}
	rows, err := agreementstore.New(pdb.MustTxFromContext(ctx)).ListAgreementPartiesForAgreement(ctx, agreementID)
	if err != nil {
		return nil, err
	}
	out := make([]Party, 0, len(rows))
	for _, r := range rows {
		out = append(out, toParty(r))
	}
	return out, nil
}

// AddParty is the acceptance criterion itself: a third (or later) party joining the agreement, visible
// under RLS (rule 1) exactly like the vendor/legal-entity checks CreateWizard already makes before writing
// anything.
func (s *Service) AddParty(ctx context.Context, agreementID uuid.UUID, in AddPartyInput) (Party, error) {
	if _, err := s.GetAgreement(ctx, agreementID); err != nil {
		return Party{}, err
	}
	if !partyRoles[in.PartyRole] {
		return Party{}, fmt.Errorf("%w: party_role", ErrInvalid)
	}
	if (in.PartyID == nil) == (in.LegalEntityID == nil) {
		return Party{}, fmt.Errorf("%w: exactly one of party_id or legal_entity_id is required", ErrInvalid)
	}
	if in.PartyID != nil {
		if _, err := s.Org.GetExternalParty(ctx, *in.PartyID); err != nil {
			return Party{}, fmt.Errorf("%w: party_id", ErrInvalid)
		}
	}
	if in.LegalEntityID != nil {
		if _, err := s.Org.GetLegalEntity(ctx, *in.LegalEntityID); err != nil {
			return Party{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
		}
	}
	if in.SignatoryEmail != nil {
		e := strings.TrimSpace(*in.SignatoryEmail)
		if e == "" {
			in.SignatoryEmail = nil
		} else {
			in.SignatoryEmail = &e
		}
	}

	row, err := agreementstore.New(pdb.MustTxFromContext(ctx)).InsertAgreementPartyFull(ctx, agreementstore.InsertAgreementPartyFullParams{
		ID: uuid.New(), AgreementID: agreementID, PartyID: pgUUID(in.PartyID), LegalEntityID: pgUUID(in.LegalEntityID),
		PartyRole: in.PartyRole, SignatoryName: in.SignatoryName, SignatoryEmail: in.SignatoryEmail,
	})
	if err != nil {
		return Party{}, err
	}
	p := toParty(row)
	if err := s.audit(ctx, "agreement.party.add", agreementID, nil, map[string]any{"party_row_id": p.ID, "party_role": p.PartyRole}); err != nil {
		return Party{}, err
	}
	return p, nil
}

// RemoveParty drops one party row — the counterparty InsertAgreementParty wrote at creation can be removed
// the same way as any party added afterward; CreateWizard's own `counterparty_id` on agreement.agreements
// itself is untouched, so the agreement always keeps at least its original relationship on record even if
// every agreement.parties row is later removed.
func (s *Service) RemoveParty(ctx context.Context, agreementID, partyRowID uuid.UUID) error {
	if _, err := s.GetAgreement(ctx, agreementID); err != nil {
		return err
	}
	n, err := agreementstore.New(pdb.MustTxFromContext(ctx)).DeleteAgreementParty(ctx, agreementstore.DeleteAgreementPartyParams{
		ID: partyRowID, AgreementID: agreementID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "agreement.party.remove", agreementID, nil, map[string]any{"party_row_id": partyRowID})
}
