package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	orgstore "pdpa-platform/internal/org/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// ORG-07 master data. Every tenant sees the platform's default rows (tenant_id NULL; seeded as a draft
// pending legal review, decisions.md Q-20) plus its own; defaults are read-only for tenants, lawful bases and
// countries entirely. Modules refer to rows by id, so a change is seen everywhere at once.

const (
	KindDataCategories   = "data_categories"
	KindSubjectTypes     = "data_subject_types"
	KindPurposes         = "processing_purposes"
	KindLawfulBases      = "lawful_bases"
	KindCountries        = "countries"
	MasterDataEntityType = "master_data"
)

var (
	ErrReadOnly = errors.New("org: this master data is read-only for the tenant")
	ErrInUse    = errors.New("org: this entry is in use")
	masterCode  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
)

// MasterItem is one entry of any master-data kind; fields that don't apply to a kind stay zero.
type MasterItem struct {
	ID         *uuid.UUID // nil for lawful bases and countries (keyed by code)
	Code       string
	NameTh     string
	NameEn     string
	Global     bool // a platform default
	RowVersion int32
	// data categories
	IsSensitive   bool
	SensitiveType string
	ParentID      *uuid.UUID
	// data subject types
	IsVulnerable bool
	// processing purposes
	Category string
	// lawful bases
	SectionRef      string
	ForSensitive    bool
	RequiresConsent bool
	RequiresLIA     bool
	// countries
	AdequacyStatus string
	Region         string
}

func editableKind(kind string) bool {
	return kind == KindDataCategories || kind == KindSubjectTypes || kind == KindPurposes
}

// ListMaster returns a kind's entries: defaults first, then the tenant's.
func (s *Service) ListMaster(ctx context.Context, kind string) ([]MasterItem, error) {
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	var out []MasterItem
	switch kind {
	case KindDataCategories:
		rows, err := q.ListDataCategories(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, fromCategory(orgstore.GetDataCategoryRow(r)))
		}
	case KindSubjectTypes:
		rows, err := q.ListDataSubjectTypes(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, fromSubject(orgstore.GetDataSubjectTypeRow(r)))
		}
	case KindPurposes:
		rows, err := q.ListProcessingPurposes(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, fromPurpose(orgstore.GetProcessingPurposeRow(r)))
		}
	case KindLawfulBases:
		rows, err := q.ListLawfulBases(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, MasterItem{Code: r.Code, NameTh: r.NameTh, NameEn: deref(r.NameEn), Global: true, SectionRef: r.SectionRef,
				ForSensitive: r.ForSensitive, RequiresConsent: r.RequiresConsent, RequiresLIA: r.RequiresLia})
		}
	case KindCountries:
		rows, err := q.ListCountries(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, MasterItem{Code: strings.TrimSpace(r.Code), NameTh: r.NameTh, NameEn: r.NameEn, Global: true, AdequacyStatus: r.AdequacyStatus, Region: deref(r.Region)})
		}
	default:
		return nil, ErrNotFound
	}
	return out, nil
}

func (it *MasterItem) normalize(kind string, create bool) error {
	it.NameTh, it.NameEn, it.Code = strings.TrimSpace(it.NameTh), strings.TrimSpace(it.NameEn), strings.TrimSpace(it.Code)
	if create && !masterCode.MatchString(it.Code) {
		return fmt.Errorf("%w: code", ErrInvalid)
	}
	if it.NameTh == "" || len([]rune(it.NameTh)) > 200 || len([]rune(it.NameEn)) > 200 {
		return fmt.Errorf("%w: name", ErrInvalid)
	}
	switch kind {
	case KindDataCategories:
		it.SensitiveType = strings.TrimSpace(it.SensitiveType)
		if !it.IsSensitive {
			it.SensitiveType = ""
		}
		if len(it.SensitiveType) > 40 {
			return fmt.Errorf("%w: sensitive_type", ErrInvalid)
		}
	case KindPurposes:
		it.Category = strings.TrimSpace(it.Category)
		if len(it.Category) > 40 {
			return fmt.Errorf("%w: category", ErrInvalid)
		}
	}
	return nil
}

