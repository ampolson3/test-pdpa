// Package cookiehttp holds the admin endpoints of the cookie module (CON-01): domain registration,
// category management and versioned banner configs. Types in cookie.gen.go are generated from
// api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package cookiehttp

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	cookie "pdpa-platform/internal/cookie/service"
	"pdpa-platform/internal/pkg/httpx"
)

type Strict struct {
	svc *cookie.Service
}

func NewStrict(svc *cookie.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

// ---- domains ----

func domainWire(d cookie.Domain) CookieDomain {
	return CookieDomain{Id: d.ID, Domain: d.Domain, LegalEntityId: d.LegalEntityID, Status: CookieDomainStatus(statusOf(d)),
		PublicKey: d.PublicKey, RowVersion: int(d.RowVersion)}
}

// statusOf maps the service's own pending/active/disabled status down to the wire's own pending/verified
// pair — the acceptance criterion only cares whether a domain is verified yet, not the disabled state this
// pass never reaches (no deactivation path exists yet).
func statusOf(d cookie.Domain) string {
	if d.Verified {
		return "verified"
	}
	return "pending"
}

func (h *Strict) CookieListDomains(ctx context.Context, _ CookieListDomainsRequestObject) (CookieListDomainsResponseObject, error) {
	list, err := h.svc.ListDomains(ctx)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := CookieListDomains200JSONResponse{Data: make([]CookieDomain, 0, len(list))}
	for _, d := range list {
		out.Data = append(out.Data, domainWire(d))
	}
	return out, nil
}

func (h *Strict) CookieRegisterDomain(ctx context.Context, req CookieRegisterDomainRequestObject) (CookieRegisterDomainResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("domain")
	}
	d, err := h.svc.RegisterDomain(ctx, req.Body.Domain, req.Body.LegalEntityId)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieRegisterDomain201JSONResponse(domainWire(d)), nil
}

