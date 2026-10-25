// DPO-03 (ความเป็นอิสระและผลประโยชน์ทับซ้อน, ม.42): dpo.independence_declarations was already fully
// specified in the baseline migrations (00016_dpo.sql, FKs in 00018) -- a thin, one-row-per-appointment-
// per-year record of the DPO's own other duties plus a yes/no conflict flag, signed once a year. The
// acceptance criterion ("มีคำรับรองที่ลงนามทุกปี") is read literally: every appointment year on record
// must have a signed declaration -- RecordDeclaration creates or edits the still-unsigned draft for a
// given year, and SignDeclaration is the one write that actually satisfies it.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dpostore "pdpa-platform/internal/dpo/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// IndependenceDeclarationEntityType is the PLT-09 attachment entity for the signed declaration's own
// evidence file (e.g. a scanned signature page) -- optional, same as the appointment order/PDPC evidence.
const IndependenceDeclarationEntityType = "dpo_independence_declaration"

// IndependenceDeclaration is one year's attestation (ม.42): the DPO's other duties at the organization,
// whether any of them conflicts with the DPO role, and -- once SignDeclaration is called -- when it was
// signed and, optionally, the signed evidence file.
type IndependenceDeclaration struct {
	ID            uuid.UUID
	AppointmentID uuid.UUID
	Year          int
	OtherDuties   string
	HasConflict   bool
	SignedAt      *time.Time
	FileID        *uuid.UUID
	RowVersion    int32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (d *IndependenceDeclaration) normalize() error {
	if d.Year < 2000 || d.Year > 9999 {
		return fmt.Errorf("%w: year", ErrInvalid)
	}
	d.OtherDuties = strings.TrimSpace(d.OtherDuties)
	return nil
}

// ListIndependenceDeclarations returns one appointment's own declarations, newest year first.
func (s *Service) ListIndependenceDeclarations(ctx context.Context, appointmentID uuid.UUID) ([]IndependenceDeclaration, error) {
	if _, err := s.GetAppointment(ctx, appointmentID); err != nil {
		return nil, err
	}
	rows, err := dpostore.New(pdb.MustTxFromContext(ctx)).ListIndependenceDeclarations(ctx, appointmentID)
	if err != nil {
		return nil, err
	}
	out := make([]IndependenceDeclaration, 0, len(rows))
	for _, r := range rows {
		out = append(out, toIndependenceDeclaration(dpostore.GetIndependenceDeclarationRow(r)))
	}
	return out, nil
}

// GetIndependenceDeclaration returns one declaration by id.
func (s *Service) GetIndependenceDeclaration(ctx context.Context, id uuid.UUID) (IndependenceDeclaration, error) {
	r, err := dpostore.New(pdb.MustTxFromContext(ctx)).GetIndependenceDeclaration(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return IndependenceDeclaration{}, ErrNotFound
	}
	if err != nil {
		return IndependenceDeclaration{}, err
	}
	return toIndependenceDeclaration(r), nil
}

// RecordDeclaration creates or edits the still-unsigned declaration for one appointment+year -- at most
// one per year, resolved by looking the pair up first rather than a unique index, since an existing but
// already-signed one must never be overwritten by a plain edit (sign a fresh year instead, or correct the
// signed one deliberately by unsigning it first -- not built, since no acceptance criterion asks for it).
func (s *Service) RecordDeclaration(ctx context.Context, appointmentID uuid.UUID, year int, otherDuties string, hasConflict bool) (IndependenceDeclaration, error) {
	d := IndependenceDeclaration{AppointmentID: appointmentID, Year: year, OtherDuties: otherDuties, HasConflict: hasConflict}
	if err := d.normalize(); err != nil {
		return IndependenceDeclaration{}, err
	}
	if _, err := s.GetAppointment(ctx, appointmentID); err != nil {
		return IndependenceDeclaration{}, fmt.Errorf("%w: appointment_id", ErrInvalid)
	}

	q := dpostore.New(pdb.MustTxFromContext(ctx))
	existing, err := q.GetIndependenceDeclarationForYear(ctx, dpostore.GetIndependenceDeclarationForYearParams{
		AppointmentID: appointmentID, Year: int16(year),
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		id, uerr := uuid.NewV7()
		if uerr != nil {
			return IndependenceDeclaration{}, uerr
		}
		row, ierr := q.InsertIndependenceDeclaration(ctx, dpostore.InsertIndependenceDeclarationParams{
			ID: id, AppointmentID: appointmentID, Year: int16(year), OtherDuties: optStr(otherDuties), HasConflict: hasConflict,
		})
		if ierr != nil {
			return IndependenceDeclaration{}, ierr
		}
		out := toIndependenceDeclaration(dpostore.GetIndependenceDeclarationRow(row))
		return out, s.audit(ctx, "dpo.independence_declaration.create", out.ID, nil, independenceDeclarationAudit(out))
	case err != nil:
		return IndependenceDeclaration{}, err
	default:
		before := toIndependenceDeclaration(dpostore.GetIndependenceDeclarationRow(existing))
		if before.SignedAt != nil {
			return IndependenceDeclaration{}, fmt.Errorf("%w: declaration already signed for this year", ErrInvalid)
		}
		row, uerr := q.UpdateIndependenceDeclaration(ctx, dpostore.UpdateIndependenceDeclarationParams{
			ID: before.ID, RowVersion: before.RowVersion, OtherDuties: optStr(otherDuties), HasConflict: hasConflict,
		})
		if errors.Is(uerr, pgx.ErrNoRows) {
			return IndependenceDeclaration{}, ErrVersionMismatch
		}
		if uerr != nil {
			return IndependenceDeclaration{}, uerr
		}
		out := toIndependenceDeclaration(dpostore.GetIndependenceDeclarationRow(row))
		return out, s.audit(ctx, "dpo.independence_declaration.update", out.ID, independenceDeclarationAudit(before), independenceDeclarationAudit(out))
	}
}

// SignDeclaration is the acceptance criterion itself ("มีคำรับรองที่ลงนามทุกปี"): stamps signed_at once
// (a redelivered sign call against an already-signed row is refused, not a harmless no-op -- re-signing
// would silently move the attestation date) and optionally attaches the caller's own clean,
// still-unattached PLT-09 upload as the signed evidence, the same pattern DPO-01's own appointment order
// already uses.
func (s *Service) SignDeclaration(ctx context.Context, id uuid.UUID, version int32, fileID *uuid.UUID) (IndependenceDeclaration, error) {
	before, err := s.GetIndependenceDeclaration(ctx, id)
	if err != nil {
		return IndependenceDeclaration{}, err
	}
	if before.SignedAt != nil {
		return IndependenceDeclaration{}, fmt.Errorf("%w: already signed", ErrInvalid)
	}
	if before.RowVersion != version {
		return IndependenceDeclaration{}, ErrVersionMismatch
	}
	if fileID != nil {
		f, ferr := s.Files.Get(ctx, *fileID)
		if ferr != nil || f.EntityType != "" || f.AVStatus != "clean" {
			return IndependenceDeclaration{}, ErrFileNotUsable
		}
		if aerr := s.Files.AttachSystem(ctx, *fileID, IndependenceDeclarationEntityType, id); aerr != nil {
			return IndependenceDeclaration{}, aerr
		}
	}
	now := s.now()
	row, err := dpostore.New(pdb.MustTxFromContext(ctx)).SignIndependenceDeclaration(ctx, dpostore.SignIndependenceDeclarationParams{
		ID: id, RowVersion: version, SignedAt: pgtype.Timestamptz{Time: now, Valid: true}, FileID: pgUUID(fileID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return IndependenceDeclaration{}, ErrVersionMismatch
	}
	if err != nil {
		return IndependenceDeclaration{}, err
	}
	out := toIndependenceDeclaration(dpostore.GetIndependenceDeclarationRow(row))
	return out, s.audit(ctx, "dpo.independence_declaration.sign", out.ID, independenceDeclarationAudit(before), independenceDeclarationAudit(out))
}

func independenceDeclarationAudit(d IndependenceDeclaration) map[string]any {
	return map[string]any{"appointment_id": d.AppointmentID, "year": d.Year, "has_conflict": d.HasConflict, "signed_at": d.SignedAt}
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toIndependenceDeclaration(r dpostore.GetIndependenceDeclarationRow) IndependenceDeclaration {
	return IndependenceDeclaration{
		ID: r.ID, AppointmentID: r.AppointmentID, Year: int(r.Year), OtherDuties: deref(r.OtherDuties), HasConflict: r.HasConflict,
		SignedAt: tsPtr(r.SignedAt), FileID: uuidPtr(r.FileID), RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func tsPtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
