package service_test

import (
	"context"
	"strings"
	"testing"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestExportProcessorActivities_ContainsAllTopics is ROPA-04's acceptance criterion: exporting the
// processor RoPA produces a CSV with every mandatory-topic column and the processor activity's real
// data, and never includes a controller-role activity.
func TestExportProcessorActivities_ContainsAllTopics(t *testing.T) {
	e := setup(t, "ropaexport")
	var le orgservice.LegalEntity
	var unit orgservice.OrgUnit
	var controller, recipient orgservice.ExternalParty
	var plain orgservice.MasterItem
	var subjectType orgservice.MasterItem
	var processor, notExported ropaservice.Activity

	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "ฝ่ายไอที", UnitType: "department"}); err != nil {
			return err
		}
		if controller, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "บริษัท ผู้ว่าจ้าง จำกัด", CountryCode: "TH"}, 0); err != nil {
			return err
		}
		if recipient, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการคลาวด์", CountryCode: "TH"}, 0); err != nil {
			return err
		}
		_, plain = twoCategories(t, ctx, e.org)
		subjects, err := e.org.ListMaster(ctx, "data_subject_types")
		if err != nil {
			return err
		}
		if len(subjects) == 0 {
			t.Fatal("expected data subject types to be seeded (ORG-07)")
		}
		subjectType = subjects[0]

		processor, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "PROC-01",
			Name: "ประมวลผลข้อมูลแทนผู้ว่าจ้าง", Role: "processor", ControllerPartyID: &controller.ID, RightsAndAccess: "ติดต่อ DPO"}, 0)
		if err != nil {
			return err
		}
		notExported, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "CTRL-01",
			Name: "กิจกรรมของผู้ควบคุมเอง", Role: "controller"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: processor.ID, DataCategoryID: *plain.ID,
			SubjectTypeID: *subjectType.ID, Source: "direct"}); err != nil {
			return err
		}
		if _, err := e.svc.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: processor.ID, RetentionBasis: "ตามสัญญา",
			TriggerEvent: "สิ้นสุดสัญญา", DisposalMethod: "delete"}); err != nil {
			return err
		}
		_, err := e.svc.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: processor.ID, PartyID: recipient.ID,
			RecipientRole: "processor", DisclosureBasis: "ตามสัญญาประมวลผลข้อมูล"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		buf, n, err := e.svc.ExportProcessorActivities(ctx)
		if err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("rows exported: %d, want 1", n)
		}
		csv := buf.String()
		for _, col := range []string{"code", "name", "controller", "org_unit", "owner", "data_categories", "data_subjects", "retention", "recipients", "transfers"} {
			if !strings.Contains(csv, col) {
				t.Errorf("export missing column %q:\n%s", col, csv)
			}
		}
		if !strings.Contains(csv, "PROC-01") {
			t.Errorf("export missing the processor activity's code:\n%s", csv)
		}
		if !strings.Contains(csv, controller.NameTh) {
			t.Errorf("export missing the controller's name:\n%s", csv)
		}
		if !strings.Contains(csv, recipient.NameTh) {
			t.Errorf("export missing the recipient's name:\n%s", csv)
		}
		if strings.Contains(csv, "CTRL-01") || strings.Contains(csv, notExported.Name) {
			t.Errorf("export must not include a controller-role activity:\n%s", csv)
		}
		return nil
	})
}
