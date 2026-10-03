// Package service is the dpo module: the DPO appointment register (DPO-01) — internal, external or group
// appointments, the appointment order and any PDPC-notification evidence as PLT-09 uploads, and the current
// appointment's contact channel exposed to PLT-16 documents as merge fields — and the security-measures
// checklist (DPO-09) on top of a PLT-06 form, whose failed items automatically open dpo.tasks remediation
// work. It works in the transaction of the context (CLAUDE.md rule 1) and reads other modules only through
// their services (rule 9).
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

	breachservice "pdpa-platform/internal/breach/service"
	dpostore "pdpa-platform/internal/dpo/store"
	dsarservice "pdpa-platform/internal/dsar/service"
	iamservice "pdpa-platform/internal/iam/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/forms"
)

// AppointmentEntityType is the PLT-09 attachment entity for both a signed appointment order and any
// PDPC-notification evidence — two different files can share it, same as breach's incident evidence.
const AppointmentEntityType = "dpo_appointment"

var (
	ErrInvalid         = errors.New("dpo: invalid input")
	ErrNotFound        = errors.New("dpo: not found")
	ErrVersionMismatch = errors.New("dpo: version mismatch")
	ErrFileNotUsable   = errors.New("dpo: file not usable")
)

var dpoTypes = []string{"internal", "external", "group"}

// Org is what dpo reads from the organization module (rule 9): the legal entity an appointment belongs to.
type Org interface {
	GetLegalEntity(ctx context.Context, id uuid.UUID) (orgservice.LegalEntity, error)
}

// Files is what dpo reads/writes in the file store (PLT-09): the caller's own clean upload for the
// appointment order or PDPC evidence.
type Files interface {
	Get(ctx context.Context, id uuid.UUID) (files.File, error)
	AttachSystem(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error
}

type Service struct {
	Audit  *audit.Service
	Org    Org
	Files  Files
	Forms  *forms.Service // DPO-09's security-measures checklist (PLT-06); nil until that feature is wired
	Dsar   *dsarservice.Service
	Breach *breachservice.Service
	Now    func() time.Time
}

// Appointment is one DPO appointment (ม.41): internal (an existing iam.users row), external (a named person
// at another company) or group (a shared DPO across several tenants — recorded the same way as external here,
// since no cross-tenant DPO directory exists yet). EndedAt marks the appointment over; a legal entity can have
// several appointments over time but at most one current (ended_at IS NULL) one, enforced in SaveAppointment.
type Appointment struct {
	ID                 uuid.UUID
	LegalEntityID      uuid.UUID
	DPOType            string // internal | external | group
	UserID             *uuid.UUID
	ExternalName       string
	ExternalCompany    string
	ContactEmail       string
	ContactPhone       string
	AppointedAt        time.Time
	AppointmentFileID  *uuid.UUID
	PDPCNotifiedAt     *time.Time
	PDPCEvidenceFileID *uuid.UUID
	EndedAt            *time.Time
	RowVersion         int32
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type AppointmentCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

type AppointmentFilter struct {
	LegalEntityID *uuid.UUID
	After         *AppointmentCursor
	Limit         int
}

const appointmentPageSize = 50

func (a *Appointment) normalize() error {
	if !contains(dpoTypes, a.DPOType) {
		return fmt.Errorf("%w: dpo_type", ErrInvalid)
	}
	a.ExternalName = strings.TrimSpace(a.ExternalName)
	a.ExternalCompany = strings.TrimSpace(a.ExternalCompany)
	a.ContactEmail = strings.TrimSpace(strings.ToLower(a.ContactEmail))
	a.ContactPhone = strings.TrimSpace(a.ContactPhone)
	if a.ContactEmail == "" || !strings.Contains(a.ContactEmail, "@") {
		return fmt.Errorf("%w: contact_email", ErrInvalid)
	}
	if a.AppointedAt.IsZero() {
		return fmt.Errorf("%w: appointed_at", ErrInvalid)
	}
	switch a.DPOType {
	case "internal":
		if a.UserID == nil {
			return fmt.Errorf("%w: user_id", ErrInvalid)
		}
		a.ExternalName, a.ExternalCompany = "", ""
	case "external", "group":
		a.UserID = nil
		if a.ExternalName == "" {
			return fmt.Errorf("%w: external_name", ErrInvalid)
		}
	}
	if a.EndedAt != nil && a.EndedAt.Before(a.AppointedAt) {
		return fmt.Errorf("%w: ended_at", ErrInvalid)
	}
	if a.PDPCNotifiedAt != nil && a.PDPCNotifiedAt.Before(a.AppointedAt) {
		return fmt.Errorf("%w: pdpc_notified_at", ErrInvalid)
	}
	return nil
}

func (s *Service) ListAppointments(ctx context.Context, f AppointmentFilter) ([]Appointment, *AppointmentCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > appointmentPageSize {
		limit = appointmentPageSize
	}
	p := dpostore.ListAppointmentsParams{Lim: int32(limit + 1)}
	p.LegalEntityID = pgUUID(f.LegalEntityID)
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.CreatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := dpostore.New(pdb.MustTxFromContext(ctx)).ListAppointments(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Appointment, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &AppointmentCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
		}
		out = append(out, toAppointment(dpostore.GetAppointmentRow(r)))
	}
	return out, nil, nil
}

func (s *Service) GetAppointment(ctx context.Context, id uuid.UUID) (Appointment, error) {
	r, err := dpostore.New(pdb.MustTxFromContext(ctx)).GetAppointment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Appointment{}, ErrNotFound
	}
	if err != nil {
		return Appointment{}, err
	}
	return toAppointment(r), nil
}

