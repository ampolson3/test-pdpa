package templates_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	templatesservice "pdpa-platform/internal/ropa/templates"
)

func grants(tenant dbtest.Tenant, perms ...string) authz.Grants {
	return authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: perms}
}

// cleanupCustomTemplates deletes whatever custom ropa.template_sets/activity_templates rows a test
// wrote for this tenant — SeedTenant's own cleanup only knows about platform.tenants/iam.users, so every
// test that writes a tenant-owned row elsewhere must clean it up itself (the same convention DPA-08's own
// notes established for platform.document_downloads before platform.documents), or the tenant teardown
// fails its own FK check. Registered after SeedTenant so it runs first (t.Cleanup is LIFO).
func cleanupCustomTemplates(t *testing.T, app *pgxpool.Pool, tenantID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), app, tenantID.String(), "", func(ctx context.Context) error {
			tx, _ := pdb.TxFromContext(ctx)
			if _, err := tx.Exec(ctx, `DELETE FROM ropa.activity_templates WHERE tenant_id = $1`, tenantID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `DELETE FROM ropa.template_sets WHERE tenant_id = $1`, tenantID)
			return err
		})
	})
}

func sampleActivity(code string) templatesservice.ActivityTemplateInput {
	return templatesservice.ActivityTemplateInput{
		Code: code, NameTh: "กิจกรรมทดสอบ " + code, JobCategory: "it_support", Role: "controller",
		Defaults:  []byte(`{"purposes":[{"purpose_text_th":"x","lawful_basis_code":"CONTRACT"}]}`),
		Rationale: []byte(`{"note":"test"}`),
	}
}

