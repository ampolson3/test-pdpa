// Package docshttp holds the document composer endpoints (PLT-16): documents, their export and comparison, the
// clause library and document templates. Submitting, approving and publishing a document's draft go through the
// PLT-08 version endpoints. Types in docs.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see
// oapi-codegen.yaml).
package docshttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/versioning"
)

type Strict struct {
	svc *docs.Service
}

func NewStrict(svc *docs.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

// ---- documents ----

func (h *Strict) PlatformListDocumentTypes(ctx context.Context, _ PlatformListDocumentTypesRequestObject) (PlatformListDocumentTypesResponseObject, error) {
	types := []map[string]any{}
	canClauses := false
	for _, a := range h.svc.Access(ctx) {
		types = append(types, map[string]any{"doc_type": a.DocType, "entity_type": docs.EntityType(a.DocType), "can_create": a.Create,
			"can_update": a.Update, "can_publish": a.Publish, "can_read_templates": a.TemplateRead, "can_write_templates": a.TemplateWrite})
		canClauses = canClauses || a.Update
	}
	fields := []map[string]any{}
	for _, f := range docs.Fields {
		fields = append(fields, map[string]any{"key": f.Key, "source": f.Source, "label": f.Label})
	}
	var out DocumentTypesInfo
	err := convert(map[string]any{"types": types, "fields": fields, "can_manage_clauses": canClauses}, &out)
	return PlatformListDocumentTypes200JSONResponse(out), err
}

func (h *Strict) PlatformListDocuments(ctx context.Context, req PlatformListDocumentsRequestObject) (PlatformListDocumentsResponseObject, error) {
	p := req.Params
	f := docs.ListFilter{}
	if p.DocType != nil {
		f.DocType = string(*p.DocType)
	}
	if p.Q != nil {
		f.Query = *p.Q
	}
	if p.Limit != nil {
		f.Limit = *p.Limit
	}
	if p.Cursor != nil {
		c, err := decodeCursor(*p.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &c
	}
	list, next, err := h.svc.List(ctx, f)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := PlatformListDocuments200JSONResponse{Data: make([]DocumentSummary, 0, len(list))}
	for _, d := range list {
		var w DocumentSummary
		if err := convert(summaryWire(d), &w); err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	if next != nil {
		c := encodeCursor(*next)
		out.NextCursor = &c
	}
	return out, nil
}

func (h *Strict) PlatformCreateDocument(ctx context.Context, req PlatformCreateDocumentRequestObject) (PlatformCreateDocumentResponseObject, error) {
	b := req.Body
	d, err := h.svc.Create(ctx, docs.CreateInput{DocType: string(b.DocType), Title: b.Title, LegalEntityID: b.LegalEntityId, TemplateID: b.TemplateId})
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := documentWire(d)
	return PlatformCreateDocument201JSONResponse{Body: w, Headers: PlatformCreateDocument201ResponseHeaders{ETag: etag(d.RowVersion)}}, err
}

func (h *Strict) PlatformGetDocument(ctx context.Context, req PlatformGetDocumentRequestObject) (PlatformGetDocumentResponseObject, error) {
	d, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := documentWire(d)
	return PlatformGetDocument200JSONResponse{Body: w, Headers: PlatformGetDocument200ResponseHeaders{ETag: etag(d.RowVersion)}}, err
}

func (h *Strict) PlatformSaveDocumentDraft(ctx context.Context, req PlatformSaveDocumentDraftRequestObject) (PlatformSaveDocumentDraftResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	var d docs.Draft
	if err := convert(req.Body, &d); err != nil {
		return nil, httpx.RequestInvalid(err.Error())
	}
	doc, err := h.svc.SaveDraft(ctx, req.Id, v, d)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := documentWire(doc)
	return PlatformSaveDocumentDraft200JSONResponse{Body: w, Headers: PlatformSaveDocumentDraft200ResponseHeaders{ETag: etag(doc.RowVersion)}}, err
}

func (h *Strict) PlatformListPublishedDocumentVersions(ctx context.Context, req PlatformListPublishedDocumentVersionsRequestObject) (PlatformListPublishedDocumentVersionsResponseObject, error) {
	vs, err := h.svc.Versions(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := PlatformListPublishedDocumentVersions200JSONResponse{Data: make([]PublishedDocumentVersion, 0, len(vs))}
	for _, v := range vs {
		files := []map[string]any{}
		for _, lang := range []string{"th", "en"} {
			for _, format := range []string{"pdf", "docx"} {
				if id, ok := v.Files[lang][format]; ok {
					files = append(files, map[string]any{"language": lang, "format": format, "file_id": id})
				}
			}
		}
		m := map[string]any{"id": v.ID, "version": v.VersionNo, "languages": v.Languages, "render_status": v.RenderStatus, "files": files,
			"created_at": v.CreatedAt, "approved_by": v.ApprovedBy, "approved_at": v.ApprovedAt}
		if v.ChangeSummary != "" {
			m["change_summary"] = v.ChangeSummary
		}
		if v.EffectiveFrom != nil {
			m["effective_from"] = v.EffectiveFrom.Format(time.DateOnly)
		}
		var w PublishedDocumentVersion
		if err := convert(m, &w); err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	return out, nil
}

func (h *Strict) PlatformExportDocument(ctx context.Context, req PlatformExportDocumentRequestObject) (PlatformExportDocumentResponseObject, error) {
	b, name, err := h.svc.Export(ctx, req.Id, req.Params.VersionId, string(req.Params.Language), string(req.Params.Format))
	if err != nil {
		return nil, ToProblem(err)
	}
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return exportResponse{PlatformExportDocument200ApplicationoctetStreamResponse{Body: bytes.NewReader(b), ContentLength: int64(len(b)),
		Headers: PlatformExportDocument200ResponseHeaders{ContentDisposition: &cd}}, contentType(name)}, nil
}

// exportResponse sets the file's real media type (the contract says octet-stream so one response covers both).
type exportResponse struct {
	PlatformExportDocument200ApplicationoctetStreamResponse
	ctype string
}

func (r exportResponse) VisitPlatformExportDocumentResponse(w http.ResponseWriter) error {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := r.PlatformExportDocument200ApplicationoctetStreamResponse.VisitPlatformExportDocumentResponse(&typedWriter{ResponseWriter: w, ctype: r.ctype}); err != nil {
		return err
	}
	return nil
}

// typedWriter replaces the Content-Type the generated code sets.
type typedWriter struct {
	http.ResponseWriter
	ctype string
}

func (t *typedWriter) WriteHeader(code int) {
	t.ResponseWriter.Header().Set("Content-Type", t.ctype)
	t.ResponseWriter.WriteHeader(code)
}

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(name, ".docx"):
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	return "application/octet-stream"
}

func (h *Strict) PlatformCompareDocumentVersions(ctx context.Context, req PlatformCompareDocumentVersionsRequestObject) (PlatformCompareDocumentVersionsResponseObject, error) {
	p := req.Params
	c, err := h.svc.Compare(ctx, req.Id, p.From, p.To, string(p.Language))
	if err != nil {
		return nil, ToProblem(err)
	}
	ref := func(v versioning.Version) map[string]any { return map[string]any{"id": v.ID, "version": v.No, "status": v.Status} }
	var out DocumentComparison
	err = convert(map[string]any{"from": ref(c.From), "to": ref(c.To), "changes": c.Changes, "summary": c.Summary}, &out)
	return PlatformCompareDocumentVersions200JSONResponse(out), err
}

// ---- clause library ----

func (h *Strict) PlatformListClauses(ctx context.Context, req PlatformListClausesRequestObject) (PlatformListClausesResponseObject, error) {
	p := req.Params
	f := docs.ClauseFilter{}
	if p.Category != nil {
		f.Category = *p.Category
	}
	if p.AppliesTo != nil {
		f.AppliesTo = string(*p.AppliesTo)
	}
	if p.PublishedOnly != nil {
		f.PublishedOnly = *p.PublishedOnly
	}
	list, err := h.svc.ListClauses(ctx, f)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := PlatformListClauses200JSONResponse{Data: make([]DocumentClause, 0, len(list))}
	for _, c := range list {
		var w DocumentClause
		if err := convert(clauseWire(c), &w); err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	return out, nil
}

func clauseInput(body any) (docs.ClauseInput, error) {
	var raw struct {
		Code        string                     `json:"code"`
		Category    string                     `json:"category"`
		Body        map[string]docs.ClauseBody `json:"body"`
		LegalRef    string                     `json:"legal_ref"`
		AppliesTo   []string                   `json:"applies_to"`
		IsMandatory bool                       `json:"is_mandatory"`
	}
	if err := convert(body, &raw); err != nil {
		return docs.ClauseInput{}, httpx.RequestInvalid(err.Error())
	}
	return docs.ClauseInput(raw), nil
}

func (h *Strict) PlatformCreateClause(ctx context.Context, req PlatformCreateClauseRequestObject) (PlatformCreateClauseResponseObject, error) {
	in, err := clauseInput(req.Body)
	if err != nil {
		return nil, err
	}
	c, err := h.svc.CreateClause(ctx, in)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w DocumentClause
	err = convert(clauseWire(c), &w)
	return PlatformCreateClause201JSONResponse{Body: w, Headers: PlatformCreateClause201ResponseHeaders{ETag: etag(c.RowVersion)}}, err
}

func (h *Strict) PlatformGetClause(ctx context.Context, req PlatformGetClauseRequestObject) (PlatformGetClauseResponseObject, error) {
	c, versions, err := h.svc.GetClause(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	m := clauseWire(c)
	vs := []map[string]any{}
	for _, v := range versions {
		vs = append(vs, clauseWire(v))
	}
	m["versions"] = vs
	var w DocumentClauseDetail
	err = convert(m, &w)
	return PlatformGetClause200JSONResponse{Body: w, Headers: PlatformGetClause200ResponseHeaders{ETag: etag(c.RowVersion)}}, err
}

func (h *Strict) PlatformUpdateClause(ctx context.Context, req PlatformUpdateClauseRequestObject) (PlatformUpdateClauseResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in, err := clauseInput(req.Body)
	if err != nil {
		return nil, err
	}
	c, err := h.svc.UpdateClause(ctx, req.Id, v, in)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w DocumentClause
	err = convert(clauseWire(c), &w)
	return PlatformUpdateClause200JSONResponse{Body: w, Headers: PlatformUpdateClause200ResponseHeaders{ETag: etag(c.RowVersion)}}, err
}

func (h *Strict) PlatformPublishClause(ctx context.Context, req PlatformPublishClauseRequestObject) (PlatformPublishClauseResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	c, err := h.svc.PublishClause(ctx, req.Id, v)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w DocumentClause
	err = convert(clauseWire(c), &w)
	return PlatformPublishClause200JSONResponse{Body: w, Headers: PlatformPublishClause200ResponseHeaders{ETag: etag(c.RowVersion)}}, err
}

func (h *Strict) PlatformRetireClause(ctx context.Context, req PlatformRetireClauseRequestObject) (PlatformRetireClauseResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	c, err := h.svc.RetireClause(ctx, req.Id, v)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w DocumentClause
	err = convert(clauseWire(c), &w)
	return PlatformRetireClause200JSONResponse{Body: w, Headers: PlatformRetireClause200ResponseHeaders{ETag: etag(c.RowVersion)}}, err
}

// ---- templates ----

func (h *Strict) PlatformListDocumentTemplates(ctx context.Context, req PlatformListDocumentTemplatesRequestObject) (PlatformListDocumentTemplatesResponseObject, error) {
	docType, published := "", false
	if req.Params.DocType != nil {
		docType = string(*req.Params.DocType)
	}
	if req.Params.PublishedOnly != nil {
		published = *req.Params.PublishedOnly
	}
	list, err := h.svc.ListTemplates(ctx, docType, published)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := PlatformListDocumentTemplates200JSONResponse{Data: make([]DocumentTemplate, 0, len(list))}
	for _, t := range list {
		var w DocumentTemplate
		if err := convert(templateWire(t), &w); err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	return out, nil
}

func (h *Strict) PlatformCreateDocumentTemplate(ctx context.Context, req PlatformCreateDocumentTemplateRequestObject) (PlatformCreateDocumentTemplateResponseObject, error) {
	var in docs.TemplateInput
	var c render.Content
	if err := convert(req.Body.Content, &c); err != nil {
		return nil, httpx.RequestInvalid(err.Error())
	}
	in = docs.TemplateInput{DocType: string(req.Body.DocType), Code: req.Body.Code, Name: req.Body.Name, Content: c}
	t, err := h.svc.CreateTemplate(ctx, in)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w DocumentTemplate
	err = convert(templateWire(t), &w)
	return PlatformCreateDocumentTemplate201JSONResponse{Body: w, Headers: PlatformCreateDocumentTemplate201ResponseHeaders{ETag: etag(t.RowVersion)}}, err
}

func (h *Strict) PlatformGetDocumentTemplate(ctx context.Context, req PlatformGetDocumentTemplateRequestObject) (PlatformGetDocumentTemplateResponseObject, error) {
	t, err := h.svc.GetTemplate(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w DocumentTemplate
	err = convert(templateWire(t), &w)
	return PlatformGetDocumentTemplate200JSONResponse{Body: w, Headers: PlatformGetDocumentTemplate200ResponseHeaders{ETag: etag(t.RowVersion)}}, err
}

func (h *Strict) PlatformUpdateDocumentTemplate(ctx context.Context, req PlatformUpdateDocumentTemplateRequestObject) (PlatformUpdateDocumentTemplateResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	var c render.Content
	if err := convert(req.Body.Content, &c); err != nil {
		return nil, httpx.RequestInvalid(err.Error())
	}
	t, err := h.svc.UpdateTemplate(ctx, req.Id, v, docs.TemplateInput{Name: req.Body.Name, Content: c})
	if err != nil {
		return nil, ToProblem(err)
	}
	var w DocumentTemplate
	err = convert(templateWire(t), &w)
	return PlatformUpdateDocumentTemplate200JSONResponse{Body: w, Headers: PlatformUpdateDocumentTemplate200ResponseHeaders{ETag: etag(t.RowVersion)}}, err
}

func (h *Strict) PlatformPublishDocumentTemplate(ctx context.Context, req PlatformPublishDocumentTemplateRequestObject) (PlatformPublishDocumentTemplateResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	t, err := h.svc.PublishTemplate(ctx, req.Id, v)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w DocumentTemplate
	err = convert(templateWire(t), &w)
	return PlatformPublishDocumentTemplate200JSONResponse{Body: w, Headers: PlatformPublishDocumentTemplate200ResponseHeaders{ETag: etag(t.RowVersion)}}, err
}

// ---- wire ----

func summaryWire(d docs.Document) map[string]any {
	return map[string]any{"id": d.ID, "doc_type": d.DocType, "title": d.Title, "legal_entity_id": d.LegalEntityID, "template_id": d.TemplateID,
		"status": d.Status, "current_version_id": d.CurrentVersionID, "row_version": d.RowVersion, "created_at": d.CreatedAt, "updated_at": d.UpdatedAt}
}

func documentWire(d docs.Document) (Document, error) {
	m := summaryWire(d)
	m["entity_type"] = docs.EntityType(d.DocType)
	if d.Latest != nil {
		m["latest"] = map[string]any{"id": d.Latest.ID, "version": d.Latest.No, "status": d.Latest.Status, "row_version": d.Latest.RowVersion}
	}
	if d.Draft != nil {
		dm := map[string]any{"title": d.Draft.Title, "legal_entity_id": d.Draft.LegalEntityID, "content": d.Draft.Content}
		if d.Draft.ChangeSummary != "" {
			dm["change_summary"] = d.Draft.ChangeSummary
		}
		if d.Draft.EffectiveFrom != "" {
			dm["effective_from"] = d.Draft.EffectiveFrom
		}
		m["draft"] = dm
	}
	if d.Missing != nil {
		m["missing"] = map[string]any{"fields": nonNil(d.Missing.Fields), "clauses": nonNil(d.Missing.Clauses)}
	}
	var w Document
	return w, convert(m, &w)
}

func clauseWire(c docs.Clause) map[string]any {
	m := map[string]any{"id": c.ID, "global": c.Global, "code": c.Code, "category": c.Category, "body": c.Body, "applies_to": nonNil(c.AppliesTo),
		"is_mandatory": c.IsMandatory, "version": c.VersionNo, "status": c.Status, "row_version": c.RowVersion, "updated_at": c.UpdatedAt}
	if c.LegalRef != "" {
		m["legal_ref"] = c.LegalRef
	}
	return m
}

func templateWire(t docs.Template) map[string]any {
	return map[string]any{"id": t.ID, "global": t.Global, "doc_type": t.DocType, "code": t.Code, "name": t.Name, "content": t.Content,
		"version": t.VersionNo, "status": t.Status, "row_version": t.RowVersion, "updated_at": t.UpdatedAt}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// dropNulls removes null values so optional fields stay absent (nil pointers in the wire maps).
func dropNulls(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if e == nil {
				delete(x, k)
			} else {
				x[k] = dropNulls(e)
			}
		}
	case []any:
		for i := range x {
			x[i] = dropNulls(x[i])
		}
	}
	return v
}

func convert(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return err
	}
	if b, err = json.Marshal(dropNulls(generic)); err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func encodeCursor(c docs.Cursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.UpdatedAt.Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

func decodeCursor(s string) (docs.Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return docs.Cursor{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return docs.Cursor{}, errors.New("cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return docs.Cursor{}, err
	}
	u, err := uuid.Parse(id)
	return docs.Cursor{UpdatedAt: t, ID: u}, err
}

func etag(v int32) *string {
	s := fmt.Sprintf("%q", strconv.Itoa(int(v)))
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

// ToProblem maps service errors to RFC 9457 problems with stable codes.
func ToProblem(err error) error {
	var inc *docs.IncompleteError
	switch {
	case errors.As(err, &inc):
		p := httpx.Problem{Status: http.StatusUnprocessableEntity, Code: "docs.incomplete", Title: "The document is incomplete"}
		for _, f := range inc.Fields {
			p.Errors = append(p.Errors, httpx.FieldError{Field: f, Code: "missing_field"})
		}
		for _, c := range inc.Clauses {
			p.Errors = append(p.Errors, httpx.FieldError{Field: c, Code: "missing_clause"})
		}
		return p
	case errors.Is(err, docs.ErrNotFound), errors.Is(err, versioning.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, docs.ErrForbidden), errors.Is(err, versioning.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, docs.ErrUnknownType):
		return httpx.UnprocessableEntity("docs.unknown_type", "No module offers this document type")
	case errors.Is(err, docs.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, docs.ErrInvalidRequest):
		return httpx.UnprocessableEntity("docs.invalid", err.Error())
	case errors.Is(err, docs.ErrInvalidState):
		return httpx.Problem{Status: http.StatusConflict, Code: "docs.invalid_state", Title: "Not allowed in the current state"}
	case errors.Is(err, docs.ErrCodeTaken):
		return httpx.Problem{Status: http.StatusConflict, Code: "docs.code_taken", Title: "Code already used"}
	case errors.Is(err, versioning.ErrLocked):
		return httpx.Problem{Status: http.StatusConflict, Code: "versioning.locked", Title: "Version locked"}
	case errors.Is(err, render.ErrNoRenderer):
		return httpx.Problem{Status: http.StatusServiceUnavailable, Code: "docs.no_renderer", Title: "PDF rendering is not configured"}
	}
	return err
}
