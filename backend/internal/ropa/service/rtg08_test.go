package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	templatesservice "pdpa-platform/internal/ropa/templates"
)

// TestCreateActivityFromTemplate_FromPublishedCustomSet is RTG-08's own acceptance criterion end to end
// ("บริษัทในเครือใช้ template มาตรฐานของกลุ่มได้"): a legal entity under this tenant creates a real RoPA
// activity from a custom template set the tenant itself authored and published — the exact same ROPA-05
// CreateActivityFromTemplate path RTG-01's own global standard set already uses, proving the custom set
// is immediately and fully usable with no extra wiring anywhere else in this codebase.
func TestCreateActivityFromTemplate_FromPublishedCustomSet(t *testing.T) {
	e := setup(t, "ropartg08")
	var unit orgservice.OrgUnit
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัทในเครือ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		return err
	})

	var tplID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		set, err := e.templates.CreateTemplateSet(ctx, "group standard template")
		if err != nil {
			t.Fatalf("CreateTemplateSet: %v", err)
		}
		a, err := e.templates.AddActivityTemplate(ctx, set.ID, templatesservice.ActivityTemplateInput{
			Code: "grp_payroll", NameTh: "เงินเดือนกลุ่มบริษัท", JobCategory: "payroll", Role: "controller",
			Defaults: []byte(`{
				"purposes": [{"purpose_text_th": "จ่ายเงินเดือน", "lawful_basis_code": "CONTRACT"}],
				"data": [{"data_category_code": "financial", "subject_type_code": "employee", "source": "direct"}],
				"retention": [{"retention_months": 60, "retention_basis_th": "กฎหมายแรงงาน", "trigger_event_th": "สิ้นสุดสัญญา", "disposal_method": "destroy"}],
				"security_controls": ["AC_ROLE_BASED"]
			}`),
			Rationale: []byte(`{"note": "ตัวอย่าง"}`),
		})
		if err != nil {
			t.Fatalf("AddActivityTemplate: %v", err)
		}
		published, err := e.templates.PublishTemplateSet(ctx, set.ID, set.RowVersion)
		if err != nil {
			t.Fatalf("PublishTemplateSet: %v", err)
		}
		if published.Status != "published" {
			t.Fatalf("expected published, got %q", published.Status)
		}
		tplID = a.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		activity, err := e.svc.CreateActivityFromTemplate(ctx, tplID, unit.LegalEntityID, unit.ID, "GRP-01", nil)
		if err != nil {
			t.Fatalf("CreateActivityFromTemplate against the published custom set: %v", err)
		}
		if activity.Role != "controller" || activity.Name == "" {
			t.Errorf("unexpected activity %+v", activity)
		}
		purposes, err := e.svc.ListActivityPurposes(ctx, activity.ID)
		if err != nil {
			return err
		}
		if len(purposes) == 0 {
			t.Error("expected the custom template's own purpose to be copied in")
		}
		return nil
	})
}
