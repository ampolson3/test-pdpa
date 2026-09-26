package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	orgstore "pdpa-platform/internal/org/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/files"
)

// ORG-01 legal entities and ORG-04 the org-unit tree.

const (
	LegalEntityType = "legal_entity"
	OrgUnitType     = "org_unit"
)

// FileStore is what the org module needs from PLT-09 for logos (files.Service implements it).
type FileStore interface {
	Get(ctx context.Context, id uuid.UUID) (files.File, error)
	Attach(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error
}

var (
	ErrCycle      = errors.New("org: a unit or entity can't be moved under itself")
	ErrHasActive  = errors.New("org: close the active units below it first")
	ErrLogoNotUse = errors.New("org: the logo must be your own clean PNG or JPEG upload")
)

func entityTypeOf(action string) string {
	switch {
	case strings.HasPrefix(action, "org.legal_entity."):
		return LegalEntityType
	case strings.HasPrefix(action, "org.unit."):
		return OrgUnitType
	case strings.HasPrefix(action, "org.master_data."):
		return MasterDataEntityType
	case strings.HasPrefix(action, "org.settings."):
		return OrgSettingsEntityType
	case strings.HasPrefix(action, "org.party."):
		return ExternalPartyEntityType
	}
	return CalendarEntityType
}

// ValidThaiID checks a 13-digit Thai identification number — juristic persons' registration numbers
// (and so their tax ids) use the same mod-11 check digit as national ids.
func ValidThaiID(s string) bool {
	if len(s) != 13 {
		return false
	}
	sum := 0
	for i := 0; i < 13; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		if i < 12 {
			sum += int(s[i]-'0') * (13 - i)
		}
	}
	return (11-sum%11)%10 == int(s[12]-'0')
}

