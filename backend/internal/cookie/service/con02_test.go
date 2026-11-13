package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	cookie "pdpa-platform/internal/cookie/service"
)

// Acceptance CON-02 (backend note): registering a tenant's first domain seeds the 4 default categories
// (Strictly Necessary / Functional / Analytics / Marketing) — only "necessary" is required.
func TestRegisterDomain_SeedsFourDefaultCategoriesOnlyOnce(t *testing.T) {
	e := setup(t, "cookiedefault")
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RegisterDomain(ctx, "first.example", e.legalEntity); err != nil {
			return err
		}
		cats, err := e.svc.ListCategories(ctx)
		if err != nil || len(cats) != 4 {
			t.Fatalf("categories after first domain = %+v (%v), want 4 defaults", cats, err)
		}
		byCode := map[string]cookie.Category{}
		for _, c := range cats {
			byCode[c.Code] = c
		}
		for _, code := range []string{"necessary", "functional", "analytics", "marketing"} {
			if _, ok := byCode[code]; !ok {
				t.Errorf("missing default category %q", code)
			}
		}
		if !byCode["necessary"].IsRequired {
			t.Error("necessary should be required")
		}
		for _, code := range []string{"functional", "analytics", "marketing"} {
			if byCode[code].IsRequired {
				t.Errorf("%s should not be required", code)
			}
		}

		// A second domain for the same tenant must not re-seed (the DPO may have already edited the set).
		if err := e.svc.DeleteCategory(ctx, byCode["marketing"].ID); err != nil {
			return err
		}
		if _, err := e.svc.RegisterDomain(ctx, "second.example", e.legalEntity); err != nil {
			return err
		}
		cats, err = e.svc.ListCategories(ctx)
		if err != nil || len(cats) != 3 {
			t.Errorf("categories after second domain = %+v (%v), want still 3 (no re-seed)", cats, err)
		}
		return nil
	})
}

func (e env) withDomainAndCategory(t *testing.T, ctx context.Context, domain, catCode string) (domainID, categoryID uuid.UUID) {
	t.Helper()
	d, err := e.svc.RegisterDomain(ctx, domain, e.legalEntity)
	if err != nil {
		t.Fatal(err)
	}
	// RegisterDomain already seeded the 4 defaults; reuse "analytics" unless a different code is wanted.
	cats, err := e.svc.ListCategories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cats {
		if c.Code == catCode {
			return d.ID, c.ID
		}
	}
	t.Fatalf("category %q not found among %+v", catCode, cats)
	return uuid.Nil, uuid.Nil
}

