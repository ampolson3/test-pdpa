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

	breachstore "pdpa-platform/internal/breach/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/publickeys"
)

// BRE-01 (แบบฟอร์มแจ้งเหตุ): BRE-02's own admin app already gives EMP a login-gated intake form
// (reported_via "employee_form", breach.incident.create). What's missing is PUBLIC's own channel — an
// external person with no login — the same role dsar.intake_forms (DSAR-01) plays for DSAR's own public
// submission. A report_form is a thin, publishable wrapper (breach.report_forms) a legal entity opens one
// of, issuing a platform.public_keys key once active.

const (
	reportContactClass   = "breach"
	reportContactContext = "breach.incidents.reporter_contact_enc"
)

// ReportForm is one breach.report_forms row.
type ReportForm struct {
	ID            uuid.UUID
	LegalEntityID uuid.UUID
	Name          string
	Status        string // draft | active | retired
	PublicKey     *string
	RowVersion    int32
	UpdatedAt     time.Time
}

func toReportForm(r breachstore.GetReportFormRow) ReportForm {
	return ReportForm{ID: r.ID, LegalEntityID: r.LegalEntityID, Name: r.Name, Status: r.Status, PublicKey: r.PublicKey,
		RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}
}

type CreateReportFormInput struct {
	LegalEntityID uuid.UUID
	Name          string
}

