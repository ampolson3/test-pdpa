// Package consentpublichttp holds the /public/v1 consent endpoints (BP-01): a published collection point's form
// and the submission of its decisions. The tenant comes from the public key (publickeys.Middleware), not a token.
// Types in public.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package consentpublichttp

import (
	"context"
	"encoding/json"
	"net/http"

	consenthttp "pdpa-platform/internal/consent/http"
	consent "pdpa-platform/internal/consent/service"
	"pdpa-platform/internal/pkg/clientip"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/publickeys"
)

type Strict struct {
	svc *consent.Service
}

func NewStrict(svc *consent.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

type uaKey struct{}

// RequestInfo is a strict middleware that keeps the request's user agent for the consent transaction's evidence.
func RequestInfo(f StrictHandlerFunc, _ string) StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
		return f(context.WithValue(ctx, uaKey{}, r.UserAgent()), w, r, req)
	}
}

// collectionPoint is the key of this request, which must stand for a collection point.
func collectionPoint(ctx context.Context) (publickeys.Key, error) {
	k, ok := publickeys.FromContext(ctx)
	if !ok || k.EntityType != publickeys.EntityCollectionPoint {
		return publickeys.Key{}, httpx.NotFound()
	}
	return k, nil
}

func (h *Strict) ConsentGetPublicCollectionPoint(ctx context.Context, req ConsentGetPublicCollectionPointRequestObject) (ConsentGetPublicCollectionPointResponseObject, error) {
	k, err := collectionPoint(ctx)
	if err != nil {
		return nil, err
	}
	lang := "th"
	if req.Params.AcceptLanguage != nil {
		lang = string(*req.Params.AcceptLanguage)
	}
	v, err := h.svc.PublicView(ctx, k.EntityID, lang)
	if err != nil {
		return nil, consenthttp.ToProblem(err)
	}
	out := PublicCollectionPoint{Code: v.Code, Name: v.Name, Channel: PublicCollectionPointChannel(v.Channel),
		DoubleOptIn: false, VerificationMethod: "none", Purposes: []PublicPurpose{}}
	ageGate := false
	for _, p := range v.Purposes {
		pp := PublicPurpose{Code: p.Code, VersionNo: int(p.VersionNo), Name: p.Name, Text: p.Text,
			IsSensitive: p.IsSensitive, RequiresExplicit: p.IsSensitive, Required: &p.Required}
		if p.Description != "" {
			pp.Description = &p.Description
		}
		if p.ExplicitText != "" {
			pp.ExplicitText = &p.ExplicitText
		}
		if p.MinAge != nil {
			a := int(*p.MinAge)
			pp.MinAge = &a
			ageGate = true
		}
		if len(p.Preferences) > 0 {
			b, _ := json.Marshal(prefsWire(p.Preferences))
			var prefs []struct {
				Code    string `json:"code"`
				Name    string `json:"name"`
				Options *[]struct {
					Label string `json:"label"`
					Value string `json:"value"`
				} `json:"options,omitempty"`
				PrefType PublicPurposePreferencesPrefType `json:"pref_type"`
			}
			if err := json.Unmarshal(b, &prefs); err != nil {
				return nil, err
			}
			pp.Preferences = &prefs
		}
		out.Purposes = append(out.Purposes, pp)
	}
	out.AgeGate = &ageGate
	return ConsentGetPublicCollectionPoint200JSONResponse{Body: out}, nil
}

func prefsWire(in []consent.PublicPreference) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, p := range in {
		m := map[string]any{"code": p.Code, "name": p.Name, "pref_type": p.Type}
		if len(p.Options) > 0 {
			opts := make([]map[string]string, 0, len(p.Options))
			for _, o := range p.Options {
				opts = append(opts, map[string]string{"value": o.Value, "label": o.Label})
			}
			m["options"] = opts
		}
		out = append(out, m)
	}
	return out
}

func (h *Strict) ConsentSubmitPublicConsent(ctx context.Context, req ConsentSubmitPublicConsentRequestObject) (ConsentSubmitPublicConsentResponseObject, error) {
	k, err := collectionPoint(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	source := "web"
	if cp, err := h.svc.GetCollectionPoint(ctx, k.EntityID); err == nil && cp.Channel == "app" {
		source = "app"
	}
	sub := consent.Submission{
		CollectionPointID: k.EntityID,
		Identifiers:       consenthttp.Identifiers(b.Subject.Identifiers),
		Decisions:         consenthttp.Decisions(b.Decisions),
		Source:            source,
		IdempotencyKey:    req.Params.IdempotencyKey,
		Public:            true,
	}
	if b.Language != nil {
		sub.Language = string(*b.Language)
	}
	if ip, ok := clientip.From(ctx); ok {
		sub.IP = &ip
	}
	if ua, ok := ctx.Value(uaKey{}).(string); ok {
		sub.UserAgent = ua
	}
	r, err := h.svc.Record(ctx, sub)
	if err != nil {
		return nil, consenthttp.ToProblem(err)
	}
	var w ConsentResult
	bs, _ := json.Marshal(consenthttp.ResultMap(r))
	if err := json.Unmarshal(bs, &w); err != nil {
		return nil, err
	}
	return ConsentSubmitPublicConsent201JSONResponse(w), nil
}
