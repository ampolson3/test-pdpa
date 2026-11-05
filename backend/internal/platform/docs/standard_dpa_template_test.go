package docs_test

import (
	"context"
	"testing"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/docs"
)

// TestStandardDpaTemplate_SeededReady is DPA-01's own acceptance criterion
// ("มี template ภาษาไทยและอังกฤษพร้อมใช้"): a published, global "dpa" template with both Thai and English
// content already exists (migration 00054, docs/decisions.md Q-32) — no new backend code needed, since
// PLT-16's own generic template library (Service.ListTemplates/CreateTemplate/PublishTemplate on
// platform.templates) already serves any registered doc type, "dpa" included. Deliberately bypasses
// docstest.Setup (which unconditionally needs S3+clamd, unrelated to what this reads) for a bare Service
// with "dpa" registered — Access/ListTemplates touch neither.
func TestStandardDpaTemplate_SeededReady(t *testing.T) {
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, context.Background(), app, dbtest.PlatformPool(t), "dpaTemplate")
	svc := &docs.Service{}
	svc.Register("dpa", docs.Policy{Read: "agreement.dpa.read", Create: "agreement.dpa.create", Update: "agreement.dpa.update",
		Publish: "agreement.dpa.publish", Approver: "DPO", TemplateRead: "agreement.dpa.read", TemplateWrite: "agreement.dpa.update"})

	err := pdb.WithTenantTx(context.Background(), app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"agreement.dpa.read", "agreement.dpa.update"}})
		list, err := svc.ListTemplates(ctx, "dpa", true)
		if err != nil {
			return err
		}
		found := false
		for _, tpl := range list {
			if tpl.Code != "standard_dpa" {
				continue
			}
			found = true
			if !tpl.Global {
				t.Errorf("standard_dpa template is not global: %+v", tpl)
			}
			if tpl.Status != "published" {
				t.Errorf("standard_dpa template status %q, want published", tpl.Status)
			}
			if _, ok := tpl.Content["th"]; !ok {
				t.Error("standard_dpa template missing Thai content")
			}
			if _, ok := tpl.Content["en"]; !ok {
				t.Error("standard_dpa template missing English content")
			}
		}
		if !found {
			t.Fatal("expected a published global 'standard_dpa' template among dpa templates")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestStandardDpaTemplate_VisibleToAnyTenant proves the seeded template is a tenant-independent global
// default (the same pattern ORG-07/ROPA-09/PNG-03/DPIA-01/VEN-04 already established) — a second, entirely
// separate tenant with none of its own sees it too.
func TestStandardDpaTemplate_VisibleToAnyTenant(t *testing.T) {
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, context.Background(), app, dbtest.PlatformPool(t), "dpaTemplateB")
	svc := &docs.Service{}
	svc.Register("dpa", docs.Policy{Read: "agreement.dpa.read", Create: "agreement.dpa.create", Update: "agreement.dpa.update",
		Publish: "agreement.dpa.publish", Approver: "DPO", TemplateRead: "agreement.dpa.read", TemplateWrite: "agreement.dpa.update"})

	err := pdb.WithTenantTx(context.Background(), app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(),
			Permissions: []string{"agreement.dpa.read"}})
		list, err := svc.ListTemplates(ctx, "dpa", true)
		if err != nil {
			return err
		}
		for _, tpl := range list {
			if tpl.Code == "standard_dpa" {
				return nil
			}
		}
		t.Fatal("tenant B does not see the global standard_dpa template")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
