package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	cookiestore "pdpa-platform/internal/cookie/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/publickeys"
)

// BannerEntityType is this module's own audit entity type for cookie.banner_configs.
const BannerEntityType = "cookie_banner_config"

var bannerLayouts = []string{"bar", "modal", "corner"}

// BannerTexts is the banner's own copy, bilingual (CLAUDE.md rule 12 — no literal UI strings baked into Go,
// these are tenant-authored content, not platform i18n keys). Title/description are the banner's lead
// copy; the three button labels are deliberately equal-weight fields — none of them defaults to anything
// more prominent than another (the acceptance criterion's own "ปฏิเสธทั้งหมดได้ในคลิกเดียว... น้ำหนักเท่ากัน").
// Translations (CON-06) is a tenant-added, open-ended map of ISO language code -> this same copy in a
// third (or further) language — the SDK picks one by the visitor's own browser language, falling back to
// en then th; unlike PLT-16's documents (hard-coded to exactly th/en across rendering/validation), banner
// copy is plain visitor-facing text with no merge fields or legal clause machinery to generalize, so a
// third UI language costs nothing extra here.
type BannerTexts struct {
	TitleTh       string                       `json:"title_th"`
	TitleEn       string                       `json:"title_en,omitempty"`
	DescriptionTh string                       `json:"description_th"`
	DescriptionEn string                       `json:"description_en,omitempty"`
	AcceptAllTh   string                       `json:"accept_all_th"`
	AcceptAllEn   string                       `json:"accept_all_en,omitempty"`
	RejectAllTh   string                       `json:"reject_all_th"`
	RejectAllEn   string                       `json:"reject_all_en,omitempty"`
	SettingsTh    string                       `json:"settings_th"`
	SettingsEn    string                       `json:"settings_en,omitempty"`
	Translations  map[string]BannerTranslation `json:"translations,omitempty"`
}

// BannerTranslation is one extra language's own copy of BannerTexts, flat (no th/en suffix — the map key
// is already the language code).
type BannerTranslation struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	AcceptAll   string `json:"accept_all"`
	RejectAll   string `json:"reject_all"`
	Settings    string `json:"settings"`
}

// BannerTheme is the banner's own look (CON-06): colors, an optional logo, which edge of the viewport it
// docks to, and a tenant's own limited custom CSS. CustomCSS is capped and screened for the obvious
// script/navigation vectors (sanitizeCustomCSS) rather than parsed — this is cosmetic styling injected into
// a <style> tag on the tenant's own site, not untrusted third-party content, but the cap and screen keep an
// honest mistake (an accidentally pasted <script> block) from doing anything.
type BannerTheme struct {
	PrimaryColor    string `json:"primary_color,omitempty"`
	TextColor       string `json:"text_color,omitempty"`
	ButtonTextColor string `json:"button_text_color,omitempty"`
	LogoURL         string `json:"logo_url,omitempty"`
	Position        string `json:"position,omitempty"` // top | bottom | center
	CustomCSS       string `json:"custom_css,omitempty"`
}

var bannerThemePositions = []string{"", "top", "bottom", "center"}

const maxCustomCSSLen = 2000

func validateTheme(t BannerTheme) error {
	if !contains(bannerThemePositions, t.Position) {
		return fmt.Errorf("%w: position", ErrInvalid)
	}
	if len(t.CustomCSS) > maxCustomCSSLen {
		return fmt.Errorf("%w: custom_css exceeds %d characters", ErrInvalid, maxCustomCSSLen)
	}
	lower := strings.ToLower(t.CustomCSS)
	for _, bad := range []string{"<script", "javascript:", "@import", "expression(", "</style"} {
		if strings.Contains(lower, bad) {
			return fmt.Errorf("%w: custom_css contains a disallowed token %q", ErrInvalid, bad)
		}
	}
	return nil
}

// BannerConfig is one version of a domain's banner.
type BannerConfig struct {
	ID         uuid.UUID
	DomainID   uuid.UUID
	VersionNo  int32
	Status     string // draft | published | archived
	Layout     string
	Theme      BannerTheme
	Texts      BannerTexts
	RowVersion int32
}

func toBannerConfig(id, domainID uuid.UUID, versionNo int32, status, layout string, themeRaw, textsRaw []byte, rowVersion int32) (BannerConfig, error) {
	var t BannerTexts
	if len(textsRaw) > 0 {
		if err := json.Unmarshal(textsRaw, &t); err != nil {
			return BannerConfig{}, err
		}
	}
	var th BannerTheme
	if len(themeRaw) > 0 {
		if err := json.Unmarshal(themeRaw, &th); err != nil {
			return BannerConfig{}, err
		}
	}
	return BannerConfig{ID: id, DomainID: domainID, VersionNo: versionNo, Status: status, Layout: layout, Theme: th, Texts: t, RowVersion: rowVersion}, nil
}

// CreateBannerConfigInput is a new draft version's own content.
type CreateBannerConfigInput struct {
	Layout string
	Theme  BannerTheme
	Texts  BannerTexts
}

