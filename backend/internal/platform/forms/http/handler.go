// Package formshttp holds the PLT-06 form & assessment engine endpoints. Types in forms.gen.go are generated
// from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml). Every endpoint is `authenticated`:
// the permission comes from the form type's policy and is checked in the service.
package formshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/forms"
)

type Strict struct {
	svc *forms.Service
}

func NewStrict(svc *forms.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

// ---- definitions ----

func (h *Strict) PlatformListFormTypes(ctx context.Context, _ PlatformListFormTypesRequestObject) (PlatformListFormTypesResponseObject, error) {
	resp := PlatformListFormTypes200JSONResponse{Data: []FormTypeInfo{}}
	for _, t := range h.svc.TypeInfos(ctx) {
		resp.Data = append(resp.Data, FormTypeInfo{FormType: t.Type, CanCreate: t.CanCreate, CanUpdate: t.CanUpdate, CanPublish: t.CanPublish, CanRespond: t.CanRespond})
	}
	return resp, nil
}

func (h *Strict) PlatformListForms(ctx context.Context, req PlatformListFormsRequestObject) (PlatformListFormsResponseObject, error) {
	formType := ""
	if req.Params.FormType != nil {
		formType = *req.Params.FormType
	}
	list, err := h.svc.ListForms(ctx, formType)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformListForms200JSONResponse{Data: make([]FormSummary, 0, len(list))}
	for _, f := range list {
		var w FormSummary
		if err := convert(summaryMap(f), &w); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) PlatformCreateForm(ctx context.Context, req PlatformCreateFormRequestObject) (PlatformCreateFormResponseObject, error) {
	d, err := draft(req.Body.Draft)
	if err != nil {
		return nil, err
	}
	f, err := h.svc.CreateForm(ctx, req.Body.Code, req.Body.Name, req.Body.FormType, d)
	if err != nil {
		return nil, problem(err)
	}
	w, err := formWire(f)
	if err != nil {
		return nil, err
	}
	return PlatformCreateForm201JSONResponse{Body: w, Headers: PlatformCreateForm201ResponseHeaders{ETag: etag(f.RowVersion)}}, nil
}

func (h *Strict) PlatformGetForm(ctx context.Context, req PlatformGetFormRequestObject) (PlatformGetFormResponseObject, error) {
	f, err := h.svc.GetForm(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	w, err := formWire(f)
	if err != nil {
		return nil, err
	}
	return PlatformGetForm200JSONResponse(w), nil
}

func (h *Strict) PlatformCreateFormDraft(ctx context.Context, req PlatformCreateFormDraftRequestObject) (PlatformCreateFormDraftResponseObject, error) {
	d, err := draft(*req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.SaveDraft(ctx, req.Id, 0, d)
	if errors.Is(err, forms.ErrVersionMismatch) {
		return nil, httpx.Problem{Status: http.StatusConflict, Code: "forms.draft_exists", Title: "Draft exists"}
	}
	if err != nil {
		return nil, problem(err)
	}
	w, err := versionWire(v)
	if err != nil {
		return nil, err
	}
	return PlatformCreateFormDraft201JSONResponse{Body: w, Headers: PlatformCreateFormDraft201ResponseHeaders{ETag: etag(v.RowVersion)}}, nil
}

func (h *Strict) PlatformSaveFormDraft(ctx context.Context, req PlatformSaveFormDraftRequestObject) (PlatformSaveFormDraftResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil || ver == 0 {
		return nil, httpx.VersionMismatch()
	}
	d, err := draft(*req.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.SaveDraft(ctx, req.Id, ver, d)
	if err != nil {
		return nil, problem(err)
	}
	w, err := versionWire(v)
	if err != nil {
		return nil, err
	}
	return PlatformSaveFormDraft200JSONResponse{Body: w, Headers: PlatformSaveFormDraft200ResponseHeaders{ETag: etag(v.RowVersion)}}, nil
}

func (h *Strict) PlatformPublishForm(ctx context.Context, req PlatformPublishFormRequestObject) (PlatformPublishFormResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	f, err := h.svc.Publish(ctx, req.Id, ver)
	if err != nil {
		return nil, problem(err)
	}
	w, err := formWire(f)
	if err != nil {
		return nil, err
	}
	return PlatformPublishForm200JSONResponse(w), nil
}

// ---- responses ----

func (h *Strict) PlatformListFormResponses(ctx context.Context, req PlatformListFormResponsesRequestObject) (PlatformListFormResponsesResponseObject, error) {
	list, err := h.svc.ListResponses(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformListFormResponses200JSONResponse{Data: make([]FormResponseSummary, 0, len(list))}
	for _, r := range list {
		var w FormResponseSummary
		if err := convert(responseSummaryMap(r), &w); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) PlatformStartFormResponse(ctx context.Context, req PlatformStartFormResponseRequestObject) (PlatformStartFormResponseResponseObject, error) {
	entityType := ""
	var entityID *Uuid
	if req.Body != nil {
		if req.Body.EntityType != nil {
			entityType = *req.Body.EntityType
		}
		entityID = req.Body.EntityId
	}
	if (entityType == "") != (entityID == nil) {
		return nil, httpx.UnprocessableEntity("forms.invalid_request", "entity_type and entity_id go together")
	}
	r, err := h.svc.StartResponse(ctx, req.Id, entityType, entityID)
	if err != nil {
		return nil, problem(err)
	}
	w, err := responseWire(r)
	if err != nil {
		return nil, err
	}
	return PlatformStartFormResponse201JSONResponse{Body: w, Headers: PlatformStartFormResponse201ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func (h *Strict) PlatformGetFormResponse(ctx context.Context, req PlatformGetFormResponseRequestObject) (PlatformGetFormResponseResponseObject, error) {
	r, err := h.svc.GetResponse(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	w, err := responseWire(r)
	if err != nil {
		return nil, err
	}
	return PlatformGetFormResponse200JSONResponse{Body: w, Headers: PlatformGetFormResponse200ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func (h *Strict) PlatformSaveFormAnswers(ctx context.Context, req PlatformSaveFormAnswersRequestObject) (PlatformSaveFormAnswersResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	r, err := h.svc.SaveAnswers(ctx, req.Id, ver, forms.Answers(req.Body.Answers))
	if err != nil {
		return nil, problem(err)
	}
	w, err := responseWire(r)
	if err != nil {
		return nil, err
	}
	return PlatformSaveFormAnswers200JSONResponse{Body: w, Headers: PlatformSaveFormAnswers200ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func (h *Strict) PlatformAssignFormSection(ctx context.Context, req PlatformAssignFormSectionRequestObject) (PlatformAssignFormSectionResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	r, err := h.svc.Assign(ctx, req.Id, ver, req.Section, req.Body.AssigneeUserId)
	if err != nil {
		return nil, problem(err)
	}
	w, err := responseWire(r)
	if err != nil {
		return nil, err
	}
	return PlatformAssignFormSection200JSONResponse{Body: w, Headers: PlatformAssignFormSection200ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func (h *Strict) PlatformCompleteFormSection(ctx context.Context, req PlatformCompleteFormSectionRequestObject) (PlatformCompleteFormSectionResponseObject, error) {
	r, err := h.svc.CompleteSection(ctx, req.Id, req.Section)
	if err != nil {
		return nil, problem(err)
	}
	w, err := responseWire(r)
	if err != nil {
		return nil, err
	}
	return PlatformCompleteFormSection200JSONResponse{Body: w, Headers: PlatformCompleteFormSection200ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func (h *Strict) PlatformSubmitFormResponse(ctx context.Context, req PlatformSubmitFormResponseRequestObject) (PlatformSubmitFormResponseResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	r, err := h.svc.Submit(ctx, req.Id, ver)
	if err != nil {
		return nil, problem(err)
	}
	w, err := responseWire(r)
	if err != nil {
		return nil, err
	}
	return PlatformSubmitFormResponse200JSONResponse{Body: w, Headers: PlatformSubmitFormResponse200ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func (h *Strict) PlatformListMyFormSections(ctx context.Context, _ PlatformListMyFormSectionsRequestObject) (PlatformListMyFormSectionsResponseObject, error) {
	items, err := h.svc.MyAssignments(ctx)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformListMyFormSections200JSONResponse{Data: make([]FormSectionAssignmentItem, 0, len(items))}
	for _, it := range items {
		var w FormSectionAssignmentItem
		if err := convert(map[string]any{"response_id": it.ResponseID, "form_id": it.FormID, "form_name": it.FormName, "form_type": it.FormType,
			"section": it.Section, "section_title": it.SectionTitle, "created_at": it.CreatedAt.UTC()}, &w); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

// ---- wire ----

func draft(in FormDraft) (forms.Draft, error) {
	var d forms.Draft
	if err := convert(in.Schema, &d.Schema); err != nil {
		return d, err
	}
	if in.Scoring != nil {
		d.Scoring = &forms.Scoring{}
		if err := convert(in.Scoring, d.Scoring); err != nil {
			return d, err
		}
	}
	if in.Languages != nil {
		for _, l := range *in.Languages {
			d.Languages = append(d.Languages, string(l))
		}
	}
	return d, nil
}

func summaryMap(f forms.Form) map[string]any {
	m := map[string]any{"id": f.ID, "global": f.Global, "code": f.Code, "name": f.Name, "form_type": f.Type, "status": f.Status,
		"latest_version": f.LatestVersion, "row_version": f.RowVersion, "updated_at": f.UpdatedAt.UTC()}
	if f.CurrentVersionID != nil {
		m["current_version_id"] = f.CurrentVersionID
	}
	return m
}

func versionMap(v forms.Version) map[string]any {
	m := map[string]any{"id": v.ID, "form_id": v.FormID, "version": v.No, "schema": v.Schema, "languages": v.Languages,
		"row_version": v.RowVersion, "updated_at": v.UpdatedAt.UTC()}
	if v.Scoring != nil {
		m["scoring"] = v.Scoring
	}
	if v.PublishedAt != nil {
		m["published_at"] = v.PublishedAt.UTC()
	}
	return m
}

func formWire(f forms.Form) (Form, error) {
	m := summaryMap(f)
	versions := make([]map[string]any, 0, len(f.Versions))
	for _, v := range f.Versions {
		versions = append(versions, versionMap(v))
	}
	m["versions"] = versions
	var w Form
	return w, convert(m, &w)
}

func versionWire(v forms.Version) (FormVersion, error) {
	var w FormVersion
	return w, convert(versionMap(v), &w)
}

func resultMap(r forms.Result) map[string]any {
	errs := r.Errors
	if errs == nil {
		errs = []forms.FieldError{}
	}
	visible := r.Visible
	if visible == nil {
		visible = []string{}
	}
	answers := r.Answers
	if answers == nil {
		answers = forms.Answers{}
	}
	m := map[string]any{"visible": visible, "answers": answers, "errors": errs, "score": r.Score, "max_score": r.MaxScore}
	if r.Band != "" {
		m["band"] = r.Band
	}
	return m
}

func responseSummaryMap(r forms.Response) map[string]any {
	m := map[string]any{"id": r.ID, "status": r.Status, "row_version": r.RowVersion, "result": resultMap(r.Result)}
	if r.OwnerID != nil {
		m["owner_id"] = r.OwnerID
		if r.OwnerName != "" {
			m["owner_name"] = r.OwnerName
		}
	}
	if r.SubmittedAt != nil {
		m["submitted_at"] = r.SubmittedAt.UTC().Format(time.RFC3339Nano)
	}
	return m
}

func responseWire(r forms.Response) (FormResponse, error) {
	m := responseSummaryMap(r)
	m["form"], m["version"] = summaryMap(r.Form), versionMap(r.Version)
	answers := r.Answers
	if answers == nil {
		answers = forms.Answers{}
	}
	m["answers"], m["is_owner"] = answers, r.IsOwner
	canAnswer := r.CanAnswer
	if canAnswer == nil {
		canAnswer = []string{}
	}
	m["can_answer"] = canAnswer
	assignments := make([]map[string]any, 0, len(r.Assignments))
	for _, a := range r.Assignments {
		am := map[string]any{"id": a.ID, "section": a.Section, "assignee_user_id": a.AssigneeID, "assignee_name": a.AssigneeName, "status": a.Status, "row_version": a.RowVersion}
		if a.CompletedAt != nil {
			am["completed_at"] = a.CompletedAt.UTC().Format(time.RFC3339Nano)
		}
		assignments = append(assignments, am)
	}
	m["assignments"] = assignments
	if r.EntityType != "" {
		m["entity_type"], m["entity_id"] = r.EntityType, r.EntityID
	}
	var w FormResponse
	return w, convert(m, &w)
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

func problem(err error) error {
	var ae *forms.AnswerErrors
	switch {
	case errors.As(err, &ae):
		p := httpx.Problem{Status: http.StatusUnprocessableEntity, Code: "forms.invalid_answers", Title: "Invalid answers"}
		for _, f := range ae.Fields {
			p.Errors = append(p.Errors, httpx.FieldError{Field: f.Question, Code: f.Code})
		}
		return p
	case errors.Is(err, forms.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, forms.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, forms.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, forms.ErrInvalidState):
		return httpx.Problem{Status: http.StatusConflict, Code: "forms.invalid_state", Title: "Not allowed in the current state"}
	case errors.Is(err, forms.ErrUnknownType):
		return httpx.UnprocessableEntity("forms.unknown_type", err.Error())
	case errors.Is(err, forms.ErrInvalidSchema):
		return httpx.UnprocessableEntity("forms.invalid_schema", err.Error())
	case errors.Is(err, forms.ErrInvalidRequest):
		return httpx.UnprocessableEntity("forms.invalid_request", err.Error())
	}
	return err
}
