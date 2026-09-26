// Package service is the ropa module's business logic. So far: the system/asset register (ROPA-02),
// which ROPA-01's data inventory and later ROPA-03's processing activities both point at.
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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	ropastore "pdpa-platform/internal/ropa/store"
)

const AssetEntityType = "asset"

var assetTypes = []string{"application", "database", "file_share", "saas", "paper", "device", "other"}
var hostingTypes = []string{"on_prem", "cloud", "hybrid"}
var classifications = []string{"public", "internal", "confidential", "restricted"}

var (
	ErrNotFound        = errors.New("ropa: not found")
	ErrInvalid         = errors.New("ropa: invalid input")
	ErrVersionMismatch = errors.New("ropa: version mismatch")
)

// Org is what ropa reads from the organization module (rule 9): visibility checks for FKs that bypass
// RLS (rule 1), before writing an org_unit_id or provider_party_id onto an asset.
type Org interface {
	GetOrgUnit(ctx context.Context, id uuid.UUID) (orgservice.OrgUnit, error)
	GetExternalParty(ctx context.Context, id uuid.UUID) (orgservice.ExternalParty, error)
	GetMaster(ctx context.Context, kind string, id uuid.UUID) (orgservice.MasterItem, error)
}

type Service struct {
	Audit *audit.Service
	Org   Org
}

// Asset is a system, application, database or other place personal data lives — ROPA-02's registry
// that ROPA-01 (data inventory) and ROPA-03 (processing activities) both reference by id.
type Asset struct {
	ID                 uuid.UUID
	Name               string
	AssetType          string
	OrgUnitID          *uuid.UUID
	OwnerUserID        *uuid.UUID
	ProviderPartyID    *uuid.UUID
	HostingCountryCode string
	HostingType        string
	Classification     string
	Status             string // active | retired
	RowVersion         int32
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// AssetCursor is the position after the last asset of a page.
type AssetCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// AssetFilter narrows ListAssets.
type AssetFilter struct {
	AssetType string
	OrgUnitID *uuid.UUID
	Status    string
	Query     string
	After     *AssetCursor
	Limit     int
}

const assetPageSize = 50

func (a *Asset) normalize() error {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" || len([]rune(a.Name)) > 300 {
		return fmt.Errorf("%w: name", ErrInvalid)
	}
	if !slices.Contains(assetTypes, a.AssetType) {
		return fmt.Errorf("%w: asset_type", ErrInvalid)
	}
	a.HostingCountryCode = strings.ToUpper(strings.TrimSpace(a.HostingCountryCode))
	if a.HostingCountryCode != "" && len(a.HostingCountryCode) != 2 {
		return fmt.Errorf("%w: hosting_country_code", ErrInvalid)
	}
	if a.HostingType != "" && !slices.Contains(hostingTypes, a.HostingType) {
		return fmt.Errorf("%w: hosting_type", ErrInvalid)
	}
	if a.Classification != "" && !slices.Contains(classifications, a.Classification) {
		return fmt.Errorf("%w: classification", ErrInvalid)
	}
	if a.Status == "" {
		a.Status = "active"
	}
	if a.Status != "active" && a.Status != "retired" {
		return fmt.Errorf("%w: status", ErrInvalid)
	}
	return nil
}

func (s *Service) ListAssets(ctx context.Context, f AssetFilter) ([]Asset, *AssetCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > assetPageSize {
		limit = assetPageSize
	}
	p := ropastore.ListAssetsParams{Lim: int32(limit + 1)}
	if f.AssetType != "" {
		p.AssetType = &f.AssetType
	}
	if f.OrgUnitID != nil {
		p.OrgUnitID = pgUUID(f.OrgUnitID)
	}
	if f.Status != "" {
		p.Status = &f.Status
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
		p.Q = &esc
	}
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.CreatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListAssets(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Asset, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &AssetCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
		}
		out = append(out, toAsset(ropastore.GetAssetRow(r)))
	}
	return out, nil, nil
}

func (s *Service) GetAsset(ctx context.Context, id uuid.UUID) (Asset, error) {
	r, err := ropastore.New(pdb.MustTxFromContext(ctx)).GetAsset(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Asset{}, ErrNotFound
	}
	if err != nil {
		return Asset{}, err
	}
	return toAsset(r), nil
}

