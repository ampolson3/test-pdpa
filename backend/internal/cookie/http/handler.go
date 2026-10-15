// Package cookiehttp holds the admin endpoints of the cookie module (CON-01): domain registration,
// category management and versioned banner configs. Types in cookie.gen.go are generated from
// api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package cookiehttp

import (
	"context"
	"errors"
	"strconv"
	"strings"

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
	return CookieBannerTexts{TitleTh: t.TitleTh, TitleEn: strPtr(t.TitleEn), DescriptionTh: strPtr(t.DescriptionTh), DescriptionEn: strPtr(t.DescriptionEn),
		AcceptAllTh: t.AcceptAllTh, AcceptAllEn: strPtr(t.AcceptAllEn), RejectAllTh: t.RejectAllTh, RejectAllEn: strPtr(t.RejectAllEn),
		SettingsTh: t.SettingsTh, SettingsEn: strPtr(t.SettingsEn)}
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
	return out
}

func bannerConfigWire(b cookie.BannerConfig) CookieBannerConfig {
	return CookieBannerConfig{Id: b.ID, DomainId: b.DomainID, VersionNo: int(b.VersionNo), Status: CookieBannerConfigStatus(b.Status),
		Layout: CookieBannerConfigLayout(b.Layout), Texts: bannerTextsWire(b.Texts), RowVersion: int(b.RowVersion)}
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
	bc, err := h.svc.CreateBannerConfig(ctx, req.Id, cookie.CreateBannerConfigInput{Layout: string(b.Layout), Texts: bannerTextsFromWire(b.Texts)})
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
	bc, err := h.svc.UpdateBannerConfigDraft(ctx, req.Id, v, cookie.CreateBannerConfigInput{Layout: string(b.Layout), Texts: bannerTextsFromWire(b.Texts)})
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
