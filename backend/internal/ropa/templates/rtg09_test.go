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

// TestListActivityTemplates_EveryTemplateIsBilingual is RTG-09's own acceptance criterion read literally:
// every seeded template (RTG-01's standard set and RTG-02's industry sets alike) carries both th and en
// content for its name, description, and every purpose — not just a Thai name with an English label.
func TestListActivityTemplates_EveryTemplateIsBilingual(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg09")
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		checked := 0
		for _, set := range sets {
			activities, err := svc.ListActivityTemplates(ctx, set.ID, nil)
			if err != nil {
				return err
			}
			for _, a := range activities {
				checked++
				if a.NameEn == "" {
					t.Errorf("set %q activity %q has no name_en", set.Name, a.Code)
				}
				var defaults struct {
					DescriptionEn string `json:"description_en"`
					Purposes      []struct {
						PurposeTextEn string `json:"purpose_text_en"`
					} `json:"purposes"`
				}
				if err := json.Unmarshal(a.Defaults, &defaults); err != nil {
					t.Fatalf("set %q activity %q: defaults not valid JSON: %v", set.Name, a.Code, err)
				}
				if defaults.DescriptionEn == "" {
					t.Errorf("set %q activity %q has no description_en", set.Name, a.Code)
				}
				for i, p := range defaults.Purposes {
					if p.PurposeTextEn == "" {
						t.Errorf("set %q activity %q purpose[%d] has no purpose_text_en", set.Name, a.Code, i)
					}
				}
			}
		}
		if checked == 0 {
			t.Fatal("no activity templates found to check")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
