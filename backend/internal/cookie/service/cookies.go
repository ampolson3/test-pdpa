package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	cookiestore "pdpa-platform/internal/cookie/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// CookieEntryEntityType is this module's own audit entity type for cookie.cookies.
const CookieEntryEntityType = "cookie_entry"

var storageTypes = []string{"http_cookie", "js_cookie", "local_storage", "session_storage", "pixel"}

// CookieEntry is one cookie catalogued under a domain's category — CON-02's own "รายการคุกกี้ต่อหมวด".
type CookieEntry struct {
	ID             uuid.UUID
	DomainID       uuid.UUID
	Name           string
	ProviderDomain string
	CategoryID     *uuid.UUID
	StorageType    string
	Duration       string
	DescriptionTh  string
	DescriptionEn  string
	IsActive       bool
	RowVersion     int32
}

func toCookieEntry(id, domainID uuid.UUID, name, providerDomain string, categoryID pgtype.UUID, storageType string, duration *string, descRaw []byte, isActive bool, rowVersion int32) (CookieEntry, error) {
	var d names
	if len(descRaw) > 0 {
		if err := json.Unmarshal(descRaw, &d); err != nil {
			return CookieEntry{}, err
		}
	}
	e := CookieEntry{ID: id, DomainID: domainID, Name: name, ProviderDomain: providerDomain, StorageType: storageType,
		DescriptionTh: d.Th, DescriptionEn: d.En, IsActive: isActive, RowVersion: rowVersion}
	if categoryID.Valid {
		cid := uuid.UUID(categoryID.Bytes)
		e.CategoryID = &cid
	}
	if duration != nil {
		e.Duration = *duration
	}
	return e, nil
}

// CreateCookieInput is what the DPO gives to catalogue a cookie under a domain.
type CreateCookieInput struct {
	Name           string
	ProviderDomain string
	CategoryID     *uuid.UUID
	StorageType    string
	Duration       string
	DescriptionTh  string
	DescriptionEn  string
}

// CreateCookie adds a cookie entry for a domain — the category, when given, must already be visible under
// the caller's own RLS (rule 1, via GetCategory).
func (s *Service) CreateCookie(ctx context.Context, domainID uuid.UUID, in CreateCookieInput) (CookieEntry, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.create") {
		return CookieEntry{}, ErrForbidden
	}
	if _, err := s.GetDomain(ctx, domainID); err != nil {
		return CookieEntry{}, err
	}
	name := strings.TrimSpace(in.Name)
	provider := strings.TrimSpace(in.ProviderDomain)
	if name == "" || provider == "" {
		return CookieEntry{}, fmt.Errorf("%w: name and provider_domain required", ErrInvalid)
	}
	if !contains(storageTypes, in.StorageType) {
		return CookieEntry{}, fmt.Errorf("%w: storage_type", ErrInvalid)
	}
	var categoryParam pgtype.UUID
	if in.CategoryID != nil {
		if _, err := s.GetCategory(ctx, *in.CategoryID); err != nil {
			return CookieEntry{}, fmt.Errorf("%w: category_id", ErrInvalid)
		}
		categoryParam = pgtype.UUID{Bytes: *in.CategoryID, Valid: true}
	}
	descJSON, err := json.Marshal(names{Th: in.DescriptionTh, En: in.DescriptionEn})
	if err != nil {
		return CookieEntry{}, err
	}
	var duration *string
	if d := strings.TrimSpace(in.Duration); d != "" {
		duration = &d
	}
	id, err := uuid.NewV7()
	if err != nil {
		return CookieEntry{}, err
	}
	row, err := cookiestore.New(pdb.MustTxFromContext(ctx)).InsertCookie(ctx, cookiestore.InsertCookieParams{
		ID: id, DomainID: domainID, Name: name, ProviderDomain: provider, CategoryID: categoryParam,
		StorageType: in.StorageType, Duration: duration, Descriptions: descJSON,
	})
	if err != nil {
		return CookieEntry{}, err
	}
	e, err := toCookieEntry(row.ID, row.DomainID, row.Name, row.ProviderDomain, row.CategoryID, row.StorageType, row.Duration, row.Descriptions, row.IsActive, row.RowVersion)
	if err != nil {
		return CookieEntry{}, err
	}
	if err := s.audit(ctx, "cookie.cookie.create", CookieEntryEntityType, id, nil, map[string]any{"domain_id": domainID, "name": name}); err != nil {
		return CookieEntry{}, err
	}
	return e, nil
}

// ListCookies returns every cookie catalogued for a domain, by name.
func (s *Service) ListCookies(ctx context.Context, domainID uuid.UUID) ([]CookieEntry, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.read") {
		return nil, ErrForbidden
	}
	if _, err := s.GetDomain(ctx, domainID); err != nil {
		return nil, err
	}
	rows, err := cookiestore.New(pdb.MustTxFromContext(ctx)).ListCookiesByDomain(ctx, domainID)
	if err != nil {
		return nil, err
	}
	out := make([]CookieEntry, 0, len(rows))
	for _, r := range rows {
		e, err := toCookieEntry(r.ID, r.DomainID, r.Name, r.ProviderDomain, r.CategoryID, r.StorageType, r.Duration, r.Descriptions, r.IsActive, r.RowVersion)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// DeleteCookie removes a cookie entry.
func (s *Service) DeleteCookie(ctx context.Context, id uuid.UUID) error {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.delete") {
		return ErrForbidden
	}
	n, err := cookiestore.New(pdb.MustTxFromContext(ctx)).DeleteCookie(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "cookie.cookie.delete", CookieEntryEntityType, id, nil, nil)
}