// CreateMaster adds a tenant entry. Its code may not repeat a default's (defaults can't be shadowed).
func (s *Service) CreateMaster(ctx context.Context, kind string, it MasterItem) (MasterItem, error) {
	if !editableKind(kind) {
		if kind == KindLawfulBases || kind == KindCountries {
			return MasterItem{}, ErrReadOnly
		}
		return MasterItem{}, ErrNotFound
	}
	if err := it.normalize(kind, true); err != nil {
		return MasterItem{}, err
	}
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	if taken, err := q.GlobalCodeExists(ctx, orgstore.GlobalCodeExistsParams{Kind: kind, Code: it.Code}); err != nil {
		return MasterItem{}, err
	} else if taken {
		return MasterItem{}, fmt.Errorf("%w: code is a default entry's", ErrInvalid)
	}
	if err := s.checkParent(ctx, kind, it.ParentID, nil); err != nil {
		return MasterItem{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return MasterItem{}, err
	}
	var out MasterItem
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		q := orgstore.New(pdb.MustTxFromContext(ctx))
		switch kind {
		case KindDataCategories:
			r, err := q.InsertDataCategory(ctx, orgstore.InsertDataCategoryParams{ID: id, Code: it.Code, NameTh: it.NameTh, NameEn: opt(it.NameEn),
				IsSensitive: it.IsSensitive, SensitiveType: opt(it.SensitiveType), ParentID: pgUUID(it.ParentID)})
			out = fromCategory(orgstore.GetDataCategoryRow(r))
			return err
		case KindSubjectTypes:
			r, err := q.InsertDataSubjectType(ctx, orgstore.InsertDataSubjectTypeParams{ID: id, Code: it.Code, NameTh: it.NameTh, NameEn: opt(it.NameEn), IsVulnerable: it.IsVulnerable})
			out = fromSubject(orgstore.GetDataSubjectTypeRow(r))
			return err
		default:
			r, err := q.InsertProcessingPurpose(ctx, orgstore.InsertProcessingPurposeParams{ID: id, Code: it.Code, NameTh: it.NameTh, NameEn: opt(it.NameEn), Category: opt(it.Category)})
			out = fromPurpose(orgstore.GetProcessingPurposeRow(r))
			return err
		}
	})
	if isUnique(err) {
		return MasterItem{}, fmt.Errorf("%w: code exists", ErrInvalid)
	}
	if err != nil {
		return MasterItem{}, err
	}
	return out, s.audit(ctx, "org.master_data.create", id, nil, masterAudit(kind, out))
}

// UpdateMaster changes a tenant entry (If-Match version); the code identifies it and doesn't change.
func (s *Service) UpdateMaster(ctx context.Context, kind string, id uuid.UUID, version int32, it MasterItem) (MasterItem, error) {
	before, err := s.masterItem(ctx, kind, id)
	if err != nil {
		return MasterItem{}, err
	}
	if before.Global {
		return MasterItem{}, ErrReadOnly
	}
	if err := it.normalize(kind, false); err != nil {
		return MasterItem{}, err
	}
	if err := s.checkParent(ctx, kind, it.ParentID, &id); err != nil {
		return MasterItem{}, err
	}
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	var out MasterItem
	switch kind {
	case KindDataCategories:
		var r orgstore.UpdateDataCategoryRow
		r, err = q.UpdateDataCategory(ctx, orgstore.UpdateDataCategoryParams{ID: id, RowVersion: version, NameTh: it.NameTh, NameEn: opt(it.NameEn),
			IsSensitive: it.IsSensitive, SensitiveType: opt(it.SensitiveType), ParentID: pgUUID(it.ParentID)})
		out = fromCategory(orgstore.GetDataCategoryRow(r))
	case KindSubjectTypes:
		var r orgstore.UpdateDataSubjectTypeRow
		r, err = q.UpdateDataSubjectType(ctx, orgstore.UpdateDataSubjectTypeParams{ID: id, RowVersion: version, NameTh: it.NameTh, NameEn: opt(it.NameEn), IsVulnerable: it.IsVulnerable})
		out = fromSubject(orgstore.GetDataSubjectTypeRow(r))
	default:
		var r orgstore.UpdateProcessingPurposeRow
		r, err = q.UpdateProcessingPurpose(ctx, orgstore.UpdateProcessingPurposeParams{ID: id, RowVersion: version, NameTh: it.NameTh, NameEn: opt(it.NameEn), Category: opt(it.Category)})
		out = fromPurpose(orgstore.GetProcessingPurposeRow(r))
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return MasterItem{}, ErrVersionMismatch
	}
	if err != nil {
		return MasterItem{}, err
	}
	return out, s.audit(ctx, "org.master_data.update", id, masterAudit(kind, before), masterAudit(kind, out))
}

