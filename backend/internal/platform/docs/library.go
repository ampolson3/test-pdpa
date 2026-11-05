package docs

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/docs/render"
	docsstore "pdpa-platform/internal/platform/docs/store"
)

// Library entity types for the audit log.
const (
	ClauseEntityType   = "document_clause"
	TemplateEntityType = "document_template"
)

var codeRE = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,79}$`)

// ClauseBody is a clause's text in one language: its title and a ProseMirror doc (paragraphs, lists, merge
// fields — but no clauses).
type ClauseBody struct {
	Title string      `json:"title"`
	Doc   render.Node `json:"doc"`
}

// Clause is one version of a library clause. Versions of a code move draft → published → retired: publishing a
// new version retires the previous one, so new documents pick the latest (DSA-06) while documents that cite an
// older version keep rendering it.
type Clause struct {
	ID          uuid.UUID
	Global      bool // a platform clause, read-only for tenants
	Code        string
	Category    string
	Body        map[string]ClauseBody // th (required), en
	LegalRef    string
	AppliesTo   []string
	IsMandatory bool
	VersionNo   int32
	Status      string
	UpdatedAt   time.Time
	RowVersion  int32
}

// ClauseInput is a clause's editable content.
type ClauseInput struct {
	Code        string
	Category    string
	Body        map[string]ClauseBody
	LegalRef    string
	AppliesTo   []string
	IsMandatory bool
}

// ClauseFilter narrows ListClauses.
type ClauseFilter struct {
	Category      string
	AppliesTo     string
	PublishedOnly bool
}

// canUseClauses: the library's readers, and every editor of documents (they insert clauses).
func (s *Service) canUseClauses(ctx context.Context) bool {
	if can(ctx, ClauseRead) {
		return true
	}
	for _, a := range s.Access(ctx) {
		if a.Update {
			return true
		}
	}
	return false
}

// ListClauses returns the latest version of each clause code (platform and tenant). Document editors without
// agreement.clause.read see published clauses only.
func (s *Service) ListClauses(ctx context.Context, f ClauseFilter) ([]Clause, error) {
	if !s.canUseClauses(ctx) {
		return nil, ErrForbidden
	}
	p := docsstore.ListLatestClausesParams{PublishedOnly: f.PublishedOnly || !can(ctx, ClauseRead)}
	if f.Category != "" {
		p.Category = &f.Category
	}
	if f.AppliesTo != "" {
		p.AppliesTo = &f.AppliesTo
	}
	rows, err := docsstore.New(pdb.MustTxFromContext(ctx)).ListLatestClauses(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]Clause, 0, len(rows))
	for _, r := range rows {
		c, err := toClause(docsstore.GetClauseRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Clause) int { return strings.Compare(a.Category+"\x00"+a.Code, b.Category+"\x00"+b.Code) })
	return out, nil
}

// GetClause returns one clause version and every version of its code, newest first.
func (s *Service) GetClause(ctx context.Context, id uuid.UUID) (Clause, []Clause, error) {
	if !s.canUseClauses(ctx) {
		return Clause{}, nil, ErrNotFound
	}
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetClause(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Clause{}, nil, ErrNotFound
	}
	if err != nil {
		return Clause{}, nil, err
	}
	c, err := toClause(r)
	if err != nil {
		return Clause{}, nil, err
	}
	if c.Status == "draft" && !can(ctx, ClauseRead) {
		return Clause{}, nil, ErrNotFound
	}
	rows, err := q.ClauseVersions(ctx, docsstore.ClauseVersionsParams{Code: r.Code, TenantID: r.TenantID})
	if err != nil {
		return Clause{}, nil, err
	}
	versions := []Clause{}
	for _, x := range rows {
		v, err := toClause(docsstore.GetClauseRow(x))
		if err != nil {
			return Clause{}, nil, err
		}
		if v.Status != "draft" || can(ctx, ClauseRead) {
			versions = append(versions, v)
		}
	}
	return c, versions, nil
}

// CreateClause adds a new clause code to the tenant's library, as a draft (version 1).
func (s *Service) CreateClause(ctx context.Context, in ClauseInput) (Clause, error) {
	if !can(ctx, ClauseCreate) {
		return Clause{}, ErrForbidden
	}
	if err := s.checkClause(&in); err != nil {
		return Clause{}, err
	}
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	global, err := q.ClauseVersions(ctx, docsstore.ClauseVersionsParams{Code: in.Code})
	if err != nil {
		return Clause{}, err
	}
	if len(global) > 0 {
		return Clause{}, ErrCodeTaken
	}
	if _, err := q.LatestTenantClause(ctx, in.Code); err == nil {
		return Clause{}, ErrCodeTaken
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Clause{}, err
	}
	id, err := s.insertClause(ctx, in, 1)
	if err != nil {
		return Clause{}, err
	}
	if err := s.audit(ctx, "platform.clause.create", ClauseEntityType, id, nil, map[string]any{"code": in.Code, "version": 1}); err != nil {
		return Clause{}, err
	}
	c, _, err := s.GetClause(ctx, id)
	return c, err
}

// UpdateClause edits the draft of a clause, or — given the latest published version — starts the next version as a
// draft with the new content. rowVersion is that of the version given.
func (s *Service) UpdateClause(ctx context.Context, id uuid.UUID, rowVersion int32, in ClauseInput) (Clause, error) {
	if !can(ctx, ClauseUpdate) {
		return Clause{}, ErrForbidden
	}
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.GetClause(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Clause{}, ErrNotFound
	}
	if err != nil {
		return Clause{}, err
	}
	if !cur.TenantID.Valid {
		return Clause{}, ErrForbidden // platform clauses are read-only
	}
	latest, err := q.LatestTenantClause(ctx, cur.Code) // locks the newest version of the code
	if err != nil {
		return Clause{}, err
	}
	if latest.ID != id {
		return Clause{}, ErrInvalidState // only the newest version can be edited or superseded
	}
	if latest.RowVersion != rowVersion {
		return Clause{}, ErrVersionMismatch
	}
	in.Code = cur.Code
	if err := s.checkClause(&in); err != nil {
		return Clause{}, err
	}
	target := id
	switch latest.Status {
	case "draft":
		th, en := bodies(in)
		if err := q.UpdateClauseDraft(ctx, docsstore.UpdateClauseDraftParams{ID: id, Category: in.Category, Title: in.Body["th"].Title,
			BodyTh: th, BodyEn: en, LegalRef: optional(in.LegalRef), AppliesTo: in.AppliesTo, IsMandatory: in.IsMandatory}); err != nil {
			return Clause{}, err
		}
	case "published":
		if target, err = s.insertClause(ctx, in, latest.VersionNo+1); err != nil {
			return Clause{}, err
		}
	default:
		return Clause{}, ErrInvalidState
	}
	if err := s.audit(ctx, "platform.clause.update", ClauseEntityType, target, nil, map[string]any{"code": in.Code}); err != nil {
		return Clause{}, err
	}
	c, _, err := s.GetClause(ctx, target)
	return c, err
}

// PublishClause releases a draft clause version; the previously published version of the code is retired.
func (s *Service) PublishClause(ctx context.Context, id uuid.UUID, rowVersion int32) (Clause, error) {
	return s.clauseStatus(ctx, id, rowVersion, "draft", "published")
}

// RetireClause withdraws a published clause from new documents.
func (s *Service) RetireClause(ctx context.Context, id uuid.UUID, rowVersion int32) (Clause, error) {
	return s.clauseStatus(ctx, id, rowVersion, "published", "retired")
}

func (s *Service) clauseStatus(ctx context.Context, id uuid.UUID, rowVersion int32, from, to string) (Clause, error) {
	if !can(ctx, ClausePublish) {
		return Clause{}, ErrForbidden
	}
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.GetClause(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Clause{}, ErrNotFound
	}
	if err != nil {
		return Clause{}, err
	}
	if !cur.TenantID.Valid {
		return Clause{}, ErrForbidden
	}
	latest, err := q.LatestTenantClause(ctx, cur.Code)
	if err != nil {
		return Clause{}, err
	}
	if latest.ID != id || latest.Status != from {
		return Clause{}, ErrInvalidState
	}
	if latest.RowVersion != rowVersion {
		return Clause{}, ErrVersionMismatch
	}
	if err := q.SetClauseStatus(ctx, docsstore.SetClauseStatusParams{ID: id, Status: to}); err != nil {
		return Clause{}, err
	}
	if to == "published" {
		if err := q.RetireOtherClauseVersions(ctx, docsstore.RetireOtherClauseVersionsParams{Code: cur.Code, ID: id}); err != nil {
			return Clause{}, err
		}
	}
	if err := s.audit(ctx, "platform.clause."+map[string]string{"published": "publish", "retired": "retire"}[to], ClauseEntityType, id,
		map[string]any{"status": from}, map[string]any{"status": to, "code": cur.Code, "version": cur.VersionNo}); err != nil {
		return Clause{}, err
	}
	c, _, err := s.GetClause(ctx, id)
	return c, err
}

func (s *Service) insertClause(ctx context.Context, in ClauseInput, no int32) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	th, en := bodies(in)
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		_, err := docsstore.New(pdb.MustTxFromContext(ctx)).InsertClause(ctx, docsstore.InsertClauseParams{ID: id, Code: in.Code, Category: in.Category,
			Title: in.Body["th"].Title, BodyTh: th, BodyEn: en, LegalRef: optional(in.LegalRef), AppliesTo: in.AppliesTo, IsMandatory: in.IsMandatory, VersionNo: no})
		return err
	})
	if isUnique(err) {
		return uuid.Nil, ErrCodeTaken // a concurrent request created the code or its draft first
	}
	return id, err
}

func (s *Service) checkClause(in *ClauseInput) error {
	in.Code, in.Category, in.LegalRef = strings.TrimSpace(in.Code), strings.TrimSpace(in.Category), strings.TrimSpace(in.LegalRef)
	if !codeRE.MatchString(in.Code) {
		return invalid("code")
	}
	if in.Category == "" || utf8.RuneCountInString(in.Category) > 60 {
		return invalid("category")
	}
	if utf8.RuneCountInString(in.LegalRef) > 500 {
		return invalid("legal_ref")
	}
	if _, ok := in.Body["th"]; !ok {
		return invalid("body.th is required")
	}
	for lang, b := range in.Body {
		if lang != "th" && lang != "en" {
			return invalid("language %s", lang)
		}
		b.Title = strings.TrimSpace(b.Title)
		if b.Title == "" || utf8.RuneCountInString(b.Title) > maxTitle {
			return invalid("body.%s.title", lang)
		}
		if err := (render.Content{"th": b.Doc}).Validate(); err != nil {
			return invalid("body.%s: %v", lang, err)
		}
		if len(render.ClauseRefs(b.Doc)) > 0 {
			return invalid("body.%s: a clause can't contain clauses", lang)
		}
		for _, k := range render.FieldKeys(b.Doc) {
			if !knownField(k) {
				return invalid("unknown merge field %s", k)
			}
		}
		in.Body[lang] = b
	}
	if len(in.AppliesTo) == 0 {
		in.AppliesTo = []string{"dpa", "dsa"}
	}
	slices.Sort(in.AppliesTo)
	in.AppliesTo = slices.Compact(in.AppliesTo)
	for _, t := range in.AppliesTo {
		if !docTypes[t] {
			return invalid("applies_to %s", t)
		}
	}
	return nil
}

// docTypes are the values of platform.documents.doc_type.
var docTypes = map[string]bool{"notice": true, "policy": true, "dpa": true, "dsa": true, "dsar_letter": true, "pdpc_form": true,
	"breach_letter": true, "report": true, "other": true}

func bodies(in ClauseInput) ([]byte, []byte) {
	th, _ := json.Marshal(in.Body["th"])
	var en []byte
	if b, ok := in.Body["en"]; ok {
		en, _ = json.Marshal(b)
	}
	return th, en
}

func toClause(r docsstore.GetClauseRow) (Clause, error) {
	c := Clause{ID: r.ID, Global: !r.TenantID.Valid, Code: r.Code, Category: r.Category, Body: map[string]ClauseBody{}, AppliesTo: r.AppliesTo,
		IsMandatory: r.IsMandatory, VersionNo: r.VersionNo, Status: r.Status, UpdatedAt: r.UpdatedAt.Time, RowVersion: r.RowVersion}
	var th ClauseBody
	if err := json.Unmarshal(r.BodyTh, &th); err != nil {
		return Clause{}, err
	}
	c.Body["th"] = th
	if len(r.BodyEn) > 0 {
		var en ClauseBody
		if err := json.Unmarshal(r.BodyEn, &en); err != nil {
			return Clause{}, err
		}
		c.Body["en"] = en
	}
	if r.LegalRef != nil {
		c.LegalRef = *r.LegalRef
	}
	return c, nil
}

// Template is one version of a document template: the starting content of new documents of its type. Like clauses,
// versions move draft → published → retired, and only published templates are offered.
type Template struct {
	ID         uuid.UUID
	Global     bool
	DocType    string
	Code       string
	Name       string
	Content    render.Content
	VersionNo  int32
	Status     string
	UpdatedAt  time.Time
	RowVersion int32
}

// TemplateInput is a template's editable content.
type TemplateInput struct {
	DocType string
	Code    string
	Name    string
	Content render.Content
}

// templateTypes are the document types whose templates the caller can see: template readers see every version,
// document creators the published ones.
func (s *Service) templateTypes(ctx context.Context, docType string) (all, published []string) {
	for _, a := range s.Access(ctx) {
		if docType != "" && a.DocType != docType {
			continue
		}
		if a.TemplateRead || a.TemplateWrite {
			all = append(all, a.DocType)
		} else if a.Create {
			published = append(published, a.DocType)
		}
	}
	return
}

// ListTemplates returns the latest version of each template of the types the caller can see.
func (s *Service) ListTemplates(ctx context.Context, docType string, publishedOnly bool) ([]Template, error) {
	all, pub := s.templateTypes(ctx, docType)
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	out := []Template{}
	for _, set := range []struct {
		types []string
		only  bool
	}{{all, publishedOnly}, {pub, true}} {
		if len(set.types) == 0 {
			continue
		}
		rows, err := q.ListLatestTemplates(ctx, docsstore.ListLatestTemplatesParams{Types: set.types, PublishedOnly: set.only})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			t, err := toTemplate(docsstore.GetTemplateRow(r))
			if err != nil {
				return nil, err
			}
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b Template) int { return strings.Compare(a.DocType+"\x00"+a.Name, b.DocType+"\x00"+b.Name) })
	return out, nil
}

// GetTemplate returns one template version the caller may see.
func (s *Service) GetTemplate(ctx context.Context, id uuid.UUID) (Template, error) {
	r, err := docsstore.New(pdb.MustTxFromContext(ctx)).GetTemplate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	all, pub := s.templateTypes(ctx, r.TemplateType)
	if r.Language != "mul" || !(len(all) > 0 || (len(pub) > 0 && r.Status == "published")) {
		return Template{}, ErrNotFound
	}
	return toTemplate(r)
}

// CreateTemplate adds a template code for a document type, as a draft.
func (s *Service) CreateTemplate(ctx context.Context, in TemplateInput) (Template, error) {
	if _, err := s.need(ctx, in.DocType, func(p Policy) string { return p.TemplateWrite }); err != nil {
		return Template{}, err
	}
	if err := s.checkTemplate(ctx, &in); err != nil {
		return Template{}, err
	}
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	if _, err := q.LatestTenantTemplate(ctx, docsstore.LatestTenantTemplateParams{TemplateType: in.DocType, Code: in.Code}); err == nil {
		return Template{}, ErrCodeTaken
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Template{}, err
	}
	id, err := s.insertTemplate(ctx, in, 1)
	if err != nil {
		return Template{}, err
	}
	if err := s.audit(ctx, "platform.document_template.create", TemplateEntityType, id, nil, map[string]any{"doc_type": in.DocType, "code": in.Code}); err != nil {
		return Template{}, err
	}
	return s.GetTemplate(ctx, id)
}

// UpdateTemplate edits a template's draft, or starts the next version after the published one.
func (s *Service) UpdateTemplate(ctx context.Context, id uuid.UUID, rowVersion int32, in TemplateInput) (Template, error) {
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.GetTemplate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	if _, err := s.need(ctx, cur.TemplateType, func(p Policy) string { return p.TemplateWrite }); err != nil {
		return Template{}, err
	}
	if !cur.TenantID.Valid {
		return Template{}, ErrForbidden
	}
	latest, err := q.LatestTenantTemplate(ctx, docsstore.LatestTenantTemplateParams{TemplateType: cur.TemplateType, Code: cur.Code})
	if err != nil {
		return Template{}, err
	}
	if latest.ID != id {
		return Template{}, ErrInvalidState
	}
	if latest.RowVersion != rowVersion {
		return Template{}, ErrVersionMismatch
	}
	in.DocType, in.Code = cur.TemplateType, cur.Code
	if err := s.checkTemplate(ctx, &in); err != nil {
		return Template{}, err
	}
	target := id
	switch latest.Status {
	case "draft":
		body, _ := json.Marshal(in.Content)
		if err := q.UpdateTemplateDraft(ctx, docsstore.UpdateTemplateDraftParams{ID: id, Name: in.Name, Content: body}); err != nil {
			return Template{}, err
		}
	case "published":
		if target, err = s.insertTemplate(ctx, in, latest.VersionNo+1); err != nil {
			return Template{}, err
		}
	default:
		return Template{}, ErrInvalidState
	}
	if err := s.audit(ctx, "platform.document_template.update", TemplateEntityType, target, nil, map[string]any{"code": in.Code}); err != nil {
		return Template{}, err
	}
	return s.GetTemplate(ctx, target)
}

// PublishTemplate releases a draft template (needs the type's publish permission: its wording is legal text); the
// previous published version is retired.
func (s *Service) PublishTemplate(ctx context.Context, id uuid.UUID, rowVersion int32) (Template, error) {
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.GetTemplate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	if _, err := s.need(ctx, cur.TemplateType, func(p Policy) string { return p.Publish }); err != nil {
		return Template{}, err
	}
	if !cur.TenantID.Valid {
		return Template{}, ErrForbidden
	}
	latest, err := q.LatestTenantTemplate(ctx, docsstore.LatestTenantTemplateParams{TemplateType: cur.TemplateType, Code: cur.Code})
	if err != nil {
		return Template{}, err
	}
	if latest.ID != id || latest.Status != "draft" {
		return Template{}, ErrInvalidState
	}
	if latest.RowVersion != rowVersion {
		return Template{}, ErrVersionMismatch
	}
	if err := q.SetTemplateStatus(ctx, docsstore.SetTemplateStatusParams{ID: id, Status: "published"}); err != nil {
		return Template{}, err
	}
	if err := q.RetireOtherTemplateVersions(ctx, docsstore.RetireOtherTemplateVersionsParams{TemplateType: cur.TemplateType, Code: cur.Code, ID: id}); err != nil {
		return Template{}, err
	}
	if err := s.audit(ctx, "platform.document_template.publish", TemplateEntityType, id, map[string]any{"status": "draft"},
		map[string]any{"status": "published", "code": cur.Code, "version": cur.VersionNo}); err != nil {
		return Template{}, err
	}
	return s.GetTemplate(ctx, id)
}

func (s *Service) insertTemplate(ctx context.Context, in TemplateInput, no int32) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	body, _ := json.Marshal(in.Content)
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		_, err := docsstore.New(pdb.MustTxFromContext(ctx)).InsertTemplate(ctx, docsstore.InsertTemplateParams{ID: id, TemplateType: in.DocType,
			Code: in.Code, Name: in.Name, Content: body, VersionNo: no})
		return err
	})
	if isUnique(err) {
		return uuid.Nil, ErrCodeTaken
	}
	return id, err
}

func (s *Service) checkTemplate(ctx context.Context, in *TemplateInput) error {
	in.Code, in.Name = strings.TrimSpace(in.Code), strings.TrimSpace(in.Name)
	if !codeRE.MatchString(in.Code) {
		return invalid("code")
	}
	if in.Name == "" || utf8.RuneCountInString(in.Name) > maxTitle {
		return invalid("name")
	}
	d := Draft{Title: in.Name, Content: in.Content}
	return s.checkDraft(ctx, &d)
}

func toTemplate(r docsstore.GetTemplateRow) (Template, error) {
	t := Template{ID: r.ID, Global: !r.TenantID.Valid, DocType: r.TemplateType, Code: r.Code, Name: r.Name, VersionNo: r.VersionNo,
		Status: r.Status, UpdatedAt: r.UpdatedAt.Time, RowVersion: r.RowVersion}
	if err := json.Unmarshal(r.Content, &t.Content); err != nil {
		return Template{}, err
	}
	return t, nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}
