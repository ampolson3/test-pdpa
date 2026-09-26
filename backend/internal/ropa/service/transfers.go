package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	pdb "pdpa-platform/internal/pkg/db"
	ropastore "pdpa-platform/internal/ropa/store"
)

var transferBases = []string{"adequacy", "bcr", "standard_clauses", "certification", "exemption", "consent"}

// ActivityTransfer is a cross-border transfer (ROPA-08, ม.28/29): the destination country and the
// legal mechanism relied on, optionally tied to one of the activity's recipients.
type ActivityTransfer struct {
	ID            uuid.UUID
	ActivityID    uuid.UUID
	RecipientID   *uuid.UUID
	CountryCode   string
	TransferBasis string // adequacy | bcr | standard_clauses | certification | exemption | consent
	Safeguards    string
	RowVersion    int32
	CreatedAt     time.Time
}

func (tr *ActivityTransfer) normalize() error {
	tr.CountryCode = strings.ToUpper(strings.TrimSpace(tr.CountryCode))
	if len(tr.CountryCode) != 2 {
		return fmt.Errorf("%w: country_code", ErrInvalid)
	}
	if !slices.Contains(transferBases, tr.TransferBasis) {
		return fmt.Errorf("%w: transfer_basis", ErrInvalid)
	}
	tr.Safeguards = strings.TrimSpace(tr.Safeguards)
	return nil
}

func (s *Service) ListActivityTransfers(ctx context.Context, activityID uuid.UUID) ([]ActivityTransfer, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListActivityTransfers(ctx, activityID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityTransfer, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActivityTransfer(ropastore.InsertActivityTransferRow(r)))
	}
	return out, nil
}

func (s *Service) AddActivityTransfer(ctx context.Context, tr ActivityTransfer) (ActivityTransfer, error) {
	if err := tr.normalize(); err != nil {
		return ActivityTransfer{}, err
	}
	if _, err := s.mustActivity(ctx, tr.ActivityID); err != nil {
		return ActivityTransfer{}, err
	}
	if err := s.validCountry(ctx, tr.CountryCode); err != nil {
		return ActivityTransfer{}, err
	}
	if tr.RecipientID != nil {
		recipients, err := s.ListActivityRecipients(ctx, tr.ActivityID)
		if err != nil {
			return ActivityTransfer{}, err
		}
		found := false
		for _, r := range recipients {
			if r.ID == *tr.RecipientID {
				found = true
				break
			}
		}
		if !found {
			return ActivityTransfer{}, fmt.Errorf("%w: recipient_id", ErrInvalid)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ActivityTransfer{}, err
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertActivityTransfer(ctx, ropastore.InsertActivityTransferParams{ID: id, ActivityID: tr.ActivityID,
		RecipientID: pgUUID(tr.RecipientID), CountryCode: tr.CountryCode, TransferBasis: tr.TransferBasis, Safeguards: opt(tr.Safeguards)})
	if err != nil {
		return ActivityTransfer{}, err
	}
	out := toActivityTransfer(row)
	if _, _, err := s.recompute(ctx, tr.ActivityID); err != nil {
		return ActivityTransfer{}, err
	}
	return out, s.audit(ctx, "ropa.activity_transfer.create", tr.ActivityID, nil, transferAudit(out))
}

func (s *Service) DeleteActivityTransfer(ctx context.Context, activityID, id uuid.UUID) error {
	if _, err := s.mustActivity(ctx, activityID); err != nil {
		return err
	}
	n, err := ropastore.New(pdb.MustTxFromContext(ctx)).DeleteActivityTransfer(ctx, ropastore.DeleteActivityTransferParams{ID: id, ActivityID: activityID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if _, _, err := s.recompute(ctx, activityID); err != nil {
		return err
	}
	return s.audit(ctx, "ropa.activity_transfer.delete", activityID, map[string]any{"id": id}, nil)
}

// validCountry checks a code against ORG-07's countries list — like validLawfulBasis, keyed by code
// not id, so it scans ListMaster rather than a by-id GetMaster lookup.
func (s *Service) validCountry(ctx context.Context, code string) error {
	items, err := s.Org.ListMaster(ctx, "countries")
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Code == code {
			return nil
		}
	}
	return fmt.Errorf("%w: country_code", ErrInvalid)
}

func transferAudit(tr ActivityTransfer) map[string]any {
	return map[string]any{"id": tr.ID, "country_code": tr.CountryCode, "transfer_basis": tr.TransferBasis, "recipient_id": tr.RecipientID}
}

func toActivityTransfer(r ropastore.InsertActivityTransferRow) ActivityTransfer {
	return ActivityTransfer{ID: r.ID, ActivityID: r.ActivityID, RecipientID: uuidPtr(r.RecipientID), CountryCode: r.CountryCode,
		TransferBasis: r.TransferBasis, Safeguards: deref(r.Safeguards), RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
}
