package templates_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	templatesservice "pdpa-platform/internal/ropa/templates"
)

// TestListTemplateSets_SixIndustrySetsSeeded is RTG-02's own acceptance criterion read literally: a
// complete activity set exists for all 6 industries (migration 00082, the other axis of RTG-01's own
// set_type='standard' seed — set_type='industry' + the industry column, both already in the baseline
// migrations, unused until now).
func TestListTemplateSets_SixIndustrySetsSeeded(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg02a")
	svc := templatesservice.New()

	wantIndustries := []string{"retail", "hospital", "finance_insurance", "education", "real_estate", "government"}

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		byIndustry := map[string]templatesservice.TemplateSet{}
		for _, s := range sets {
			if s.SetType == "industry" {
				byIndustry[s.Industry] = s
			}
		}
		for _, want := range wantIndustries {
			set, ok := byIndustry[want]
			if !ok {
				t.Errorf("missing industry template set %q", want)
				continue
			}
			activities, err := svc.ListActivityTemplates(ctx, set.ID, nil)
			if err != nil {
				return err
			}
			if len(activities) == 0 {
				t.Errorf("industry %q has no activities, want a complete activity set", want)
			}
			for _, a := range activities {
				var defaults map[string]any
				if err := json.Unmarshal(a.Defaults, &defaults); err != nil {
					t.Fatalf("industry %q activity %q: defaults not valid JSON: %v", want, a.Code, err)
				}
				for _, topic := range []string{"purposes", "data", "retention", "security_controls"} {
					v, ok := defaults[topic]
					if !ok {
						t.Errorf("industry %q activity %q missing ม.39 topic %q", want, a.Code, topic)
						continue
					}
					if arr, ok := v.([]any); !ok || len(arr) == 0 {
						t.Errorf("industry %q activity %q: defaults.%s is empty", want, a.Code, topic)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestListTemplateSets_IndustryAndStandardAreSeparateSets proves the two axes of RTG-01/RTG-02's shared
// schema never mix: an industry set's activities are never found inside the standard set and vice versa.
func TestListTemplateSets_IndustryAndStandardAreSeparateSets(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg02b")
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		var standardID, hospitalID uuid.UUID
		for _, s := range sets {
			if s.SetType == "standard" {
				standardID = s.ID
			}
			if s.SetType == "industry" && s.Industry == "hospital" {
				hospitalID = s.ID
			}
		}
		if standardID == uuid.Nil || hospitalID == uuid.Nil {
			t.Fatal("expected both a standard set and the hospital industry set")
		}

		standardActivities, err := svc.ListActivityTemplates(ctx, standardID, nil)
		if err != nil {
			return err
		}
		hospitalActivities, err := svc.ListActivityTemplates(ctx, hospitalID, nil)
		if err != nil {
			return err
		}
		for _, h := range hospitalActivities {
			for _, s := range standardActivities {
				if h.ID == s.ID {
					t.Errorf("hospital activity %q leaked into the standard set", h.Code)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
