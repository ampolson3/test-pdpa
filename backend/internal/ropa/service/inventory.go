package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	ropastore "pdpa-platform/internal/ropa/store"
)

const InventoryEntityType = "data_inventory"

var sources = []string{"direct", "indirect", "derived"}

// DataInventoryItem is one kind of personal data held in one asset (ROPA-01) — what data, where it's
// stored, who's responsible, and whether it's sensitive under s.26 (from its data category, ORG-07).
type DataInventoryItem struct {
	ID             uuid.UUID
	AssetID        uuid.UUID
	DataCategoryID uuid.UUID
	OrgUnitID      *uuid.UUID
	OwnerUserID    *uuid.UUID
	Source         string // direct | indirect | derived
	LocationDetail string
	RowVersion     int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
	IsSensitive    bool
	SensitiveType  string
	CategoryNameTh string
	CategoryNameEn string
}

// InventoryCursor is the position after the last item of a page.
type InventoryCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// InventoryFilter narrows ListDataInventory. SensitiveOnly is the acceptance criterion's cross-department
// filter — leaving OrgUnitID unset searches every department at once.
type InventoryFilter struct {
	AssetID        *uuid.UUID
	OrgUnitID      *uuid.UUID
	DataCategoryID *uuid.UUID
	SensitiveOnly  bool
	After          *InventoryCursor
	Limit          int
}

const inventoryPageSize = 50

func (it *DataInventoryItem) normalize() error {
	if it.AssetID == uuid.Nil {
		return fmt.Errorf("%w: asset_id", ErrInvalid)
	}
	if it.DataCategoryID == uuid.Nil {
		return fmt.Errorf("%w: data_category_id", ErrInvalid)
	}
	it.Source = strings.TrimSpace(it.Source)
	if it.Source != "" && !slices.Contains(sources, it.Source) {
		return fmt.Errorf("%w: source", ErrInvalid)
	}
	it.LocationDetail = strings.TrimSpace(it.LocationDetail)
	if len([]rune(it.LocationDetail)) > 500 {
		return fmt.Errorf("%w: location_detail", ErrInvalid)
	}
	return nil
}

func (s *Service) ListDataInventory(ctx context.Context, f InventoryFilter) ([]DataInventoryItem, *InventoryCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > inventoryPageSize {
		limit = inventoryPageSize
	}
	p := ropastore.ListDataInventoryParams{Lim: int32(limit + 1), SensitiveOnly: f.SensitiveOnly}
	p.AssetID = pgUUID(f.AssetID)
	p.OrgUnitID = pgUUID(f.OrgUnitID)
	p.DataCategoryID = pgUUID(f.DataCategoryID)
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.CreatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListDataInventory(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]DataInventoryItem, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &InventoryCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
		}
		out = append(out, toInventoryItem(ropastore.GetDataInventoryRow(r)))
	}
	return out, nil, nil
}

func (s *Service) GetDataInventoryItem(ctx context.Context, id uuid.UUID) (DataInventoryItem, error) {
	r, err := ropastore.New(pdb.MustTxFromContext(ctx)).GetDataInventory(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DataInventoryItem{}, ErrNotFound
	}
	if err != nil {
		return DataInventoryItem{}, err
	}
	return toInventoryItem(r), nil
}

// SaveDataInventoryItem creates (zero ID) or updates (with the If-Match version) an inventory entry.
func (s *Service) SaveDataInventoryItem(ctx context.Context, it DataInventoryItem, version int32) (DataInventoryItem, error) {
	if err := it.normalize(); err != nil {
		return DataInventoryItem{}, err
	}
	if _, err := s.GetAsset(ctx, it.AssetID); err != nil {
		return DataInventoryItem{}, fmt.Errorf("%w: asset_id", ErrInvalid)
	}
	if _, err := s.Org.GetMaster(ctx, "data_categories", it.DataCategoryID); err != nil {
		return DataInventoryItem{}, fmt.Errorf("%w: data_category_id", ErrInvalid)
	}
	if it.OrgUnitID != nil {
		if _, err := s.Org.GetOrgUnit(ctx, *it.OrgUnitID); err != nil {
			return DataInventoryItem{}, fmt.Errorf("%w: org_unit_id", ErrInvalid)
		}
	}
	if it.OwnerUserID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*it.OwnerUserID})
		if err != nil {
			return DataInventoryItem{}, err
		}
		if _, ok := names[*it.OwnerUserID]; !ok {
			return DataInventoryItem{}, fmt.Errorf("%w: owner_user_id", ErrInvalid)
		}
	}
	var before *DataInventoryItem
	if it.ID != uuid.Nil {
		cur, err := s.GetDataInventoryItem(ctx, it.ID)
		if err != nil {
			return DataInventoryItem{}, err
		}
		if cur.RowVersion != version {
			return DataInventoryItem{}, ErrVersionMismatch
		}
		before = &cur
	}
	isNew := it.ID == uuid.Nil
	if isNew {
		id, err := uuid.NewV7()
		if err != nil {
			return DataInventoryItem{}, err
		}
		it.ID = id
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	var err error
	if isNew {
		_, err = q.InsertDataInventory(ctx, ropastore.InsertDataInventoryParams{ID: it.ID, AssetID: it.AssetID, DataCategoryID: it.DataCategoryID,
			OrgUnitID: pgUUID(it.OrgUnitID), OwnerUserID: pgUUID(it.OwnerUserID), Source: opt(it.Source), LocationDetail: opt(it.LocationDetail)})
	} else {
		_, err = q.UpdateDataInventory(ctx, ropastore.UpdateDataInventoryParams{ID: it.ID, RowVersion: version, AssetID: it.AssetID,
			DataCategoryID: it.DataCategoryID, OrgUnitID: pgUUID(it.OrgUnitID), OwnerUserID: pgUUID(it.OwnerUserID), Source: opt(it.Source),
			LocationDetail: opt(it.LocationDetail)})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return DataInventoryItem{}, ErrVersionMismatch
	}
	if err != nil {
		return DataInventoryItem{}, err
	}
	// The insert/update queries don't return the joined category columns; re-read for the full, current view.
	out, err := s.GetDataInventoryItem(ctx, it.ID)
	if err != nil {
		return DataInventoryItem{}, err
	}
	action, b := "ropa.data_inventory.create", any(nil)
	if !isNew {
		action, b = "ropa.data_inventory.update", inventoryAudit(*before)
	}
	return out, s.audit(ctx, action, out.ID, b, inventoryAudit(out))
}

func inventoryAudit(it DataInventoryItem) map[string]any {
	return map[string]any{"asset_id": it.AssetID, "data_category_id": it.DataCategoryID, "org_unit_id": it.OrgUnitID,
		"owner_user_id": it.OwnerUserID, "source": it.Source, "location_detail": it.LocationDetail}
}

func toInventoryItem(r ropastore.GetDataInventoryRow) DataInventoryItem {
	return DataInventoryItem{ID: r.ID, AssetID: r.AssetID, DataCategoryID: r.DataCategoryID, OrgUnitID: uuidPtr(r.OrgUnitID),
		OwnerUserID: uuidPtr(r.OwnerUserID), Source: deref(r.Source), LocationDetail: deref(r.LocationDetail), RowVersion: r.RowVersion,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time, IsSensitive: r.IsSensitive, SensitiveType: deref(r.SensitiveType),
		CategoryNameTh: r.CategoryNameTh, CategoryNameEn: deref(r.CategoryNameEn)}
}
