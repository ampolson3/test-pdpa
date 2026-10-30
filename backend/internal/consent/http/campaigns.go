package consenthttp

import (
	"context"

	consent "pdpa-platform/internal/consent/service"
)

func campaignWire(c consent.Campaign) (ConsentCampaign, error) {
	purposeIDs := make([]Uuid, 0, len(c.PurposeIDs))
	for _, id := range c.PurposeIDs {
		purposeIDs = append(purposeIDs, id)
	}
	w := ConsentCampaign{Id: c.ID, Name: c.Name, PurposeIds: purposeIDs, Channel: ConsentCampaignChannel(c.Channel),
		Status: ConsentCampaignStatus(c.Status), SentCount: int(c.SentCount), OpenedCount: int(c.OpenedCount),
		ResponseCount: int(c.ResponseCount), RowVersion: int(c.RowVersion)}
	if c.TemplateID != nil {
		id := Uuid(*c.TemplateID)
		w.TemplateId = &id
	}
	return w, nil
}

func (h *Strict) ConsentListCampaigns(ctx context.Context, _ ConsentListCampaignsRequestObject) (ConsentListCampaignsResponseObject, error) {
	list, err := h.svc.ListCampaigns(ctx)
	if err != nil {
		return nil, ToProblem(err)
	}
	resp := ConsentListCampaigns200JSONResponse{Data: make([]ConsentCampaign, 0, len(list))}
	for _, c := range list {
		w, err := campaignWire(c)
		if err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) ConsentCreateCampaign(ctx context.Context, req ConsentCreateCampaignRequestObject) (ConsentCreateCampaignResponseObject, error) {
	b := req.Body
	in := consent.CampaignInput{Name: b.Name, PurposeIDs: b.PurposeIds, Channel: string(b.Channel), Identifiers: Identifiers(b.Identifiers)}
	if b.TemplateId != nil {
		id := *b.TemplateId
		in.TemplateID = &id
	}
	c, err := h.svc.CreateCampaign(ctx, in)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := campaignWire(c)
	if err != nil {
		return nil, err
	}
	return ConsentCreateCampaign201JSONResponse(w), nil
}

func (h *Strict) ConsentGetCampaign(ctx context.Context, req ConsentGetCampaignRequestObject) (ConsentGetCampaignResponseObject, error) {
	c, err := h.svc.GetCampaign(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := campaignWire(c)
	if err != nil {
		return nil, err
	}
	return ConsentGetCampaign200JSONResponse(w), nil
}

func (h *Strict) ConsentSendCampaign(ctx context.Context, req ConsentSendCampaignRequestObject) (ConsentSendCampaignResponseObject, error) {
	lang := "th"
	if req.Params.AcceptLanguage != nil {
		lang = string(*req.Params.AcceptLanguage)
	}
	c, err := h.svc.SendCampaign(ctx, req.Id, lang)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := campaignWire(c)
	if err != nil {
		return nil, err
	}
	return ConsentSendCampaign200JSONResponse(w), nil
}

func (h *Strict) ConsentResendCampaign(ctx context.Context, req ConsentResendCampaignRequestObject) (ConsentResendCampaignResponseObject, error) {
	lang := "th"
	if req.Params.AcceptLanguage != nil {
		lang = string(*req.Params.AcceptLanguage)
	}
	c, err := h.svc.ResendCampaign(ctx, req.Id, lang)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := campaignWire(c)
	if err != nil {
		return nil, err
	}
	return ConsentResendCampaign200JSONResponse(w), nil
}
