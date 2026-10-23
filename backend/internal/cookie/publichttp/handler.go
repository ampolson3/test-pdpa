// Package cookiepublichttp holds the /public/v1 cookie-banner endpoints (CON-01): the SDK's own fetch of a
// domain's published banner + categories, and the visitor's consent decision — no login, the tenant comes
// from the public key (publickeys.Middleware), the same mechanism every other /public/v1 surface in this
// codebase uses. Types in public.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see
// oapi-codegen.yaml).
package cookiepublichttp

import (
	"context"
	"errors"
	"net/http"

	cookie "pdpa-platform/internal/cookie/service"
	"pdpa-platform/internal/pkg/clientip"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/publickeys"
)

type Strict struct {
	svc *cookie.Service
}

func NewStrict(svc *cookie.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

type uaKey struct{}

// RequestInfo is a strict middleware that keeps the request's user agent for the consent record's evidence
// — the same pattern consent's own publichttp package already uses.
func RequestInfo(f StrictHandlerFunc, _ string) StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
		return f(context.WithValue(ctx, uaKey{}, r.UserAgent()), w, r, req)
	}
}

// cookieDomain is the key of this request, which must stand for a cookie.domains row.
func cookieDomain(ctx context.Context) (publickeys.Key, error) {
	k, ok := publickeys.FromContext(ctx)
	if !ok || k.EntityType != publickeys.EntityCookieDomain {
		return publickeys.Key{}, httpx.NotFound()
	}
	return k, nil
}

func textsWire(t cookie.BannerTexts) CookieBannerTexts {
	out := CookieBannerTexts{TitleTh: t.TitleTh, TitleEn: strPtr(t.TitleEn), DescriptionTh: strPtr(t.DescriptionTh), DescriptionEn: strPtr(t.DescriptionEn),
		AcceptAllTh: t.AcceptAllTh, AcceptAllEn: strPtr(t.AcceptAllEn), RejectAllTh: t.RejectAllTh, RejectAllEn: strPtr(t.RejectAllEn),
		SettingsTh: t.SettingsTh, SettingsEn: strPtr(t.SettingsEn)}
	if len(t.Translations) > 0 {
		m := make(map[string]CookieBannerTranslation, len(t.Translations))
		for lang, tr := range t.Translations {
			m[lang] = CookieBannerTranslation{Title: tr.Title, Description: strPtr(tr.Description), AcceptAll: tr.AcceptAll, RejectAll: tr.RejectAll, Settings: tr.Settings}
		}
		out.Translations = &m
	}
	return out
}

func themeWire(t cookie.BannerTheme) *CookieBannerTheme {
	out := &CookieBannerTheme{PrimaryColor: strPtr(t.PrimaryColor), TextColor: strPtr(t.TextColor), ButtonTextColor: strPtr(t.ButtonTextColor), LogoUrl: strPtr(t.LogoURL), CustomCss: strPtr(t.CustomCSS)}
	if t.Position != "" {
		v := CookieBannerThemePosition(t.Position)
		out.Position = &v
	}
	return out
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (h *Strict) CookieGetPublicBanner(ctx context.Context, _ CookieGetPublicBannerRequestObject) (CookieGetPublicBannerResponseObject, error) {
	k, err := cookieDomain(ctx)
	if err != nil {
		return nil, err
	}
	b, err := h.svc.PublicBannerConfig(ctx, k.EntityID)
	if err != nil {
		return nil, problem(err)
	}
	cats, err := h.svc.PublicCategories(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := h.svc.PublicScriptRules(ctx, k.EntityID)
	if err != nil {
		return nil, err
	}
	out := CookiePublicBanner{Id: b.ID, DomainId: b.DomainID, VersionNo: int(b.VersionNo), Layout: CookiePublicBannerLayout(b.Layout), Theme: themeWire(b.Theme), Texts: textsWire(b.Texts),
		Categories: make([]CookieCategory, 0, len(cats))}
	for _, c := range cats {
		out.Categories = append(out.Categories, CookieCategory{Id: c.ID, Code: c.Code, NameTh: c.NameTh, NameEn: strPtr(c.NameEn),
			DescriptionTh: strPtr(c.DescTh), DescriptionEn: strPtr(c.DescEn), IsRequired: c.IsRequired, DisplayOrder: int(c.DisplayOrder), RowVersion: int(c.RowVersion)})
	}
	scriptRules := make([]CookiePublicScriptRule, 0, len(rules))
	for _, r := range rules {
		scriptRules = append(scriptRules, CookiePublicScriptRule{Pattern: r.Pattern, CategoryCode: r.CategoryCode, Action: CookiePublicScriptRuleAction(r.Action)})
	}
	out.ScriptRules = &scriptRules
	return CookieGetPublicBanner200JSONResponse(out), nil
}

// CookieGetPublicPolicy is CON-05's own acceptance criterion: the Cookie Policy table, always a live read
// of the domain's active cookies so it can never drift from the latest CON-04 scan.
func (h *Strict) CookieGetPublicPolicy(ctx context.Context, _ CookieGetPublicPolicyRequestObject) (CookieGetPublicPolicyResponseObject, error) {
	k, err := cookieDomain(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := h.svc.PublicCookiePolicy(ctx, k.EntityID)
	if err != nil {
		return nil, problem(err)
	}
	out := CookiePublicPolicy{DomainId: k.EntityID, Cookies: make([]CookiePolicyEntry, 0, len(entries))}
	for _, e := range entries {
		out.Cookies = append(out.Cookies, CookiePolicyEntry{
			Name: e.Name, ProviderDomain: e.ProviderDomain, StorageType: CookiePolicyEntryStorageType(e.StorageType),
			CategoryCode: strPtr(e.CategoryCode), CategoryNameTh: strPtr(e.CategoryNameTh), CategoryNameEn: strPtr(e.CategoryNameEn),
			Duration: strPtr(e.Duration), DescriptionTh: strPtr(e.DescriptionTh), DescriptionEn: strPtr(e.DescriptionEn),
		})
	}
	return CookieGetPublicPolicy200JSONResponse(out), nil
}

func (h *Strict) CookieSubmitConsent(ctx context.Context, req CookieSubmitConsentRequestObject) (CookieSubmitConsentResponseObject, error) {
	if _, err := cookieDomain(ctx); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.RequestInvalid("action")
	}
	b := *req.Body
	in := cookie.RecordConsentInput{DomainID: b.DomainId, VisitorID: b.VisitorId, BannerConfigID: b.BannerConfigId, Action: string(b.Action)}
	if b.Choices != nil {
		in.Choices = *b.Choices
	}
	if ip, ok := clientip.From(ctx); ok {
		in.IP = ip
	}
	if ua, ok := ctx.Value(uaKey{}).(string); ok {
		in.UserAgent = ua
	}
	if err := h.svc.RecordConsent(ctx, in); err != nil {
		return nil, problem(err)
	}
	return CookieSubmitConsent201JSONResponse{Recorded: true}, nil
}

func problem(err error) error {
	switch {
	case errors.Is(err, cookie.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, cookie.ErrInvalid):
		return httpx.UnprocessableEntity("cookie.invalid", err.Error())
	}
	return err
}
