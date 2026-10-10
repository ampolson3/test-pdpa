package docs_test

import (
	"context"
	"testing"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/docs"
)

var standardDsaTemplateCodes = []string{"one_way", "two_way", "government", "research"}

func dsaPolicy() docs.Policy {
	return docs.Policy{Read: "agreement.dsa.read", Create: "agreement.dsa.create", Update: "agreement.dsa.update",
		Publish: "agreement.dsa.publish", Approver: "DPO", TemplateRead: "agreement.dsa.read", TemplateWrite: "agreement.dsa.update"}
}

// TestStandardDsaTemplates_SeededReady is DSA-03's own acceptance criterion ("มี template ครบ 4 แบบ
// TH/EN"): four published, global "dsa" templates (one-way disclosure, two-way exchange, government
// agency, research) each with both Thai and English content already exist (migration 00060,
// docs/decisions.md Q-34) — no new backend code needed, the exact same shape DPA-01's own migration
// already took for "dpa". Bypasses docstest.Setup (needs S3+clamd, unrelated to what this reads) for a
// bare Service with "dsa" registered — Access/ListTemplates touch neither.
func TestStandardDsaTemplates_SeededReady(t *testing.T) {
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, context.Background(), app, dbtest.PlatformPool(t), "dsaTemplates")
	svc := &docs.Service{}
	svc.Register("dsa", dsaPolicy())

	err := pdb.WithTenantTx(context.Background(), app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"agreement.dsa.read", "agreement.dsa.update"}})
		list, err := svc.ListTemplates(ctx, "dsa", true)
		if err != nil {
			return err
		}
		found := map[string]bool{}
		for _, tpl := range list {
			found[tpl.Code] = true
			if !tpl.Global {
				t.Errorf("%s template is not global: %+v", tpl.Code, tpl)
			}
			if tpl.Status != "published" {
				t.Errorf("%s template status %q, want published", tpl.Code, tpl.Status)
			}
			if _, ok := tpl.Content["th"]; !ok {
				t.Errorf("%s template missing Thai content", tpl.Code)
			}
			if _, ok := tpl.Content["en"]; !ok {
				t.Errorf("%s template missing English content", tpl.Code)
			}
		}
		for _, code := range standardDsaTemplateCodes {
			if !found[code] {
				t.Errorf("expected a published global %q template among dsa templates", code)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestStandardDsaTemplates_VisibleToAnyTenant proves the seeded templates are tenant-independent global
// defaults (the same pattern ORG-07/ROPA-09/PNG-03/DPIA-01/VEN-04/DPA-01 already established) — a second,
// entirely separate tenant with none of its own sees all four too.
func TestStandardDsaTemplates_VisibleToAnyTenant(t *testing.T) {
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, context.Background(), app, dbtest.PlatformPool(t), "dsaTemplatesB")
	svc := &docs.Service{}
	svc.Register("dsa", dsaPolicy())

	err := pdb.WithTenantTx(context.Background(), app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"agreement.dsa.read"}})
		list, err := svc.ListTemplates(ctx, "dsa", true)
		if err != nil {
			return err
		}
		found := map[string]bool{}
		for _, tpl := range list {
			found[tpl.Code] = true
		}
		for _, code := range standardDsaTemplateCodes {
			if !found[code] {
				t.Errorf("tenant B does not see the global %q template", code)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
