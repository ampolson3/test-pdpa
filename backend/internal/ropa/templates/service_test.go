package templates_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	templatesservice "pdpa-platform/internal/ropa/templates"
)

func TestListTemplateSets_SeesGlobalStandardSet(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtgtpl1")
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, s := range sets {
			if s.SetType == "standard" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected the seeded global standard set to be visible, got %+v", sets)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestListActivityTemplates_AtLeast50WithJobCategoryFilter is RTG-01's acceptance criterion: at least 50
// standard activities, and a job_category filter narrows the list.
func TestListActivityTemplates_AtLeast50WithJobCategoryFilter(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtgtpl2")
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		var setID uuid.UUID
		for _, s := range sets {
			if s.SetType == "standard" {
				setID = s.ID
			}
		}
		if setID == uuid.Nil {
			t.Fatal("no standard set found")
		}

		all, err := svc.ListActivityTemplates(ctx, setID, nil)
		if err != nil {
			return err
		}
		if len(all) < 50 {
			t.Errorf("got %d standard activities, want at least 50 (ม.39 acceptance criterion)", len(all))
		}

		recruitment := "recruitment"
		filtered, err := svc.ListActivityTemplates(ctx, setID, &recruitment)
		if err != nil {
			return err
		}
		if len(filtered) == 0 || len(filtered) >= len(all) {
			t.Errorf("job_category filter didn't narrow the list: filtered=%d all=%d", len(filtered), len(all))
		}
		for _, a := range filtered {
			if a.JobCategory != "recruitment" {
				t.Errorf("filtered result leaked another job_category: %+v", a)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestGetActivityTemplate_HasEveryM39Topic proves one seeded template's defaults cover every ม.39 topic
// this feature claims to: purposes, data, retention and security_controls.
func TestGetActivityTemplate_HasEveryM39Topic(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtgtpl3")
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		var setID uuid.UUID
		for _, s := range sets {
			if s.SetType == "standard" {
				setID = s.ID
			}
		}
		list, err := svc.ListActivityTemplates(ctx, setID, nil)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			t.Fatal("no activity templates found")
		}
		got, err := svc.GetActivityTemplate(ctx, list[0].ID)
		if err != nil {
			return err
		}
		var defaults map[string]any
		if err := json.Unmarshal(got.Defaults, &defaults); err != nil {
			t.Fatalf("defaults not valid JSON: %v", err)
		}
		for _, topic := range []string{"purposes", "data", "retention", "security_controls"} {
			v, ok := defaults[topic]
			if !ok {
				t.Errorf("defaults missing ม.39 topic %q", topic)
				continue
			}
			if arr, ok := v.([]any); !ok || len(arr) == 0 {
				t.Errorf("defaults.%s is empty, want at least one entry", topic)
			}
		}
		if got.Role != "controller" && got.Role != "processor" {
			t.Errorf("unexpected role %q", got.Role)
		}
		if len(got.LegalRefs) == 0 {
			t.Error("legal_refs is empty")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetActivityTemplate_UnknownIDRefused(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtgtpl4")
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		_, err := svc.GetActivityTemplate(ctx, uuid.New())
		if !errors.Is(err, templatesservice.ErrNotFound) {
			t.Errorf("unknown id: %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestListTemplateSets_TwoTenantIsolation proves the global standard set is visible to both tenants (by
// design — it's a platform-wide default, the same visibility ORG-07's master data uses), while remaining
// otherwise tenant-scoped (no tenant-authored set exists yet to isolate — RTG-08, not built).
func TestListTemplateSets_TwoTenantIsolation(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	a := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtgtpliso-a")
	b := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtgtpliso-b")
	svc := templatesservice.New()

	for _, tn := range []dbtest.Tenant{a, b} {
		err := pdb.WithTenantTx(ctx, app, tn.ID.String(), tn.UserID.String(), func(ctx context.Context) error {
			ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tn.ID.String(), UserID: tn.UserID.String(), Permissions: []string{"ropa.template.read"}})
			sets, err := svc.ListTemplateSets(ctx)
			if err != nil {
				return err
			}
			found := false
			for _, s := range sets {
				if s.SetType == "standard" {
					found = true
				}
			}
			if !found {
				t.Errorf("tenant %s: expected the global standard set to be visible", tn.ID)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
