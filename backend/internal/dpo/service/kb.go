package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dpostore "pdpa-platform/internal/dpo/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// DPO-12 (knowledge base): categorised, versioned, searchable legal documents and FAQs. Global starter
// articles (tenant_id NULL, migration 00102) are read-only for everyone; a tenant authors its own.
var (
	kbSources = map[string]bool{"law": true, "pdpc": true, "policy": true, "faq": true, "internal": true}
	kbLangs   = map[string]bool{"th": true, "en": true}
)

type KbInput struct {
	Category string
	Title    string
	Body     map[string]any
	Language string
	Tags     []string
	Source   string
}

type KbFilter struct {
	Category, Language, Source, Tag, Q string
	IncludeAll                         bool
}

func (in *KbInput) normalize() error {
	in.Category, in.Title = strings.TrimSpace(in.Category), strings.TrimSpace(in.Title)
	if in.Language == "" {
		in.Language = "th"
	}
	if in.Category == "" || len(in.Category) > 40 {
		return fmt.Errorf("%w: category", ErrInvalid)
	}
	if in.Title == "" {
		return fmt.Errorf("%w: title", ErrInvalid)
	}
	if !kbLangs[in.Language] {
		return fmt.Errorf("%w: language", ErrInvalid)
	}
	if !kbSources[in.Source] {
		return fmt.Errorf("%w: source", ErrInvalid)
	}
	if len(in.Body) == 0 {
		return fmt.Errorf("%w: body", ErrInvalid)
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	return nil
}

func kbHas(ctx context.Context, perm string) bool {
	g, ok := authz.FromContext(ctx)
	return ok && g.Has(perm)
}

func kbActor(ctx context.Context) *uuid.UUID {
	g, _ := authz.FromContext(ctx)
	if u, err := uuid.Parse(g.UserID); err == nil {
		return &u
	}
	return nil
}

// ListKbArticles: readers (dpo.kb.read) see published articles; IncludeAll (drafts/archived) needs
// dpo.kb.update. The search is a substring match on title, body and tags — Thai has no word boundaries.
func (s *Service) ListKbArticles(ctx context.Context, f KbFilter) ([]KbArticle, error) {
	if f.IncludeAll && !kbHas(ctx, "dpo.kb.update") {
		return nil, ErrForbidden
	}
	rows, err := dpostore.New(pdb.MustTxFromContext(ctx)).ListKbArticles(ctx, dpostore.ListKbArticlesParams{
		IncludeAll: f.IncludeAll, Category: optStrPtr(f.Category), Language: optStrPtr(f.Language),
		Source: optStrPtr(f.Source), Tag: optStrPtr(f.Tag), Q: optStrPtr(strings.TrimSpace(f.Q)),
	})
	if err != nil {
		return nil, err
	}
	out := make([]KbArticle, 0, len(rows))
	for _, r := range rows {
		a, err := toKbArticle(r)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// GetKbArticle reads one article; a non-published one is visible only to a manager.
func (s *Service) GetKbArticle(ctx context.Context, id uuid.UUID) (KbArticle, error) {
	row, err := dpostore.New(pdb.MustTxFromContext(ctx)).GetKbArticleAnyStatus(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return KbArticle{}, ErrNotFound
	}
	if err != nil {
		return KbArticle{}, err
	}
	if row.Status != "published" && !kbHas(ctx, "dpo.kb.update") {
		return KbArticle{}, ErrNotFound
	}
	return toKbArticle(row)
}

func (s *Service) CreateKbArticle(ctx context.Context, in KbInput) (KbArticle, error) {
	if !kbHas(ctx, "dpo.kb.create") {
		return KbArticle{}, ErrForbidden
	}
	if err := in.normalize(); err != nil {
		return KbArticle{}, err
	}
	body, err := json.Marshal(in.Body)
	if err != nil {
		return KbArticle{}, err
	}
	row, err := dpostore.New(pdb.MustTxFromContext(ctx)).InsertKbArticleDraft(ctx, dpostore.InsertKbArticleDraftParams{
		Category: in.Category, Title: in.Title, Body: body, Language: in.Language, Tags: in.Tags, Source: in.Source, Actor: pgUUID(kbActor(ctx)),
	})
	if err != nil {
		return KbArticle{}, err
	}
	out, err := toKbArticle(row)
	if err != nil {
		return KbArticle{}, err
	}
	return out, s.audit(ctx, "dpo.kb.create", out.ID, nil, map[string]any{"title": out.Title, "status": out.Status})
}

// ownEditable loads an article and refuses a global default (read-only for everyone).
func (s *Service) ownEditable(ctx context.Context, id uuid.UUID) (dpostore.DpoKbArticle, error) {
	row, err := dpostore.New(pdb.MustTxFromContext(ctx)).GetKbArticleAnyStatus(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	if !row.TenantID.Valid {
		return row, fmt.Errorf("%w: global article is read-only", ErrInvalid)
	}
	return row, nil
}

// UpdateKbArticle edits a draft or published article of the tenant's own; editing a published one starts
// its next version_no (the "เวอร์ชัน" of the module description).
func (s *Service) UpdateKbArticle(ctx context.Context, id uuid.UUID, in KbInput, version int32) (KbArticle, error) {
	if !kbHas(ctx, "dpo.kb.update") {
		return KbArticle{}, ErrForbidden
	}
	if err := in.normalize(); err != nil {
		return KbArticle{}, err
	}
	cur, err := s.ownEditable(ctx, id)
	if err != nil {
		return KbArticle{}, err
	}
	body, err := json.Marshal(in.Body)
	if err != nil {
		return KbArticle{}, err
	}
	row, err := dpostore.New(pdb.MustTxFromContext(ctx)).UpdateKbArticle(ctx, dpostore.UpdateKbArticleParams{
		ID: id, RowVersion: version, Category: in.Category, Title: in.Title, Body: body, Language: in.Language,
		Tags: in.Tags, Source: in.Source, Actor: pgUUID(kbActor(ctx)),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if cur.Status == "archived" {
			return KbArticle{}, fmt.Errorf("%w: archived article", ErrInvalid)
		}
		return KbArticle{}, ErrVersionMismatch
	}
	if err != nil {
		return KbArticle{}, err
	}
	out, err := toKbArticle(row)
	if err != nil {
		return KbArticle{}, err
	}
	return out, s.audit(ctx, "dpo.kb.update", id, map[string]any{"version_no": cur.VersionNo}, map[string]any{"version_no": row.VersionNo})
}

// SetKbArticleStatus publishes (draft/archived → published, dpo.kb.publish) or archives
// (draft/published → archived, dpo.kb.delete).
func (s *Service) SetKbArticleStatus(ctx context.Context, id uuid.UUID, to string, version int32) (KbArticle, error) {
	var from []string
	switch to {
	case "published":
		if !kbHas(ctx, "dpo.kb.publish") {
			return KbArticle{}, ErrForbidden
		}
		from = []string{"draft", "archived"}
	case "archived":
		if !kbHas(ctx, "dpo.kb.delete") {
			return KbArticle{}, ErrForbidden
		}
		from = []string{"draft", "published"}
	default:
		return KbArticle{}, fmt.Errorf("%w: status", ErrInvalid)
	}
	cur, err := s.ownEditable(ctx, id)
	if err != nil {
		return KbArticle{}, err
	}
	row, err := dpostore.New(pdb.MustTxFromContext(ctx)).SetKbArticleStatus(ctx, dpostore.SetKbArticleStatusParams{
		ID: id, RowVersion: version, Status: to, FromStatuses: from, Actor: pgUUID(kbActor(ctx)),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		for _, f := range from {
			if cur.Status == f {
				return KbArticle{}, ErrVersionMismatch
			}
		}
		return KbArticle{}, fmt.Errorf("%w: cannot move from %s to %s", ErrInvalid, cur.Status, to)
	}
	if err != nil {
		return KbArticle{}, err
	}
	out, err := toKbArticle(row)
	if err != nil {
		return KbArticle{}, err
	}
	return out, s.audit(ctx, "dpo.kb."+to, id, map[string]any{"status": cur.Status}, map[string]any{"status": to})
}
