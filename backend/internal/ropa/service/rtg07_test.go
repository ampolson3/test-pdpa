package service_test

import (
	"context"
	"testing"

	orgservice "pdpa-platform/internal/org/service"
)

// TestCreateActivityFromTemplate_ProcessorRoleFromProcessorTemplate is RTG-07's own acceptance
// criterion directly: "สร้าง RoPA ผู้ประมวลผลจาก template ได้" — creating an activity from one of the
// dedicated set_type='processor' templates (migration 00087) yields a real role="processor" RoPA
// activity with that template's own purposes/data/recipients/retention/security-controls copied in,
// the same ROPA-05 from-template path RTG-01's standard set already uses. Unlike a controller-role
// template, completeness never reaches 100% right after creation — the recipient's own
// controller_party_id (the hiring party) is a real external-party row only this tenant can supply, the
// same "left for the OWNER to add" gap RTG-01's own 3 processor-role examples already document.
func TestCreateActivityFromTemplate_ProcessorRoleFromProcessorTemplate(t *testing.T) {
	e := setup(t, "ropaproctpl")
	var unit orgservice.OrgUnit
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: false, IsProcessor: true}, 0)
		if err != nil {
			return err
		}
		unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		tplID := templateIDByCode(t, ctx, "proc_payroll_outsourcing")
		a, err := e.svc.CreateActivityFromTemplate(ctx, tplID, unit.LegalEntityID, unit.ID, "IT-PROC-01", nil)
		if err != nil {
			t.Fatalf("CreateActivityFromTemplate: %v", err)
		}
		if a.Role != "processor" {
			t.Errorf("expected a processor-role activity, got role %q", a.Role)
		}

		purposes, err := e.svc.ListActivityPurposes(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(purposes) == 0 || purposes[0].LawfulBasisCode != "CONTRACT" {
			t.Errorf("expected the template's CONTRACT-basis purpose to be copied, got %+v", purposes)
		}

		data, err := e.svc.ListActivityData(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			t.Error("expected the template's data categories to be copied")
		}

		retention, err := e.svc.ListRetentionRules(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(retention) == 0 {
			t.Error("expected the template's retention rule to be copied")
		}

		controls, err := e.svc.ListActivityControls(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(controls) == 0 {
			t.Error("expected the template's security controls to be copied")
		}
		return nil
	})
}