// SaveAsset creates (zero ID) or updates (with the If-Match version) an asset.
func (s *Service) SaveAsset(ctx context.Context, a Asset, version int32) (Asset, error) {
	if err := a.normalize(); err != nil {
		return Asset{}, err
	}
	if a.OrgUnitID != nil {
		if _, err := s.Org.GetOrgUnit(ctx, *a.OrgUnitID); err != nil {
			return Asset{}, fmt.Errorf("%w: org_unit_id", ErrInvalid)
		}
	}
	if a.ProviderPartyID != nil {
		if _, err := s.Org.GetExternalParty(ctx, *a.ProviderPartyID); err != nil {
			return Asset{}, fmt.Errorf("%w: provider_party_id", ErrInvalid)
		}
	}
	if a.OwnerUserID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*a.OwnerUserID})
		if err != nil {
			return Asset{}, err
		}
		if _, ok := names[*a.OwnerUserID]; !ok {
			return Asset{}, fmt.Errorf("%w: owner_user_id", ErrInvalid)
		}
	}
	var before *Asset
	if a.ID != uuid.Nil {
		cur, err := s.GetAsset(ctx, a.ID)
		if err != nil {
			return Asset{}, err
		}
		if cur.RowVersion != version {
			return Asset{}, ErrVersionMismatch
		}
		before = &cur
	}
	isNew := a.ID == uuid.Nil
	if isNew {
		id, err := uuid.NewV7()
		if err != nil {
			return Asset{}, err
		}
		a.ID = id
	}
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	var row ropastore.GetAssetRow
	var err error
	if isNew {
		var r ropastore.InsertAssetRow
		r, err = q.InsertAsset(ctx, ropastore.InsertAssetParams{ID: a.ID, Name: a.Name, AssetType: a.AssetType, OrgUnitID: pgUUID(a.OrgUnitID),
			OwnerUserID: pgUUID(a.OwnerUserID), ProviderPartyID: pgUUID(a.ProviderPartyID), HostingCountryCode: opt(a.HostingCountryCode),
			HostingType: opt(a.HostingType), Classification: opt(a.Classification)})
		row = ropastore.GetAssetRow(r)
	} else {
		var r ropastore.UpdateAssetRow
		r, err = q.UpdateAsset(ctx, ropastore.UpdateAssetParams{ID: a.ID, RowVersion: version, Name: a.Name, AssetType: a.AssetType,
			OrgUnitID: pgUUID(a.OrgUnitID), OwnerUserID: pgUUID(a.OwnerUserID), ProviderPartyID: pgUUID(a.ProviderPartyID),
			HostingCountryCode: opt(a.HostingCountryCode), HostingType: opt(a.HostingType), Classification: opt(a.Classification), Status: a.Status})
		row = ropastore.GetAssetRow(r)
	}
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23503":
		return Asset{}, fmt.Errorf("%w: hosting_country_code", ErrInvalid)
	case errors.Is(err, pgx.ErrNoRows):
		return Asset{}, ErrVersionMismatch
	case err != nil:
		return Asset{}, err
	}
	out := toAsset(row)
	action, b := "ropa.asset.create", any(nil)
	if !isNew {
		action, b = "ropa.asset.update", assetAudit(*before)
	}
	return out, s.audit(ctx, action, out.ID, b, assetAudit(out))
}

func (s *Service) audit(ctx context.Context, action string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil { // a worker job: the tenant is the transaction's
		var t string
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("ropa: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: AssetEntityType, EntityID: &id, Before: before, After: after}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}

func assetAudit(a Asset) map[string]any {
	return map[string]any{"name": a.Name, "asset_type": a.AssetType, "org_unit_id": a.OrgUnitID, "owner_user_id": a.OwnerUserID,
		"provider_party_id": a.ProviderPartyID, "hosting_country_code": a.HostingCountryCode, "hosting_type": a.HostingType,
		"classification": a.Classification, "status": a.Status}
}

func toAsset(r ropastore.GetAssetRow) Asset {
	return Asset{ID: r.ID, Name: r.Name, AssetType: r.AssetType, OrgUnitID: uuidPtr(r.OrgUnitID), OwnerUserID: uuidPtr(r.OwnerUserID),
		ProviderPartyID: uuidPtr(r.ProviderPartyID), HostingCountryCode: deref(r.HostingCountryCode), HostingType: deref(r.HostingType),
		Classification: deref(r.Classification), Status: r.Status, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}

func pgUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	u := uuid.UUID(v.Bytes)
	return &u
}

func opt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