// DeleteMaster removes a tenant entry nothing refers to.
func (s *Service) DeleteMaster(ctx context.Context, kind string, id uuid.UUID, version int32) error {
	before, err := s.masterItem(ctx, kind, id)
	if err != nil {
		return err
	}
	if before.Global {
		return ErrReadOnly
	}
	var n int64
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		q := orgstore.New(pdb.MustTxFromContext(ctx))
		var err error
		switch kind {
		case KindDataCategories:
			n, err = q.DeleteDataCategory(ctx, orgstore.DeleteDataCategoryParams{ID: id, RowVersion: version})
		case KindSubjectTypes:
			n, err = q.DeleteDataSubjectType(ctx, orgstore.DeleteDataSubjectTypeParams{ID: id, RowVersion: version})
		default:
			n, err = q.DeleteProcessingPurpose(ctx, orgstore.DeleteProcessingPurposeParams{ID: id, RowVersion: version})
		}
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrInUse
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrVersionMismatch
	}
	return s.audit(ctx, "org.master_data.delete", id, masterAudit(kind, before), nil)
}

func (s *Service) masterItem(ctx context.Context, kind string, id uuid.UUID) (MasterItem, error) {
	if !editableKind(kind) {
		if kind == KindLawfulBases || kind == KindCountries {
			return MasterItem{}, ErrReadOnly
		}
		return MasterItem{}, ErrNotFound
	}
	q := orgstore.New(pdb.MustTxFromContext(ctx))
	var it MasterItem
	var err error
	switch kind {
	case KindDataCategories:
		var r orgstore.GetDataCategoryRow
		r, err = q.GetDataCategory(ctx, id)
		it = fromCategory(r)
	case KindSubjectTypes:
		var r orgstore.GetDataSubjectTypeRow
		r, err = q.GetDataSubjectType(ctx, id)
		it = fromSubject(r)
	default:
		var r orgstore.GetProcessingPurposeRow
		r, err = q.GetProcessingPurpose(ctx, id)
		it = fromPurpose(r)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return MasterItem{}, ErrNotFound
	}
	return it, err
}

// checkParent: a data category's parent must be a visible category (FKs bypass RLS) and not itself.
func (s *Service) checkParent(ctx context.Context, kind string, parent, self *uuid.UUID) error {
	if parent == nil {
		return nil
	}
	if kind != KindDataCategories || (self != nil && *parent == *self) {
		return fmt.Errorf("%w: parent", ErrInvalid)
	}
	if _, err := s.masterItem(ctx, kind, *parent); err != nil {
		return fmt.Errorf("%w: parent", ErrInvalid)
	}
	return nil
}

func fromCategory(r orgstore.GetDataCategoryRow) MasterItem {
	id := r.ID
	return MasterItem{ID: &id, Code: r.Code, NameTh: r.NameTh, NameEn: deref(r.NameEn), Global: !r.TenantID.Valid, RowVersion: r.RowVersion,
		IsSensitive: r.IsSensitive, SensitiveType: deref(r.SensitiveType), ParentID: uuidPtr(r.ParentID)}
}

func fromSubject(r orgstore.GetDataSubjectTypeRow) MasterItem {
	id := r.ID
	return MasterItem{ID: &id, Code: r.Code, NameTh: r.NameTh, NameEn: deref(r.NameEn), Global: !r.TenantID.Valid, RowVersion: r.RowVersion, IsVulnerable: r.IsVulnerable}
}

func fromPurpose(r orgstore.GetProcessingPurposeRow) MasterItem {
	id := r.ID
	return MasterItem{ID: &id, Code: r.Code, NameTh: r.NameTh, NameEn: deref(r.NameEn), Global: !r.TenantID.Valid, RowVersion: r.RowVersion, Category: deref(r.Category)}
}

func masterAudit(kind string, it MasterItem) map[string]any {
	m := map[string]any{"kind": kind, "code": it.Code, "name_th": it.NameTh, "name_en": it.NameEn}
	switch kind {
	case KindDataCategories:
		m["is_sensitive"], m["sensitive_type"], m["parent_id"] = it.IsSensitive, it.SensitiveType, it.ParentID
	case KindSubjectTypes:
		m["is_vulnerable"] = it.IsVulnerable
	case KindPurposes:
		m["category"] = it.Category
	}
	return m
}
