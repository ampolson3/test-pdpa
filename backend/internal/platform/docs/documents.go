package docs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/docs/render"
	docsstore "pdpa-platform/internal/platform/docs/store"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/versioning"
)

// Draft is the editable part of a document: the PLT-08 snapshot of each version.
type Draft struct {
	Title         string         `json:"title"`
	LegalEntityID *uuid.UUID     `json:"legal_entity_id,omitempty"`
	Content       render.Content `json:"content"`
	ChangeSummary string         `json:"change_summary,omitempty"`
	EffectiveFrom string         `json:"effective_from,omitempty"` // YYYY-MM-DD
}

// Document is a composed document with its newest version (the open draft, or the last published one).
type Document struct {
	ID               uuid.UUID
	DocType          string
	Title            string
	TemplateID       *uuid.UUID
	LegalEntityID    *uuid.UUID
	Status           string // draft (never published) | published
	CurrentVersionID *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
	RowVersion       int32
	// Latest is the newest PLT-08 version and Draft its content; Missing lists what it still lacks to be published.
	Latest  *versioning.Version
	Draft   *Draft
	Missing *IncompleteError
}

// frozen is a published version's content (platform.document_versions.content): everything needed to render it
// again exactly as approved, whatever changes later in the organization's details or the clause library.
type frozen struct {
	Title           string                           `json:"title"`
	Content         render.Content                   `json:"content"`
	Fields          map[string]map[string]string     `json:"fields"`  // language → key → value
	Clauses         map[string]map[string]clauseText `json:"clauses"` // language → code@version → text
	RecordVersionID uuid.UUID                        `json:"record_version_id"`
}

// clauseText is one clause version's text in one language, as stored in body_th / body_en.
type clauseText struct {
	Title string      `json:"title"`
	Doc   render.Node `json:"doc"`
}

const (
	maxTitle = 300
	pageSize = 50
)

// CreateInput starts a document.
type CreateInput struct {
	DocType       string
	Title         string
	LegalEntityID *uuid.UUID
	TemplateID    *uuid.UUID // a published template of the same type to start from
}

// Create starts a document with its first draft (the template's content, or an empty page).
func (s *Service) Create(ctx context.Context, in CreateInput) (Document, error) {
	if _, err := s.need(ctx, in.DocType, func(p Policy) string { return p.Create }); err != nil {
		return Document{}, err
	}
	d := Draft{Title: strings.TrimSpace(in.Title), LegalEntityID: in.LegalEntityID,
		Content: render.Content{"th": {Type: "doc", Content: []render.Node{{Type: "paragraph"}}}}}
	if in.TemplateID != nil {
		t, err := s.GetTemplate(ctx, *in.TemplateID)
		if err != nil || t.DocType != in.DocType || t.Status != "published" {
			return Document{}, invalid("template_id")
		}
		d.Content = t.Content
	}
	if err := s.checkDraft(ctx, &d); err != nil {
		return Document{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Document{}, err
	}
	if _, err := docsstore.New(pdb.MustTxFromContext(ctx)).InsertDocument(ctx, docsstore.InsertDocumentParams{
		ID: id, DocType: in.DocType, Title: d.Title, TemplateID: pgUUID(in.TemplateID), LegalEntityID: pgUUID(in.LegalEntityID),
	}); err != nil {
		return Document{}, err
	}
	if _, err := s.Versioning.SaveDraft(ctx, EntityType(in.DocType), id, d); err != nil {
		return Document{}, err
	}
	if err := s.audit(ctx, "platform.document.create", EntityType(in.DocType), id, nil, map[string]any{"doc_type": in.DocType, "title": d.Title}); err != nil {
		return Document{}, err
	}
	return s.Get(ctx, id)
}

// Get returns a document the caller may read, with its newest version.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Document, error) {
	r, err := docsstore.New(pdb.MustTxFromContext(ctx)).GetDocument(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, err
	}
	if _, err := s.need(ctx, r.DocType, func(p Policy) string { return p.Read }); err != nil {
		return Document{}, ErrNotFound
	}
	doc := toDocument(docsstore.ListDocumentsRow(r))
	vs, err := s.Versioning.List(ctx, EntityType(r.DocType), id)
	if err != nil && !errors.Is(err, versioning.ErrNotFound) {
		return Document{}, err
	}
	if len(vs) > 0 {
		latest := vs[0]
		var d Draft
		if err := json.Unmarshal(latest.Snapshot, &d); err != nil {
			return Document{}, err
		}
		doc.Latest, doc.Draft = &latest, &d
		if latest.Status != "published" && latest.Status != "superseded" {
			in, err := s.inputs(ctx, d, latest.No)
			if err != nil {
				return Document{}, err
			}
			doc.Missing = missing(in)
		}
	}
	return doc, nil
}

