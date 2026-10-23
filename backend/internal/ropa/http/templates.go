package ropahttp

import (
	"context"
	"encoding/json"

	ropaservice "pdpa-platform/internal/ropa/service"
	templatesservice "pdpa-platform/internal/ropa/templates"
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
	return w
}

func toActivityTemplateWire(a templatesservice.ActivityTemplate) (RopaActivityTemplate, error) {
	w := RopaActivityTemplate{
		Id: a.ID, TemplateSetId: a.TemplateSetID, Code: a.Code, NameTh: a.NameTh, NameEn: ptr(a.NameEn),
		JobCategory: a.JobCategory, Role: RopaActivityTemplateRole(a.Role), LegalRefs: a.LegalRefs, VersionNo: a.VersionNo,
	}
	if err := json.Unmarshal(a.Defaults, &w.Defaults); err != nil {
		return RopaActivityTemplate{}, err
	}
	if err := json.Unmarshal(a.Rationale, &w.Rationale); err != nil {
		return RopaActivityTemplate{}, err
	}
	return w, nil
}