// Address is a legal entity's address (org.legal_entities.address).
type Address struct {
	Line1       string `json:"line1,omitempty"`
	Line2       string `json:"line2,omitempty"`
	Subdistrict string `json:"subdistrict,omitempty"`
	District    string `json:"district,omitempty"`
	Province    string `json:"province,omitempty"`
	PostalCode  string `json:"postal_code,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
}

// LegalEntity is a company of the tenant (ORG-01).
type LegalEntity struct {
	ID             uuid.UUID
	ParentID       *uuid.UUID
	NameTh         string
	NameEn         string
	RegistrationNo string
	TaxID          string
	Address        Address
	ContactEmail   string
	ContactPhone   string
	LogoFileID     *uuid.UUID
	IsController   bool
	IsProcessor    bool
	Status         string
	RowVersion     int32
	UpdatedAt      time.Time
}

var (
	postalRE = regexp.MustCompile(`^\d{5}$`)
	phoneRE  = regexp.MustCompile(`^[0-9+\-() ]{3,30}$`)
)

func (e *LegalEntity) normalize() error {
	e.NameTh, e.NameEn = strings.TrimSpace(e.NameTh), strings.TrimSpace(e.NameEn)
	if e.NameTh == "" || len([]rune(e.NameTh)) > 300 || len([]rune(e.NameEn)) > 300 {
		return fmt.Errorf("%w: name", ErrInvalid)
	}
	e.RegistrationNo = strings.ReplaceAll(strings.ReplaceAll(e.RegistrationNo, "-", ""), " ", "")
	e.TaxID = strings.ReplaceAll(strings.ReplaceAll(e.TaxID, "-", ""), " ", "")
	if e.RegistrationNo != "" && !ValidThaiID(e.RegistrationNo) {
		return fmt.Errorf("%w: registration_no", ErrInvalid)
	}
	if e.TaxID != "" && !ValidThaiID(e.TaxID) {
		return fmt.Errorf("%w: tax_id", ErrInvalid)
	}
	a := &e.Address
	for _, f := range []*string{&a.Line1, &a.Line2, &a.Subdistrict, &a.District, &a.Province, &a.PostalCode, &a.CountryCode} {
		*f = strings.TrimSpace(*f)
		if len([]rune(*f)) > 200 {
			return fmt.Errorf("%w: address", ErrInvalid)
		}
	}
	a.CountryCode = strings.ToUpper(a.CountryCode)
	if a.CountryCode == "" && a.PostalCode != "" {
		a.CountryCode = "TH"
	}
	if a.CountryCode != "" && len(a.CountryCode) != 2 {
		return fmt.Errorf("%w: address country", ErrInvalid)
	}
	if a.CountryCode == "TH" && a.PostalCode != "" && !postalRE.MatchString(a.PostalCode) {
		return fmt.Errorf("%w: postal_code", ErrInvalid)
	}
	e.ContactEmail = strings.TrimSpace(e.ContactEmail)
	if e.ContactEmail != "" {
		if addr, err := mail.ParseAddress(e.ContactEmail); err != nil || addr.Address != e.ContactEmail {
			return fmt.Errorf("%w: contact_email", ErrInvalid)
		}
	}
	e.ContactPhone = strings.TrimSpace(e.ContactPhone)
	if e.ContactPhone != "" && !phoneRE.MatchString(e.ContactPhone) {
		return fmt.Errorf("%w: contact_phone", ErrInvalid)
	}
	if e.Status == "" {
		e.Status = "active"
	}
	if e.Status != "active" && e.Status != "inactive" {
		return fmt.Errorf("%w: status", ErrInvalid)
	}
	return nil
}

func (s *Service) ListLegalEntities(ctx context.Context) ([]LegalEntity, error) {
	rows, err := orgstore.New(pdb.MustTxFromContext(ctx)).ListLegalEntities(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LegalEntity, 0, len(rows))
	for _, r := range rows {
		out = append(out, toLegalEntity(orgstore.GetLegalEntityRow(r)))
	}
	return out, nil
}

func (s *Service) GetLegalEntity(ctx context.Context, id uuid.UUID) (LegalEntity, error) {
	r, err := orgstore.New(pdb.MustTxFromContext(ctx)).GetLegalEntity(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return LegalEntity{}, ErrNotFound
	}
	if err != nil {
		return LegalEntity{}, err
	}
	return toLegalEntity(r), nil
}

// SaveLegalEntity creates (zero ID) or updates (with the If-Match version) a legal entity.
func (s *Service) SaveLegalEntity(ctx context.Context, e LegalEntity, version int32) (LegalEntity, error) {
	if err := e.normalize(); err != nil {
		return LegalEntity{}, err
	}
	var before *LegalEntity
	if e.ID != uuid.Nil {
		cur, err := s.GetLegalEntity(ctx, e.ID)
		if err != nil {
			return LegalEntity{}, err
		}
		if cur.RowVersion != version {
			return LegalEntity{}, ErrVersionMismatch
		}
		before = &cur
	}
	if e.ParentID != nil { // FKs bypass RLS: the parent must be visible, and not below this entity
		if _, err := s.GetLegalEntity(ctx, *e.ParentID); err != nil {
			return LegalEntity{}, fmt.Errorf("%w: parent", ErrInvalid)
		}
		if e.ID != uuid.Nil {
			chain, err := orgstore.New(pdb.MustTxFromContext(ctx)).LegalEntityAncestors(ctx, *e.ParentID)
			if err != nil {
				return LegalEntity{}, err
			}
			for _, id := range chain {
				if id == e.ID {
					return LegalEntity{}, ErrCycle
				}
			}
		}
	}
	isNew := e.ID == uuid.Nil
	if isNew {
		id, err := uuid.NewV7()
		if err != nil {
			return LegalEntity{}, err
		}
		e.ID = id
	}
	if e.LogoFileID != nil && (before == nil || before.LogoFileID == nil || *before.LogoFileID != *e.LogoFileID) {
		if err := s.attachLogo(ctx, *e.LogoFileID, e.ID); err != nil {
			return LegalEntity{}, err
		}
	}
	addr, _ := json.Marshal(e.Address)
	var row orgstore.GetLegalEntityRow
	err := pdb.Savepoint(ctx, func(ctx context.Context) error {
		q := orgstore.New(pdb.MustTxFromContext(ctx))
		if isNew {
			r, err := q.InsertLegalEntity(ctx, orgstore.InsertLegalEntityParams{ID: e.ID, ParentID: pgUUID(e.ParentID), NameTh: e.NameTh, NameEn: opt(e.NameEn),
				RegistrationNo: opt(e.RegistrationNo), TaxID: opt(e.TaxID), Address: addr, ContactEmail: opt(e.ContactEmail), ContactPhone: opt(e.ContactPhone),
				LogoFileID: pgUUID(e.LogoFileID), IsController: e.IsController, IsProcessor: e.IsProcessor, Status: e.Status})
			row = orgstore.GetLegalEntityRow(r)
			return err
		}
		r, err := q.UpdateLegalEntity(ctx, orgstore.UpdateLegalEntityParams{ID: e.ID, RowVersion: version, ParentID: pgUUID(e.ParentID), NameTh: e.NameTh, NameEn: opt(e.NameEn),
			RegistrationNo: opt(e.RegistrationNo), TaxID: opt(e.TaxID), Address: addr, ContactEmail: opt(e.ContactEmail), ContactPhone: opt(e.ContactPhone),
			LogoFileID: pgUUID(e.LogoFileID), IsController: e.IsController, IsProcessor: e.IsProcessor, Status: e.Status})
		row = orgstore.GetLegalEntityRow(r)
		return err
	})
	switch {
	case isUnique(err):
		return LegalEntity{}, fmt.Errorf("%w: registration_no is used by another legal entity", ErrInvalid)
	case errors.Is(err, pgx.ErrNoRows):
		return LegalEntity{}, ErrVersionMismatch
	case err != nil:
		return LegalEntity{}, err
	}
	out := toLegalEntity(row)
	action, b := "org.legal_entity.create", any(nil)
	if !isNew {
		action, b = "org.legal_entity.update", legalEntityAudit(*before)
	}
	return out, s.audit(ctx, action, out.ID, b, legalEntityAudit(out))
}

func (s *Service) attachLogo(ctx context.Context, fileID, entityID uuid.UUID) error {
	if s.Files == nil {
		return ErrLogoNotUse
	}
	f, err := s.Files.Get(ctx, fileID) // only the uploader sees an unattached file
	if err != nil || f.EntityType != "" || f.AVStatus != "clean" || (f.MimeType != "image/png" && f.MimeType != "image/jpeg") {
		return ErrLogoNotUse
	}
	return s.Files.Attach(ctx, fileID, LegalEntityType, entityID)
}

// MergeFields are a legal entity's values for document templates (ORG-01: notices, letters and PDPC forms
// show the organization as recorded here). Keys are stable template variable names.
func (s *Service) MergeFields(ctx context.Context, id uuid.UUID) (map[string]string, error) {
	e, err := s.GetLegalEntity(ctx, id)
	if err != nil {
		return nil, err
	}
	a := e.Address
	parts := []string{}
	for _, p := range []string{a.Line1, a.Line2, a.Subdistrict, a.District, a.Province, a.PostalCode} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return map[string]string{
		"org_name_th": e.NameTh, "org_name_en": e.NameEn, "org_registration_no": e.RegistrationNo, "org_tax_id": e.TaxID,
		"org_address": strings.Join(parts, " "), "org_email": e.ContactEmail, "org_phone": e.ContactPhone,
	}, nil
}

// OrgUnit is a node of the organization tree (ORG-04).
type OrgUnit struct {
	ID            uuid.UUID
	LegalEntityID uuid.UUID
	ParentID      *uuid.UUID
	Path          string
	Depth         int32
	Code          string
	NameTh        string
	NameEn        string
	UnitType      string
	Status        string
	ClosedAt      *time.Time
	RowVersion    int32
	UpdatedAt     time.Time
}

var (
	unitTypes = map[string]bool{"group": true, "company": true, "division": true, "department": true, "branch": true, "team": true}
	codeRE    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,39}$`)
)

