package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	orgstore "pdpa-platform/internal/org/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// ORG-06: the tenant's directory of external parties (processors, recipients, government bodies, …)
// that RoPA, DSA, DPA and Vendor all point at instead of keeping their own copy.

const ExternalPartyEntityType = "external_party"

var partyTypes = []string{"processor", "recipient", "controller", "joint_controller", "government", "other"}

var dedupeStrip = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// Contact is the free-form contact person on an external party.
type Contact struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// ExternalParty is a processor, recipient, controller or government body other modules reference by id.
type ExternalParty struct {
	ID             uuid.UUID
	PartyType      string
	NameTh         string
	NameEn         string
	RegistrationNo string
	CountryCode    string
	Contact        Contact
	Website        string
	Status         string // active | inactive
	MergedIntoID   *uuid.UUID
	RowVersion     int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// PartyCursor is the position after the last party of a page.
type PartyCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// PartyFilter narrows ListExternalParties.
type PartyFilter struct {
	PartyType   string
	CountryCode string
	Query       string
	After       *PartyCursor
	Limit       int
}

const partyPageSize = 50

var ErrPartyMerged = errors.New("org: this party was merged into another one")

func dedupeKey(nameTh, nameEn, country string) *string {
	name := nameTh
	if name == "" {
		name = nameEn
	}
	k := strings.ToLower(dedupeStrip.ReplaceAllString(name, "")) + "|" + strings.ToLower(country)
	if k == "|" {
		return nil
	}
	return &k
}

func (p *ExternalParty) normalize() error {
	p.NameTh, p.NameEn = strings.TrimSpace(p.NameTh), strings.TrimSpace(p.NameEn)
	if p.NameTh == "" || len([]rune(p.NameTh)) > 300 || len([]rune(p.NameEn)) > 300 {
		return fmt.Errorf("%w: name_th", ErrInvalid)
	}
	if !slices.Contains(partyTypes, p.PartyType) {
		return fmt.Errorf("%w: party_type", ErrInvalid)
	}
	p.CountryCode = strings.ToUpper(strings.TrimSpace(p.CountryCode))
	if len(p.CountryCode) != 2 {
		return fmt.Errorf("%w: country_code", ErrInvalid)
	}
	p.RegistrationNo = strings.TrimSpace(p.RegistrationNo)
	if len(p.RegistrationNo) > 30 {
		return fmt.Errorf("%w: registration_no", ErrInvalid)
	}
	p.Website = strings.TrimSpace(p.Website)
	p.Contact.Name = strings.TrimSpace(p.Contact.Name)
	p.Contact.Email = strings.TrimSpace(p.Contact.Email)
	if p.Contact.Email != "" {
		if addr, err := mail.ParseAddress(p.Contact.Email); err != nil || addr.Address != p.Contact.Email {
			return fmt.Errorf("%w: contact.email", ErrInvalid)
		}
	}
	p.Contact.Phone = strings.TrimSpace(p.Contact.Phone)
	if p.Contact.Phone != "" && !phoneRE.MatchString(p.Contact.Phone) {
		return fmt.Errorf("%w: contact.phone", ErrInvalid)
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if p.Status != "active" && p.Status != "inactive" {
		return fmt.Errorf("%w: status", ErrInvalid)
	}
	return nil
}

func (s *Service) ListExternalParties(ctx context.Context, f PartyFilter) ([]ExternalParty, *PartyCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > partyPageSize {
		limit = partyPageSize
	}
	p := orgstore.ListExternalPartiesParams{Lim: int32(limit + 1)}
	if f.PartyType != "" {
		p.PartyType = &f.PartyType
	}
	if f.CountryCode != "" {
		cc := strings.ToUpper(f.CountryCode)
		p.CountryCode = &cc
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
		p.Q = &esc
	}
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.CreatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := orgstore.New(pdb.MustTxFromContext(ctx)).ListExternalParties(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]ExternalParty, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &PartyCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
		}
		out = append(out, toParty(orgstore.GetExternalPartyRow(r)))
	}
	return out, nil, nil
}

func (s *Service) GetExternalParty(ctx context.Context, id uuid.UUID) (ExternalParty, error) {
	r, err := orgstore.New(pdb.MustTxFromContext(ctx)).GetExternalParty(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExternalParty{}, ErrNotFound
	}
	if err != nil {
		return ExternalParty{}, err
	}
	return toParty(r), nil
}

