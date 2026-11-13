package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	vendorstore "pdpa-platform/internal/vendormgmt/store"
)

// VEN-14 (ม.37(3), ม.40): ST-06#1's own "approved|conditional -> offboarding -> terminated" edges —
// vendor.vendors and vendor.offboardings were both already fully specified in the baseline migrations, so
// this is the service only, no new migration or permission code (vendor.vendor.update/.delete, already
// seeded for VEN-01). OffboardingEntityType registers the destruction certificate as a PLT-09 attachment.

const OffboardingEntityType = "vendor_offboarding"

var offboardingDataReturnStatuses = []string{"pending", "returned", "destroyed", "not_applicable"}

// Offboarding is one vendor.offboardings row — the vendor's own most recent one is "the" offboarding
// record this feature tracks; a new vendor can only enter "offboarding" once no prior round is still open
// (ST-06#1's own guard: the status column can't be "approved"/"conditional" again until... it never can —
// terminated is terminal — so in practice there is at most one row per vendor for the lifetime of this
// feature).
type Offboarding struct {
	ID                           uuid.UUID
	VendorID                     uuid.UUID
	DataReturnStatus             string
	DestructionCertificateFileID *uuid.UUID
	AccessRevokedAt              *time.Time
	CompletedAt                  *time.Time
	RowVersion                   int32
	CreatedAt                    time.Time
}

// offboardingRow mirrors every offboardings.sql query's own shared column layout (sqlc generates a
// distinct named Row type per query rather than reusing one shared struct — the same workaround BRE-15
// already established).
type offboardingRow struct {
	ID                           uuid.UUID
	VendorID                     uuid.UUID
	DataReturnStatus             string
	DestructionCertificateFileID pgtype.UUID
	AccessRevokedAt              pgtype.Timestamptz
	CompletedAt                  pgtype.Timestamptz
	RowVersion                   int32
	CreatedAt                    pgtype.Timestamptz
}

func toOffboarding(r offboardingRow) Offboarding {
	o := Offboarding{ID: r.ID, VendorID: r.VendorID, DataReturnStatus: r.DataReturnStatus,
		DestructionCertificateFileID: uuidPtr(r.DestructionCertificateFileID), RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.AccessRevokedAt.Valid {
		t := r.AccessRevokedAt.Time
		o.AccessRevokedAt = &t
	}
	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time
		o.CompletedAt = &t
	}
	return o
}

// GetOffboarding returns the vendor's own most recent offboarding record.
func (s *Service) GetOffboarding(ctx context.Context, vendorID uuid.UUID) (Offboarding, error) {
	if _, err := s.GetVendor(ctx, vendorID); err != nil {
		return Offboarding{}, err
	}
	r, err := vendorstore.New(pdb.MustTxFromContext(ctx)).GetOffboardingForVendor(ctx, vendorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Offboarding{}, ErrNotFound
	}
	if err != nil {
		return Offboarding{}, err
	}
	return toOffboarding(offboardingRow(r)), nil
}

type UpdateOffboardingInput struct {
	DataReturnStatus             string
	DestructionCertificateFileID *uuid.UUID
	AccessRevoked                bool
}