func (u *OrgUnit) normalize() error {
	u.Code, u.NameTh, u.NameEn = strings.TrimSpace(u.Code), strings.TrimSpace(u.NameTh), strings.TrimSpace(u.NameEn)
	if !codeRE.MatchString(u.Code) {
		return fmt.Errorf("%w: code", ErrInvalid)
	}
	if u.NameTh == "" || len([]rune(u.NameTh)) > 300 || len([]rune(u.NameEn)) > 300 {
		return fmt.Errorf("%w: name", ErrInvalid)
	}
	if !unitTypes[u.UnitType] {
		return fmt.Errorf("%w: unit_type", ErrInvalid)
	}
	return nil
}

// ListOrgUnits returns the tree in path order (parents before children), optionally of one legal entity.
func (s *Service) ListOrgUnits(ctx context.Context, legalEntityID *uuid.UUID, includeClosed bool) ([]OrgUnit, error) {
	rows, err := orgstore.New(pdb.MustTxFromContext(ctx)).ListOrgUnits(ctx, orgstore.ListOrgUnitsParams{LegalEntityID: pgUUID(legalEntityID), IncludeClosed: includeClosed})
	if err != nil {
		return nil, err
	}
	out := make([]OrgUnit, 0, len(rows))
	for _, r := range rows {
		out = append(out, toOrgUnit(orgstore.GetOrgUnitRow(r)))
	}
	return out, nil
}