// TestCreateTemplateSet_StartsEmptyDraft is the groundwork for both acceptance-criterion halves: a new
// custom set always starts draft, empty, owned by this tenant — not usable by anyone until published.
func TestCreateTemplateSet_StartsEmptyDraft(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg08a")
	cleanupCustomTemplates(t, app, tenant.ID)
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, grants(tenant, "ropa.template.create", "ropa.template.read"))
		set, err := svc.CreateTemplateSet(ctx, "ชุดของกลุ่มบริษัท")
		if err != nil {
			t.Fatalf("CreateTemplateSet: %v", err)
		}
		if set.SetType != "custom" || set.Status != "draft" {
			t.Errorf("expected a draft custom set, got set_type=%q status=%q", set.SetType, set.Status)
		}
		own, err := svc.ListOwnTemplateSets(ctx)
		if err != nil {
			return err
		}
		if len(own) != 1 || own[0].ID != set.ID {
			t.Errorf("expected the new set in ListOwnTemplateSets, got %+v", own)
		}
		// a draft set never shows up in the published-only browse list every other RTG feature relies on.
		published, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		for _, p := range published {
			if p.ID == set.ID {
				t.Errorf("a draft custom set must not appear in the published browse list")
			}
		}
		if _, err := svc.CreateTemplateSet(ctx, ""); !errors.Is(err, templatesservice.ErrInvalid) {
			t.Errorf("expected ErrInvalid for an empty name, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestAddUpdateDeleteActivityTemplate_OnlyOnOwnCustomSet is both the activity-CRUD happy path and the
// ownership guard: a tenant can freely add/edit/delete activities on its own custom set, but never on the
// global standard set (RLS already refuses the write; this proves the service gives a clear ErrInvalid
// instead of a confusing silent failure).
func TestAddUpdateDeleteActivityTemplate_OnlyOnOwnCustomSet(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg08b")
	cleanupCustomTemplates(t, app, tenant.ID)
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, grants(tenant, "ropa.template.create", "ropa.template.read", "ropa.template.update", "ropa.template.delete"))
		set, err := svc.CreateTemplateSet(ctx, "custom set")
		if err != nil {
			return err
		}

		a, err := svc.AddActivityTemplate(ctx, set.ID, sampleActivity("cust_01"))
		if err != nil {
			t.Fatalf("AddActivityTemplate: %v", err)
		}
		if a.Code != "cust_01" || a.RowVersion == 0 {
			t.Errorf("unexpected activity %+v", a)
		}

		// duplicate code within the same set is refused, not a raw constraint-violation 500.
		if _, err := svc.AddActivityTemplate(ctx, set.ID, sampleActivity("cust_01")); !errors.Is(err, templatesservice.ErrInvalid) {
			t.Errorf("expected ErrInvalid for a duplicate code, got %v", err)
		}

		// global standard set: writing is refused with a clear error, not 0-rows-silently-affected.
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		var standardID uuid.UUID
		for _, s := range sets {
			if s.SetType == "standard" {
				standardID = s.ID
			}
		}
		if standardID == uuid.Nil {
			t.Fatal("expected the seeded global standard set")
		}
		if _, err := svc.AddActivityTemplate(ctx, standardID, sampleActivity("cust_02")); !errors.Is(err, templatesservice.ErrInvalid) {
			t.Errorf("expected ErrInvalid writing to the global standard set, got %v", err)
		}

		edited := sampleActivity("cust_01")
		edited.NameTh = "แก้ไขแล้ว"
		updated, err := svc.UpdateActivityTemplate(ctx, a.ID, edited, a.RowVersion)
		if err != nil {
			t.Fatalf("UpdateActivityTemplate: %v", err)
		}
		if updated.NameTh != "แก้ไขแล้ว" {
			t.Errorf("expected the edit to take effect, got %+v", updated)
		}

		// a stale row_version is refused, not silently applied.
		if _, err := svc.UpdateActivityTemplate(ctx, a.ID, edited, a.RowVersion); !errors.Is(err, templatesservice.ErrVersionMismatch) {
			t.Errorf("expected ErrVersionMismatch on a stale update, got %v", err)
		}

		if err := svc.DeleteActivityTemplate(ctx, a.ID, updated.RowVersion); err != nil {
			t.Fatalf("DeleteActivityTemplate: %v", err)
		}
		if _, err := svc.GetActivityTemplate(ctx, a.ID); !errors.Is(err, templatesservice.ErrNotFound) {
			t.Errorf("expected the deleted activity to be gone, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestCloneTemplateSet_CopiesIndependently is the acceptance criterion's own first half ("คัดลอก template
// กลางมาปรับ"): cloning copies every activity verbatim into a brand-new set, and editing the clone
// afterward never touches the source.
func TestCloneTemplateSet_CopiesIndependently(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg08c")
	cleanupCustomTemplates(t, app, tenant.ID)
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, grants(tenant, "ropa.template.create", "ropa.template.read", "ropa.template.update"))
		source, err := svc.CreateTemplateSet(ctx, "source set")
		if err != nil {
			return err
		}
		a1, err := svc.AddActivityTemplate(ctx, source.ID, sampleActivity("src_01"))
		if err != nil {
			return err
		}
		if _, err := svc.AddActivityTemplate(ctx, source.ID, sampleActivity("src_02")); err != nil {
			return err
		}

		clone, err := svc.CloneTemplateSet(ctx, source.ID, "cloned set")
		if err != nil {
			t.Fatalf("CloneTemplateSet: %v", err)
		}
		if clone.ID == source.ID {
			t.Fatal("the clone must be a distinct set")
		}
		cloned, err := svc.ListActivityTemplates(ctx, clone.ID, nil)
		if err != nil {
			return err
		}
		if len(cloned) != 2 {
			t.Fatalf("expected 2 cloned activities, got %d", len(cloned))
		}
		var clonedA1 templatesservice.ActivityTemplate
		for _, a := range cloned {
			if a.Code == "src_01" {
				clonedA1 = a
			}
		}
		if clonedA1.ID == uuid.Nil || clonedA1.ID == a1.ID {
			t.Fatalf("expected a distinct cloned row for src_01, got %+v", clonedA1)
		}

		// editing the clone's own copy never touches the source.
		edited := sampleActivity("src_01")
		edited.NameTh = "แก้ไขใน clone"
		if _, err := svc.UpdateActivityTemplate(ctx, clonedA1.ID, edited, clonedA1.RowVersion); err != nil {
			t.Fatalf("UpdateActivityTemplate on the clone: %v", err)
		}
		sourceA1, err := svc.GetActivityTemplate(ctx, a1.ID)
		if err != nil {
			return err
		}
		if sourceA1.NameTh == "แก้ไขใน clone" {
			t.Error("editing the clone must not affect the source activity")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestPublishAndRetireTemplateSet is the acceptance criterion's own second half ("กำหนด template มาตรฐาน
// ของกลุ่มบริษัท"): publishing a draft custom set is what makes it show up in the published browse list
// every legal entity/org unit under this tenant already reads from; retiring withdraws it again.
func TestPublishAndRetireTemplateSet(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg08d")
	cleanupCustomTemplates(t, app, tenant.ID)
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, grants(tenant, "ropa.template.create", "ropa.template.read", "ropa.template.publish", "ropa.template.delete"))
		set, err := svc.CreateTemplateSet(ctx, "group standard")
		if err != nil {
			return err
		}

		published, err := svc.PublishTemplateSet(ctx, set.ID, set.RowVersion)
		if err != nil {
			t.Fatalf("PublishTemplateSet: %v", err)
		}
		if published.Status != "published" {
			t.Errorf("expected status=published, got %q", published.Status)
		}

		// the acceptance criterion itself: it now appears in the same published browse list RTG-01's own
		// standard set already uses — "บริษัทในเครือใช้ template มาตรฐานของกลุ่มได้".
		list, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, s := range list {
			if s.ID == set.ID {
				found = true
			}
		}
		if !found {
			t.Error("expected the published custom set to appear in the published browse list")
		}

		// publishing an already-published (or now-stale) set is refused, not silently re-applied.
		if _, err := svc.PublishTemplateSet(ctx, set.ID, published.RowVersion); !errors.Is(err, templatesservice.ErrInvalid) {
			t.Errorf("expected ErrInvalid re-publishing an already-published set, got %v", err)
		}

		retired, err := svc.RetireTemplateSet(ctx, set.ID, published.RowVersion)
		if err != nil {
			t.Fatalf("RetireTemplateSet: %v", err)
		}
		if retired.Status != "retired" {
			t.Errorf("expected status=retired, got %q", retired.Status)
		}
		list, err = svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		for _, s := range list {
			if s.ID == set.ID {
				t.Error("a retired set must no longer appear in the published browse list")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestOwnTemplateSets_TwoTenantIsolation proves RLS: tenant B's management list never shows tenant A's
// own custom set, and tenant B can't write to it either (the ownership check refuses, not a silent no-op).
func TestOwnTemplateSets_TwoTenantIsolation(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenantA := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg08e1")
	cleanupCustomTemplates(t, app, tenantA.ID)
	tenantB := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg08e2")
	cleanupCustomTemplates(t, app, tenantB.ID)
	svc := templatesservice.New()

	var setID uuid.UUID
	err := pdb.WithTenantTx(ctx, app, tenantA.ID.String(), tenantA.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, grants(tenantA, "ropa.template.create"))
		set, err := svc.CreateTemplateSet(ctx, "tenant A's own")
		if err != nil {
			return err
		}
		setID = set.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = pdb.WithTenantTx(ctx, app, tenantB.ID.String(), tenantB.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, grants(tenantB, "ropa.template.create", "ropa.template.read", "ropa.template.publish"))
		own, err := svc.ListOwnTemplateSets(ctx)
		if err != nil {
			return err
		}
		for _, s := range own {
			if s.ID == setID {
				t.Error("tenant B must never see tenant A's own custom set")
			}
		}
		if _, err := svc.PublishTemplateSet(ctx, setID, 1); !errors.Is(err, templatesservice.ErrInvalid) {
			t.Errorf("expected ErrInvalid publishing another tenant's set, got %v", err)
		}
		if _, err := svc.AddActivityTemplate(ctx, setID, sampleActivity("x")); !errors.Is(err, templatesservice.ErrInvalid) {
			t.Errorf("expected ErrInvalid adding an activity to another tenant's set, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
