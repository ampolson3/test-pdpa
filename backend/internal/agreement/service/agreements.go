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

	agreementstore "pdpa-platform/internal/agreement/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/docs"
)

var agreementTypes = map[string]bool{"dpa": true, "dsa": true, "joint_controller": true, "inbound_dpa": true}

// supportedAgreementTypes are the types this pass actually wires to a document type/counterparty-role
// convention — "dsa"/"joint_controller"/"inbound_dpa" are real values in the DB's own CHECK constraint
// (for when those modules exist) but have no owning feature yet, the same "leave the column, build the
// real thing later" deferral ROPA-01's own discovered_by_finding_id already used.
var supportedAgreementTypes = map[string]bool{"dpa": true}

var ourRoles = map[string]bool{"controller": true, "processor": true, "joint_controller": true}

// Agreement is one DPA (or, once built, DSA) wrapping a PLT-16 document.
type Agreement struct {
	ID                uuid.UUID
	AgreementType     string
	AgreementNo       string
	Title             string
	OurRole           string
	CounterpartyID    uuid.UUID
	VendorID          *uuid.UUID
	TemplateID        *uuid.UUID
	DocumentID        uuid.UUID
	Status            string
	EffectiveFrom     *time.Time
	AutoRenew         bool
	RenewalNoticeDays int
	ActivityIDs       []uuid.UUID
	RowVersion        int32
	CreatedAt         time.Time
}

// CreateInput is DPA-02's wizard: pick a VEN-01 vendor and zero or more RoPA activities, optionally a
// published template, and the system drafts a complete DPA document in one call (the acceptance
// criterion — "within 10 minutes" means "one call", the same reading PNG-01's own "within 30 minutes"
// acceptance criterion used). Leaving TemplateID nil is DPA-02's own "โหมดกรอกเอง" (manual mode): the
// document starts blank, same as any other PLT-16 document created without a template.
type CreateInput struct {
	AgreementType     string
	OurRole           string
	VendorID          uuid.UUID
	LegalEntityID     uuid.UUID // our own entity, for the document's merge fields
	ActivityIDs       []uuid.UUID
	TemplateID        *uuid.UUID
	Title             string
	EffectiveFrom     *time.Time
	AutoRenew         bool
	RenewalNoticeDays int
}

func counterpartyRole(ourRole string) (string, error) {
	switch ourRole {
	case "controller":
		return "processor", nil
	case "processor":
		return "controller", nil
	case "joint_controller":
		return "joint_controller", nil
	}
	return "", fmt.Errorf("%w: our_role", ErrInvalid)
}

