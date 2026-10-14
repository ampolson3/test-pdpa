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

	dsarstore "pdpa-platform/internal/dsar/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/publickeys"
)

// DSAR-01 (เว็บฟอร์มขอใช้สิทธิสำหรับเจ้าของข้อมูล): the public, loginless counterpart to DSAR-02's
// staff-recorded channels. An intake_form is a thin, publishable wrapper (dsar.intake_forms) a legal entity
// opens one of — the same role notice.notices (PNG-06) and consent.collection_points (CON-09) play for their
// own public surfaces — issuing a platform.public_keys key once active. Deliberately not built on PLT-06's
// generic form engine: the fields a DSAR intake needs are exactly CreateRequestInput's own fixed shape, not a
// configurable question schema, so a dynamic form definition would just duplicate structure this table and
// CreateRequestInput already name directly.

// IntakeForm is one dsar.intake_forms row.
type IntakeForm struct {
	ID            uuid.UUID
	LegalEntityID uuid.UUID
	Name          string
	Status        string // draft | active | retired
	PublicKey     *string
	RowVersion    int32
	UpdatedAt     time.Time
}

func toIntakeForm(r dsarstore.GetIntakeFormRow) IntakeForm {
	return IntakeForm{ID: r.ID, LegalEntityID: r.LegalEntityID, Name: r.Name, Status: r.Status, PublicKey: r.PublicKey,
		RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}
}

type CreateIntakeFormInput struct {
	LegalEntityID uuid.UUID
	Name          string
}

// CreateIntakeForm opens a new draft form for a legal entity (FK visibility, rule 1).
func (s *Service) CreateIntakeForm(ctx context.Context, in CreateIntakeFormInput) (IntakeForm, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return IntakeForm{}, fmt.Errorf("%w: name", ErrInvalid)
	}
	if _, err := s.Org.GetLegalEntity(ctx, in.LegalEntityID); err != nil {
		return IntakeForm{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return IntakeForm{}, err
	}
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertIntakeForm(ctx, dsarstore.InsertIntakeFormParams{ID: id, LegalEntityID: in.LegalEntityID, Name: in.Name})
	if err != nil {
		return IntakeForm{}, err
	}
	out := toIntakeForm(dsarstore.GetIntakeFormRow(row))
	return out, s.audit(ctx, "dsar.intake_form.create", out.ID, nil, map[string]any{"name": out.Name, "legal_entity_id": out.LegalEntityID})
}

func (s *Service) GetIntakeForm(ctx context.Context, id uuid.UUID) (IntakeForm, error) {
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).GetIntakeForm(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return IntakeForm{}, ErrNotFound
	}
	if err != nil {
		return IntakeForm{}, err
	}
	return toIntakeForm(row), nil
}

// ListIntakeForms is the platform's own roster for a tenant — no acceptance criterion needs real pagination
// volume, but cursor shape matches every other module's list endpoint for consistency.
func (s *Service) ListIntakeForms(ctx context.Context, status string, cursor *RequestCursor, limit int32) ([]IntakeForm, error) {
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	p := dsarstore.ListIntakeFormsParams{Lim: limit}
	if status != "" {
		p.Status = &status
	}
	if cursor != nil {
		p.CursorAt = pgtype.Timestamptz{Time: cursor.ReceivedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: cursor.ID, Valid: true}
	}
	rows, err := q.ListIntakeForms(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]IntakeForm, 0, len(rows))
	for _, r := range rows {
		out = append(out, toIntakeForm(dsarstore.GetIntakeFormRow(r)))
	}
	return out, nil
}

// PublishIntakeForm makes a draft (or re-publishes a retired) form live: it gets a public key the portal's
// own public page resolves tenant from, the same mechanism CON-09/PNG-06 already established. A re-publish of
// an already-active form (no status change needed) is refused, matching every other module's own publish
// gate (e.g. consent.PublishCollectionPoint's identical `cur.Status == "retired"` shape, inverted here since
// "active" is this feature's own already-live state, not a terminal one — there is nothing to republish into).
func (s *Service) PublishIntakeForm(ctx context.Context, id uuid.UUID, version int32) (IntakeForm, error) {
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.LockIntakeForm(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return IntakeForm{}, ErrNotFound
	}
	if err != nil {
		return IntakeForm{}, err
	}
	if cur.RowVersion != version {
		return IntakeForm{}, ErrVersionMismatch
	}
	if cur.Status == "active" {
		return IntakeForm{}, ErrInvalidTransition
	}
	key := cur.PublicKey
	if key == nil {
		issued, err := publickeys.Issue(ctx, publickeys.EntityDsarForm, id, nil)
		if err != nil {
			return IntakeForm{}, err
		}
		key = &issued
	}
	if err := q.PublishIntakeForm(ctx, dsarstore.PublishIntakeFormParams{ID: id, PublicKey: key}); err != nil {
		return IntakeForm{}, err
	}
	if err := s.audit(ctx, "dsar.intake_form.publish", id, map[string]any{"status": cur.Status}, map[string]any{"status": "active"}); err != nil {
		return IntakeForm{}, err
	}
	return s.GetIntakeForm(ctx, id)
}

