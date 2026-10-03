package orghttp

import (
	"context"
	"errors"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) OrgGetSettings(ctx context.Context, _ OrgGetSettingsRequestObject) (OrgGetSettingsResponseObject, error) {
	s, err := h.svc.GetOrgSettings(ctx)
	if err != nil {
		return nil, err
	}
	return OrgGetSettings200JSONResponse{Body: toSettingsWire(s), Headers: OrgGetSettings200ResponseHeaders{ETag: etag(s.RowVersion)}}, nil
}

func (h *Strict) OrgUpdateSettings(ctx context.Context, req OrgUpdateSettingsRequestObject) (OrgUpdateSettingsResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	s, err := h.svc.SaveOrgSettings(ctx, toSettingsInput(*req.Body), v)
	if err != nil {
		return nil, settingsProblem(err)
	}
	return OrgUpdateSettings200JSONResponse{Body: toSettingsWire(s), Headers: OrgUpdateSettings200ResponseHeaders{ETag: etag(s.RowVersion)}}, nil
}

func toSettingsInput(b OrgSettingsInput) orgservice.OrgSettings {
	out := orgservice.OrgSettings{DefaultLanguage: string(b.DefaultLanguage), DateEra: string(b.DateEra)}
	if b.Branding != nil {
		out.Branding = orgservice.Branding{LogoFileID: b.Branding.LogoFileId, ThemeColor: str(b.Branding.ThemeColor), AccentColor: str(b.Branding.AccentColor)}
	}
	return out
}

func toSettingsWire(s orgservice.OrgSettings) OrgSettings {
	w := OrgSettings{
		DefaultLanguage: OrgSettingsDefaultLanguage(s.DefaultLanguage),
		DateEra:         OrgSettingsDateEra(s.DateEra),
		Branding: OrgBranding{
			LogoFileId:  s.Branding.LogoFileID,
			ThemeColor:  ptr(s.Branding.ThemeColor),
			AccentColor: ptr(s.Branding.AccentColor),
		},
		RowVersion: int(s.RowVersion),
	}
	if s.RowVersion > 0 {
		t := s.UpdatedAt.UTC()
		w.UpdatedAt = &t
	}
	return w
}

func settingsProblem(err error) error {
	switch {
	case errors.Is(err, orgservice.ErrLogoNotUse):
		return httpx.UnprocessableEntity("org.logo_not_usable", err.Error())
	case errors.Is(err, orgservice.ErrInvalid):
		return httpx.UnprocessableEntity("org.invalid_settings", err.Error())
	}
	return problem(err)
}
