package service_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	cookie "pdpa-platform/internal/cookie/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/publickeys"
)

var allPerms = []string{"cookie.banner.create", "cookie.banner.read", "cookie.banner.update", "cookie.banner.publish", "cookie.banner.delete"}

type env struct {
	app         *pgxpool.Pool
	tenant      dbtest.Tenant
	legalEntity uuid.UUID
	svc         *cookie.Service
}

// fakeOrg satisfies cookie.Service's Org interface without needing the org module's own tables — a
// RegisterDomain FK-visibility check only needs to know whether an id is visible, not any other org data.
type fakeOrg struct{ visible uuid.UUID }

func (f fakeOrg) GetLegalEntity(_ context.Context, id uuid.UUID) (orgservice.LegalEntity, error) {
	if id != f.visible {
		return orgservice.LegalEntity{}, orgservice.ErrNotFound
	}
	return orgservice.LegalEntity{ID: id}, nil
}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	legalEntity := uuid.Must(uuid.NewV7())
	// cookie.domains.legal_entity_id is a real FK to org.legal_entities, which bypasses RLS (rule 1) —
	// so beyond fakeOrg's own service-layer visibility check, a real row with this exact id must exist
	// for RegisterDomain's own insert to succeed against the live constraint.
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		tx, _ := pdb.TxFromContext(ctx)
		_, err := tx.Exec(ctx, `INSERT INTO org.legal_entities (id, tenant_id, name_th) VALUES ($1, $2, 'dbtest')`, legalEntity, tenant.ID)
		return err
	}); err != nil {
		t.Fatalf("dbtest: seed legal entity: %v", err)
	}
	return env{app: app, tenant: tenant, legalEntity: legalEntity,
		svc: &cookie.Service{Audit: audit.New(), Org: fakeOrg{visible: legalEntity}}}
}

func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: allPerms}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func fullTexts() cookie.BannerTexts {
	return cookie.BannerTexts{TitleTh: "เราใช้คุกกี้", AcceptAllTh: "ยอมรับทั้งหมด", RejectAllTh: "ปฏิเสธทั้งหมด", SettingsTh: "ตั้งค่า"}
}

