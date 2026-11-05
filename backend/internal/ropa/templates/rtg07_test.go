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

// TestListTemplateSets_ProcessorSetSeeded is RTG-07's own acceptance criterion's data half: a dedicated
// set_type='processor' template set exists (migration 00087), every one of its activities is role=
// "processor" and carries every ม.39 topic a processor's own RoPA record needs, with lawful_basis_code
// always CONTRACT — the processor's own basis for performing the service contract, never the
// controller's basis for the underlying processing (see ROPA-04's own processor-RoPA export, which
// deliberately excludes that column for the same reason).
func TestListTemplateSets_ProcessorSetSeeded(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg07a")
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		var procSet *templatesservice.TemplateSet
		for i, s := range sets {
			if s.SetType == "processor" {
				procSet = &sets[i]
			}
		}
		if procSet == nil {
			t.Fatal("expected a set_type='processor' template set (RTG-07's own processor RoPA template)")
		}
		if procSet.Industry != "" {
			t.Errorf("the processor template is its own set_type, not an industry row; got industry=%q", procSet.Industry)
		}

		activities, err := svc.ListActivityTemplates(ctx, procSet.ID, nil)
		if err != nil {
			return err
		}
		if len(activities) == 0 {
			t.Fatal("expected at least one processor activity template")
		}
		for _, a := range activities {
			if a.Role != "processor" {
				t.Errorf("activity %q: expected role=processor, got %q", a.Code, a.Role)
			}
			var defaults map[string]any
			if err := json.Unmarshal(a.Defaults, &defaults); err != nil {
				t.Fatalf("activity %q: defaults not valid JSON: %v", a.Code, err)
			}
			for _, topic := range []string{"purposes", "data", "recipients", "retention", "security_controls"} {
				v, ok := defaults[topic]
				if !ok {
					t.Errorf("activity %q missing topic %q", a.Code, topic)
					continue
				}
				if arr, ok := v.([]any); !ok || len(arr) == 0 {
					t.Errorf("activity %q: defaults.%s is empty", a.Code, topic)
				}
			}
			purposes, _ := defaults["purposes"].([]any)
			for _, p := range purposes {
				pm, _ := p.(map[string]any)
				if pm["lawful_basis_code"] != "CONTRACT" {
					t.Errorf("activity %q: expected every purpose's lawful_basis_code to be CONTRACT, got %v", a.Code, pm["lawful_basis_code"])
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestListTemplateSets_ProcessorAndStandardAreSeparateSets proves RTG-07's own set_type='processor' axis
// never mixes with RTG-01's own 3 processor-role examples embedded in the standard job-category set —
// two distinct rows, two distinct activity lists.
func TestListTemplateSets_ProcessorAndStandardAreSeparateSets(t *testing.T) {
	ctx := context.Background()
	app := dbtest.Pool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "rtg07b")
	svc := templatesservice.New()

	err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: []string{"ropa.template.read"}})
		sets, err := svc.ListTemplateSets(ctx)
		if err != nil {
			return err
		}
		var procSetID, standardSetID string
		for _, s := range sets {
			if s.SetType == "processor" {
				procSetID = s.ID.String()
			}
			if s.SetType == "standard" {
				standardSetID = s.ID.String()
			}
		}
		if procSetID == "" || standardSetID == "" {
			t.Fatal("expected both the dedicated processor set and the standard job-category set")
		}
		if procSetID == standardSetID {
			t.Fatal("the processor template must be a distinct set from the standard one")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