func TestCreateCookie_ValidatesDomainCategoryAndStorageType(t *testing.T) {
	e := setup(t, "cookiecookies")
	e.in(t, func(ctx context.Context) error {
		domainID, categoryID := e.withDomainAndCategory(t, ctx, "cookies.example", "analytics")

		if _, err := e.svc.CreateCookie(ctx, uuid.New(), cookie.CreateCookieInput{Name: "_ga", ProviderDomain: "google.com", StorageType: "http_cookie"}); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("unknown domain: %v, want ErrNotFound", err)
		}
		if _, err := e.svc.CreateCookie(ctx, domainID, cookie.CreateCookieInput{Name: "_ga", ProviderDomain: "google.com", StorageType: "not_a_type"}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("bad storage_type: %v, want ErrInvalid", err)
		}
		bad := uuid.New()
		if _, err := e.svc.CreateCookie(ctx, domainID, cookie.CreateCookieInput{Name: "_ga", ProviderDomain: "google.com", StorageType: "http_cookie", CategoryID: &bad}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("unknown category: %v, want ErrInvalid", err)
		}

		c, err := e.svc.CreateCookie(ctx, domainID, cookie.CreateCookieInput{Name: "_ga", ProviderDomain: "google.com", CategoryID: &categoryID, StorageType: "http_cookie", Duration: "2 years"})
		if err != nil {
			return err
		}
		if c.Name != "_ga" || c.CategoryID == nil || *c.CategoryID != categoryID {
			t.Errorf("created cookie = %+v, want name _ga and the given category", c)
		}

		list, err := e.svc.ListCookies(ctx, domainID)
		if err != nil || len(list) != 1 {
			t.Errorf("list = %+v (%v)", list, err)
		}

		if err := e.svc.DeleteCookie(ctx, c.ID); err != nil {
			return err
		}
		if err := e.svc.DeleteCookie(ctx, uuid.New()); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("delete unknown: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// Acceptance CON-02: a script rule declares which category a script belongs to — the mechanism the SDK
// uses to enable/disable scripts the moment a visitor's preference changes.
func TestScriptRule_ValidatesAndIsVisibleToThePublicRead(t *testing.T) {
	e := setup(t, "cookiescriptrules")
	e.in(t, func(ctx context.Context) error {
		domainID, categoryID := e.withDomainAndCategory(t, ctx, "scripts.example", "marketing")

		bad := uuid.New()
		if _, err := e.svc.CreateScriptRule(ctx, domainID, cookie.CreateScriptRuleInput{Pattern: "facebook.net", CategoryID: bad}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("unknown category: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.CreateScriptRule(ctx, domainID, cookie.CreateScriptRuleInput{Pattern: "facebook.net", CategoryID: categoryID, Action: "not_an_action"}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("bad action: %v, want ErrInvalid", err)
		}

		rule, err := e.svc.CreateScriptRule(ctx, domainID, cookie.CreateScriptRuleInput{Pattern: "connect.facebook.net", CategoryID: categoryID})
		if err != nil {
			return err
		}
		if rule.Action != "block" {
			t.Errorf("default action = %q, want block", rule.Action)
		}

		list, err := e.svc.ListScriptRules(ctx, domainID)
		if err != nil || len(list) != 1 {
			t.Errorf("list = %+v (%v)", list, err)
		}

		pub, err := e.svc.PublicScriptRules(ctx, domainID)
		if err != nil || len(pub) != 1 || pub[0].CategoryCode != "marketing" || pub[0].Pattern != "connect.facebook.net" {
			t.Errorf("public script rules = %+v (%v), want one rule keyed by category code", pub, err)
		}

		if err := e.svc.DeleteScriptRule(ctx, rule.ID); err != nil {
			return err
		}
		pub, err = e.svc.PublicScriptRules(ctx, domainID)
		if err != nil || len(pub) != 0 {
			t.Errorf("public script rules after delete = %+v (%v), want none", pub, err)
		}
		if err := e.svc.DeleteScriptRule(ctx, uuid.New()); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("delete unknown: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// Two-tenant isolation (PLT-02 rule 1): tenant B can't reach tenant A's cookies or script rules.
func TestCookiesAndScriptRules_TenantIsolation(t *testing.T) {
	a := setup(t, "cookiecrossa")
	b := setup(t, "cookiecrossb")

	var domainID, cookieID, ruleID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		did, catID := a.withDomainAndCategory(t, ctx, "cross.example", "analytics")
		domainID = did
		c, err := a.svc.CreateCookie(ctx, did, cookie.CreateCookieInput{Name: "_ga", ProviderDomain: "google.com", StorageType: "http_cookie", CategoryID: &catID})
		if err != nil {
			return err
		}
		cookieID = c.ID
		r, err := a.svc.CreateScriptRule(ctx, did, cookie.CreateScriptRuleInput{Pattern: "googletagmanager.com", CategoryID: catID})
		if err != nil {
			return err
		}
		ruleID = r.ID
		return nil
	})

	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.ListCookies(ctx, domainID); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("tenant B listing tenant A's domain cookies: %v, want ErrNotFound", err)
		}
		if err := b.svc.DeleteCookie(ctx, cookieID); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("tenant B deleting tenant A's cookie: %v, want ErrNotFound", err)
		}
		if _, err := b.svc.ListScriptRules(ctx, domainID); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("tenant B listing tenant A's domain script rules: %v, want ErrNotFound", err)
		}
		if err := b.svc.DeleteScriptRule(ctx, ruleID); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("tenant B deleting tenant A's script rule: %v, want ErrNotFound", err)
		}
		return nil
	})
}