// Acceptance CON-01: no category is pre-ticked unless the tenant itself marked it required.
func TestCreateCategory_IsRequiredControlsPreTicking(t *testing.T) {
	e := setup(t, "cookiecat")
	e.in(t, func(ctx context.Context) error {
		necessary, err := e.svc.CreateCategory(ctx, cookie.CreateCategoryInput{Code: "necessary", NameTh: "จำเป็น", IsRequired: true})
		if err != nil {
			return err
		}
		analytics, err := e.svc.CreateCategory(ctx, cookie.CreateCategoryInput{Code: "analytics", NameTh: "วิเคราะห์", IsRequired: false})
		if err != nil {
			return err
		}
		if !necessary.IsRequired || analytics.IsRequired {
			t.Errorf("necessary=%v analytics=%v", necessary.IsRequired, analytics.IsRequired)
		}
		if _, err := e.svc.CreateCategory(ctx, cookie.CreateCategoryInput{Code: "necessary", NameTh: "ซ้ำ"}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("duplicate code: %v, want ErrInvalid", err)
		}
		list, err := e.svc.ListCategories(ctx)
		if err != nil || len(list) != 2 {
			t.Errorf("list = %+v %v", list, err)
		}
		if err := e.svc.DeleteCategory(ctx, analytics.ID); err != nil {
			return err
		}
		list, err = e.svc.ListCategories(ctx)
		if err != nil || len(list) != 1 {
			t.Errorf("after delete = %+v %v", list, err)
		}
		if err := e.svc.DeleteCategory(ctx, uuid.New()); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("delete unknown: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func TestRegisterDomain_ValidatesLegalEntityAndUniqueness(t *testing.T) {
	e := setup(t, "cookiedom")
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RegisterDomain(ctx, "example.com", uuid.New()); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("unknown legal entity: %v, want ErrInvalid", err)
		}
		d, err := e.svc.RegisterDomain(ctx, " Example.com ", e.legalEntity)
		if err != nil {
			return err
		}
		if d.Domain != "example.com" || d.Verified {
			t.Errorf("registered domain = %+v, want lower-cased \"example.com\" and not yet verified", d)
		}
		if _, err := e.svc.RegisterDomain(ctx, "example.com", e.legalEntity); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("duplicate domain: %v, want ErrInvalid", err)
		}
		verified, err := e.svc.VerifyDomain(ctx, d.ID, d.RowVersion)
		if err != nil || !verified.Verified {
			t.Errorf("verify = %+v %v", verified, err)
		}
		if _, err := e.svc.VerifyDomain(ctx, d.ID, d.RowVersion); !errors.Is(err, cookie.ErrVersionMismatch) {
			t.Errorf("stale verify: %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

// Acceptance CON-01: publishing archives every sibling published version, and the install snippet's own
// public key is issued only on first publish (a second publish reuses it).
func TestBannerConfig_PublishLifecycleAndPublicKey(t *testing.T) {
	e := setup(t, "cookieban")
	var domainID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		d, err := e.svc.RegisterDomain(ctx, "banner.example", e.legalEntity)
		if err != nil {
			return err
		}
		domainID = d.ID

		v1, err := e.svc.CreateBannerConfig(ctx, domainID, cookie.CreateBannerConfigInput{Layout: "bar", Texts: fullTexts()})
		if err != nil {
			return err
		}
		if _, err := e.svc.CreateBannerConfig(ctx, domainID, cookie.CreateBannerConfigInput{Layout: "nope", Texts: fullTexts()}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("bad layout: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.CreateBannerConfig(ctx, domainID, cookie.CreateBannerConfigInput{Layout: "bar", Texts: cookie.BannerTexts{}}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("missing required texts: %v, want ErrInvalid", err)
		}

		if _, ok, err := publickeys.Get(ctx, publickeys.EntityCookieDomain, domainID); err != nil {
			return err
		} else if ok {
			t.Error("no key before any publish")
		}

		published, err := e.svc.PublishBannerConfig(ctx, v1.ID, v1.RowVersion)
		if err != nil || published.Status != "published" {
			t.Errorf("publish v1 = %+v %v", published, err)
		}
		key1, ok, err := publickeys.Get(ctx, publickeys.EntityCookieDomain, domainID)
		if err != nil || !ok || key1 == "" {
			t.Fatalf("key after first publish: %v %v %v", key1, ok, err)
		}

		v2, err := e.svc.CreateBannerConfig(ctx, domainID, cookie.CreateBannerConfigInput{Layout: "modal", Texts: fullTexts()})
		if err != nil {
			return err
		}
		if _, err := e.svc.PublishBannerConfig(ctx, v2.ID, v2.RowVersion); err != nil {
			return err
		}
		key2, ok, err := publickeys.Get(ctx, publickeys.EntityCookieDomain, domainID)
		if err != nil || !ok || key2 != key1 {
			t.Errorf("second publish should reuse the same key: %v vs %v (%v)", key2, key1, err)
		}

		list, err := e.svc.ListBannerConfigs(ctx, domainID)
		if err != nil || len(list) != 2 {
			return err
		}
		for _, c := range list {
			if c.ID == v1.ID && c.Status != "archived" {
				t.Errorf("v1 should be archived once v2 published: %+v", c)
			}
			if c.ID == v2.ID && c.Status != "published" {
				t.Errorf("v2 should be published: %+v", c)
			}
		}

		pub, err := e.svc.PublicBannerConfig(ctx, domainID)
		if err != nil || pub.ID != v2.ID || pub.VersionNo != v2.VersionNo {
			t.Errorf("public banner = %+v %v, want the published version %+v", pub, err, v2)
		}
		return nil
	})
}

// Acceptance CON-01: the evidence a reject-all click writes — one call, the right action, the IP hashed
// rather than stored raw (rule 3: the schema's own ip_hash column proves this at the data-model level, so
// this test checks the service never even offers a way to read back the clear-text address).
func TestRecordConsent_RejectAllInOneCallAndHashesIP(t *testing.T) {
	e := setup(t, "cookiecon")
	var domainID, bannerID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		d, err := e.svc.RegisterDomain(ctx, "consent.example", e.legalEntity)
		if err != nil {
			return err
		}
		domainID = d.ID
		v1, err := e.svc.CreateBannerConfig(ctx, domainID, cookie.CreateBannerConfigInput{Layout: "bar", Texts: fullTexts()})
		if err != nil {
			return err
		}
		if _, err := e.svc.PublishBannerConfig(ctx, v1.ID, v1.RowVersion); err != nil {
			return err
		}
		bannerID = v1.ID

		ip := netip.MustParseAddr("203.0.113.9")
		if err := e.svc.RecordConsent(ctx, cookie.RecordConsentInput{
			DomainID: domainID, VisitorID: "visitor-1", BannerConfigID: bannerID, Action: "reject_all", IP: ip, UserAgent: "test-agent",
		}); err != nil {
			return err
		}

		var action string
		var ipHash *string
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT action, ip_hash FROM cookie.consent_records WHERE domain_id = $1 AND visitor_id = 'visitor-1'`, domainID,
		).Scan(&action, &ipHash); err != nil {
			return err
		}
		if action != "reject_all" {
			t.Errorf("action = %q, want reject_all", action)
		}
		if ipHash == nil || *ipHash == "" || *ipHash == ip.String() {
			t.Errorf("ip_hash = %v, want a hash (never the raw address)", ipHash)
		}

		if err := e.svc.RecordConsent(ctx, cookie.RecordConsentInput{DomainID: domainID, VisitorID: "", BannerConfigID: bannerID, Action: "accept_all"}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("missing visitor_id: %v, want ErrInvalid", err)
		}
		if err := e.svc.RecordConsent(ctx, cookie.RecordConsentInput{DomainID: domainID, VisitorID: "v2", BannerConfigID: uuid.New(), Action: "accept_all"}); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("unknown banner_config_id: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// Two-tenant isolation (PLT-02 rule 1): tenant B's own transaction can never see tenant A's domains,
// categories or banner configs, even by a direct id lookup.
func TestCookieModule_TenantIsolation(t *testing.T) {
	a := setup(t, "cookieisoa")
	b := setup(t, "cookieisob")

	var domainID uuid.UUID
	var categoryID uuid.UUID
	var bannerID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		d, err := a.svc.RegisterDomain(ctx, "isolation.example", a.legalEntity)
		if err != nil {
			return err
		}
		domainID = d.ID
		// "analytics" is already seeded as one of the 4 defaults the moment the domain above was
		// registered (seedDefaultCategoriesIfNone) — a different code avoids colliding with it.
		c, err := a.svc.CreateCategory(ctx, cookie.CreateCategoryInput{Code: "custom_cat", NameTh: "กำหนดเอง"})
		if err != nil {
			return err
		}
		categoryID = c.ID
		bc, err := a.svc.CreateBannerConfig(ctx, domainID, cookie.CreateBannerConfigInput{Layout: "bar", Texts: fullTexts()})
		if err != nil {
			return err
		}
		bannerID = bc.ID
		return nil
	})

	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetDomain(ctx, domainID); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's domain: %v, want ErrNotFound", err)
		}
		list, err := b.svc.ListDomains(ctx)
		if err != nil || len(list) != 0 {
			t.Errorf("tenant B's domain list should be empty, got %+v (%v)", list, err)
		}
		cats, err := b.svc.ListCategories(ctx)
		if err != nil || len(cats) != 0 {
			t.Errorf("tenant B's category list should not include tenant A's: %+v (%v)", cats, err)
		}
		for _, c := range cats {
			if c.ID == categoryID {
				t.Error("tenant B saw tenant A's category id")
			}
		}
		if _, err := b.svc.GetBannerConfig(ctx, bannerID); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("tenant B should not reach tenant A's banner config: %v, want ErrNotFound", err)
		}
		if _, err := b.svc.ListBannerConfigs(ctx, domainID); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("tenant B should not list tenant A's domain's banner configs: %v, want ErrNotFound", err)
		}
		return nil
	})
}