// SaveAppointment creates (zero ID) or updates (with the If-Match version) a DPO appointment. A file id
// (appointment order or PDPC evidence) is checked and attached only when it's new or changed — re-attaching
// a file already attached to this same appointment would fail the "unattached" check on every plain update.
func (s *Service) SaveAppointment(ctx context.Context, a Appointment, version int32) (Appointment, error) {
	if err := a.normalize(); err != nil {
		return Appointment{}, err
	}
	if _, err := s.Org.GetLegalEntity(ctx, a.LegalEntityID); err != nil {
		return Appointment{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	if a.DPOType == "internal" {
		names, err := iamservice.Names(ctx, []uuid.UUID{*a.UserID})
		if err != nil {
			return Appointment{}, err
		}
		if _, ok := names[*a.UserID]; !ok {
			return Appointment{}, fmt.Errorf("%w: user_id", ErrInvalid)
		}
	}

	var before *Appointment
	isNew := a.ID == uuid.Nil
	if !isNew {
		cur, err := s.GetAppointment(ctx, a.ID)
		if err != nil {
			return Appointment{}, err
		}
		if cur.RowVersion != version {
			return Appointment{}, ErrVersionMismatch
		}
		before = &cur
	}
	if a.AppointmentFileID != nil && (before == nil || before.AppointmentFileID == nil || *before.AppointmentFileID != *a.AppointmentFileID) {
		if err := s.attachFile(ctx, *a.AppointmentFileID, a.ID); err != nil {
			return Appointment{}, err
		}
	}
	if a.PDPCEvidenceFileID != nil && (before == nil || before.PDPCEvidenceFileID == nil || *before.PDPCEvidenceFileID != *a.PDPCEvidenceFileID) {
		if err := s.attachFile(ctx, *a.PDPCEvidenceFileID, a.ID); err != nil {
			return Appointment{}, err
		}
	}

	if isNew {
		id, err := uuid.NewV7()
		if err != nil {
			return Appointment{}, err
		}
		a.ID = id
	}
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	var err error
	if isNew {
		_, err = q.InsertAppointment(ctx, dpostore.InsertAppointmentParams{ID: a.ID, LegalEntityID: a.LegalEntityID, DpoType: a.DPOType,
			UserID: pgUUID(a.UserID), ExternalName: opt(a.ExternalName), ExternalCompany: opt(a.ExternalCompany), ContactEmail: a.ContactEmail,
			ContactPhone: opt(a.ContactPhone), AppointedAt: pgDate(&a.AppointedAt), AppointmentFileID: pgUUID(a.AppointmentFileID)})
	} else {
		_, err = q.UpdateAppointment(ctx, dpostore.UpdateAppointmentParams{ID: a.ID, RowVersion: version, LegalEntityID: a.LegalEntityID,
			DpoType: a.DPOType, UserID: pgUUID(a.UserID), ExternalName: opt(a.ExternalName), ExternalCompany: opt(a.ExternalCompany),
			ContactEmail: a.ContactEmail, ContactPhone: opt(a.ContactPhone), AppointedAt: pgDate(&a.AppointedAt),
			AppointmentFileID: pgUUID(a.AppointmentFileID), PdpcNotifiedAt: pgDate(a.PDPCNotifiedAt), PdpcEvidenceFileID: pgUUID(a.PDPCEvidenceFileID),
			EndedAt: pgDate(a.EndedAt)})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Appointment{}, ErrVersionMismatch
	}
	if err != nil {
		return Appointment{}, err
	}
	out, err := s.GetAppointment(ctx, a.ID)
	if err != nil {
		return Appointment{}, err
	}
	action, b := "dpo.appointment.create", any(nil)
	if !isNew {
		action, b = "dpo.appointment.update", appointmentAudit(*before)
	}
	return out, s.audit(ctx, action, out.ID, b, appointmentAudit(out))
}

func (s *Service) attachFile(ctx context.Context, fileID, appointmentID uuid.UUID) error {
	f, err := s.Files.Get(ctx, fileID)
	if err != nil || f.EntityType != "" || f.AVStatus != "clean" {
		return ErrFileNotUsable
	}
	return s.Files.AttachSystem(ctx, fileID, AppointmentEntityType, appointmentID)
}

// Contact is the current DPO's contact channel for a legal entity (the acceptance criterion: every notice
// shows the latest one) — a name (the internal user's, or the external person/company) plus e-mail and phone.
type Contact struct {
	Name  string
	Email string
	Phone string
}

// CurrentContact returns the legal entity's active (not yet ended) appointment's contact channel, or false
// if none is on record.
func (s *Service) CurrentContact(ctx context.Context, legalEntityID uuid.UUID) (Contact, bool, error) {
	r, err := dpostore.New(pdb.MustTxFromContext(ctx)).CurrentAppointment(ctx, legalEntityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Contact{}, false, nil
	}
	if err != nil {
		return Contact{}, false, err
	}
	a := toAppointment(dpostore.GetAppointmentRow(r))
	name := strings.TrimSpace(a.ExternalName + " " + a.ExternalCompany)
	if a.DPOType == "internal" && a.UserID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*a.UserID})
		if err != nil {
			return Contact{}, false, err
		}
		name = names[*a.UserID]
	}
	return Contact{Name: name, Email: a.ContactEmail, Phone: a.ContactPhone}, true, nil
}