// CreateOrgUnit adds a unit under parent (nil = a root of its legal entity).
func (s *Service) CreateOrgUnit(ctx context.Context, u OrgUnit) (OrgUnit, error) {
	if err := u.normalize(); err != nil {
		return OrgUnit{}, err
	}
	if _, err := s.GetLegalEntity(ctx, u.LegalEntityID); err != nil {
		return OrgUnit{}, fmt.Errorf("%w: legal entity", ErrInvalid)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return OrgUnit{}, err
	}
	path := label(id)
	if u.ParentID != nil {
		parent, err := s.unit(ctx, *u.ParentID)
		if err != nil || parent.LegalEntityID != u.LegalEntityID || parent.Status != "active" {
			return OrgUnit{}, fmt.Errorf("%w: parent", ErrInvalid)
		}
		path = parent.Path + "." + path
	}
	var row orgstore.GetOrgUnitRow
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		r, err := orgstore.New(pdb.MustTxFromContext(ctx)).InsertOrgUnit(ctx, orgstore.InsertOrgUnitParams{ID: id, LegalEntityID: u.LegalEntityID,
			ParentID: pgUUID(u.ParentID), Path: path, Code: u.Code, NameTh: u.NameTh, NameEn: opt(u.NameEn), UnitType: u.UnitType})
		row = orgstore.GetOrgUnitRow(r)
		return err
	})
	if isUnique(err) {
		return OrgUnit{}, fmt.Errorf("%w: code is used by another unit of this legal entity", ErrInvalid)
	}
	if err != nil {
		return OrgUnit{}, err
	}
	out := toOrgUnit(row)
	return out, s.audit(ctx, "org.unit.create", out.ID, nil, unitAudit(out))
}

// UpdateOrgUnit renames or recodes a unit (the tree position changes only through MoveOrgUnit).
func (s *Service) UpdateOrgUnit(ctx context.Context, id uuid.UUID, version int32, u OrgUnit) (OrgUnit, error) {
	if err := u.normalize(); err != nil {
		return OrgUnit{}, err
	}
	before, err := s.unit(ctx, id)
	if err != nil {
		return OrgUnit{}, err
	}
	var row orgstore.GetOrgUnitRow
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		r, err := orgstore.New(pdb.MustTxFromContext(ctx)).UpdateOrgUnit(ctx, orgstore.UpdateOrgUnitParams{ID: id, RowVersion: version, Code: u.Code,
			NameTh: u.NameTh, NameEn: opt(u.NameEn), UnitType: u.UnitType})
		row = orgstore.GetOrgUnitRow(r)
		return err
	})
	switch {
	case isUnique(err):
		return OrgUnit{}, fmt.Errorf("%w: code is used by another unit of this legal entity", ErrInvalid)
	case errors.Is(err, pgx.ErrNoRows):
		return OrgUnit{}, ErrVersionMismatch
	case err != nil:
		return OrgUnit{}, err
	}
	out := toOrgUnit(row)
	return out, s.audit(ctx, "org.unit.update", id, unitAudit(before), unitAudit(out))
}

// MoveOrgUnit puts a unit (with everything below it) under another parent of the same legal entity, or at
// its root (nil). Paths change in the same statement, so scope checks on the tree (UnitWithin) follow at
// once (ORG-04 acceptance).
func (s *Service) MoveOrgUnit(ctx context.Context, id uuid.UUID, version int32, parentID *uuid.UUID) (OrgUnit, error) {
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.LockOrgUnit(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgUnit{}, ErrNotFound
	}
	if err != nil {
		return OrgUnit{}, err
	}
	u := toOrgUnit(orgstore.GetOrgUnitRow(cur))
	if u.RowVersion != version {
		return OrgUnit{}, ErrVersionMismatch
	}
	if u.Status != "active" {
		return OrgUnit{}, fmt.Errorf("%w: a closed unit can't move", ErrInvalid)
	}
	newPath := label(u.ID)
	if parentID != nil {
		parent, err := s.unit(ctx, *parentID)
		if err != nil || parent.LegalEntityID != u.LegalEntityID || parent.Status != "active" {
			return OrgUnit{}, fmt.Errorf("%w: parent", ErrInvalid)
		}
		if parent.ID == u.ID || strings.HasPrefix(parent.Path+".", u.Path+".") {
			return OrgUnit{}, ErrCycle
		}
		newPath = parent.Path + "." + newPath
	}
	if _, err := q.MoveOrgSubtree(ctx, orgstore.MoveOrgSubtreeParams{NewPath: newPath, OldPath: u.Path}); err != nil {
		return OrgUnit{}, err
	}
	if err := q.SetOrgUnitParent(ctx, orgstore.SetOrgUnitParentParams{ID: u.ID, ParentID: pgUUID(parentID)}); err != nil {
		return OrgUnit{}, err
	}
	out, err := s.unit(ctx, id)
	if err != nil {
		return OrgUnit{}, err
	}
	return out, s.audit(ctx, "org.unit.move", id, map[string]any{"parent_id": u.ParentID, "path": u.Path}, map[string]any{"parent_id": parentID, "path": out.Path})
}

