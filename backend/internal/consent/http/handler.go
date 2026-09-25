// Package consenthttp holds the admin endpoints of the consent module (CON). Types in consent.gen.go are generated
// from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml); the public form endpoints are in
// ../publichttp because they run under a different middleware chain.
package consenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	consent "pdpa-platform/internal/consent/service"
	"pdpa-platform/internal/pkg/authz"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/versioning"
)

type Strict struct {
	svc *consent.Service
}

func NewStrict(svc *consent.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

// ---- purposes ----

func (h *Strict) ConsentListPurposes(ctx context.Context, _ ConsentListPurposesRequestObject) (ConsentListPurposesResponseObject, error) {
	list, err := h.svc.ListPurposes(ctx)
	if err != nil {
		return nil, ToProblem(err)
	}
	resp := ConsentListPurposes200JSONResponse{Data: make([]ConsentPurpose, 0, len(list))}
	for _, p := range list {
		var w ConsentPurpose
		if err := convert(purposeMap(p), &w); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) ConsentCreatePurpose(ctx context.Context, req ConsentCreatePurposeRequestObject) (ConsentCreatePurposeResponseObject, error) {
	var c consent.PurposeContent
	if err := convert(req.Body.Content, &c); err != nil {
		return nil, err
	}
	p, err := h.svc.CreatePurpose(ctx, req.Body.Code, req.Body.LegalEntityId, c)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w ConsentPurpose
	return ConsentCreatePurpose201JSONResponse(w), convert(purposeMap(p), &w)
}

func (h *Strict) ConsentGetPurpose(ctx context.Context, req ConsentGetPurposeRequestObject) (ConsentGetPurposeResponseObject, error) {
	p, err := h.svc.GetPurpose(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w ConsentPurpose
	if err := convert(purposeMap(p), &w); err != nil {
		return nil, err
	}
	return ConsentGetPurpose200JSONResponse(w), nil
}

func (h *Strict) ConsentSavePurposeDraft(ctx context.Context, req ConsentSavePurposeDraftRequestObject) (ConsentSavePurposeDraftResponseObject, error) {
	var c consent.PurposeContent
	if err := convert(req.Body, &c); err != nil {
		return nil, err
	}
	v, err := h.svc.SavePurposeDraft(ctx, req.Id, c)
	if err != nil {
		return nil, ToProblem(err)
	}
	return ConsentSavePurposeDraft200JSONResponse{VersionId: v.ID, Version: int(v.No), Status: v.Status, RowVersion: int(v.RowVersion)}, nil
}

func (h *Strict) ConsentRetirePurpose(ctx context.Context, req ConsentRetirePurposeRequestObject) (ConsentRetirePurposeResponseObject, error) {
	if err := h.svc.RetirePurpose(ctx, req.Id); err != nil {
		return nil, ToProblem(err)
	}
	p, err := h.svc.GetPurpose(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w ConsentPurpose
	if err := convert(purposeMap(p), &w); err != nil {
		return nil, err
	}
	return ConsentRetirePurpose200JSONResponse(w), nil
}

// ---- collection points ----

func (h *Strict) ConsentListCollectionPoints(ctx context.Context, _ ConsentListCollectionPointsRequestObject) (ConsentListCollectionPointsResponseObject, error) {
	list, err := h.svc.ListCollectionPoints(ctx)
	if err != nil {
		return nil, ToProblem(err)
	}
	resp := ConsentListCollectionPoints200JSONResponse{Data: make([]ConsentCollectionPoint, 0, len(list))}
	for _, cp := range list {
		var w ConsentCollectionPoint
		if err := convert(cpMap(cp), &w); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func cpInput(code, name, channel string, entity uuid.UUID, origins *[]string, purposes []struct {
	PurposeId Uuid `json:"purpose_id"`
	Required  bool `json:"required"`
}) consent.CollectionPointInput {
	in := consent.CollectionPointInput{Code: code, Name: name, Channel: channel, LegalEntityID: entity}
	if origins != nil {
		in.AllowedOrigins = *origins
	}
	for _, p := range purposes {
		in.Purposes = append(in.Purposes, consent.CPPurposeInput{PurposeID: p.PurposeId, Required: p.Required})
	}
	return in
}

func (h *Strict) ConsentCreateCollectionPoint(ctx context.Context, req ConsentCreateCollectionPointRequestObject) (ConsentCreateCollectionPointResponseObject, error) {
	b := req.Body
	cp, err := h.svc.CreateCollectionPoint(ctx, cpInput(b.Code, b.Name, string(b.Channel), b.LegalEntityId, b.AllowedOrigins, b.Purposes))
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := cpWire(cp)
	return ConsentCreateCollectionPoint201JSONResponse{Body: w, Headers: ConsentCreateCollectionPoint201ResponseHeaders{ETag: etag(cp.RowVersion)}}, err
}

func (h *Strict) ConsentGetCollectionPoint(ctx context.Context, req ConsentGetCollectionPointRequestObject) (ConsentGetCollectionPointResponseObject, error) {
	cp, err := h.svc.GetCollectionPoint(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := cpWire(cp)
	return ConsentGetCollectionPoint200JSONResponse{Body: w, Headers: ConsentGetCollectionPoint200ResponseHeaders{ETag: etag(cp.RowVersion)}}, err
}

func (h *Strict) ConsentUpdateCollectionPoint(ctx context.Context, req ConsentUpdateCollectionPointRequestObject) (ConsentUpdateCollectionPointResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := req.Body
	cp, err := h.svc.UpdateCollectionPoint(ctx, req.Id, ver, cpInput("", b.Name, string(b.Channel), b.LegalEntityId, b.AllowedOrigins, b.Purposes))
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := cpWire(cp)
	return ConsentUpdateCollectionPoint200JSONResponse{Body: w, Headers: ConsentUpdateCollectionPoint200ResponseHeaders{ETag: etag(cp.RowVersion)}}, err
}

func (h *Strict) ConsentPublishCollectionPoint(ctx context.Context, req ConsentPublishCollectionPointRequestObject) (ConsentPublishCollectionPointResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := req.Body
	cp, err := h.svc.PublishCollectionPoint(ctx, req.Id, ver, consent.Checklist{SeparateText: b.SeparateText, NotBundled: b.NotBundled, PlainLanguage: b.PlainLanguage, WithdrawalInfo: b.WithdrawalInfo})
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := cpWire(cp)
	return ConsentPublishCollectionPoint200JSONResponse{Body: w, Headers: ConsentPublishCollectionPoint200ResponseHeaders{ETag: etag(cp.RowVersion)}}, err
}

func (h *Strict) ConsentRetireCollectionPoint(ctx context.Context, req ConsentRetireCollectionPointRequestObject) (ConsentRetireCollectionPointResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	cp, err := h.svc.RetireCollectionPoint(ctx, req.Id, ver)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := cpWire(cp)
	return ConsentRetireCollectionPoint200JSONResponse{Body: w, Headers: ConsentRetireCollectionPoint200ResponseHeaders{ETag: etag(cp.RowVersion)}}, err
}

// ---- data subjects ----

func (h *Strict) ConsentListSubjects(ctx context.Context, _ ConsentListSubjectsRequestObject) (ConsentListSubjectsResponseObject, error) {
	list, err := h.svc.Subjects(ctx, "", "")
	if err != nil {
		return nil, ToProblem(err)
	}
	resp := ConsentListSubjects200JSONResponse{Data: make([]ConsentSubjectSummary, 0, len(list))}
	for _, s := range list {
		var w ConsentSubjectSummary
		if err := convert(summaryMap(s), &w); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) ConsentSearchSubjects(ctx context.Context, req ConsentSearchSubjectsRequestObject) (ConsentSearchSubjectsResponseObject, error) {
	value := ""
	if req.Body.Identifier.Value != nil {
		value = *req.Body.Identifier.Value
	}
	list, err := h.svc.Subjects(ctx, string(req.Body.Identifier.Type), value)
	if err != nil {
		return nil, ToProblem(err)
	}
	resp := ConsentSearchSubjects200JSONResponse{Data: make([]ConsentSubjectSummary, 0, len(list))}
	for _, s := range list {
		var w ConsentSubjectSummary
		if err := convert(summaryMap(s), &w); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) ConsentGetSubject(ctx context.Context, req ConsentGetSubjectRequestObject) (ConsentGetSubjectResponseObject, error) {
	p, err := h.svc.GetProfile(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	m := summaryMap(p.SubjectSummary)
	statuses := make([]map[string]any, 0, len(p.Statuses))
	for _, s := range p.Statuses {
		sm := map[string]any{"purpose_id": s.PurposeID, "purpose_code": s.PurposeCode, "purpose_name": text(s.PurposeName), "is_sensitive": s.IsSensitive,
			"status": s.Status, "version": s.VersionNo, "current_version": s.CurrentVersionNo, "needs_reconsent": s.NeedsReconsent,
			"preferences": raw(s.Preferences), "updated_at": s.UpdatedAt.UTC()}
		if s.ExpiresAt != nil {
			sm["expires_at"] = s.ExpiresAt.UTC()
		}
		statuses = append(statuses, sm)
	}
	history := make([]map[string]any, 0, len(p.History))
	for _, x := range p.History {
		hm := map[string]any{"id": x.ID, "occurred_at": x.OccurredAt.UTC(), "type": x.Type, "purpose_code": x.PurposeCode, "purpose_name": text(x.PurposeName),
			"version": x.VersionNo, "preferences": raw(x.Preferences), "source": x.Source, "receipt_no": x.ReceiptNo, "channel": x.Channel,
			"collection_point": x.CollectionPointName}
		if x.ReasonCode != "" {
			hm["reason_code"] = x.ReasonCode
		}
		if x.CapturedByName != "" {
			hm["captured_by"] = x.CapturedByName
		}
		if x.ExpiresAt != nil {
			hm["expires_at"] = x.ExpiresAt.UTC()
		}
		history = append(history, hm)
	}
	m["statuses"], m["history"] = statuses, history
	var w ConsentSubjectProfile
	if err := convert(m, &w); err != nil {
		return nil, err
	}
	return ConsentGetSubject200JSONResponse(w), nil
}

func (h *Strict) ConsentVerifySubject(ctx context.Context, req ConsentVerifySubjectRequestObject) (ConsentVerifySubjectResponseObject, error) {
	r, err := h.svc.VerifySubject(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	m := map[string]any{"ok": r.OK, "checked": r.Checked}
	if !r.OK {
		m["broken_at"], m["reason"] = r.BrokenAt, r.Reason
	}
	var w ConsentVerifyResult
	if err := convert(m, &w); err != nil {
		return nil, err
	}
	return ConsentVerifySubject200JSONResponse(w), nil
}

func (h *Strict) ConsentRecordOnBehalf(ctx context.Context, req ConsentRecordOnBehalfRequestObject) (ConsentRecordOnBehalfResponseObject, error) {
	b := req.Body
	g, _ := authz.FromContext(ctx)
	sub := consent.Submission{CollectionPointID: b.CollectionPointId, SubjectID: b.SubjectId, Source: "staff", Decisions: Decisions(b.Decisions)}
	if u, err := uuid.Parse(g.UserID); err == nil {
		sub.CapturedBy = &u
	}
	if b.Identifiers != nil {
		sub.Identifiers = Identifiers(*b.Identifiers)
	}
	if b.Language != nil {
		sub.Language = string(*b.Language)
	}
	r, err := h.svc.Record(ctx, sub)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w ConsentResult
	if err := convert(ResultMap(r), &w); err != nil {
		return nil, err
	}
	return ConsentRecordOnBehalf201JSONResponse(w), nil
}

func (h *Strict) ConsentGetSettings(ctx context.Context, _ ConsentGetSettingsRequestObject) (ConsentGetSettingsResponseObject, error) {
	s, err := h.svc.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	return ConsentGetSettings200JSONResponse{DataRegion: s.DataRegion, IdentifierEncryption: "aes-256-gcm", KeyManagement: ConsentSettingsKeyManagement(s.KeyManagement)}, nil
}

// ---- wire (shared with ../publichttp) ----

// Decisions converts generated decision inputs (any package's, same JSON) into service decisions.
func Decisions[T any](in []T) []consent.Decision {
	var raw []struct {
		PurposeCode      string         `json:"purpose_code"`
		PurposeVersionNo int32          `json:"purpose_version_no"`
		Decision         string         `json:"decision"`
		Preferences      map[string]any `json:"preferences"`
		ReasonCode       string         `json:"reason_code"`
	}
	_ = convert(in, &raw)
	out := make([]consent.Decision, 0, len(raw))
	for _, d := range raw {
		out = append(out, consent.Decision{PurposeCode: d.PurposeCode, PurposeVersionNo: d.PurposeVersionNo, Decision: d.Decision, Preferences: d.Preferences, ReasonCode: d.ReasonCode})
	}
	return out
}

// Identifiers converts generated identifiers into service identifiers.
func Identifiers[T any](in []T) []consent.Identifier {
	var raw []struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	_ = convert(in, &raw)
	out := make([]consent.Identifier, 0, len(raw))
	for _, i := range raw {
		out = append(out, consent.Identifier{Type: i.Type, Value: i.Value})
	}
	return out
}

// ResultMap is a receipt as ConsentResult JSON.
func ResultMap(r consent.Receipt) map[string]any {
	txs := make([]map[string]any, 0, len(r.Transactions))
	for _, t := range r.Transactions {
		txs = append(txs, map[string]any{"id": t.ID, "purpose_code": t.PurposeCode, "transaction_type": t.Type, "status": t.Status})
	}
	return map[string]any{"receipt_no": r.No, "subject_ref": r.SubjectID, "transactions": txs}
}

func text(t consent.Text) map[string]any {
	m := map[string]any{"th": t.Th}
	if t.En != "" {
		m["en"] = t.En
	}
	return m
}

func raw(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func contentMap(c consent.PurposeContent) map[string]any {
	prefs := make([]map[string]any, 0, len(c.Preferences))
	for _, p := range c.Preferences {
		opts := make([]map[string]any, 0, len(p.Options))
		for _, o := range p.Options {
			opts = append(opts, map[string]any{"value": o.Value, "label": text(o.Label)})
		}
		prefs = append(prefs, map[string]any{"code": p.Code, "name": text(p.Name), "type": p.Type, "options": opts})
	}
	cats := c.DataCategoryCodes
	if cats == nil {
		cats = []string{}
	}
	m := map[string]any{"name": text(c.Name), "consent_text": text(c.ConsentText), "data_category_codes": cats, "requires_reconsent": c.RequiresReconsent, "preferences": prefs,
		"min_age": c.MinAge, "lifespan_days": c.LifespanDays}
	if c.Description.Th != "" {
		m["description"] = text(c.Description)
	}
	if c.ExplicitText.Th != "" {
		m["explicit_text"] = text(c.ExplicitText)
	}
	if c.ChangeType == "minor" || c.ChangeType == "material" {
		m["change_type"] = c.ChangeType
	}
	return m
}

func purposeMap(p consent.Purpose) map[string]any {
	versions := make([]map[string]any, 0, len(p.Versions))
	for _, v := range p.Versions {
		vm := map[string]any{"id": v.ID, "version": v.No, "consent_text": text(v.ConsentText), "change_type": v.ChangeType, "requires_reconsent": v.RequiresReconsent,
			"published_at": v.PublishedAt.UTC()}
		if v.ExplicitText.Th != "" {
			vm["explicit_text"] = text(v.ExplicitText)
		}
		versions = append(versions, vm)
	}
	m := map[string]any{"id": p.ID, "code": p.Code, "legal_entity_id": p.LegalEntityID, "status": p.Status, "is_sensitive": p.IsSensitive,
		"requires_explicit": p.RequiresExplicit, "lawful_basis": p.LawfulBasis, "live": contentMap(p.Live), "versions": versions,
		"row_version": p.RowVersion, "updated_at": p.UpdatedAt.UTC()}
	if p.CurrentVersion != nil {
		m["current_version"] = p.CurrentVersion.No
	}
	return m
}

func cpMap(cp consent.CollectionPoint) map[string]any {
	ps := make([]map[string]any, 0, len(cp.Purposes))
	for _, p := range cp.Purposes {
		ps = append(ps, map[string]any{"purpose_id": p.PurposeID, "code": p.Code, "name": text(p.Name), "status": p.Status, "is_sensitive": p.IsSensitive,
			"required": p.Required, "current_version": p.CurrentVersionNo})
	}
	m := map[string]any{"id": cp.ID, "code": cp.Code, "name": cp.Name, "channel": cp.Channel, "legal_entity_id": cp.LegalEntityID, "status": cp.Status,
		"allowed_origins": cp.AllowedOrigins, "purposes": ps, "row_version": cp.RowVersion, "updated_at": cp.UpdatedAt.UTC()}
	if cp.PublicKey != "" {
		m["public_key"] = cp.PublicKey
	}
	if cp.Checklist != nil {
		m["checklist"] = cp.Checklist
	}
	if cp.PublishedAt != nil {
		m["published_at"] = cp.PublishedAt.UTC()
	}
	return m
}

func cpWire(cp consent.CollectionPoint) (ConsentCollectionPoint, error) {
	var w ConsentCollectionPoint
	return w, convert(cpMap(cp), &w)
}

func summaryMap(s consent.SubjectSummary) map[string]any {
	ids := make([]map[string]any, 0, len(s.Identifiers))
	for _, i := range s.Identifiers {
		ids = append(ids, map[string]any{"type": i.Type, "masked": i.Masked, "primary": i.Primary, "verified": i.Verified})
	}
	m := map[string]any{"id": s.ID, "key": s.Key, "identifiers": ids, "created_at": s.CreatedAt.UTC()}
	if s.LastActivity != nil {
		m["last_activity_at"] = s.LastActivity.UTC().Format(time.RFC3339Nano)
	}
	return m
}

func convert(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
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

// ToProblem maps service errors to problem+json (shared with ../publichttp).
func ToProblem(err error) error {
	var de *consent.DecisionError
	var ce *consent.CheckError
	switch {
	case errors.As(err, &de):
		p := httpx.Problem{Status: http.StatusUnprocessableEntity, Code: "consent.invalid_decisions", Title: "Invalid decisions"}
		for _, f := range de.Fields {
			p.Errors = append(p.Errors, httpx.FieldError{Field: f.Field, Code: f.Code})
		}
		return p
	case errors.As(err, &ce):
		p := httpx.Problem{Status: http.StatusUnprocessableEntity, Code: "consent.publish_checks", Title: "Publish checks failed"}
		for _, f := range ce.Failed {
			p.Errors = append(p.Errors, httpx.FieldError{Field: "checks", Code: f})
		}
		return p
	case errors.Is(err, consent.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, consent.ErrForbidden), errors.Is(err, versioning.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, consent.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, consent.ErrInvalidTransition):
		return httpx.Problem{Status: http.StatusConflict, Code: "consent.invalid_transition", Title: "Invalid state transition"}
	case errors.Is(err, versioning.ErrLocked):
		return httpx.Problem{Status: http.StatusConflict, Code: "versioning.locked", Title: "Version locked"}
	case errors.Is(err, consent.ErrSubjectConflict):
		return httpx.Problem{Status: http.StatusConflict, Code: "consent.subject_conflict", Title: "Identifiers of different data subjects"}
	case errors.Is(err, consent.ErrInUse):
		return httpx.Problem{Status: http.StatusConflict, Code: "consent.in_use", Title: "In use"}
	case errors.Is(err, consent.ErrInvalidRequest):
		return httpx.UnprocessableEntity("consent.invalid_request", err.Error())
	}
	return err
}
