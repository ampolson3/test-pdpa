package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	cookiestore "pdpa-platform/internal/cookie/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// CategoryEntityType is this module's own audit entity type for cookie.categories.
const CategoryEntityType = "cookie_category"

// Category is one cookie category a banner's settings panel offers a toggle for (e.g. necessary,
// analytics, marketing). No seeded defaults — ORG-20's own "no seeded holiday data" precedent: the DPO
// creates the ones this tenant actually uses.
type Category struct {
	ID           uuid.UUID
	Code         string
	NameTh       string
	NameEn       string
	DescTh       string
	DescEn       string
	IsRequired   bool
	DisplayOrder int16
	RowVersion   int32
}

type names struct {
	Th string `json:"th"`
	En string `json:"en,omitempty"`
}

func toCategory(id uuid.UUID, code string, namesRaw, descRaw []byte, isRequired bool, order int16, rowVersion int32) (Category, error) {
	var n, d names
	if err := json.Unmarshal(namesRaw, &n); err != nil {
		return Category{}, err
	}
	if len(descRaw) > 0 {
		if err := json.Unmarshal(descRaw, &d); err != nil {
			return Category{}, err
		}
	}
	return Category{ID: id, Code: code, NameTh: n.Th, NameEn: n.En, DescTh: d.Th, DescEn: d.En, IsRequired: isRequired, DisplayOrder: order, RowVersion: rowVersion}, nil
}

// CreateCategoryInput is what the DPO gives to add a category.
type CreateCategoryInput struct {
	Code         string
	NameTh       string
	NameEn       string
	DescTh       string
	DescEn       string
	IsRequired   bool
	DisplayOrder int16
}

// CreateCategory adds a cookie category for the tenant's own banners.
func (s *Service) CreateCategory(ctx context.Context, in CreateCategoryInput) (Category, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.create") {
		return Category{}, ErrForbidden
	}
	code := strings.ToLower(strings.TrimSpace(in.Code))
	if code == "" || strings.TrimSpace(in.NameTh) == "" {
		return Category{}, fmt.Errorf("%w: code and name_th required", ErrInvalid)
	}
	namesJSON, err := json.Marshal(names{Th: in.NameTh, En: in.NameEn})
	if err != nil {
		return Category{}, err
	}
	descJSON, err := json.Marshal(names{Th: in.DescTh, En: in.DescEn})
	if err != nil {
		return Category{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Category{}, err
	}
	var row cookiestore.InsertCategoryRow
	if err := pdb.Savepoint(ctx, func(ctx context.Context) error {
		var err error
		row, err = cookiestore.New(pdb.MustTxFromContext(ctx)).InsertCategory(ctx, cookiestore.InsertCategoryParams{
			ID: id, Code: code, Names: namesJSON, Descriptions: descJSON, IsRequired: in.IsRequired, DisplayOrder: in.DisplayOrder,
		})
		return err
	}); err != nil {
		return Category{}, fmt.Errorf("%w: code already in use", ErrInvalid)
	}
	cat, err := toCategory(row.ID, row.Code, row.Names, row.Descriptions, row.IsRequired, row.DisplayOrder, row.RowVersion)
	if err != nil {
		return Category{}, err
	}
	if err := s.audit(ctx, "cookie.category.create", CategoryEntityType, id, nil, map[string]any{"code": code}); err != nil {
		return Category{}, err
	}
	return cat, nil
}

// ListCategories returns every category the tenant has defined, in display order.
func (s *Service) ListCategories(ctx context.Context) ([]Category, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.read") {
		return nil, ErrForbidden
	}
	rows, err := cookiestore.New(pdb.MustTxFromContext(ctx)).ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Category, 0, len(rows))
	for _, r := range rows {
		cat, err := toCategory(r.ID, r.Code, r.Names, r.Descriptions, r.IsRequired, r.DisplayOrder, r.RowVersion)
		if err != nil {
			return nil, err
		}
		out = append(out, cat)
	}
	return out, nil
}

// DeleteCategory removes a category — this pass has no script_rules or cookies yet pointing at one, so
// there's no in-use check to make (unlike ORG-06's own external-party delete, which does need one).
func (s *Service) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.delete") {
		return ErrForbidden
	}
	n, err := cookiestore.New(pdb.MustTxFromContext(ctx)).DeleteCategory(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "cookie.category.delete", CategoryEntityType, id, nil, nil)
}