// ListFilter narrows List.
type ListFilter struct {
	DocType string
	Query   string
	After   *Cursor
	Limit   int
}

// Cursor is the position after the last document of a page.
type Cursor struct {
	UpdatedAt time.Time
	ID        uuid.UUID
}

// List returns documents of the types the caller can read, most recently changed first.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Document, *Cursor, error) {
	types := s.readableTypes(ctx)
	if f.DocType != "" && !slices.Contains(types, f.DocType) {
		if _, ok := s.policy(f.DocType); !ok {
			return nil, nil, ErrUnknownType
		}
		return nil, nil, ErrForbidden
	}
	limit := f.Limit
	if limit <= 0 || limit > pageSize {
		limit = pageSize
	}
	p := docsstore.ListDocumentsParams{Types: types, Lim: int32(limit + 1)}
	if f.DocType != "" {
		p.DocType = &f.DocType
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
		p.Q = &esc
	}
	if f.After != nil {
		p.AfterUpdated = pgtype.Timestamptz{Time: f.After.UpdatedAt, Valid: true}
		p.AfterID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := docsstore.New(pdb.MustTxFromContext(ctx)).ListDocuments(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Document, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &Cursor{UpdatedAt: last.UpdatedAt, ID: last.ID}, nil
		}
		out = append(out, toDocument(r))
	}
	return out, nil, nil
}

// SaveDraft replaces the document's open draft (or starts the next version after a published one). rowVersion is
// the document's, from its ETag.
func (s *Service) SaveDraft(ctx context.Context, id uuid.UUID, rowVersion int32, d Draft) (Document, error) {
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.LockDocument(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, err
	}
	if _, err := s.need(ctx, r.DocType, func(p Policy) string { return p.Read }); err != nil {
		return Document{}, ErrNotFound
	}
	if _, err := s.need(ctx, r.DocType, func(p Policy) string { return p.Update }); err != nil {
		return Document{}, err
	}
	if r.RowVersion != rowVersion {
		return Document{}, ErrVersionMismatch
	}
	if err := s.checkDraft(ctx, &d); err != nil {
		return Document{}, err
	}
	if _, err := s.Versioning.SaveDraft(ctx, EntityType(r.DocType), id, d); err != nil {
		return Document{}, err
	}
	if _, err := q.UpdateDocumentDraft(ctx, docsstore.UpdateDocumentDraftParams{ID: id, Title: d.Title, LegalEntityID: pgUUID(d.LegalEntityID)}); err != nil {
		return Document{}, err
	}
	if err := s.audit(ctx, "platform.document.save_draft", EntityType(r.DocType), id, map[string]any{"title": r.Title}, map[string]any{"title": d.Title}); err != nil {
		return Document{}, err
	}
	return s.Get(ctx, id)
}

