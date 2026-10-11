package service

import (
	"context"

	"github.com/google/uuid"

	dsarstore "pdpa-platform/internal/dsar/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// LegalHold is the minimal shape DPX-05 needs from dsar.legal_holds (rule 9: gov reads this module
// only through its own exported service, never dsar's schema directly).
type LegalHold struct {
	ID      uuid.UUID
	AssetID uuid.UUID
	LawRef  string
}

// ActiveHoldForAsset returns the first active legal hold naming this asset, if any. A gov disposal job
// against an asset under an active hold must not proceed (DPX-05's own "legal hold suspends disposal" rule).
func (s *Service) ActiveHoldForAsset(ctx context.Context, assetID uuid.UUID) (*LegalHold, error) {
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListActiveLegalHoldsForAsset(ctx, pgUUID(&assetID))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	r := rows[0]
	return &LegalHold{ID: r.ID, AssetID: assetID, LawRef: r.LawRef}, nil
}