// RetireIntakeForm closes a form for good: its key stops resolving, every request already submitted through
// it is untouched.
func (s *Service) RetireIntakeForm(ctx context.Context, id uuid.UUID, version int32) (IntakeForm, error) {
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.LockIntakeForm(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return IntakeForm{}, ErrNotFound
	}
	if err != nil {
		return IntakeForm{}, err
	}
	if cur.RowVersion != version {
		return IntakeForm{}, ErrVersionMismatch
	}
	if cur.Status == "retired" {
		return IntakeForm{}, ErrInvalidTransition
	}
	if cur.PublicKey != nil {
		if err := publickeys.Revoke(ctx, *cur.PublicKey); err != nil {
			return IntakeForm{}, err
		}
	}
	if err := q.RetireIntakeForm(ctx, id); err != nil {
		return IntakeForm{}, err
	}
	if err := s.audit(ctx, "dsar.intake_form.retire", id, map[string]any{"status": cur.Status}, map[string]any{"status": "retired"}); err != nil {
		return IntakeForm{}, err
	}
	return s.GetIntakeForm(ctx, id)
}

// PublicIntakeForm is what the portal's own public page needs to render the form: the legal entity's own
// right-type picker. Refuses a form that isn't active — a draft or retired form is never resolvable by a
// public key that still happens to exist, matching PNG-06's own PublicNotice gate.
func (s *Service) PublicIntakeForm(ctx context.Context, id uuid.UUID) (IntakeForm, []RequestType, error) {
	f, err := s.GetIntakeForm(ctx, id)
	if err != nil {
		return IntakeForm{}, nil, err
	}
	if f.Status != "active" {
		return IntakeForm{}, nil, ErrNotFound
	}
	types, err := s.ListRequestTypes(ctx)
	if err != nil {
		return IntakeForm{}, nil, err
	}
	return f, types, nil
}

func (s *Service) requestTypeByCode(ctx context.Context, code string) (RequestType, error) {
	types, err := s.ListRequestTypes(ctx)
	if err != nil {
		return RequestType{}, err
	}
	for _, t := range types {
		if t.Code == code {
			return t, nil
		}
	}
	return RequestType{}, fmt.Errorf("%w: request_type_code", ErrInvalid)
}

// CreatePublicSubmission is the acceptance criterion itself: a data subject fills in the form behind the
// intake form's public key and gets their request number back immediately — no staff involved. It is a thin
// wrapper over CreateRequest (channel "web", not on behalf of anyone) plus two things a staff-recorded intake
// doesn't need: an immediate confirmation notice to the contact the requester themselves just gave (PLT-04,
// `dsar.request_received`), and an identity-verification attempt opened right away so DSAR-06's own OTP flow
// is ready for the requester to complete without a second visit — this feature's own "«include» DSAR-06"
// relationship, closing a "no portal consumer yet" deferral both DSAR-06 and IAM-05 flagged.
type PublicSubmission struct {
	RequestTypeCode string
	RequesterName   string
	RequesterEmail  string
	RequesterPhone  string
	DataSource      *string
	OTPMethod       string // otp_email | otp_sms — which contact to verify first
}

func (s *Service) CreatePublicSubmission(ctx context.Context, formID uuid.UUID, in PublicSubmission) (Request, *Verification, error) {
	form, err := s.GetIntakeForm(ctx, formID)
	if err != nil {
		return Request{}, nil, err
	}
	if form.Status != "active" {
		return Request{}, nil, ErrNotFound
	}
	rt, err := s.requestTypeByCode(ctx, in.RequestTypeCode)
	if err != nil {
		return Request{}, nil, err
	}
	email := strings.TrimSpace(in.RequesterEmail)
	phone := strings.TrimSpace(in.RequesterPhone)
	if (email == "") == (phone == "") {
		return Request{}, nil, fmt.Errorf("%w: give exactly one of requester_email or requester_phone", ErrInvalid)
	}
	contact, kind, notifyChannel := email, crypto.KindEmail, notify.ChannelEmail
	if phone != "" {
		contact, kind, notifyChannel = phone, crypto.KindPhone, notify.ChannelSMS
	}

	req, err := s.CreateRequest(ctx, CreateRequestInput{
		RequestTypeID: rt.ID, LegalEntityID: form.LegalEntityID, Channel: "web", OnBehalf: false,
		RequesterName: in.RequesterName, RequesterContact: contact, ContactKind: kind, DataSource: in.DataSource,
	})
	if err != nil {
		return Request{}, nil, err
	}

	if s.Notify != nil {
		if _, err := s.Notify.Send(ctx, notify.Request{
			TemplateCode: "dsar.request_received", Channel: notifyChannel, RecipientAddress: contact,
			Vars:       map[string]any{"request_no": req.RequestNo, "request_type_name": rt.NameTh},
			EntityType: "dsar_request", EntityID: &req.ID, Urgent: true,
		}); err != nil {
			return Request{}, nil, err
		}
	}

	var verification *Verification
	if s.Verification != nil {
		otpMethod := "otp_email"
		if phone != "" {
			otpMethod = "otp_sms"
		}
		v, err := s.StartOTPVerification(ctx, req.ID, otpMethod)
		if err != nil {
			return Request{}, nil, err
		}
		verification = &v
	}
	return req, verification, nil
}