// UpdateOffboarding records data-return progress: the chosen disposition, an optional destruction
// certificate (the caller's own clean, still-unattached PLT-09 upload — DSAR-04's own "attach the agent's
// authority document" pattern), and the access-revoked flag. Transition's own guard checks these two before
// letting the vendor move to "terminated" — the acceptance criterion itself.
func (s *Service) UpdateOffboarding(ctx context.Context, vendorID uuid.UUID, in UpdateOffboardingInput, version int32) (Offboarding, error) {
	if !slices.Contains(offboardingDataReturnStatuses, in.DataReturnStatus) {
		return Offboarding{}, fmt.Errorf("%w: data_return_status", ErrInvalid)
	}
	cur, err := s.GetOffboarding(ctx, vendorID)
	if err != nil {
		return Offboarding{}, err
	}
	if cur.RowVersion != version {
		return Offboarding{}, ErrVersionMismatch
	}

	certID := cur.DestructionCertificateFileID
	if in.DestructionCertificateFileID != nil && (cur.DestructionCertificateFileID == nil || *in.DestructionCertificateFileID != *cur.DestructionCertificateFileID) {
		f, err := s.Files.Get(ctx, *in.DestructionCertificateFileID)
		if err != nil || f.EntityType != "" {
			return Offboarding{}, fmt.Errorf("%w: destruction_certificate_file_id", ErrInvalid)
		}
		if err := s.Files.Attach(ctx, *in.DestructionCertificateFileID, OffboardingEntityType, cur.ID); err != nil {
			return Offboarding{}, err
		}
		certID = in.DestructionCertificateFileID
	}

	accessRevokedAt := pgtype.Timestamptz{}
	switch {
	case cur.AccessRevokedAt != nil:
		accessRevokedAt = pgtype.Timestamptz{Time: *cur.AccessRevokedAt, Valid: true}
	case in.AccessRevoked:
		accessRevokedAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
	}

	q := vendorstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.UpdateOffboarding(ctx, vendorstore.UpdateOffboardingParams{
		ID: cur.ID, RowVersion: version, DataReturnStatus: in.DataReturnStatus,
		DestructionCertificateFileID: pgUUID(certID), AccessRevokedAt: accessRevokedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Offboarding{}, ErrVersionMismatch
	}
	if err != nil {
		return Offboarding{}, err
	}
	out := toOffboarding(offboardingRow(row))
	return out, s.audit(ctx, "vendor.offboarding.update", vendorID,
		map[string]any{"data_return_status": cur.DataReturnStatus, "access_revoked": cur.AccessRevokedAt != nil},
		map[string]any{"data_return_status": out.DataReturnStatus, "access_revoked": out.AccessRevokedAt != nil})
}

// TransitionVendor moves the vendor along ST-06#1's own "approved|conditional -> offboarding -> terminated"
// edges. Entering "offboarding" opens a fresh offboarding record (data_return_status "pending"); entering
// "terminated" is this feature's own acceptance criterion and additionally needs the permanent-status-change
// permission (vendor.vendor.delete, the same reasoning ORG-06's party merge and DPIA-03's template retire
// already use for a change that can't be undone by a plain update).
func (s *Service) TransitionVendor(ctx context.Context, vendorID uuid.UUID, to string, version int32) (Vendor, error) {
	v, err := s.GetVendor(ctx, vendorID)
	if err != nil {
		return Vendor{}, err
	}
	if v.RowVersion != version {
		return Vendor{}, ErrVersionMismatch
	}

	q := vendorstore.New(pdb.MustTxFromContext(ctx))
	switch to {
	case "offboarding":
		row, err := q.TransitionVendorToOffboarding(ctx, vendorstore.TransitionVendorToOffboardingParams{ID: vendorID, RowVersion: version})
		if errors.Is(err, pgx.ErrNoRows) {
			return Vendor{}, ErrInvalidTransition
		}
		if err != nil {
			return Vendor{}, err
		}
		id, err := uuid.NewV7()
		if err != nil {
			return Vendor{}, err
		}
		if _, err := q.InsertOffboarding(ctx, vendorstore.InsertOffboardingParams{ID: id, VendorID: vendorID}); err != nil {
			return Vendor{}, err
		}
		out := toVendor(vendorstore.GetVendorRow(row))
		return out, s.audit(ctx, "vendor.vendor.transition", vendorID, map[string]any{"status": v.Status}, map[string]any{"status": out.Status})

	case "terminated":
		g, _ := authz.FromContext(ctx)
		if !g.Has("vendor.vendor.delete") {
			return Vendor{}, ErrForbidden
		}
		off, err := s.GetOffboarding(ctx, vendorID)
		if err != nil {
			return Vendor{}, err
		}
		// The acceptance criterion itself: terminate only once access has been revoked and either a
		// destruction certificate is on file, or no data was ever shared with this vendor in the first
		// place (data_return_status = "not_applicable" — existence over judgement, the same reading
		// DSAR-04's own agent-document guard already uses).
		if off.AccessRevokedAt == nil {
			return Vendor{}, ErrOffboardingIncomplete
		}
		if off.DestructionCertificateFileID == nil && off.DataReturnStatus != "not_applicable" {
			return Vendor{}, ErrOffboardingIncomplete
		}
		row, err := q.TransitionVendorToTerminated(ctx, vendorstore.TransitionVendorToTerminatedParams{ID: vendorID, RowVersion: version})
		if errors.Is(err, pgx.ErrNoRows) {
			return Vendor{}, ErrInvalidTransition
		}
		if err != nil {
			return Vendor{}, err
		}
		if _, err := q.CompleteOffboarding(ctx, off.ID); err != nil {
			return Vendor{}, err
		}
		out := toVendor(vendorstore.GetVendorRow(row))
		return out, s.audit(ctx, "vendor.vendor.transition", vendorID, map[string]any{"status": v.Status}, map[string]any{"status": out.Status})

	default:
		return Vendor{}, fmt.Errorf("%w: to", ErrInvalid)
	}
}