// MergeFields is docs.Service's second merge-field source (alongside org's): the current DPO's contact
// channel for the document's legal entity. Empty when no appointment is on record — the draft then shows
// the merge field as [dpo_name]/[dpo_email]/[dpo_phone] and publishing refuses it, same as any other
// unresolved merge field (rule 8: never hard-code the contact channel into a template).
func (s *Service) MergeFields(ctx context.Context, legalEntityID uuid.UUID) (map[string]string, error) {
	c, ok, err := s.CurrentContact(ctx, legalEntityID)
	if err != nil || !ok {
		return nil, err
	}
	return map[string]string{"dpo_name": c.Name, "dpo_email": c.Email, "dpo_phone": c.Phone}, nil
}

func (s *Service) audit(ctx context.Context, action string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		var t string
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("dpo: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: AppointmentEntityType, EntityID: &id, Before: before, After: after}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}

func appointmentAudit(a Appointment) map[string]any {
	return map[string]any{"legal_entity_id": a.LegalEntityID, "dpo_type": a.DPOType, "user_id": a.UserID,
		"contact_email": a.ContactEmail, "ended_at": a.EndedAt}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func toAppointment(r dpostore.GetAppointmentRow) Appointment {
	return Appointment{ID: r.ID, LegalEntityID: r.LegalEntityID, DPOType: r.DpoType, UserID: uuidPtr(r.UserID),
		ExternalName: deref(r.ExternalName), ExternalCompany: deref(r.ExternalCompany), ContactEmail: r.ContactEmail,
		ContactPhone: deref(r.ContactPhone), AppointedAt: dateVal(r.AppointedAt), AppointmentFileID: uuidPtr(r.AppointmentFileID),
		PDPCNotifiedAt: datePtr(r.PdpcNotifiedAt), PDPCEvidenceFileID: uuidPtr(r.PdpcEvidenceFileID), EndedAt: datePtr(r.EndedAt),
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
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
	id := uuid.UUID(v.Bytes)
	return &id
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

func pgDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func dateVal(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}

func datePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}
