package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dsarstore "pdpa-platform/internal/dsar/store"
	pdb "pdpa-platform/internal/pkg/db"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// ErrExemptionCheckRequired is DSAR-10's own acceptance criterion: an erasure request can't enter completed
// without at least one recorded exemption check (ม.33) — existence, not a particular result, is the gate.
var ErrExemptionCheckRequired = errors.New("dsar: an exemption check is required before an erasure request can complete")

// LegalHold is dsar.legal_holds — data this tenant must keep despite an otherwise-valid erasure request
// (ม.33's own exemption: retention required by another law, or needed to establish/exercise/defend a legal
// claim). Scoped by AssetID (a specific system) and/or DataCategoryID (every asset holding that category of
// data) — DPX-05's own disposal-job check only ever matches by AssetID; DSAR-10 (below) matches both.
type LegalHold struct {
	ID             uuid.UUID
	DataCategoryID *uuid.UUID
	AssetID        uuid.UUID
	LawRef         string
	HoldRule       map[string]any
	IsActive       bool
	RowVersion     int32
	CreatedAt      time.Time
}

func (h *LegalHold) normalize() error {
	h.LawRef = strings.TrimSpace(h.LawRef)
	if h.LawRef == "" {
		return fmt.Errorf("%w: law_ref", ErrInvalid)
	}
	if h.DataCategoryID == nil && h.AssetID == uuid.Nil {
		return fmt.Errorf("%w: data_category_id or asset_id required", ErrInvalid)
	}
	return nil
}

// ActiveHoldForAsset returns the first active legal hold naming this asset directly, if any. A gov disposal
// job against an asset under an active hold must not proceed (DPX-05's own "legal hold suspends disposal"
// rule) — scoped to asset_id only, the same narrower match DPX-05 itself established.
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

// CreateLegalHold records a new hold for the DPO to manage directly (no state machine — a flat is_active
// flag, deactivated rather than deleted, the same convention this codebase's own permanent-record tables use).
func (s *Service) CreateLegalHold(ctx context.Context, h LegalHold) (LegalHold, error) {
	if err := h.normalize(); err != nil {
		return LegalHold{}, err
	}
	holdRule, err := json.Marshal(h.HoldRule)
	if err != nil {
		return LegalHold{}, fmt.Errorf("%w: hold_rule", ErrInvalid)
	}
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).CreateLegalHold(ctx, dsarstore.CreateLegalHoldParams{
		DataCategoryID: pgUUID(h.DataCategoryID), AssetID: pgUUID(nilIfZero(h.AssetID)), LawRef: h.LawRef, HoldRule: holdRule,
	})
	if err != nil {
		return LegalHold{}, err
	}
	out := toLegalHold(legalHoldRow(row))
	if err := s.audit(ctx, "dsar.legal_hold.create", out.ID, nil, map[string]any{"law_ref": out.LawRef}); err != nil {
		return LegalHold{}, err
	}
	return out, nil
}

// ListLegalHolds returns every legal hold on record, newest first — tenant-wide, no pagination (the same
// "a tenant's own reference list is small" reasoning DPO-05/RRA-09's own tenant-wide lists already use).
func (s *Service) ListLegalHolds(ctx context.Context) ([]LegalHold, error) {
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListLegalHolds(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LegalHold, 0, len(rows))
	for _, r := range rows {
		out = append(out, toLegalHold(legalHoldRow(r)))
	}
	return out, nil
}

// DeactivateLegalHold flips is_active to false — never a hard delete, so a past exemption_checks row's own
// legal_hold_id reference stays resolvable.
func (s *Service) DeactivateLegalHold(ctx context.Context, id uuid.UUID, rowVersion int32) (LegalHold, error) {
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).DeactivateLegalHold(ctx, dsarstore.DeactivateLegalHoldParams{ID: id, RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return LegalHold{}, ErrVersionMismatch
	}
	if err != nil {
		return LegalHold{}, err
	}
	out := toLegalHold(legalHoldRow(row))
	if err := s.audit(ctx, "dsar.legal_hold.deactivate", out.ID, nil, nil); err != nil {
		return LegalHold{}, err
	}
	return out, nil
}

// ExemptionCheck is dsar.exemption_checks — one recorded, reasoned decision on whether a request's own
// erasure work can proceed against the given assets, or must retain some/all of them under an active hold.
type ExemptionCheck struct {
	ID          uuid.UUID
	RequestID   uuid.UUID
	LegalHoldID *uuid.UUID
	Result      string // can_delete | must_retain | partial
	Reason      string
	CreatedAt   time.Time
}

// ListExemptionChecks returns every check run against this request, newest first.
func (s *Service) ListExemptionChecks(ctx context.Context, requestID uuid.UUID) ([]ExemptionCheck, error) {
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return nil, err
	}
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListExemptionChecksForRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	out := make([]ExemptionCheck, 0, len(rows))
	for _, r := range rows {
		out = append(out, toExemptionCheck(exemptionCheckRow(r)))
	}
	return out, nil
}

func (s *Service) hasExemptionCheck(ctx context.Context, requestID uuid.UUID) (bool, error) {
	n, err := dsarstore.New(pdb.MustTxFromContext(ctx)).CountExemptionChecksForRequest(ctx, requestID)
	return n > 0, err
}

