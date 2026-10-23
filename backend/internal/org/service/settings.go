package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	orgstore "pdpa-platform/internal/org/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// OrgSettingsEntityType is the audit/file entity type of the tenant's one settings row (ORG-20: language,
// date era and branding — the calendar half of ORG-20 lives in calendars.go).
const OrgSettingsEntityType = "org_settings"

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Branding is the tenant's logo and theme colors, used on the portal and generated documents.
type Branding struct {
	LogoFileID  *uuid.UUID `json:"logo_file_id,omitempty"`
	ThemeColor  string     `json:"theme_color,omitempty"`
	AccentColor string     `json:"accent_color,omitempty"`
}

// OrgSettings is the tenant's one settings row. A tenant that never saved one yet reads as the defaults
// below (RowVersion 0), same as ORG-20's calendars default to Mon–Fri Asia/Bangkok until one is created.
type OrgSettings struct {
	DefaultLanguage string
	DateEra         string // BE | CE
	Branding        Branding
	RowVersion      int32
	UpdatedAt       time.Time
}

func defaultOrgSettings() OrgSettings {
	return OrgSettings{DefaultLanguage: "th", DateEra: "BE"}
}

func (s *Service) GetOrgSettings(ctx context.Context) (OrgSettings, error) {
	r, err := orgstore.New(pdb.MustTxFromContext(ctx)).GetOrgSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultOrgSettings(), nil
	}
	if err != nil {
		return OrgSettings{}, err
	}
	return toOrgSettings(r.DefaultLanguage, r.DateEra, r.Branding, r.RowVersion, r.UpdatedAt.Time), nil
}

// SaveOrgSettings validates and upserts the tenant's settings. version must match the row_version the
// caller last read (0 for a tenant with no row yet) — a mismatch, including a first save racing another
// first save, is ErrVersionMismatch.
func (s *Service) SaveOrgSettings(ctx context.Context, in OrgSettings, version int32) (OrgSettings, error) {
	in.DefaultLanguage = normalizeLocale(in.DefaultLanguage)
	if in.DefaultLanguage != "th" && in.DefaultLanguage != "en" {
		return OrgSettings{}, fmt.Errorf("%w: default_language must be th or en", ErrInvalid)
	}
	if in.DateEra != "BE" && in.DateEra != "CE" {
		return OrgSettings{}, fmt.Errorf("%w: date_era must be BE or CE", ErrInvalid)
	}
	if in.Branding.ThemeColor != "" && !hexColor.MatchString(in.Branding.ThemeColor) {
		return OrgSettings{}, fmt.Errorf("%w: theme_color must be #RRGGBB", ErrInvalid)
	}
	if in.Branding.AccentColor != "" && !hexColor.MatchString(in.Branding.AccentColor) {
		return OrgSettings{}, fmt.Errorf("%w: accent_color must be #RRGGBB", ErrInvalid)
	}
	before, err := s.GetOrgSettings(ctx)
	if err != nil {
		return OrgSettings{}, err
	}
	if before.RowVersion != version {
		return OrgSettings{}, ErrVersionMismatch
	}
	if in.Branding.LogoFileID != nil && (before.Branding.LogoFileID == nil || *before.Branding.LogoFileID != *in.Branding.LogoFileID) {
		if err := s.attachOrgLogo(ctx, *in.Branding.LogoFileID); err != nil {
			return OrgSettings{}, err
		}
	}
	branding, _ := json.Marshal(in.Branding)
	r, err := orgstore.New(pdb.MustTxFromContext(ctx)).UpsertOrgSettings(ctx, orgstore.UpsertOrgSettingsParams{
		DefaultLanguage: in.DefaultLanguage, DateEra: in.DateEra, Branding: branding, RowVersion: version,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgSettings{}, ErrVersionMismatch
	}
	if err != nil {
		return OrgSettings{}, err
	}
	out := toOrgSettings(r.DefaultLanguage, r.DateEra, r.Branding, r.RowVersion, r.UpdatedAt.Time)
	return out, s.auditSettings(ctx, orgSettingsAudit(before), orgSettingsAudit(out))
}

func (s *Service) attachOrgLogo(ctx context.Context, fileID uuid.UUID) error {
	if s.Files == nil {
		return ErrLogoNotUse
	}
	f, err := s.Files.Get(ctx, fileID) // only the uploader sees an unattached file
	if err != nil || f.EntityType != "" || f.AVStatus != "clean" || (f.MimeType != "image/png" && f.MimeType != "image/jpeg") {
		return ErrLogoNotUse
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		return fmt.Errorf("org: attach logo without a tenant: %w", err)
	}
	return s.Files.Attach(ctx, fileID, OrgSettingsEntityType, tenant)
}

// auditSettings writes the audit row keyed by the tenant itself (org_settings has no id of its own).
func (s *Service) auditSettings(ctx context.Context, before, after any) error {
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		return fmt.Errorf("org: audit without a tenant: %w", err)
	}
	return s.audit(ctx, "org.settings.update", tenant, before, after)
}

func orgSettingsAudit(v OrgSettings) map[string]any {
	return map[string]any{"default_language": v.DefaultLanguage, "date_era": v.DateEra, "branding": v.Branding}
}

func toOrgSettings(lang, era string, branding []byte, version int32, updatedAt time.Time) OrgSettings {
	var b Branding
	_ = json.Unmarshal(branding, &b)
	return OrgSettings{DefaultLanguage: lang, DateEra: era, Branding: b, RowVersion: version, UpdatedAt: updatedAt}
}

func normalizeLocale(s string) string {
	if len(s) > 2 {
		s = s[:2]
	}
	return s
}