// checkDraft normalizes and validates a draft: the content model, known merge fields, clause references that
// exist in the library, a visible legal entity.
func (s *Service) checkDraft(ctx context.Context, d *Draft) error {
	d.Title = strings.TrimSpace(d.Title)
	d.ChangeSummary = strings.TrimSpace(d.ChangeSummary)
	if d.Title == "" || utf8.RuneCountInString(d.Title) > maxTitle {
		return invalid("title")
	}
	if utf8.RuneCountInString(d.ChangeSummary) > 2000 {
		return invalid("change_summary")
	}
	if d.EffectiveFrom != "" {
		if _, err := time.Parse(time.DateOnly, d.EffectiveFrom); err != nil {
			return invalid("effective_from")
		}
	}
	if err := d.Content.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	var refs []string
	for _, doc := range d.Content {
		for _, k := range render.FieldKeys(doc) {
			if !knownField(k) {
				return invalid("unknown merge field %s", k)
			}
		}
		refs = append(refs, render.ClauseRefs(doc)...)
	}
	if len(refs) > 0 {
		found, err := s.clauseTexts(ctx, refs)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			if _, ok := found["th"][ref]; !ok {
				return invalid("unknown clause %s", ref)
			}
		}
	}
	if d.LegalEntityID != nil && s.Org != nil {
		if _, err := s.Org.MergeFields(ctx, *d.LegalEntityID); err != nil {
			return invalid("legal_entity_id")
		}
	}
	return nil
}

// inputs prepares a draft for rendering in each of its languages, with today's field values and clause texts.
func (s *Service) inputs(ctx context.Context, d Draft, versionNo int32) (map[string]render.Input, error) {
	var eff *time.Time
	if d.EffectiveFrom != "" {
		t, err := time.Parse(time.DateOnly, d.EffectiveFrom)
		if err != nil {
			return nil, invalid("effective_from")
		}
		eff = &t
	}
	fields, err := s.fieldValues(ctx, d.Title, d.LegalEntityID, versionNo, eff)
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, doc := range d.Content {
		refs = append(refs, render.ClauseRefs(doc)...)
	}
	clauses, err := s.clauseTexts(ctx, refs)
	if err != nil {
		return nil, err
	}
	return buildInputs(frozen{Title: d.Title, Content: d.Content, Fields: fields, Clauses: clauses}), nil
}

// buildInputs turns stored or resolved content into one render.Input per language.
func buildInputs(f frozen) map[string]render.Input {
	out := map[string]render.Input{}
	for lang, body := range f.Content {
		cl := map[string]render.Clause{}
		for ref, t := range f.Clauses[lang] {
			cl[ref] = render.Clause{Title: t.Title, Body: t.Doc}
		}
		out[lang] = render.Input{Title: f.Title, Language: lang, Body: body, Fields: f.Fields[lang], Clauses: cl}
	}
	return out
}

func missing(in map[string]render.Input) *IncompleteError {
	var e IncompleteError
	for _, lang := range []string{"th", "en"} {
		x, ok := in[lang]
		if !ok {
			continue
		}
		f, c := x.Missing()
		for _, k := range f {
			if !slices.Contains(e.Fields, k) {
				e.Fields = append(e.Fields, k)
			}
		}
		for _, k := range c {
			if !slices.Contains(e.Clauses, k) {
				e.Clauses = append(e.Clauses, k)
			}
		}
	}
	if len(e.Fields) == 0 && len(e.Clauses) == 0 {
		return nil
	}
	return &e
}

// clauseTexts loads the referenced clause versions (code@version) in both languages; an English body falls back to
// the Thai one. Unknown references are simply absent.
func (s *Service) clauseTexts(ctx context.Context, refs []string) (map[string]map[string]clauseText, error) {
	out := map[string]map[string]clauseText{"th": {}, "en": {}}
	if len(refs) == 0 {
		return out, nil
	}
	codes := []string{}
	for _, r := range refs {
		code, _, _ := strings.Cut(r, "@")
		if !slices.Contains(codes, code) {
			codes = append(codes, code)
		}
	}
	rows, err := docsstore.New(pdb.MustTxFromContext(ctx)).ClausesByRef(ctx, codes)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		ref := render.ClauseKey(r.Code, int(r.VersionNo))
		if !slices.Contains(refs, ref) {
			continue
		}
		// th and en are decoded into separate zero values: en := th (a shallow copy) would share th.Doc.Content's
		// backing array, and decoding BodyEn into that copy would silently overwrite th's paragraphs too.
		var th, en clauseText
		if err := json.Unmarshal(r.BodyTh, &th); err != nil {
			return nil, err
		}
		if len(r.BodyEn) > 0 {
			if err := json.Unmarshal(r.BodyEn, &en); err != nil {
				return nil, err
			}
		} else {
			en = th // no English body: fall back to the Thai text, in a clause nothing will decode into again
		}
		out["th"][ref], out["en"][ref] = th, en
	}
	return out, nil
}