func (h *Strict) CookieGetDomain(ctx context.Context, req CookieGetDomainRequestObject) (CookieGetDomainResponseObject, error) {
	d, err := h.svc.GetDomain(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieGetDomain200JSONResponse(domainWire(d)), nil
}

func (h *Strict) CookieVerifyDomain(ctx context.Context, req CookieVerifyDomainRequestObject) (CookieVerifyDomainResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	d, err := h.svc.VerifyDomain(ctx, req.Id, v)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieVerifyDomain200JSONResponse(domainWire(d)), nil
}

// ---- categories ----

func categoryWire(c cookie.Category) CookieCategory {
	return CookieCategory{Id: c.ID, Code: c.Code, NameTh: c.NameTh, NameEn: strPtr(c.NameEn), DescriptionTh: strPtr(c.DescTh),
		DescriptionEn: strPtr(c.DescEn), IsRequired: c.IsRequired, DisplayOrder: int(c.DisplayOrder), RowVersion: int(c.RowVersion)}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (h *Strict) CookieListCategories(ctx context.Context, _ CookieListCategoriesRequestObject) (CookieListCategoriesResponseObject, error) {
	list, err := h.svc.ListCategories(ctx)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := CookieListCategories200JSONResponse{Data: make([]CookieCategory, 0, len(list))}
	for _, c := range list {
		out.Data = append(out.Data, categoryWire(c))
	}
	return out, nil
}

func (h *Strict) CookieCreateCategory(ctx context.Context, req CookieCreateCategoryRequestObject) (CookieCreateCategoryResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("code")
	}
	b := *req.Body
	in := cookie.CreateCategoryInput{Code: b.Code, NameTh: b.NameTh, IsRequired: b.IsRequired != nil && *b.IsRequired}
	if b.NameEn != nil {
		in.NameEn = *b.NameEn
	}
	if b.DescriptionTh != nil {
		in.DescTh = *b.DescriptionTh
	}
	if b.DescriptionEn != nil {
		in.DescEn = *b.DescriptionEn
	}
	if b.DisplayOrder != nil {
		in.DisplayOrder = int16(*b.DisplayOrder)
	}
	c, err := h.svc.CreateCategory(ctx, in)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieCreateCategory201JSONResponse(categoryWire(c)), nil
}

func (h *Strict) CookieDeleteCategory(ctx context.Context, req CookieDeleteCategoryRequestObject) (CookieDeleteCategoryResponseObject, error) {
	if err := h.svc.DeleteCategory(ctx, req.Id); err != nil {
		return nil, ToProblem(err)
	}
	return CookieDeleteCategory204Response{}, nil
}

// ---- banner configs ----

func bannerTextsWire(t cookie.BannerTexts) CookieBannerTexts {
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

func bannerTextsFromWire(t CookieBannerTexts) cookie.BannerTexts {
	out := cookie.BannerTexts{TitleTh: t.TitleTh, AcceptAllTh: t.AcceptAllTh, RejectAllTh: t.RejectAllTh, SettingsTh: t.SettingsTh}
	if t.TitleEn != nil {
		out.TitleEn = *t.TitleEn
	}
	if t.DescriptionTh != nil {
		out.DescriptionTh = *t.DescriptionTh
	}
	if t.DescriptionEn != nil {
		out.DescriptionEn = *t.DescriptionEn
	}
	if t.AcceptAllEn != nil {
		out.AcceptAllEn = *t.AcceptAllEn
	}
	if t.RejectAllEn != nil {
		out.RejectAllEn = *t.RejectAllEn
	}
	if t.SettingsEn != nil {
		out.SettingsEn = *t.SettingsEn
	}
	if t.Translations != nil && len(*t.Translations) > 0 {
		out.Translations = make(map[string]cookie.BannerTranslation, len(*t.Translations))
		for lang, tr := range *t.Translations {
			desc := ""
			if tr.Description != nil {
				desc = *tr.Description
			}
			out.Translations[lang] = cookie.BannerTranslation{Title: tr.Title, Description: desc, AcceptAll: tr.AcceptAll, RejectAll: tr.RejectAll, Settings: tr.Settings}
		}
	}
	return out
}

func bannerThemeWire(t cookie.BannerTheme) *CookieBannerTheme {
	return &CookieBannerTheme{PrimaryColor: strPtr(t.PrimaryColor), TextColor: strPtr(t.TextColor), ButtonTextColor: strPtr(t.ButtonTextColor),
		LogoUrl: strPtr(t.LogoURL), Position: themePositionPtr(t.Position), CustomCss: strPtr(t.CustomCSS)}
}

func themePositionPtr(p string) *CookieBannerThemePosition {
	if p == "" {
		return nil
	}
	v := CookieBannerThemePosition(p)
	return &v
}

func bannerThemeFromWire(t *CookieBannerTheme) cookie.BannerTheme {
	if t == nil {
		return cookie.BannerTheme{}
	}
	out := cookie.BannerTheme{}
	if t.PrimaryColor != nil {
		out.PrimaryColor = *t.PrimaryColor
	}
	if t.TextColor != nil {
		out.TextColor = *t.TextColor
	}
	if t.ButtonTextColor != nil {
		out.ButtonTextColor = *t.ButtonTextColor
	}
	if t.LogoUrl != nil {
		out.LogoURL = *t.LogoUrl
	}
	if t.Position != nil {
		out.Position = string(*t.Position)
	}
	if t.CustomCss != nil {
		out.CustomCSS = *t.CustomCss
	}
	return out
}

func bannerConfigWire(b cookie.BannerConfig) CookieBannerConfig {
	return CookieBannerConfig{Id: b.ID, DomainId: b.DomainID, VersionNo: int(b.VersionNo), Status: CookieBannerConfigStatus(b.Status),
		Layout: CookieBannerConfigLayout(b.Layout), Theme: bannerThemeWire(b.Theme), Texts: bannerTextsWire(b.Texts), RowVersion: int(b.RowVersion)}
}

func (h *Strict) CookieListBannerConfigs(ctx context.Context, req CookieListBannerConfigsRequestObject) (CookieListBannerConfigsResponseObject, error) {
	list, err := h.svc.ListBannerConfigs(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := CookieListBannerConfigs200JSONResponse{Data: make([]CookieBannerConfig, 0, len(list))}
	for _, b := range list {
		out.Data = append(out.Data, bannerConfigWire(b))
	}
	return out, nil
}

func (h *Strict) CookieCreateBannerConfig(ctx context.Context, req CookieCreateBannerConfigRequestObject) (CookieCreateBannerConfigResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("layout")
	}
	b := *req.Body
	bc, err := h.svc.CreateBannerConfig(ctx, req.Id, cookie.CreateBannerConfigInput{Layout: string(b.Layout), Theme: bannerThemeFromWire(b.Theme), Texts: bannerTextsFromWire(b.Texts)})
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieCreateBannerConfig201JSONResponse(bannerConfigWire(bc)), nil
}

func (h *Strict) CookieGetBannerConfig(ctx context.Context, req CookieGetBannerConfigRequestObject) (CookieGetBannerConfigResponseObject, error) {
	bc, err := h.svc.GetBannerConfig(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieGetBannerConfig200JSONResponse(bannerConfigWire(bc)), nil
}

func (h *Strict) CookieUpdateBannerConfigDraft(ctx context.Context, req CookieUpdateBannerConfigDraftRequestObject) (CookieUpdateBannerConfigDraftResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("layout")
	}
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := *req.Body
	bc, err := h.svc.UpdateBannerConfigDraft(ctx, req.Id, v, cookie.CreateBannerConfigInput{Layout: string(b.Layout), Theme: bannerThemeFromWire(b.Theme), Texts: bannerTextsFromWire(b.Texts)})
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieUpdateBannerConfigDraft200JSONResponse(bannerConfigWire(bc)), nil
}

func (h *Strict) CookiePublishBannerConfig(ctx context.Context, req CookiePublishBannerConfigRequestObject) (CookiePublishBannerConfigResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	bc, err := h.svc.PublishBannerConfig(ctx, req.Id, v)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookiePublishBannerConfig200JSONResponse(bannerConfigWire(bc)), nil
}

// ---- cookies (CON-02: "รายการคุกกี้ต่อหมวด") ----

func cookieEntryWire(e cookie.CookieEntry) CookieCookie {
	w := CookieCookie{Id: e.ID, DomainId: e.DomainID, Name: e.Name, ProviderDomain: e.ProviderDomain,
		StorageType: CookieCookieStorageType(e.StorageType), DescriptionTh: strPtr(e.DescriptionTh), DescriptionEn: strPtr(e.DescriptionEn),
		Duration: strPtr(e.Duration), IsActive: e.IsActive, RowVersion: int(e.RowVersion)}
	if e.CategoryID != nil {
		w.CategoryId = e.CategoryID
	}
	return w
}

func (h *Strict) CookieListCookies(ctx context.Context, req CookieListCookiesRequestObject) (CookieListCookiesResponseObject, error) {
	list, err := h.svc.ListCookies(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := CookieListCookies200JSONResponse{Data: make([]CookieCookie, 0, len(list))}
	for _, e := range list {
		out.Data = append(out.Data, cookieEntryWire(e))
	}
	return out, nil
}

func (h *Strict) CookieCreateCookie(ctx context.Context, req CookieCreateCookieRequestObject) (CookieCreateCookieResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("name")
	}
	b := *req.Body
	in := cookie.CreateCookieInput{Name: b.Name, ProviderDomain: b.ProviderDomain, StorageType: string(b.StorageType), CategoryID: b.CategoryId}
	if b.Duration != nil {
		in.Duration = *b.Duration
	}
	if b.DescriptionTh != nil {
		in.DescriptionTh = *b.DescriptionTh
	}
	if b.DescriptionEn != nil {
		in.DescriptionEn = *b.DescriptionEn
	}
	e, err := h.svc.CreateCookie(ctx, req.Id, in)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieCreateCookie201JSONResponse(cookieEntryWire(e)), nil
}

func (h *Strict) CookieDeleteCookie(ctx context.Context, req CookieDeleteCookieRequestObject) (CookieDeleteCookieResponseObject, error) {
	if err := h.svc.DeleteCookie(ctx, req.Id); err != nil {
		return nil, ToProblem(err)
	}
	return CookieDeleteCookie204Response{}, nil
}

// ---- script rules (CON-02's own enforcement mechanism) ----

func scriptRuleWire(r cookie.ScriptRule) CookieScriptRule {
	return CookieScriptRule{Id: r.ID, DomainId: r.DomainID, Pattern: r.Pattern, CategoryId: r.CategoryID,
		Action: CookieScriptRuleAction(r.Action), RowVersion: int(r.RowVersion)}
}

func (h *Strict) CookieListScriptRules(ctx context.Context, req CookieListScriptRulesRequestObject) (CookieListScriptRulesResponseObject, error) {
	list, err := h.svc.ListScriptRules(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := CookieListScriptRules200JSONResponse{Data: make([]CookieScriptRule, 0, len(list))}
	for _, r := range list {
		out.Data = append(out.Data, scriptRuleWire(r))
	}
	return out, nil
}

func (h *Strict) CookieCreateScriptRule(ctx context.Context, req CookieCreateScriptRuleRequestObject) (CookieCreateScriptRuleResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("pattern")
	}
	b := *req.Body
	in := cookie.CreateScriptRuleInput{Pattern: b.Pattern, CategoryID: b.CategoryId}
	if b.Action != nil {
		in.Action = string(*b.Action)
	}
	r, err := h.svc.CreateScriptRule(ctx, req.Id, in)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieCreateScriptRule201JSONResponse(scriptRuleWire(r)), nil
}

func (h *Strict) CookieDeleteScriptRule(ctx context.Context, req CookieDeleteScriptRuleRequestObject) (CookieDeleteScriptRuleResponseObject, error) {
	if err := h.svc.DeleteScriptRule(ctx, req.Id); err != nil {
		return nil, ToProblem(err)
	}
	return CookieDeleteScriptRule204Response{}, nil
}

// ---- scans (CON-04: automated cookie scanner) ----

func scanWire(sc cookie.Scan) CookieScan {
	w := CookieScan{Id: sc.ID, DomainId: sc.DomainID, Trigger: CookieScanTrigger(sc.Trigger), Status: CookieScanStatus(sc.Status), RowVersion: int(sc.RowVersion)}
	if sc.StartedAt != nil {
		w.StartedAt = sc.StartedAt
	}
	if sc.FinishedAt != nil {
		w.FinishedAt = sc.FinishedAt
	}
	if sc.PagesScanned != nil {
		v := int(*sc.PagesScanned)
		w.PagesScanned = &v
	}
	if sc.CookiesFound != nil {
		v := int(*sc.CookiesFound)
		w.CookiesFound = &v
	}
	if sc.NewCookies != nil {
		v := int(*sc.NewCookies)
		w.NewCookies = &v
	}
	return w
}

func scanFindingWire(f cookie.ScanFinding) CookieScanFinding {
	return CookieScanFinding{Id: f.ID, ScanId: f.ScanID, CookieName: f.CookieName, CookieDomain: f.CookieDomain, PageUrl: f.PageURL,
		SetBeforeConsent: f.SetBeforeConsent, SuggestedCategory: f.SuggestedCategory, MatchedCookieId: f.MatchedCookieID, RowVersion: int(f.RowVersion)}
}

func (h *Strict) CookieListScans(ctx context.Context, req CookieListScansRequestObject) (CookieListScansResponseObject, error) {
	var after *cookie.ScanCursor
	if req.Params.Cursor != nil {
		c, err := decodeScanCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		after = &c
	}
	var limit int32
	if req.Params.Limit != nil {
		limit = int32(*req.Params.Limit)
	}
	list, next, err := h.svc.ListScans(ctx, req.Id, after, limit)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := CookieListScans200JSONResponse{Data: make([]CookieScan, 0, len(list))}
	for _, sc := range list {
		out.Data = append(out.Data, scanWire(sc))
	}
	if next != nil {
		c := encodeScanCursor(*next)
		out.NextCursor = &c
	}
	return out, nil
}

func (h *Strict) CookieStartScan(ctx context.Context, req CookieStartScanRequestObject) (CookieStartScanResponseObject, error) {
	trigger := "manual"
	if req.Body != nil && req.Body.Trigger != nil {
		trigger = string(*req.Body.Trigger)
	}
	sc, err := h.svc.StartScan(ctx, req.Id, trigger)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieStartScan201JSONResponse(scanWire(sc)), nil
}

func (h *Strict) CookieGetScan(ctx context.Context, req CookieGetScanRequestObject) (CookieGetScanResponseObject, error) {
	sc, err := h.svc.GetScan(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieGetScan200JSONResponse(scanWire(sc)), nil
}

func (h *Strict) CookieListScanFindings(ctx context.Context, req CookieListScanFindingsRequestObject) (CookieListScanFindingsResponseObject, error) {
	var offset, limit int32
	if req.Params.Offset != nil {
		offset = int32(*req.Params.Offset)
	}
	limit = 50
	if req.Params.Limit != nil {
		limit = int32(*req.Params.Limit)
	}
	list, err := h.svc.ListScanFindings(ctx, req.Id, offset, limit)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := CookieListScanFindings200JSONResponse{Data: make([]CookieScanFinding, 0, len(list))}
	for _, f := range list {
		out.Data = append(out.Data, scanFindingWire(f))
	}
	return out, nil
}

func (h *Strict) CookieClassifyScanFinding(ctx context.Context, req CookieClassifyScanFindingRequestObject) (CookieClassifyScanFindingResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("category_id")
	}
	f, err := h.svc.ClassifyFinding(ctx, req.Id, req.Body.CategoryId)
	if err != nil {
		return nil, ToProblem(err)
	}
	return CookieClassifyScanFinding200JSONResponse(scanFindingWire(f)), nil
}

func encodeScanCursor(c cookie.ScanCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.CreatedAt.Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

func decodeScanCursor(s string) (cookie.ScanCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cookie.ScanCursor{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return cookie.ScanCursor{}, errors.New("cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return cookie.ScanCursor{}, err
	}
	u, err := uuid.Parse(id)
	return cookie.ScanCursor{CreatedAt: t, ID: u}, err
}

// ---- wire helpers ----

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

// ToProblem maps service errors to RFC 9457 problems with stable codes.
func ToProblem(err error) error {
	switch {
	case errors.Is(err, cookie.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, cookie.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, cookie.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, cookie.ErrInvalid):
		return httpx.UnprocessableEntity("cookie.invalid", err.Error())
	}
	return err
}