func validateTexts(t BannerTexts) error {
	if strings.TrimSpace(t.TitleTh) == "" || strings.TrimSpace(t.AcceptAllTh) == "" ||
		strings.TrimSpace(t.RejectAllTh) == "" || strings.TrimSpace(t.SettingsTh) == "" {
		return fmt.Errorf("%w: title_th, accept_all_th, reject_all_th and settings_th are required", ErrInvalid)
	}
	for lang, tr := range t.Translations {
		if strings.TrimSpace(lang) == "" {
			return fmt.Errorf("%w: translations has an empty language code", ErrInvalid)
		}
		if strings.TrimSpace(tr.Title) == "" || strings.TrimSpace(tr.AcceptAll) == "" ||
			strings.TrimSpace(tr.RejectAll) == "" || strings.TrimSpace(tr.Settings) == "" {
			return fmt.Errorf("%w: translations[%s] is missing a required field", ErrInvalid, lang)
		}
	}
	return nil
}

// CreateBannerConfig opens a new draft version for a domain — the previous published version keeps serving
// the SDK until this one is published (cookie.banner_configs' own versioning, not PLT-08's).
func (s *Service) CreateBannerConfig(ctx context.Context, domainID uuid.UUID, in CreateBannerConfigInput) (BannerConfig, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.create") {
		return BannerConfig{}, ErrForbidden
	}
	if _, err := s.GetDomain(ctx, domainID); err != nil {
		return BannerConfig{}, err
	}
	if !contains(bannerLayouts, in.Layout) {
		return BannerConfig{}, fmt.Errorf("%w: layout", ErrInvalid)
	}
	if err := validateTexts(in.Texts); err != nil {
		return BannerConfig{}, err
	}
	if err := validateTheme(in.Theme); err != nil {
		return BannerConfig{}, err
	}
	themeJSON, err := json.Marshal(in.Theme)
	if err != nil {
		return BannerConfig{}, err
	}
	textsJSON, err := json.Marshal(in.Texts)
	if err != nil {
		return BannerConfig{}, err
	}
	q := cookiestore.New(pdb.MustTxFromContext(ctx))
	versionNo, err := q.NextBannerVersionNo(ctx, domainID)
	if err != nil {
		return BannerConfig{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return BannerConfig{}, err
	}
	row, err := q.InsertBannerConfig(ctx, cookiestore.InsertBannerConfigParams{
		ID: id, DomainID: domainID, VersionNo: versionNo, Layout: in.Layout, Theme: themeJSON, Texts: textsJSON,
	})
	if err != nil {
		return BannerConfig{}, err
	}
	bc, err := toBannerConfig(row.ID, row.DomainID, row.VersionNo, row.Status, row.Layout, row.Theme, row.Texts, row.RowVersion)
	if err != nil {
		return BannerConfig{}, err
	}
	if err := s.audit(ctx, "cookie.banner_config.create", BannerEntityType, id, nil, map[string]any{"domain_id": domainID, "version_no": versionNo}); err != nil {
		return BannerConfig{}, err
	}
	return bc, nil
}

// GetBannerConfig returns one version.
func (s *Service) GetBannerConfig(ctx context.Context, id uuid.UUID) (BannerConfig, error) {
	row, err := cookiestore.New(pdb.MustTxFromContext(ctx)).GetBannerConfig(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return BannerConfig{}, ErrNotFound
	}
	if err != nil {
		return BannerConfig{}, err
	}
	return toBannerConfig(row.ID, row.DomainID, row.VersionNo, row.Status, row.Layout, row.Theme, row.Texts, row.RowVersion)
}

// ListBannerConfigs returns every version of a domain's banner, newest first.
func (s *Service) ListBannerConfigs(ctx context.Context, domainID uuid.UUID) ([]BannerConfig, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.read") {
		return nil, ErrForbidden
	}
	if _, err := s.GetDomain(ctx, domainID); err != nil {
		return nil, err
	}
	rows, err := cookiestore.New(pdb.MustTxFromContext(ctx)).ListBannerConfigs(ctx, domainID)
	if err != nil {
		return nil, err
	}
	out := make([]BannerConfig, 0, len(rows))
	for _, r := range rows {
		bc, err := toBannerConfig(r.ID, r.DomainID, r.VersionNo, r.Status, r.Layout, r.Theme, r.Texts, r.RowVersion)
		if err != nil {
			return nil, err
		}
		out = append(out, bc)
	}
	return out, nil
}