// publish is the PLT-08 OnPublish hook: the approved draft is frozen into an immutable document version and its
// files are rendered in the background.
func (s *Service) publish(ctx context.Context, docType string, id uuid.UUID, snapshot json.RawMessage) error {
	var d Draft
	if err := json.Unmarshal(snapshot, &d); err != nil {
		return err
	}
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	if _, err := q.LockDocument(ctx, id); err != nil {
		return err
	}
	pub, err := s.Versioning.Published(ctx, EntityType(docType), id)
	if err != nil {
		return err
	}
	full, err := s.Versioning.Get(ctx, pub.ID)
	if err != nil {
		return err
	}
	in, err := s.inputs(ctx, d, pub.No)
	if err != nil {
		return err
	}
	if m := missing(in); m != nil {
		return m
	}
	var eff *time.Time
	if d.EffectiveFrom != "" {
		t, _ := time.Parse(time.DateOnly, d.EffectiveFrom)
		eff = &t
	}
	fields, err := s.fieldValues(ctx, d.Title, d.LegalEntityID, pub.No, eff)
	if err != nil {
		return err
	}
	var refs []string
	langs := []string{}
	for _, lang := range []string{"th", "en"} {
		if doc, ok := d.Content[lang]; ok {
			langs = append(langs, lang)
			refs = append(refs, render.ClauseRefs(doc)...)
		}
	}
	clauses, err := s.clauseTexts(ctx, refs)
	if err != nil {
		return err
	}
	body, err := json.Marshal(frozen{Title: d.Title, Content: d.Content, Fields: fields, Clauses: clauses, RecordVersionID: pub.ID})
	if err != nil {
		return err
	}
	p := docsstore.InsertDocumentVersionParams{DocumentID: id, VersionNo: pub.No, Content: body, Languages: langs,
		ApprovedAt: pgtype.Timestamptz{Time: s.now(), Valid: true}}
	if p.ID, err = uuid.NewV7(); err != nil {
		return err
	}
	if d.ChangeSummary != "" {
		p.ChangeSummary = &d.ChangeSummary
	}
	if eff != nil {
		p.EffectiveFrom = pgtype.Date{Time: *eff, Valid: true}
	}
	for _, a := range full.Approvals { // the last level's approver
		if a.Decision == "approved" && a.ApproverID != nil {
			p.ApprovedBy = pgUUID(a.ApproverID)
			if a.DecidedAt != nil {
				p.ApprovedAt = pgtype.Timestamptz{Time: *a.DecidedAt, Valid: true}
			}
		}
	}
	vid, err := q.InsertDocumentVersion(ctx, p)
	if err != nil {
		return err
	}
	if err := q.SetDocumentPublished(ctx, docsstore.SetDocumentPublishedParams{ID: id, Title: d.Title, VersionID: pgtype.UUID{Bytes: vid, Valid: true}}); err != nil {
		return err
	}
	_, g, _ := currentUser(ctx)
	if _, err := jobs.Enqueue(ctx, s.River, RenderArgs{TenantArgs: jobs.TenantArgs{TenantID: g.TenantID}, VersionID: vid.String()},
		&river.InsertOpts{MaxAttempts: renderAttempts}); err != nil {
		return err
	}
	return s.audit(ctx, "platform.document.publish", EntityType(docType), id, nil, map[string]any{"version": pub.No, "document_version_id": vid, "languages": langs})
}

