package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	cookie "pdpa-platform/internal/cookie/service"
)

// Acceptance CON-06: changing the design (theme) persists and is reflected in both the draft read-back
// (admin preview) and the public read (the live site after publish).
func TestBannerConfig_ThemeAndTranslationsPersistAndPublish(t *testing.T) {
	e := setup(t, "cookietheme")
	e.in(t, func(ctx context.Context) error {
		d, err := e.svc.RegisterDomain(ctx, "theme.example", e.legalEntity)
		if err != nil {
			return err
		}

		theme := cookie.BannerTheme{PrimaryColor: "#112233", TextColor: "#ffffff", Position: "bottom", LogoURL: "https://example.com/logo.png"}
		texts := fullTexts()
		texts.Translations = map[string]cookie.BannerTranslation{
			"zh": {Title: "我们使用Cookie", AcceptAll: "接受全部", RejectAll: "拒绝全部", Settings: "设置"},
		}

		v1, err := e.svc.CreateBannerConfig(ctx, d.ID, cookie.CreateBannerConfigInput{Layout: "bar", Theme: theme, Texts: texts})
		if err != nil {
			return err
		}
		if v1.Theme.PrimaryColor != "#112233" || v1.Theme.Position != "bottom" {
			t.Errorf("draft theme not persisted: %+v", v1.Theme)
		}
		if got := v1.Texts.Translations["zh"].Title; got != "我们使用Cookie" {
			t.Errorf("draft translation not persisted: %q", got)
		}

		// Preview (admin read-back) reflects the draft immediately.
		got, err := e.svc.GetBannerConfig(ctx, v1.ID)
		if err != nil || got.Theme.PrimaryColor != "#112233" {
			t.Errorf("GetBannerConfig theme = %+v %v", got.Theme, err)
		}

		if _, err := e.svc.PublishBannerConfig(ctx, v1.ID, v1.RowVersion); err != nil {
			return err
		}

		// After publish, the live (public) read carries the same theme and translation.
		pub, err := e.svc.PublicBannerConfig(ctx, d.ID)
		if err != nil {
			return err
		}
		if pub.Theme.PrimaryColor != "#112233" || pub.Theme.Position != "bottom" {
			t.Errorf("public theme = %+v, want persisted theme", pub.Theme)
		}
		if got := pub.Texts.Translations["zh"].AcceptAll; got != "接受全部" {
			t.Errorf("public translation = %q", got)
		}
		return nil
	})
}

func TestBannerConfig_ThemeValidation(t *testing.T) {
	e := setup(t, "cookiethemebad")
	e.in(t, func(ctx context.Context) error {
		d, err := e.svc.RegisterDomain(ctx, "themebad.example", e.legalEntity)
		if err != nil {
			return err
		}
		if _, err := e.svc.CreateBannerConfig(ctx, d.ID, cookie.CreateBannerConfigInput{
			Layout: "bar", Theme: cookie.BannerTheme{Position: "left"}, Texts: fullTexts(),
		}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("unknown position: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.CreateBannerConfig(ctx, d.ID, cookie.CreateBannerConfigInput{
			Layout: "bar", Theme: cookie.BannerTheme{CustomCSS: strings.Repeat("a", 2001)}, Texts: fullTexts(),
		}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("oversized custom_css: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.CreateBannerConfig(ctx, d.ID, cookie.CreateBannerConfigInput{
			Layout: "bar", Theme: cookie.BannerTheme{CustomCSS: ".x{color:red}<script>alert(1)</script>"}, Texts: fullTexts(),
		}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("script in custom_css: %v, want ErrInvalid", err)
		}
		texts := fullTexts()
		texts.Translations = map[string]cookie.BannerTranslation{"zh": {Title: "", AcceptAll: "x", RejectAll: "y", Settings: "z"}}
		if _, err := e.svc.CreateBannerConfig(ctx, d.ID, cookie.CreateBannerConfigInput{Layout: "bar", Texts: texts}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("incomplete translation: %v, want ErrInvalid", err)
		}
		return nil
	})
}
