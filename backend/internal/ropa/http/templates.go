package ropahttp

import (
	"context"
	"encoding/json"
	"errors"

	ropaservice "pdpa-platform/internal/ropa/service"
	templatesservice "pdpa-platform/internal/ropa/templates"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) RopaListTemplateSets(ctx context.Context, req RopaListTemplateSetsRequestObject) (RopaListTemplateSetsResponseObject, error) {
	list, err := h.templates.ListTemplateSets(ctx)
	if err != nil {
		return nil, err
	}
	resp := RopaListTemplateSets200JSONResponse{Data: make([]RopaTemplateSet, 0, len(list))}
	for _, s := range list {
		resp.Data = append(resp.Data, toTemplateSetWire(s))
	}
	return resp, nil
}

func (h *Strict) RopaListActivityTemplates(ctx context.Context, req RopaListActivityTemplatesRequestObject) (RopaListActivityTemplatesResponseObject, error) {
	var jobCategory *string
	if req.Params.JobCategory != nil {
		jobCategory = req.Params.JobCategory
	}
	list, err := h.templates.ListActivityTemplates(ctx, req.Id, jobCategory)
	if err != nil {
		return nil, err
	}
	resp := RopaListActivityTemplates200JSONResponse{Data: make([]RopaActivityTemplate, 0, len(list))}
	for _, a := range list {
		w, err := toActivityTemplateWire(a)
		if err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) RopaGetActivityTemplate(ctx context.Context, req RopaGetActivityTemplateRequestObject) (RopaGetActivityTemplateResponseObject, error) {
	a, err := h.templates.GetActivityTemplate(ctx, req.ActivityTemplateId)
	if err != nil {
		return nil, problem(err)
	}
	w, err := toActivityTemplateWire(a)
	if err != nil {
		return nil, err
	}
	return RopaGetActivityTemplate200JSONResponse(w), nil
}

// RTG-08: this tenant's own custom template sets, any status — the management list.
func (h *Strict) RopaListOwnTemplateSets(ctx context.Context, req RopaListOwnTemplateSetsRequestObject) (RopaListOwnTemplateSetsResponseObject, error) {
	list, err := h.templates.ListOwnTemplateSets(ctx)
	if err != nil {
		return nil, err
	}
	resp := RopaListOwnTemplateSets200JSONResponse{Data: make([]RopaTemplateSet, 0, len(list))}
	for _, s := range list {
		resp.Data = append(resp.Data, toTemplateSetWire(s))
	}
	return resp, nil
}

func (h *Strict) RopaCreateTemplateSet(ctx context.Context, req RopaCreateTemplateSetRequestObject) (RopaCreateTemplateSetResponseObject, error) {
	s, err := h.templates.CreateTemplateSet(ctx, req.Body.Name)
	if err != nil {
		return nil, templatesProblem(err)
	}
	return RopaCreateTemplateSet201JSONResponse{Body: toTemplateSetWire(s), Headers: RopaCreateTemplateSet201ResponseHeaders{ETag: etag(s.RowVersion)}}, nil
}

func (h *Strict) RopaCloneTemplateSet(ctx context.Context, req RopaCloneTemplateSetRequestObject) (RopaCloneTemplateSetResponseObject, error) {
	s, err := h.templates.CloneTemplateSet(ctx, req.Id, req.Body.Name)
	if err != nil {
		return nil, templatesProblem(err)
	}
	return RopaCloneTemplateSet201JSONResponse{Body: toTemplateSetWire(s), Headers: RopaCloneTemplateSet201ResponseHeaders{ETag: etag(s.RowVersion)}}, nil
}

func (h *Strict) RopaPublishTemplateSet(ctx context.Context, req RopaPublishTemplateSetRequestObject) (RopaPublishTemplateSetResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	s, err := h.templates.PublishTemplateSet(ctx, req.Id, v)
	if err != nil {
		return nil, templatesProblem(err)
	}
	return RopaPublishTemplateSet200JSONResponse{Body: toTemplateSetWire(s), Headers: RopaPublishTemplateSet200ResponseHeaders{ETag: etag(s.RowVersion)}}, nil
}

func (h *Strict) RopaRetireTemplateSet(ctx context.Context, req RopaRetireTemplateSetRequestObject) (RopaRetireTemplateSetResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	s, err := h.templates.RetireTemplateSet(ctx, req.Id, v)
	if err != nil {
		return nil, templatesProblem(err)
	}
	return RopaRetireTemplateSet200JSONResponse{Body: toTemplateSetWire(s), Headers: RopaRetireTemplateSet200ResponseHeaders{ETag: etag(s.RowVersion)}}, nil
}

func (h *Strict) RopaAddActivityTemplate(ctx context.Context, req RopaAddActivityTemplateRequestObject) (RopaAddActivityTemplateResponseObject, error) {
	in, err := toActivityTemplateInput(*req.Body)
	if err != nil {
		return nil, err
	}
	a, err := h.templates.AddActivityTemplate(ctx, req.Id, in)
	if err != nil {
		return nil, templatesProblem(err)
	}
	w, err := toActivityTemplateWire(a)
	if err != nil {
		return nil, err
	}
	return RopaAddActivityTemplate201JSONResponse{Body: w, Headers: RopaAddActivityTemplate201ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaUpdateActivityTemplate(ctx context.Context, req RopaUpdateActivityTemplateRequestObject) (RopaUpdateActivityTemplateResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in, err := toActivityTemplateInput(*req.Body)
	if err != nil {
		return nil, err
	}
	a, err := h.templates.UpdateActivityTemplate(ctx, req.ActivityTemplateId, in, v)
	if err != nil {
		return nil, templatesProblem(err)
	}
	w, err := toActivityTemplateWire(a)
	if err != nil {
		return nil, err
	}
	return RopaUpdateActivityTemplate200JSONResponse{Body: w, Headers: RopaUpdateActivityTemplate200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaDeleteActivityTemplate(ctx context.Context, req RopaDeleteActivityTemplateRequestObject) (RopaDeleteActivityTemplateResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	if err := h.templates.DeleteActivityTemplate(ctx, req.ActivityTemplateId, v); err != nil {
		return nil, templatesProblem(err)
	}
	return RopaDeleteActivityTemplate204Response{}, nil
}

func (h *Strict) RopaSuggestTemplateItems(ctx context.Context, req RopaSuggestTemplateItemsRequestObject) (RopaSuggestTemplateItemsResponseObject, error) {
	sugg, err := h.svc.Suggest(ctx, req.ActivityTemplateId)
	if err != nil {
		return nil, problem(err)
	}
	return RopaSuggestTemplateItems200JSONResponse(toTemplateSuggestionsWire(sugg)), nil
}

func toTemplateSuggestionsWire(sugg ropaservice.TemplateSuggestions) RopaTemplateSuggestions {
	w := RopaTemplateSuggestions{TemplateId: sugg.TemplateID, TemplateCode: sugg.TemplateCode, TemplateName: sugg.TemplateName}
	for i, p := range sugg.Purposes {
		w.Purposes = append(w.Purposes, RopaSuggestedPurpose{
			Index: i, PurposeText: p.PurposeText, LawfulBasisCode: p.LawfulBasisCode, LawfulBasisNameTh: p.LawfulBasisNameTh, Rationale: p.Rationale,
		})
	}
	for i, d := range sugg.Data {
		w.Data = append(w.Data, RopaSuggestedData{
			Index: i, DataCategoryCode: d.DataCategoryCode, DataCategoryName: d.DataCategoryName, SubjectTypeCode: d.SubjectTypeCode,
			SubjectTypeName: d.SubjectTypeName, IsSensitive: d.IsSensitive, Source: RopaSuggestedDataSource(d.Source), Rationale: d.Rationale,
		})
	}
	for i, r := range sugg.Retention {
		w.Retention = append(w.Retention, RopaSuggestedRetention{
			Index: i, RetentionMonths: r.RetentionMonths, RetentionBasis: r.RetentionBasis, TriggerEvent: r.TriggerEvent,
			DisposalMethod: r.DisposalMethod, Rationale: r.Rationale,
		})
	}
	return w
}

func toTemplateSetWire(s templatesservice.TemplateSet) RopaTemplateSet {
	w := RopaTemplateSet{Id: s.ID, Name: s.Name, SetType: RopaTemplateSetSetType(s.SetType), VersionNo: s.VersionNo}
	if s.Industry != "" {
		w.Industry = ptr(s.Industry)
	}
	if s.Status != "" {
		st := RopaTemplateSetStatus(s.Status)
		w.Status = &st
		rv := int(s.RowVersion)
		w.RowVersion = &rv
	}
	return w
}

func toActivityTemplateWire(a templatesservice.ActivityTemplate) (RopaActivityTemplate, error) {
	w := RopaActivityTemplate{
		Id: a.ID, TemplateSetId: a.TemplateSetID, Code: a.Code, NameTh: a.NameTh, NameEn: ptr(a.NameEn),
		JobCategory: a.JobCategory, Role: RopaActivityTemplateRole(a.Role), LegalRefs: a.LegalRefs, VersionNo: a.VersionNo,
		RowVersion: ptrInt(int(a.RowVersion)),
	}
	if err := json.Unmarshal(a.Defaults, &w.Defaults); err != nil {
		return RopaActivityTemplate{}, err
	}
	if err := json.Unmarshal(a.Rationale, &w.Rationale); err != nil {
		return RopaActivityTemplate{}, err
	}
	return w, nil
}

func ptrInt(v int) *int { return &v }

// toActivityTemplateInput decodes RTG-08's own write body back into the service's input shape — the
// mirror of toActivityTemplateWire.
func toActivityTemplateInput(in RopaActivityTemplateInput) (templatesservice.ActivityTemplateInput, error) {
	defaults, err := json.Marshal(in.Defaults)
	if err != nil {
		return templatesservice.ActivityTemplateInput{}, err
	}
	rationale, err := json.Marshal(in.Rationale)
	if err != nil {
		return templatesservice.ActivityTemplateInput{}, err
	}
	legalRefs := []string{}
	if in.LegalRefs != nil {
		legalRefs = *in.LegalRefs
	}
	nameEn := ""
	if in.NameEn != nil {
		nameEn = *in.NameEn
	}
	return templatesservice.ActivityTemplateInput{
		Code: in.Code, NameTh: in.NameTh, NameEn: nameEn, JobCategory: in.JobCategory, Role: string(in.Role),
		Defaults: defaults, Rationale: rationale, LegalRefs: legalRefs,
	}, nil
}

// templatesProblem maps RTG-08's own write-path sentinels on top of the module's shared problem() mapping.
func templatesProblem(err error) error {
	switch {
	case errors.Is(err, templatesservice.ErrInvalid):
		return httpx.UnprocessableEntity("ropa.invalid_input", err.Error())
	case errors.Is(err, templatesservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	}
	return problem(err)
}
