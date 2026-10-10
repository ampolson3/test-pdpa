package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	vendorstore "pdpa-platform/internal/vendormgmt/store"
)

const VendorEntityType = "vendor"

var vendorStatuses = []string{"prospect", "onboarding", "approved", "conditional", "rejected", "offboarding", "terminated"}

// Vendor is a processor or other business party ORG-06's directory already knows, extended with the
// vendor-specific fields VEN-01 needs (ม.40). Tier/Status/NextAssessmentAt/ApprovedAt/OffboardedAt are
// read-only through this feature — ST-06's own transitions (VEN-02/05/07/08/09/14) own them.
type Vendor struct {
	ID                  uuid.UUID
	PartyID             uuid.UUID
	ServiceDescription  string
	RelationshipOwnerID *uuid.UUID
	IsProcessor         bool
	Tier                *string
	DataAccess          json.RawMessage
	ProcessingCountries []string
	Status              string
	NextAssessmentAt    *time.Time
	ApprovedAt          *time.Time
	OffboardedAt        *time.Time
	RowVersion          int32
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// VendorCursor is the position after the last vendor of a page.
type VendorCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// VendorFilter narrows ListVendors.
type VendorFilter struct {
	Status string
	After  *VendorCursor
	Limit  int
}

const vendorPageSize = 50

func (v *Vendor) normalize() error {
	v.ServiceDescription = strings.TrimSpace(v.ServiceDescription)
	if v.ServiceDescription == "" || len([]rune(v.ServiceDescription)) > 2000 {
		return fmt.Errorf("%w: service_description", ErrInvalid)
	}
	if v.ProcessingCountries == nil {
		v.ProcessingCountries = []string{}
	}
	for i, c := range v.ProcessingCountries {
		v.ProcessingCountries[i] = strings.ToUpper(strings.TrimSpace(c))
		if len(v.ProcessingCountries[i]) != 2 {
			return fmt.Errorf("%w: processing_countries", ErrInvalid)
		}
	}
	if len(v.DataAccess) == 0 {
		v.DataAccess = json.RawMessage(`{}`)
	}
	return nil
}

func (s *Service) ListVendors(ctx context.Context, f VendorFilter) ([]Vendor, *VendorCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > vendorPageSize {
		limit = vendorPageSize
	}
	p := vendorstore.ListVendorsParams{Lim: int32(limit + 1)}
	if f.Status != "" {
		p.Status = &f.Status
	}
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.CreatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := vendorstore.New(pdb.MustTxFromContext(ctx)).ListVendors(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Vendor, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &VendorCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
		}
		out = append(out, toVendor(vendorstore.GetVendorRow(r)))
	}
	return out, nil, nil
}

func (s *Service) GetVendor(ctx context.Context, id uuid.UUID) (Vendor, error) {
	r, err := vendorstore.New(pdb.MustTxFromContext(ctx)).GetVendor(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Vendor{}, ErrNotFound
	}
	if err != nil {
		return Vendor{}, err
	}
	return toVendor(r), nil
}

// SaveVendor creates (zero ID) or updates (with the If-Match version) a vendor's profile. It never
// touches status/tier/next_assessment_at/approved_at/offboarded_at — a new vendor always lands
// "prospect" (the column's own DB default), and every other ST-06 edge belongs to a sibling feature.
func (s *Service) SaveVendor(ctx context.Context, v Vendor, version int32) (Vendor, error) {
	if err := v.normalize(); err != nil {
		return Vendor{}, err
	}
	if _, err := s.Org.GetExternalParty(ctx, v.PartyID); err != nil {
		return Vendor{}, fmt.Errorf("%w: party_id", ErrInvalid)
	}
	if v.RelationshipOwnerID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*v.RelationshipOwnerID})
		if err != nil {
			return Vendor{}, err
		}
		if _, ok := names[*v.RelationshipOwnerID]; !ok {
			return Vendor{}, fmt.Errorf("%w: relationship_owner_id", ErrInvalid)
		}
	}

	var before *Vendor
	isNew := v.ID == uuid.Nil
	if !isNew {
		cur, err := s.GetVendor(ctx, v.ID)
		if err != nil {
			return Vendor{}, err
		}
		if cur.RowVersion != version {
			return Vendor{}, ErrVersionMismatch
		}
		before = &cur
	} else {
		id, err := uuid.NewV7()
		if err != nil {
			return Vendor{}, err
		}
		v.ID = id
	}

	var row vendorstore.GetVendorRow
	// uq_vendors_party_id is a real unique constraint — a duplicate party_id must not abort the whole
	// request transaction (the same pdb.Savepoint pattern org.SaveLegalEntity/ropa's own unique-constraint
	// checks already use).
	err := pdb.Savepoint(ctx, func(ctx context.Context) error {
		q := vendorstore.New(pdb.MustTxFromContext(ctx))
		var err error
		if isNew {
			var r vendorstore.InsertVendorRow
			r, err = q.InsertVendor(ctx, vendorstore.InsertVendorParams{ID: v.ID, PartyID: v.PartyID, ServiceDescription: v.ServiceDescription,
				RelationshipOwnerID: pgUUID(v.RelationshipOwnerID), IsProcessor: v.IsProcessor, DataAccess: v.DataAccess,
				ProcessingCountries: v.ProcessingCountries})
			row = vendorstore.GetVendorRow(r)
		} else {
			var r vendorstore.UpdateVendorRow
			r, err = q.UpdateVendor(ctx, vendorstore.UpdateVendorParams{ID: v.ID, RowVersion: version, PartyID: v.PartyID,
				ServiceDescription: v.ServiceDescription, RelationshipOwnerID: pgUUID(v.RelationshipOwnerID), IsProcessor: v.IsProcessor,
				DataAccess: v.DataAccess, ProcessingCountries: v.ProcessingCountries})
			row = vendorstore.GetVendorRow(r)
		}
		return err
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return Vendor{}, fmt.Errorf("%w: party_id already has a vendor", ErrInvalid)
	case errors.Is(err, pgx.ErrNoRows):
		return Vendor{}, ErrVersionMismatch
	case err != nil:
		return Vendor{}, err
	}

	out := toVendor(row)
	action, b := "vendor.vendor.create", any(nil)
	if !isNew {
		action, b = "vendor.vendor.update", vendorAudit(*before)
	}
	return out, s.audit(ctx, action, out.ID, b, vendorAudit(out))
}

func vendorAudit(v Vendor) map[string]any {
	return map[string]any{"party_id": v.PartyID, "service_description": v.ServiceDescription,
		"relationship_owner_id": v.RelationshipOwnerID, "is_processor": v.IsProcessor, "processing_countries": v.ProcessingCountries}
}

func toVendor(r vendorstore.GetVendorRow) Vendor {
	v := Vendor{ID: r.ID, PartyID: r.PartyID, ServiceDescription: r.ServiceDescription, IsProcessor: r.IsProcessor,
		Tier: r.Tier, DataAccess: r.DataAccess, ProcessingCountries: r.ProcessingCountries, Status: r.Status,
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	v.RelationshipOwnerID = uuidPtr(r.RelationshipOwnerID)
	if r.NextAssessmentAt.Valid {
		t := r.NextAssessmentAt.Time
		v.NextAssessmentAt = &t
	}
	if r.ApprovedAt.Valid {
		t := r.ApprovedAt.Time
		v.ApprovedAt = &t
	}
	if r.OffboardedAt.Valid {
		t := r.OffboardedAt.Time
		v.OffboardedAt = &t
	}
	return v
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