// CreateWizard is DPA-02's acceptance criterion: create a complete draft DPA from a vendor and its RoPA
// activities in one call. The document itself is created through docs.Service.Create (rule 9 — agreement
// never writes platform.documents directly), which already enforces the type's own agreement.dpa.create
// permission and resolves a template's content if one is given.
func (s *Service) CreateWizard(ctx context.Context, in CreateInput) (Agreement, error) {
	if !agreementTypes[in.AgreementType] {
		return Agreement{}, fmt.Errorf("%w: agreement_type", ErrInvalid)
	}
	if !supportedAgreementTypes[in.AgreementType] {
		return Agreement{}, fmt.Errorf("%w: agreement_type not yet supported", ErrInvalid)
	}
	if !ourRoles[in.OurRole] {
		return Agreement{}, fmt.Errorf("%w: our_role", ErrInvalid)
	}
	cpRole, err := counterpartyRole(in.OurRole)
	if err != nil {
		return Agreement{}, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Agreement{}, fmt.Errorf("%w: title", ErrInvalid)
	}
	if in.RenewalNoticeDays < 0 {
		return Agreement{}, fmt.Errorf("%w: renewal_notice_days", ErrInvalid)
	}

	vendor, err := s.Vendor.GetVendor(ctx, in.VendorID)
	if err != nil {
		return Agreement{}, fmt.Errorf("%w: vendor_id", ErrInvalid)
	}
	if _, err := s.Org.GetExternalParty(ctx, vendor.PartyID); err != nil {
		return Agreement{}, fmt.Errorf("%w: vendor_id", ErrInvalid)
	}
	if _, err := s.Org.GetLegalEntity(ctx, in.LegalEntityID); err != nil {
		return Agreement{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	for _, aid := range in.ActivityIDs {
		if _, err := s.Ropa.GetActivity(ctx, aid); err != nil {
			return Agreement{}, fmt.Errorf("%w: activity_ids", ErrInvalid)
		}
	}

	doc, err := s.Docs.Create(ctx, docs.CreateInput{DocType: in.AgreementType, Title: title, LegalEntityID: &in.LegalEntityID, TemplateID: in.TemplateID})
	if err != nil {
		return Agreement{}, err
	}

	q := agreementstore.New(pdb.MustTxFromContext(ctx))
	renewalDays := in.RenewalNoticeDays
	if renewalDays == 0 {
		renewalDays = 60 // the column's own default (migration 00015) — mirrored here so a zero-value input doesn't silently save 0 days.
	}
	year := time.Now().UTC().Year()
	prefix := fmt.Sprintf("%s-%d-", strings.ToUpper(in.AgreementType), year)
	if err := q.LockAgreementNumbering(ctx, prefix); err != nil {
		return Agreement{}, err
	}
	n, err := q.CountAgreementsWithPrefix(ctx, prefix)
	if err != nil {
		return Agreement{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Agreement{}, err
	}
	row, err := q.InsertAgreement(ctx, agreementstore.InsertAgreementParams{
		ID: id, AgreementType: in.AgreementType, AgreementNo: fmt.Sprintf("%s%04d", prefix, n+1), Title: title, OurRole: in.OurRole,
		CounterpartyID: vendor.PartyID, VendorID: pgUUID(&in.VendorID), TemplateID: pgUUID(in.TemplateID), DocumentID: doc.ID,
		AutoRenew: in.AutoRenew, RenewalNoticeDays: int16(renewalDays), EffectiveFrom: pgDate(in.EffectiveFrom),
	})
	if err != nil {
		return Agreement{}, err
	}

	partyID, err := uuid.NewV7()
	if err != nil {
		return Agreement{}, err
	}
	if _, err := q.InsertAgreementParty(ctx, agreementstore.InsertAgreementPartyParams{
		ID: partyID, AgreementID: id, PartyID: pgUUID(&vendor.PartyID), PartyRole: cpRole,
	}); err != nil {
		return Agreement{}, err
	}
	for _, aid := range in.ActivityIDs {
		if err := q.InsertAgreementActivity(ctx, agreementstore.InsertAgreementActivityParams{AgreementID: id, ActivityID: aid}); err != nil {
			return Agreement{}, err
		}
	}

	if err := s.audit(ctx, "agreement.agreement.create", id, nil, map[string]any{
		"agreement_type": in.AgreementType, "agreement_no": row.AgreementNo, "vendor_id": in.VendorID, "activity_count": len(in.ActivityIDs),
	}); err != nil {
		return Agreement{}, err
	}
	return s.GetAgreement(ctx, id)
}

// GetAgreement returns one agreement with its linked activity ids.
func (s *Service) GetAgreement(ctx context.Context, id uuid.UUID) (Agreement, error) {
	q := agreementstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetAgreement(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Agreement{}, ErrNotFound
	}
	if err != nil {
		return Agreement{}, err
	}
	activityIDs, err := q.ListAgreementActivityIDs(ctx, id)
	if err != nil {
		return Agreement{}, err
	}
	return toAgreement(row, activityIDs), nil
}

type AgreementCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

type AgreementFilter struct {
	AgreementType string
	VendorID      *uuid.UUID
	After         *AgreementCursor
	Limit         int
}

const agreementPageSize = 50

// ListAgreements lists the tenant's own agreements, newest first, optionally filtered by type or vendor
// (DPA-11's own "open a vendor and see its agreements" will reuse this filter).
func (s *Service) ListAgreements(ctx context.Context, f AgreementFilter) ([]Agreement, *AgreementCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > agreementPageSize {
		limit = agreementPageSize
	}
	p := agreementstore.ListAgreementsParams{Lim: int32(limit + 1)}
	if f.AgreementType != "" {
		p.AgreementType = &f.AgreementType
	}
	p.VendorID = pgUUID(f.VendorID)
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.CreatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := agreementstore.New(pdb.MustTxFromContext(ctx)).ListAgreements(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Agreement, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &AgreementCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
		}
		out = append(out, toAgreement(r, nil))
	}
	return out, nil, nil
}

func toAgreement(r agreementstore.AgreementAgreement, activityIDs []uuid.UUID) Agreement {
	a := Agreement{
		ID: r.ID, AgreementType: r.AgreementType, AgreementNo: r.AgreementNo, Title: r.Title, OurRole: r.OurRole,
		CounterpartyID: r.CounterpartyID, VendorID: uuidPtr(r.VendorID), TemplateID: uuidPtr(r.TemplateID), DocumentID: r.DocumentID,
		Status: r.Status, AutoRenew: r.AutoRenew, RenewalNoticeDays: int(r.RenewalNoticeDays), ActivityIDs: activityIDs,
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time,
	}
	if r.EffectiveFrom.Valid {
		t := r.EffectiveFrom.Time
		a.EffectiveFrom = &t
	}
	return a
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

func pgDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}
