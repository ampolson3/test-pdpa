package dpiahttp

import (
	"context"

	dpiaservice "pdpa-platform/internal/dpia/service"
	"pdpa-platform/internal/platform/forms"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DpiaListTemplates(ctx context.Context, req DpiaListTemplatesRequestObject) (DpiaListTemplatesResponseObject, error) {
	at := ""
	if req.Params.AssessmentType != nil {
		at = *req.Params.AssessmentType
	}
	list, err := h.svc.ListTemplates(ctx, at)
	if err != nil {
		return nil, err
	}
	resp := DpiaListTemplates200JSONResponse{Data: make([]DpiaTemplate, 0, len(list))}
	for _, t := range list {
		resp.Data = append(resp.Data, toTemplateWire(t))
	}
	return resp, nil
}

func (h *Strict) DpiaCreateTemplate(ctx context.Context, req DpiaCreateTemplateRequestObject) (DpiaCreateTemplateResponseObject, error) {
	legalRefs := []string{}
	if req.Body.LegalRefs != nil {
		legalRefs = *req.Body.LegalRefs
	}
	t, err := h.svc.CreateTemplate(ctx, req.Body.AssessmentType, req.Body.Code, req.Body.Name, legalRefs, toFormDraft(req.Body.Draft))
	if err != nil {
		return nil, problem(err)
	}
	return DpiaCreateTemplate201JSONResponse(toTemplateWire(t)), nil
}

func (h *Strict) DpiaGetTemplate(ctx context.Context, req DpiaGetTemplateRequestObject) (DpiaGetTemplateResponseObject, error) {
	t, err := h.svc.GetTemplateByID(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaGetTemplate200JSONResponse(toTemplateWire(t)), nil
}

func (h *Strict) DpiaCloneTemplate(ctx context.Context, req DpiaCloneTemplateRequestObject) (DpiaCloneTemplateResponseObject, error) {
	t, err := h.svc.CloneTemplate(ctx, req.Id, req.Body.Code, req.Body.Name)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaCloneTemplate201JSONResponse(toTemplateWire(t)), nil
}

func (h *Strict) DpiaPublishTemplate(ctx context.Context, req DpiaPublishTemplateRequestObject) (DpiaPublishTemplateResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.RequestInvalid("If-Match")
	}
	t, err := h.svc.PublishTemplate(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaPublishTemplate200JSONResponse(toTemplateWire(t)), nil
}

func (h *Strict) DpiaRetireTemplate(ctx context.Context, req DpiaRetireTemplateRequestObject) (DpiaRetireTemplateResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.RequestInvalid("If-Match")
	}
	t, err := h.svc.RetireTemplate(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaRetireTemplate200JSONResponse(toTemplateWire(t)), nil
}

func toFormDraft(d FormDraft) forms.Draft {
	out := forms.Draft{Schema: toFormSchema(d.Schema)}
	if d.Scoring != nil {
		s := toFormScoring(*d.Scoring)
		out.Scoring = &s
	}
	if d.Languages != nil {
		for _, l := range *d.Languages {
			out.Languages = append(out.Languages, string(l))
		}
	}
	return out
}

func toFormSchema(s FormSchema) forms.Schema {
	out := forms.Schema{}
	for _, sec := range s.Sections {
		out.Sections = append(out.Sections, toFormSection(sec))
	}
	return out
}

func toFormSection(sec FormSection) forms.Section {
	out := forms.Section{Key: sec.Key, Title: toFormText(sec.Title)}
	if sec.Description != nil {
		out.Description = toFormText(*sec.Description)
	}
	if sec.VisibleIf != nil {
		c := toFormCondition(*sec.VisibleIf)
		out.VisibleIf = &c
	}
	for _, q := range sec.Questions {
		out.Questions = append(out.Questions, toFormQuestion(q))
	}
	return out
}

func toFormQuestion(q FormQuestion) forms.Question {
	out := forms.Question{Key: q.Key, Type: string(q.Type), Label: toFormText(q.Label)}
	if q.Help != nil {
		out.Help = toFormText(*q.Help)
	}
	if q.Required != nil {
		out.Required = *q.Required
	}
	if q.Options != nil {
		for _, o := range *q.Options {
			opt := forms.Option{Value: o.Value}
			if o.Label != nil {
				opt.Label = toFormText(*o.Label)
			}
			opt.Score = f64(o.Score)
			out.Options = append(out.Options, opt)
		}
	}
	out.Min = f64(q.Min)
	out.Max = f64(q.Max)
	if q.MaxLength != nil {
		out.MaxLength = *q.MaxLength
	}
	out.Weight = f64(q.Weight)
	if q.VisibleIf != nil {
		c := toFormCondition(*q.VisibleIf)
		out.VisibleIf = &c
	}
	return out
}

func toFormCondition(c FormCondition) forms.Condition {
	out := forms.Condition{}
	if c.Question != nil {
		out.Question = *c.Question
	}
	if c.Op != nil {
		out.Op = string(*c.Op)
	}
	out.Value = c.Value
	if c.All != nil {
		for _, n := range *c.All {
			out.All = append(out.All, toFormCondition(n))
		}
	}
	if c.Any != nil {
		for _, n := range *c.Any {
			out.Any = append(out.Any, toFormCondition(n))
		}
	}
	return out
}

func toFormScoring(s FormScoring) forms.Scoring {
	out := forms.Scoring{}
	for _, b := range s.Bands {
		band := forms.Band{Key: b.Key, Label: toFormText(b.Label), Min: float64(b.Min), Max: f64(b.Max)}
		out.Bands = append(out.Bands, band)
	}
	return out
}

func toFormText(t FormText) forms.Text {
	out := forms.Text{"th": t.Th}
	if t.En != nil {
		out["en"] = *t.En
	}
	return out
}

func f64(f *float32) *float64 {
	if f == nil {
		return nil
	}
	v := float64(*f)
	return &v
}

func toTemplateWire(t dpiaservice.Template) DpiaTemplate {
	return DpiaTemplate{Id: t.ID, AssessmentType: t.AssessmentType, Code: t.Code, Name: t.Name, FormId: t.FormID,
		VersionNo: t.VersionNo, LegalRefs: &t.LegalRefs, Status: DpiaTemplateStatus(t.Status), RowVersion: int(t.RowVersion), CreatedAt: t.CreatedAt.UTC()}
}
