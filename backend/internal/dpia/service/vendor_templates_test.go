package service_test

import (
	"context"
	"testing"
)

// TestVendorAssessmentTemplates_SeededReady is VEN-04's own acceptance criterion
// ("มี template พร้อมใช้อย่างน้อย 3 ชุด"): at least 3 published, ready-to-use assessment_type='vendor'
// templates exist (migration 00053, docs/decisions.md Q-31) — seeded globally (tenant_id NULL), so any
// tenant's own ListTemplates call sees them without provisioning anything itself. This exercises the
// generic template library DPIA-03 already built (ListTemplates/GetTemplateByID take assessment_type as a
// plain parameter, not hardcoded to "dpia"), proving VEN-04 needed no new backend code.
func TestVendorAssessmentTemplates_SeededReady(t *testing.T) {
	e := setup(t, "venTemplates")
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListTemplates(ctx, "vendor")
		if err != nil {
			return err
		}
		if len(list) < 3 {
			t.Fatalf("got %d vendor templates, want at least 3", len(list))
		}
		codes := map[string]bool{}
		for _, tpl := range list {
			if tpl.Status != "published" {
				t.Errorf("template %q is %q, want published (ready to use)", tpl.Code, tpl.Status)
			}
			if tpl.AssessmentType != "vendor" {
				t.Errorf("template %q has assessment_type %q", tpl.Code, tpl.AssessmentType)
			}
			if len(tpl.LegalRefs) == 0 {
				t.Errorf("template %q has no legal_refs", tpl.Code)
			}
			codes[tpl.Code] = true

			got, err := e.svc.GetTemplateByID(ctx, tpl.ID)
			if err != nil {
				return err
			}
			if got.FormID != tpl.FormID || got.VersionNo < 1 {
				t.Errorf("template %q: GetTemplateByID mismatch: %+v", tpl.Code, got)
			}
		}
		for _, want := range []string{"vendor_pdpa", "vendor_security", "vendor_transfer"} {
			if !codes[want] {
				t.Errorf("expected seeded template code %q among the vendor templates", want)
			}
		}
		return nil
	})
}

// TestVendorAssessmentTemplates_VisibleToAnyTenant proves the seeded templates are the tenant-independent
// global defaults ORG-07/ROPA-09/PNG-03/DPIA-01 already established this pattern for — a second, entirely
// separate tenant sees exactly the same 3 (or more, if that tenant authored its own) without any setup.
func TestVendorAssessmentTemplates_VisibleToAnyTenant(t *testing.T) {
	e := setup(t, "venTemplatesB")
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListTemplates(ctx, "vendor")
		if err != nil {
			return err
		}
		if len(list) < 3 {
			t.Fatalf("second tenant sees %d vendor templates, want at least 3 (global defaults)", len(list))
		}
		return nil
	})
}