// title names a document in the approval inbox.
func (s *Service) title(ctx context.Context, id uuid.UUID) (string, error) {
	r, err := docsstore.New(pdb.MustTxFromContext(ctx)).GetDocument(ctx, id)
	if err != nil {
		return "", err
	}
	return r.Title, nil
}

// PublishedVersion is a published version of a document and its rendered files.
type PublishedVersion struct {
	ID            uuid.UUID
	VersionNo     int32
	Languages     []string
	RenderStatus  string                          // pending | done | failed
	Files         map[string]map[string]uuid.UUID // language → pdf|docx → file id
	ChangeSummary string
	EffectiveFrom *time.Time
	ApprovedBy    *uuid.UUID
	ApprovedAt    *time.Time
	CreatedAt     time.Time
}

// Versions lists a document's published versions, newest first.
func (s *Service) Versions(ctx context.Context, id uuid.UUID) ([]PublishedVersion, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := docsstore.New(pdb.MustTxFromContext(ctx)).ListDocumentVersions(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]PublishedVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPublished(docsstore.GetDocumentVersionRow{ID: r.ID, DocumentID: r.DocumentID, VersionNo: r.VersionNo,
			Languages: r.Languages, PdfFileID: r.PdfFileID, DocxFileID: r.DocxFileID, PdfEnFileID: r.PdfEnFileID, DocxEnFileID: r.DocxEnFileID,
			RenderStatus: r.RenderStatus, ChangeSummary: r.ChangeSummary, EffectiveFrom: r.EffectiveFrom, ApprovedBy: r.ApprovedBy,
			ApprovedAt: r.ApprovedAt, CreatedAt: r.CreatedAt}))
	}
	return out, nil
}

// PublishedVersionOf returns one published version (for modules that link to it, e.g. BRE-09's PDPC notice).
func (s *Service) PublishedVersionOf(ctx context.Context, versionID uuid.UUID) (PublishedVersion, uuid.UUID, error) {
	r, err := docsstore.New(pdb.MustTxFromContext(ctx)).GetDocumentVersion(ctx, versionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublishedVersion{}, uuid.Nil, ErrNotFound
	}
	if err != nil {
		return PublishedVersion{}, uuid.Nil, err
	}
	if _, err := s.Get(ctx, r.DocumentID); err != nil {
		return PublishedVersion{}, uuid.Nil, err
	}
	return toPublished(r), r.DocumentID, nil
}

// Export renders a PLT-08 version of a document (nil: the newest) in one language as pdf, docx or html. A version
// that isn't published carries the DRAFT banner; a published one is rendered from its frozen content.
func (s *Service) Export(ctx context.Context, id uuid.UUID, recordVersionID *uuid.UUID, lang, format string) ([]byte, string, error) {
	in, err := s.versionInput(ctx, id, recordVersionID, lang)
	if err != nil {
		return nil, "", err
	}
	name := fmt.Sprintf("%s-%s", in.Title, lang)
	switch format {
	case "pdf":
		if s.PDF == nil {
			return nil, "", render.ErrNoRenderer
		}
		b, err := s.PDF.PDF(ctx, render.HTML(in))
		return b, name + ".pdf", err
	case "docx":
		b, err := render.DOCX(in)
		return b, name + ".docx", err
	case "html":
		return render.HTML(in), name + ".html", nil
	}
	return nil, "", invalid("format")
}