// CloseOrgUnit closes a unit that has no active units below it; it stays in the tree for history.
func (s *Service) CloseOrgUnit(ctx context.Context, id uuid.UUID, version int32) (OrgUnit, error) {
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	if _, err := s.unit(ctx, id); err != nil {
		return OrgUnit{}, err
	}
	n, err := q.CountActiveChildren(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		return OrgUnit{}, err
	}
	if n > 0 {
		return OrgUnit{}, ErrHasActive
	}
	r, err := q.CloseOrgUnit(ctx, orgstore.CloseOrgUnitParams{ID: id, RowVersion: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgUnit{}, ErrVersionMismatch
	}
	if err != nil {
		return OrgUnit{}, err
	}
	out := toOrgUnit(orgstore.GetOrgUnitRow(r))
	return out, s.audit(ctx, "org.unit.close", id, map[string]any{"status": "active"}, map[string]any{"status": "closed"})
}

// UnitWithin reports whether unit is scope itself or, with descendants, anywhere below it on the current
// tree — what data-scope checks (org_unit scopes with include_descendants) ask. Exported for other modules.
func (s *Service) UnitWithin(ctx context.Context, unit, scope uuid.UUID, includeDescendants bool) (bool, error) {
	return orgstore.New(pdb.MustTxFromContext(ctx)).UnitWithin(ctx, orgstore.UnitWithinParams{UnitID: unit, ScopeID: scope, IncludeDescendants: includeDescendants})
}

func (s *Service) unit(ctx context.Context, id uuid.UUID) (OrgUnit, error) {
	r, err := orgstore.New(pdb.MustTxFromContext(ctx)).GetOrgUnit(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgUnit{}, ErrNotFound
	}
	if err != nil {
		return OrgUnit{}, err
	}
	return toOrgUnit(r), nil
}

// label is a unit's own ltree label: its id without dashes (stable across renames and moves).
func label(id uuid.UUID) string {
	return "u" + strings.ReplaceAll(id.String(), "-", "")
}

func toLegalEntity(r orgstore.GetLegalEntityRow) LegalEntity {
	e := LegalEntity{ID: r.ID, ParentID: uuidPtr(r.ParentID), NameTh: r.NameTh, NameEn: deref(r.NameEn), RegistrationNo: strings.TrimSpace(deref(r.RegistrationNo)),
		TaxID: strings.TrimSpace(deref(r.TaxID)), ContactEmail: deref(r.ContactEmail), ContactPhone: deref(r.ContactPhone), LogoFileID: uuidPtr(r.LogoFileID),
		IsController: r.IsController, IsProcessor: r.IsProcessor, Status: r.Status, RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}
	_ = json.Unmarshal(r.Address, &e.Address)
	return e
}

func toOrgUnit(r orgstore.GetOrgUnitRow) OrgUnit {
	u := OrgUnit{ID: r.ID, LegalEntityID: r.LegalEntityID, ParentID: uuidPtr(r.ParentID), Path: r.Path, Depth: r.Depth, Code: r.Code, NameTh: r.NameTh,
		NameEn: deref(r.NameEn), UnitType: r.UnitType, Status: r.Status, RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}
	if r.ClosedAt.Valid {
		t := r.ClosedAt.Time
		u.ClosedAt = &t
	}
	return u
}

func legalEntityAudit(e LegalEntity) map[string]any {
	return map[string]any{"name_th": e.NameTh, "name_en": e.NameEn, "registration_no": e.RegistrationNo, "tax_id": e.TaxID, "address": e.Address,
		"contact_email": e.ContactEmail, "contact_phone": e.ContactPhone, "logo_file_id": e.LogoFileID, "parent_id": e.ParentID,
		"is_controller": e.IsController, "is_processor": e.IsProcessor, "status": e.Status}
}

func unitAudit(u OrgUnit) map[string]any {
	return map[string]any{"code": u.Code, "name_th": u.NameTh, "name_en": u.NameEn, "unit_type": u.UnitType, "parent_id": u.ParentID, "legal_entity_id": u.LegalEntityID}
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