// CreateReportForm opens a new draft form for a legal entity, checked visible under RLS first (rule 1).
func (s *Service) CreateReportForm(ctx context.Context, in CreateReportFormInput) (ReportForm, error) {
	if !has(ctx, PermFormCreate) {
		return ReportForm{}, ErrForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return ReportForm{}, invalid("name", "required")
	}
	if s.Org != nil {
		if _, err := s.Org.GetLegalEntity(ctx, in.LegalEntityID); err != nil {
			return ReportForm{}, invalid("legal_entity_id", "not_found")
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ReportForm{}, err
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertReportForm(ctx, breachstore.InsertReportFormParams{ID: id, LegalEntityID: in.LegalEntityID, Name: in.Name})
	if err != nil {
		return ReportForm{}, err
	}
	out := toReportForm(breachstore.GetReportFormRow(row))
	return out, s.audit(ctx, "breach.report_form.create", "breach_report_form", out.ID, nil, map[string]any{"name": out.Name, "legal_entity_id": out.LegalEntityID})
}

func (s *Service) GetReportForm(ctx context.Context, id uuid.UUID) (ReportForm, error) {
	if !has(ctx, PermFormRead) {
		return ReportForm{}, ErrForbidden
	}
	row, err := breachstore.New(pdb.MustTxFromContext(ctx)).GetReportForm(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportForm{}, ErrNotFound
	}
	if err != nil {
		return ReportForm{}, err
	}
	return toReportForm(row), nil
}

// ListReportForms is the tenant's own roster — no acceptance criterion needs real pagination volume, but
// cursor shape matches every other module's list endpoint for consistency.
func (s *Service) ListReportForms(ctx context.Context, status string, cursorAt *time.Time, cursorID *uuid.UUID, limit int32) ([]ReportForm, error) {
	if !has(ctx, PermFormRead) {
		return nil, ErrForbidden
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	p := breachstore.ListReportFormsParams{Lim: limit}
	if status != "" {
		p.Status = &status
	}
	if cursorAt != nil && cursorID != nil {
		p.CursorAt = pgtype.Timestamptz{Time: *cursorAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: *cursorID, Valid: true}
	}
	rows, err := q.ListReportForms(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]ReportForm, 0, len(rows))
	for _, r := range rows {
		out = append(out, toReportForm(breachstore.GetReportFormRow(r)))
	}
	return out, nil
}

// PublishReportForm makes a draft (or re-publishes a retired) form live: it gets a public key the portal's
// own public page resolves tenant from (CON-09/PNG-06/DSAR-01's own mechanism). A re-publish of an
// already-active form is refused — there is nothing to republish into.
func (s *Service) PublishReportForm(ctx context.Context, id uuid.UUID, version int32) (ReportForm, error) {
	if !has(ctx, PermFormPublish) {
		return ReportForm{}, ErrForbidden
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.LockReportForm(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportForm{}, ErrNotFound
	}
	if err != nil {
		return ReportForm{}, err
	}
	if cur.RowVersion != version {
		return ReportForm{}, ErrVersionMismatch
	}
	if cur.Status == "active" {
		return ReportForm{}, ErrInvalidTransition
	}
	key := cur.PublicKey
	if key == nil {
		issued, err := publickeys.Issue(ctx, publickeys.EntityBreachForm, id, nil)
		if err != nil {
			return ReportForm{}, err
		}
		key = &issued
	}
	if err := q.PublishReportForm(ctx, breachstore.PublishReportFormParams{ID: id, PublicKey: key}); err != nil {
		return ReportForm{}, err
	}
	if err := s.audit(ctx, "breach.report_form.publish", "breach_report_form", id, map[string]any{"status": cur.Status}, map[string]any{"status": "active"}); err != nil {
		return ReportForm{}, err
	}
	return s.GetReportForm(ctx, id)
}

// RetireReportForm closes a form for good: its key stops resolving, every incident already reported through
// it is untouched.
func (s *Service) RetireReportForm(ctx context.Context, id uuid.UUID, version int32) (ReportForm, error) {
	if !has(ctx, PermFormDelete) {
		return ReportForm{}, ErrForbidden
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.LockReportForm(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportForm{}, ErrNotFound
	}
	if err != nil {
		return ReportForm{}, err
	}
	if cur.RowVersion != version {
		return ReportForm{}, ErrVersionMismatch
	}
	if cur.Status == "retired" {
		return ReportForm{}, ErrInvalidTransition
	}
	if cur.PublicKey != nil {
		if err := publickeys.Revoke(ctx, *cur.PublicKey); err != nil {
			return ReportForm{}, err
		}
	}
	if err := q.RetireReportForm(ctx, id); err != nil {
		return ReportForm{}, err
	}
	if err := s.audit(ctx, "breach.report_form.retire", "breach_report_form", id, map[string]any{"status": cur.Status}, map[string]any{"status": "retired"}); err != nil {
		return ReportForm{}, err
	}
	return s.GetReportForm(ctx, id)
}

// publicReportForm is the raw lookup PublicReportForm/CreatePublicReport share: no permission check (the
// public key, not a Grant, is what authorizes a /public/v1 caller), refuses a form that isn't active — a
// draft or retired form is never resolvable by a public key that still happens to exist, matching PNG-06's
// own PublicNotice gate.
func (s *Service) publicReportForm(ctx context.Context, id uuid.UUID) (ReportForm, error) {
	row, err := breachstore.New(pdb.MustTxFromContext(ctx)).GetReportForm(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportForm{}, ErrNotFound
	}
	if err != nil {
		return ReportForm{}, err
	}
	f := toReportForm(row)
	if f.Status != "active" {
		return ReportForm{}, ErrNotFound
	}
	return f, nil
}

// PublicReportForm is what the portal's own public page needs to render the form: the legal entity + the
// breach-type picker (BreachTypes, this package's own var).
func (s *Service) PublicReportForm(ctx context.Context, id uuid.UUID) (ReportForm, []string, error) {
	f, err := s.publicReportForm(ctx, id)
	if err != nil {
		return ReportForm{}, nil, err
	}
	return f, BreachTypes, nil
}

// PublicReport is what a loginless reporter submits: title/description/breach types, when it happened (if
// known) — aware_at is always "now", since for an externally-reported incident that is the moment the
// organization itself becomes aware, matching ม.37(4)'s own "ทราบเหตุ" — plus an optional contact (encrypted,
// PLT-13) so staff can follow up.
type PublicReport struct {
	Title           string
	Description     string
	BreachTypes     []string
	OccurredAt      *time.Time
	ReporterContact string
}

// CreatePublicReport is the acceptance criterion itself: a loginless submission creates an incident with
// reported_via "public_form", on the legal entity the form's own key names, and — through the shared create()
// core every Input goes through — alerts the owner and DPOs at once (BRE-02/BRE-07's own s.alert, no
// different for a publicly reported incident).
func (s *Service) CreatePublicReport(ctx context.Context, formID uuid.UUID, in PublicReport) (Incident, error) {
	f, err := s.publicReportForm(ctx, formID)
	if err != nil {
		return Incident{}, err
	}
	var contactEnc []byte
	contact := strings.TrimSpace(in.ReporterContact)
	if contact != "" {
		if s.Keyring == nil {
			return Incident{}, fmt.Errorf("%w: reporter_contact not supported", ErrInvalidRequest)
		}
		// A free-text contact detail for staff follow-up, not an exact-match lookup key — encrypted as given
		// (PLT-13), no blind_index computed (unlike DSAR's own requester_contact, which is searched by DSAR-17).
		enc, err := s.Keyring.Encrypt(ctx, reportContactClass, reportContactContext, []byte(contact))
		if err != nil {
			return Incident{}, err
		}
		contactEnc = enc
	}
	return s.create(ctx, Input{
		LegalEntityID: f.LegalEntityID, ReportedVia: "public_form", Title: in.Title, Description: in.Description,
		BreachTypes: in.BreachTypes, OccurredAt: in.OccurredAt, AwareAt: s.now(),
	}, contactEnc)
}
