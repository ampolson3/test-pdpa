package consentpublichttp

import (
	"context"
	"encoding/json"

	consenthttp "pdpa-platform/internal/consent/http"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/publickeys"
)

// campaignKey is the key of this request, which must stand for a bulk consent request campaign (CON-24).
func campaignKey(ctx context.Context) (publickeys.Key, error) {
	k, ok := publickeys.FromContext(ctx)
	if !ok || k.EntityType != publickeys.EntityConsentCampaign {
		return publickeys.Key{}, httpx.NotFound()
	}
	return k, nil
}

func (h *Strict) ConsentViewCampaign(ctx context.Context, req ConsentViewCampaignRequestObject) (ConsentViewCampaignResponseObject, error) {
	k, err := campaignKey(ctx)
	if err != nil {
		return nil, err
	}
	lang := "th"
	if req.Params.AcceptLanguage != nil {
		lang = string(*req.Params.AcceptLanguage)
	}
	v, err := h.svc.ViewCampaignRecipient(ctx, k.EntityID, req.Body.Token, lang)
	if err != nil {
		return nil, consenthttp.ToProblem(err)
	}
	out := ConsentCampaignView{CampaignName: v.CampaignName, Responded: v.Responded, Purposes: []PublicPurpose{}}
	for _, p := range v.Purposes {
		pp := PublicPurpose{Code: p.Code, VersionNo: int(p.VersionNo), Name: p.Name, Text: p.Text, IsSensitive: p.IsSensitive, RequiresExplicit: p.IsSensitive}
		if p.ExplicitText != "" {
			pp.ExplicitText = &p.ExplicitText
		}
		out.Purposes = append(out.Purposes, pp)
	}
	return ConsentViewCampaign200JSONResponse(out), nil
}

func (h *Strict) ConsentRespondToCampaign(ctx context.Context, req ConsentRespondToCampaignRequestObject) (ConsentRespondToCampaignResponseObject, error) {
	k, err := campaignKey(ctx)
	if err != nil {
		return nil, err
	}
	lang := "th"
	if req.Params.AcceptLanguage != nil {
		lang = string(*req.Params.AcceptLanguage)
	}
	r, err := h.svc.RespondCampaignRecipient(ctx, k.EntityID, req.Body.Token, consenthttp.Decisions(req.Body.Decisions), lang)
	if err != nil {
		return nil, consenthttp.ToProblem(err)
	}
	var w ConsentResult
	bs, _ := json.Marshal(consenthttp.ResultMap(r))
	if err := json.Unmarshal(bs, &w); err != nil {
		return nil, err
	}
	return ConsentRespondToCampaign200JSONResponse(w), nil
}
