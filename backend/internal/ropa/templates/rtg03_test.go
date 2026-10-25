package templates_test

import (
	"context"
	"encoding/json"
	"testing"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	templatesservice "pdpa-platform/internal/ropa/templates"
)

// TestListTemplateSets_GovernmentSetSeeded is RTG-03's own acceptance criterion directly: RoPA created
// from this set carries every ม.39 topic the official DGA/สพร. v1.0 (2564) government-controller
// template structure asks for (migration 00084) — the same defaults/rationale shape RTG-01/02's own
// activity templates already use, so ROPA-05's from-template create path copies it in unchanged.
func TestListTemplateSets_GovernmentSetSeeded(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg03a")
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		var govSet *templatesservice.TemplateSet
		for i, s := range sets {
			if s.SetType == "government" {
				govSet = &sets[i]
			}
		}
		if govSet == nil {
			t.Fatal("expected a set_type='government' template set (RTG-03's own สพร. structure)")
		}
		if govSet.Industry != "" {
			t.Errorf("the official government template is its own set_type, not an industry='government' row; got industry=%q", govSet.Industry)
		}

		activities, err := svc.ListActivityTemplates(ctx, govSet.ID, nil)
		if err != nil {
			return err
		}
		if len(activities) == 0 {
			t.Fatal("expected at least one government-agency activity template")
		}
		for _, a := range activities {
			if a.Role != "controller" {
				t.Errorf("activity %q: government agencies are RoPA controllers, got role %q", a.Code, a.Role)
			}
			var defaults map[string]any
			if err := json.Unmarshal(a.Defaults, &defaults); err != nil {
				t.Fatalf("activity %q: defaults not valid JSON: %v", a.Code, err)
			}
			for _, topic := range []string{"purposes", "data", "retention", "security_controls"} {
				v, ok := defaults[topic]
				if !ok {
					t.Errorf("activity %q missing ม.39 topic %q", a.Code, topic)
					continue
				}
				if arr, ok := v.([]any); !ok || len(arr) == 0 {
					t.Errorf("activity %q: defaults.%s is empty", a.Code, topic)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestListTemplateSets_GovernmentAndIndustryAreSeparateSets proves RTG-03's own set_type='government'
// axis never mixes with RTG-02's industry='government' starter set — they're two distinct rows with
// two distinct activity lists, even though both describe government agencies.
func TestListTemplateSets_GovernmentAndIndustryAreSeparateSets(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg03b")
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		var govSetID, industryGovID string
		for _, s := range sets {
			if s.SetType == "government" {
				govSetID = s.ID.String()
			}
			if s.SetType == "industry" && s.Industry == "government" {
				industryGovID = s.ID.String()
			}
		}
		if govSetID == "" || industryGovID == "" {
			t.Fatal("expected both the official government set and the generic industry='government' set")
		}
		if govSetID == industryGovID {
			t.Fatal("the official government template must be a distinct set from the generic industry one")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