// versionInput resolves one version (nil: the newest) of a document for rendering in lang.
func (s *Service) versionInput(ctx context.Context, id uuid.UUID, recordVersionID *uuid.UUID, lang string) (render.Input, error) {
	if lang != "th" && lang != "en" {
		return render.Input{}, invalid("language")
	}
	doc, err := s.Get(ctx, id)
	if err != nil {
		return render.Input{}, err
	}
	v := doc.Latest
	if recordVersionID != nil {
		x, err := s.Versioning.Get(ctx, *recordVersionID)
		if err != nil || x.EntityType != EntityType(doc.DocType) || x.EntityID != id {
			return render.Input{}, ErrNotFound
		}
		v = &x
	}
	if v == nil {
		return render.Input{}, ErrNotFound
	}
	var inputs map[string]render.Input
	if v.Status == "published" || v.Status == "superseded" {
		r, err := docsstore.New(pdb.MustTxFromContext(ctx)).GetDocumentVersionByNo(ctx, docsstore.GetDocumentVersionByNoParams{DocumentID: id, VersionNo: v.No})
		if err != nil {
			return render.Input{}, err
		}
		var f frozen
		if err := json.Unmarshal(r.Content, &f); err != nil {
			return render.Input{}, err
		}
		inputs = buildInputs(f)
	} else {
		var d Draft
		if err := json.Unmarshal(v.Snapshot, &d); err != nil {
			return render.Input{}, err
		}
		if inputs, err = s.inputs(ctx, d, v.No); err != nil {
			return render.Input{}, err
		}
		for k, x := range inputs {
			x.Draft = true
			inputs[k] = x
		}
	}
	in, ok := inputs[lang]
	if !ok {
		return render.Input{}, invalid("the version has no %s text", lang)
	}
	return in, nil
}

// Comparison is the difference between two versions of a document in one language.
type Comparison struct {
	From, To versioning.Version
	Changes  []render.Change
	Summary  map[string]int
}

// Compare lines up two PLT-08 versions of a document (drafts included) block by block, as a reader sees them.
func (s *Service) Compare(ctx context.Context, id, from, to uuid.UUID, lang string) (Comparison, error) {
	a, err := s.versionInput(ctx, id, &from, lang)
	if err != nil {
		return Comparison{}, err
	}
	b, err := s.versionInput(ctx, id, &to, lang)
	if err != nil {
		return Comparison{}, err
	}
	va, err := s.Versioning.Get(ctx, from)
	if err != nil {
		return Comparison{}, err
	}
	vb, err := s.Versioning.Get(ctx, to)
	if err != nil {
		return Comparison{}, err
	}
	changes := render.Compare(render.Blocks(a), render.Blocks(b))
	return Comparison{From: va, To: vb, Changes: changes, Summary: render.Summary(changes)}, nil
}

func toDocument(r docsstore.ListDocumentsRow) Document {
	return Document{ID: r.ID, DocType: r.DocType, Title: r.Title, TemplateID: optUUID(r.TemplateID), LegalEntityID: optUUID(r.LegalEntityID),
		Status: r.Status, CurrentVersionID: optUUID(r.CurrentVersionID), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time, RowVersion: r.RowVersion}
}

func toPublished(r docsstore.GetDocumentVersionRow) PublishedVersion {
	v := PublishedVersion{ID: r.ID, VersionNo: r.VersionNo, Languages: r.Languages, RenderStatus: r.RenderStatus,
		Files: map[string]map[string]uuid.UUID{}, ApprovedBy: optUUID(r.ApprovedBy), CreatedAt: r.CreatedAt.Time}
	add := func(lang, format string, id pgtype.UUID) {
		if id.Valid {
			if v.Files[lang] == nil {
				v.Files[lang] = map[string]uuid.UUID{}
			}
			v.Files[lang][format] = id.Bytes
		}
	}
	add("th", "pdf", r.PdfFileID)
	add("th", "docx", r.DocxFileID)
	add("en", "pdf", r.PdfEnFileID)
	add("en", "docx", r.DocxEnFileID)
	if r.ChangeSummary != nil {
		v.ChangeSummary = *r.ChangeSummary
	}
	if r.EffectiveFrom.Valid {
		t := r.EffectiveFrom.Time
		v.EffectiveFrom = &t
	}
	if r.ApprovedAt.Valid {
		t := r.ApprovedAt.Time
		v.ApprovedAt = &t
	}
	return v
}

func pgUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func optUUID(id pgtype.UUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	u := uuid.UUID(id.Bytes)
	return &u
}