// UpdateBannerConfigDraft edits a still-draft version.
func (s *Service) UpdateBannerConfigDraft(ctx context.Context, id uuid.UUID, rowVersion int32, in CreateBannerConfigInput) (BannerConfig, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.update") {
		return BannerConfig{}, ErrForbidden
	}
	if !contains(bannerLayouts, in.Layout) {
		return BannerConfig{}, fmt.Errorf("%w: layout", ErrInvalid)
	}
	if err := validateTexts(in.Texts); err != nil {
		return BannerConfig{}, err
	}
	if err := validateTheme(in.Theme); err != nil {
		return BannerConfig{}, err
	}
	themeJSON, err := json.Marshal(in.Theme)
	if err != nil {
		return BannerConfig{}, err
	}
	textsJSON, err := json.Marshal(in.Texts)
	if err != nil {
		return BannerConfig{}, err
	}
	n, err := cookiestore.New(pdb.MustTxFromContext(ctx)).UpdateBannerConfigDraft(ctx, cookiestore.UpdateBannerConfigDraftParams{
		ID: id, Layout: in.Layout, Theme: themeJSON, Texts: textsJSON, RowVersion: rowVersion,
	})
	if err != nil {
		return BannerConfig{}, err
	}
	if n == 0 {
		return BannerConfig{}, ErrVersionMismatch
	}
	if err := s.audit(ctx, "cookie.banner_config.update", BannerEntityType, id, nil, nil); err != nil {
		return BannerConfig{}, err
	}
	return s.GetBannerConfig(ctx, id)
}

// PublishBannerConfig publishes a draft version — archiving every other published version of the same
// domain in the same transaction, so a visitor's SDK fetch always sees at most one published version.
func (s *Service) PublishBannerConfig(ctx context.Context, id uuid.UUID, rowVersion int32) (BannerConfig, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.publish") {
		return BannerConfig{}, ErrForbidden
	}
	q := cookiestore.New(pdb.MustTxFromContext(ctx))
	row, err := q.LockBannerConfig(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return BannerConfig{}, ErrNotFound
	}
	if err != nil {
		return BannerConfig{}, err
	}
	if row.Status != "draft" {
		return BannerConfig{}, fmt.Errorf("%w: not a draft", ErrInvalid)
	}
	if err := q.ArchivePublishedBannerConfigs(ctx, row.DomainID); err != nil {
		return BannerConfig{}, err
	}
	n, err := q.PublishBannerConfig(ctx, cookiestore.PublishBannerConfigParams{ID: id, RowVersion: rowVersion})
	if err != nil {
		return BannerConfig{}, err
	}
	if n == 0 {
		return BannerConfig{}, ErrVersionMismatch
	}
	// A domain's own public key, so the SDK can fetch this (and every future) published version — issued
	// once, on first publish, the same coalesce-the-key pattern DSAR-01/PNG-06/BRE-01 already use, just via
	// the generic platform.public_keys lookup since cookie.domains has no public_key column of its own.
	if _, ok, err := publickeys.Get(ctx, publickeys.EntityCookieDomain, row.DomainID); err != nil {
		return BannerConfig{}, err
	} else if !ok {
		if _, err := publickeys.Issue(ctx, publickeys.EntityCookieDomain, row.DomainID, nil); err != nil {
			return BannerConfig{}, err
		}
	}
	if err := s.audit(ctx, "cookie.banner_config.publish", BannerEntityType, id, nil, map[string]any{"version_no": row.VersionNo}); err != nil {
		return BannerConfig{}, err
	}
	return s.GetBannerConfig(ctx, id)
}

// PublicBanner is CON-01's own acceptance criterion starting point: what the SDK fetches before it ever
// shows the banner. No categories bundled with the HTTP response here — the handler composes that
// alongside this from ListCategories's own permission-free read (the public key already proves tenant
// scope, rule 1).
type PublicBanner struct {
	ID        uuid.UUID
	DomainID  uuid.UUID
	VersionNo int32
	Layout    string
	Theme     BannerTheme
	Texts     BannerTexts
}

// PublicBannerConfig returns the domain's current published banner — no permission check, the public key
// itself already proves this call is scoped to the right tenant (the same shape DSAR-14/PNG-06/BRE-01's own
// public reads already use).
func (s *Service) PublicBannerConfig(ctx context.Context, domainID uuid.UUID) (PublicBanner, error) {
	row, err := cookiestore.New(pdb.MustTxFromContext(ctx)).GetPublishedBannerConfig(ctx, domainID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicBanner{}, ErrNotFound
	}
	if err != nil {
		return PublicBanner{}, err
	}
	bc, err := toBannerConfig(row.ID, row.DomainID, row.VersionNo, row.Status, row.Layout, row.Theme, row.Texts, row.RowVersion)
	if err != nil {
		return PublicBanner{}, err
	}
	return PublicBanner{ID: bc.ID, DomainID: domainID, VersionNo: bc.VersionNo, Layout: bc.Layout, Theme: bc.Theme, Texts: bc.Texts}, nil
}

// PublicCategories is the public, permission-free twin of ListCategories for the SDK's own settings panel.
func (s *Service) PublicCategories(ctx context.Context) ([]Category, error) {
	rows, err := cookiestore.New(pdb.MustTxFromContext(ctx)).ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Category, 0, len(rows))
	for _, r := range rows {
		cat, err := toCategory(r.ID, r.Code, r.Names, r.Descriptions, r.IsRequired, r.DisplayOrder, r.RowVersion)
		if err != nil {
			return nil, err
		}
		out = append(out, cat)
	}
	return out, nil
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