// SaveExternalParty creates (zero ID) or updates (with the If-Match version) an external party.
func (s *Service) SaveExternalParty(ctx context.Context, p ExternalParty, version int32) (ExternalParty, error) {
	if err := p.normalize(); err != nil {
		return ExternalParty{}, err
	}
	var before *ExternalParty
	if p.ID != uuid.Nil {
		cur, err := s.GetExternalParty(ctx, p.ID)
		if err != nil {
			return ExternalParty{}, err
		}
		if cur.RowVersion != version {
			return ExternalParty{}, ErrVersionMismatch
		}
		if cur.MergedIntoID != nil {
			return ExternalParty{}, ErrPartyMerged
		}
		before = &cur
	}
	isNew := p.ID == uuid.Nil
	if isNew {
		id, err := uuid.NewV7()
		if err != nil {
			return ExternalParty{}, err
		}
		p.ID = id
	}
	contact, _ := marshalContact(p.Contact)
	dk := dedupeKey(p.NameTh, p.NameEn, p.CountryCode)
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	var row orgstore.GetExternalPartyRow
	var err error
	if isNew {
		var r orgstore.InsertExternalPartyRow
		r, err = q.InsertExternalParty(ctx, orgstore.InsertExternalPartyParams{ID: p.ID, PartyType: p.PartyType, NameTh: p.NameTh,
			NameEn: opt(p.NameEn), RegistrationNo: opt(p.RegistrationNo), CountryCode: p.CountryCode, Contact: contact,
			Website: opt(p.Website), DedupeKey: dk})
		row = orgstore.GetExternalPartyRow(r)
	} else {
		var r orgstore.UpdateExternalPartyRow
		r, err = q.UpdateExternalParty(ctx, orgstore.UpdateExternalPartyParams{ID: p.ID, RowVersion: version, PartyType: p.PartyType,
			NameTh: p.NameTh, NameEn: opt(p.NameEn), RegistrationNo: opt(p.RegistrationNo), CountryCode: p.CountryCode, Contact: contact,
			Website: opt(p.Website), DedupeKey: dk, Status: p.Status})
		row = orgstore.GetExternalPartyRow(r)
	}
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23503":
		return ExternalParty{}, fmt.Errorf("%w: country_code", ErrInvalid)
	case errors.Is(err, pgx.ErrNoRows):
		return ExternalParty{}, ErrVersionMismatch
	case err != nil:
		return ExternalParty{}, err
	}
	out := toParty(row)
	action, b := "org.party.create", any(nil)
	if !isNew {
		action, b = "org.party.update", partyAudit(*before)
	}
	return out, s.audit(ctx, action, out.ID, b, partyAudit(out))
}

// Duplicates groups active, unmerged parties that share a dedupe_key (same normalized name + country) —
// the FE offers to merge each group.
func (s *Service) DuplicateExternalParties(ctx context.Context) (map[string][]ExternalParty, error) {
	rows, err := orgstore.New(pdb.MustTxFromContext(ctx)).ListDuplicateExternalParties(ctx)
	if err != nil {
		return nil, err
	}
	groups := map[string][]ExternalParty{}
	for _, r := range rows {
		key := deref(r.DedupeKey)
		groups[key] = append(groups[key], ExternalParty{ID: r.ID, PartyType: r.PartyType, NameTh: r.NameTh, NameEn: deref(r.NameEn), CountryCode: r.CountryCode})
	}
	return groups, nil
}

// MergeExternalParty keeps target as the one record every module resolves to; source becomes inactive
// and points at target (ORG-06 acceptance: one record per external party). Nothing today writes a real
// FK to org.external_parties from another module yet (RoPA/DSAR/Vendor/Agreement aren't built), so there
// are no cross-schema references to reassign — when the first one lands it must resolve merged_into_id.
func (s *Service) MergeExternalParty(ctx context.Context, sourceID, targetID uuid.UUID, version int32) error {
	if sourceID == targetID {
		return fmt.Errorf("%w: a party can't merge into itself", ErrInvalid)
	}
	target, err := s.GetExternalParty(ctx, targetID)
	if err != nil {
		return err
	}
	if target.MergedIntoID != nil {
		return fmt.Errorf("%w: target was itself merged away", ErrInvalid)
	}
	source, err := s.GetExternalParty(ctx, sourceID)
	if err != nil {
		return err
	}
	if source.MergedIntoID != nil {
		return ErrPartyMerged
	}
	n, err := orgstore.New(pdb.MustTxFromContext(ctx)).MergeExternalParty(ctx, orgstore.MergeExternalPartyParams{ID: sourceID, MergedIntoID: pgUUID(&targetID), RowVersion: version})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrVersionMismatch
	}
	return s.audit(ctx, "org.party.merge", sourceID, partyAudit(source), map[string]any{"merged_into_id": targetID})
}

func toParty(r orgstore.GetExternalPartyRow) ExternalParty {
	p := ExternalParty{ID: r.ID, PartyType: r.PartyType, NameTh: r.NameTh, NameEn: deref(r.NameEn), RegistrationNo: deref(r.RegistrationNo),
		CountryCode: r.CountryCode, Website: deref(r.Website), Status: r.Status, RowVersion: r.RowVersion,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	_ = unmarshalContact(r.Contact, &p.Contact)
	if r.MergedIntoID.Valid {
		id := uuid.UUID(r.MergedIntoID.Bytes)
		p.MergedIntoID = &id
	}
	return p
}

func marshalContact(c Contact) ([]byte, error) { return json.Marshal(c) }

func unmarshalContact(b []byte, c *Contact) error {
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, c)
}

func partyAudit(p ExternalParty) map[string]any {
	return map[string]any{"party_type": p.PartyType, "name_th": p.NameTh, "name_en": p.NameEn, "registration_no": p.RegistrationNo,
		"country_code": p.CountryCode, "website": p.Website, "status": p.Status}
}