// RunExemptionCheck is the acceptance criterion itself (ม.33): given the assets this erasure request's own
// work touches, match every active legal hold naming one of those assets directly, or — resolved through
// each asset's own ROPA-01 data inventory (rule 9, s.Ropa) — naming one of the data categories those assets
// actually hold. can_delete when none match, must_retain when every given asset is held, partial otherwise;
// the reason cites each matching hold's own law_ref so the decision is never a bare status with nothing to
// point to. Exactly one row is written per call, even when several holds match — this is a decision about
// the request's own current batch of assets, not a per-asset ledger.
func (s *Service) RunExemptionCheck(ctx context.Context, requestID uuid.UUID, assetIDs []uuid.UUID) (ExemptionCheck, error) {
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return ExemptionCheck{}, err
	}
	if len(assetIDs) == 0 {
		return ExemptionCheck{}, fmt.Errorf("%w: asset_ids", ErrInvalid)
	}
	seenCategories := map[uuid.UUID]bool{}
	var categoryIDs []uuid.UUID
	for _, aid := range assetIDs {
		if _, err := s.Ropa.GetAsset(ctx, aid); err != nil {
			return ExemptionCheck{}, fmt.Errorf("%w: asset_ids", ErrInvalid)
		}
		items, _, err := s.Ropa.ListDataInventory(ctx, ropaservice.InventoryFilter{AssetID: &aid})
		if err != nil {
			return ExemptionCheck{}, err
		}
		for _, it := range items {
			if !seenCategories[it.DataCategoryID] {
				seenCategories[it.DataCategoryID] = true
				categoryIDs = append(categoryIDs, it.DataCategoryID)
			}
		}
	}
	if categoryIDs == nil {
		categoryIDs = []uuid.UUID{}
	}
	holds, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListActiveLegalHoldsForAssetsOrCategories(ctx, dsarstore.ListActiveLegalHoldsForAssetsOrCategoriesParams{
		AssetIds: assetIDs, DataCategoryIds: categoryIDs,
	})
	if err != nil {
		return ExemptionCheck{}, err
	}

	heldAssets := map[uuid.UUID]bool{}
	var lawRefs []string
	var firstHoldID *uuid.UUID
	for _, h := range holds {
		if firstHoldID == nil {
			id := h.ID
			firstHoldID = &id
		}
		lawRefs = append(lawRefs, h.LawRef)
		if h.AssetID.Valid {
			heldAssets[uuid.UUID(h.AssetID.Bytes)] = true
		} else {
			// A category-matched hold (no asset_id of its own) applies to every given asset that carries
			// that category — resolved again here since the query above only proves a match exists, not
			// which of the caller's own assets triggered it.
			for _, aid := range assetIDs {
				heldAssets[aid] = true
			}
		}
	}

	var result, reason string
	switch {
	case len(heldAssets) == 0:
		result = "can_delete"
		reason = "ไม่พบ legal hold ที่มีผลกับระบบที่ตรวจ — ลบ/ทำลายข้อมูลได้ตามคำขอ"
	case len(heldAssets) >= len(assetIDs):
		result = "must_retain"
		reason = "ข้อมูลในระบบที่ตรวจทั้งหมดต้องเก็บไว้ตาม: " + strings.Join(lawRefs, ", ")
	default:
		result = "partial"
		reason = "ข้อมูลบางระบบต้องเก็บไว้ตาม: " + strings.Join(lawRefs, ", ")
	}

	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).InsertExemptionCheck(ctx, dsarstore.InsertExemptionCheckParams{
		RequestID: requestID, LegalHoldID: pgUUID(firstHoldID), Result: result, Reason: reason,
	})
	if err != nil {
		return ExemptionCheck{}, err
	}
	out := toExemptionCheck(exemptionCheckRow(row))
	if err := s.audit(ctx, "dsar.exemption_check.run", requestID, nil, map[string]any{"result": out.Result}); err != nil {
		return ExemptionCheck{}, err
	}
	return out, nil
}

func nilIfZero(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// legalHoldRow/exemptionCheckRow let every sqlc-generated Row type for these tables convert through one
// shared shape, the same "distinct Row struct per query, same column order" workaround BRE-15/VEN-06/VEN-03
// already established.
type legalHoldRow struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	DataCategoryID pgtype.UUID
	AssetID        pgtype.UUID
	LawRef         string
	HoldRule       []byte
	IsActive       bool
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
	RowVersion     int32
}

func toLegalHold(r legalHoldRow) LegalHold {
	h := LegalHold{ID: r.ID, LawRef: r.LawRef, IsActive: r.IsActive, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.DataCategoryID.Valid {
		id := uuid.UUID(r.DataCategoryID.Bytes)
		h.DataCategoryID = &id
	}
	if r.AssetID.Valid {
		h.AssetID = uuid.UUID(r.AssetID.Bytes)
	}
	if len(r.HoldRule) > 0 {
		_ = json.Unmarshal(r.HoldRule, &h.HoldRule)
	}
	return h
}

type exemptionCheckRow struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	RequestID   uuid.UUID
	LegalHoldID pgtype.UUID
	Result      string
	Reason      string
	CreatedAt   pgtype.Timestamptz
}

func toExemptionCheck(r exemptionCheckRow) ExemptionCheck {
	c := ExemptionCheck{ID: r.ID, RequestID: r.RequestID, Result: r.Result, Reason: r.Reason, CreatedAt: r.CreatedAt.Time}
	if r.LegalHoldID.Valid {
		id := uuid.UUID(r.LegalHoldID.Bytes)
		c.LegalHoldID = &id
	}
	return c
}
