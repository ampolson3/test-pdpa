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

// preferenceCenterKey is the key of this request, which must stand for a published preference centre
// (CON-18) — the key's EntityID is the legal entity it was issued for, not a dedicated table's own id.
func preferenceCenterKey(ctx context.Context) (publickeys.Key, error) {
	k, ok := publickeys.FromContext(ctx)
	if !ok || k.EntityType != publickeys.EntityPreferenceCenter {
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
		DoubleOptIn: v.DoubleOptIn, VerificationMethod: PublicCollectionPointVerificationMethod(v.VerificationMethod), Purposes: []PublicPurpose{}}
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
	if b.Subject.IsMinor != nil {
		sub.IsMinor = *b.Subject.IsMinor
	}
	if b.Subject.Guardian != nil {
		sub.Guardian = &consent.Guardian{Identifiers: consenthttp.Identifiers(b.Subject.Guardian.Identifiers), Relationship: string(b.Subject.Guardian.Relationship)}
	}
	if b.Verification != nil {
		sub.VerificationID = &b.Verification.VerificationId
		sub.VerificationCode = b.Verification.Code
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

func (h *Strict) ConsentStartVerification(ctx context.Context, req ConsentStartVerificationRequestObject) (ConsentStartVerificationResponseObject, error) {
	k, err := collectionPoint(ctx)
	if err != nil {
		return nil, err
	}
	id := consenthttp.Identifiers([]SubjectIdentifier{req.Body.Identifier})[0]
	ref, err := h.svc.StartConsentVerification(ctx, k.EntityID, id)
	if err != nil {
		return nil, consenthttp.ToProblem(err)
	}
	return ConsentStartVerification201JSONResponse{VerificationId: ref.ID, Channel: ConsentStartVerification201JSONResponseBodyChannel(ref.Channel)}, nil
}

func (h *Strict) ConsentVerifyGuardianApproval(ctx context.Context, req ConsentVerifyGuardianApprovalRequestObject) (ConsentVerifyGuardianApprovalResponseObject, error) {
	if _, err := collectionPoint(ctx); err != nil {
		return nil, err
	}
	b := req.Body
	confirmed, err := h.svc.ConfirmGuardianApproval(ctx, req.Id, b.VerificationId, b.Code)
	if err != nil {
		return nil, consenthttp.ToProblem(err)
	}
	return ConsentVerifyGuardianApproval200JSONResponse{Status: Approved, ConfirmedPurposes: confirmed}, nil
}

func (h *Strict) ConsentConfirmDoubleOptIn(ctx context.Context, req ConsentConfirmDoubleOptInRequestObject) (ConsentConfirmDoubleOptInResponseObject, error) {
	if _, err := collectionPoint(ctx); err != nil {
		return nil, err
	}
	purposeCode, err := h.svc.ConfirmDoubleOptIn(ctx, req.Body.Token)
	if err != nil {
		return nil, consenthttp.ToProblem(err)
	}
	return ConsentConfirmDoubleOptIn200JSONResponse{Status: Confirmed, PurposeCode: purposeCode}, nil
}

// ---- preference centre (CON-18) ----

func (h *Strict) ConsentStartPreferenceVerification(ctx context.Context, req ConsentStartPreferenceVerificationRequestObject) (ConsentStartPreferenceVerificationResponseObject, error) {
	if _, err := preferenceCenterKey(ctx); err != nil {
		return nil, err
	}
	id := consenthttp.Identifiers([]SubjectIdentifier{req.Body.Identifier})[0]
	ref, err := h.svc.StartPreferenceVerification(ctx, id)
	if err != nil {
		return nil, consenthttp.ToProblem(err)
	}
	return ConsentStartPreferenceVerification201JSONResponse{VerificationId: ref.ID, Channel: ConsentStartPreferenceVerification201JSONResponseBodyChannel(ref.Channel)}, nil
}

func (h *Strict) ConsentViewPreferenceCenter(ctx context.Context, req ConsentViewPreferenceCenterRequestObject) (ConsentViewPreferenceCenterResponseObject, error) {
	if _, err := preferenceCenterKey(ctx); err != nil {
		return nil, err
	}
	b := req.Body
	id := consenthttp.Identifiers([]SubjectIdentifier{b.Identifier})[0]
	code := ""
	if b.Code != nil {
		code = *b.Code
	}
	lang := "th"
	if req.Params.AcceptLanguage != nil {
		lang = string(*req.Params.AcceptLanguage)
	}
	pc, err := h.svc.GetPreferenceCenter(ctx, b.VerificationId, code, id, lang)
	if err != nil {
		return nil, consenthttp.ToProblem(err)
	}
	m := map[string]any{"identifiers": []map[string]any{}, "purposes": []map[string]any{}}
	ids := make([]map[string]any, 0, len(pc.Identifiers))
	for _, i := range pc.Identifiers {
		ids = append(ids, map[string]any{"type": i.Type, "masked": i.Masked, "primary": i.Primary, "verified": i.Verified})
	}
	m["identifiers"] = ids
	purposes := make([]map[string]any, 0, len(pc.Purposes))
	for _, p := range pc.Purposes {
		pm := map[string]any{"code": p.Code, "version": p.VersionNo, "name": p.Name, "is_sensitive": p.IsSensitive, "preferences": prefsWire(p.Preferences)}
		if p.Description != "" {
			pm["description"] = p.Description
		}
		if p.Text != "" {
			pm["text"] = p.Text
		}
		if p.ExplicitText != "" {
			pm["explicit_text"] = p.ExplicitText
		}
		if p.Decision != "" {
			pm["decision"] = p.Decision
		}
		if len(p.Current) > 0 {
			pm["current"] = json.RawMessage(p.Current)
		}
		if p.ExpiresAt != nil {
			pm["expires_at"] = p.ExpiresAt.UTC()
		}
		purposes = append(purposes, pm)
	}
	m["purposes"] = purposes
	var w ConsentPreferenceCenter
	bs, _ := json.Marshal(m)
	if err := json.Unmarshal(bs, &w); err != nil {
		return nil, err
	}
	return ConsentViewPreferenceCenter200JSONResponse(w), nil
}

func (h *Strict) ConsentUpdatePreferenceCenter(ctx context.Context, req ConsentUpdatePreferenceCenterRequestObject) (ConsentUpdatePreferenceCenterResponseObject, error) {
	if _, err := preferenceCenterKey(ctx); err != nil {
		return nil, err
	}
	b := req.Body
	id := consenthttp.Identifiers([]SubjectIdentifier{b.Identifier})[0]
	code := ""
	if b.Code != nil {
		code = *b.Code
	}
	language := "th"
	if b.Language != nil {
		language = string(*b.Language)
	}
	r, err := h.svc.UpdatePreferences(ctx, b.VerificationId, code, id, consenthttp.Decisions(b.Decisions), language)
	if err != nil {
		return nil, consenthttp.ToProblem(err)
	}
	var w ConsentResult
	bs, _ := json.Marshal(consenthttp.ResultMap(r))
	if err := json.Unmarshal(bs, &w); err != nil {
		return nil, err
	}
	return ConsentUpdatePreferenceCenter201JSONResponse(w), nil
}
