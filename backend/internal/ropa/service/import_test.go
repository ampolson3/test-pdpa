package service_test

import (
	"context"
	"testing"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/importer"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// ROPA-18 (Bulk import): the acceptance criterion is importing many RoPA activities from Excel and
// reporting the rows that still need fixing — exercised here directly against the import type's own
// Validate/Apply functions (the generic PLT-14 upload/map/confirm machinery is already covered by
// internal/platform/importer's own tests; this proves the ropa.activity type's own business rules).
func TestActivityImport_ValidRowCreatesActivityWithChildRows(t *testing.T) {
	e := setup(t, "ropaimport")
	var basis, categoryName, subjectName string
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท นำเข้า จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		if _, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "ฝ่ายบุคคล", UnitType: "department"}); err != nil {
			return err
		}
		basis = lawfulBasisCode(t, ctx, e.org)
		cats, err := e.org.ListMaster(ctx, "data_categories")
		if err != nil {
			return err
		}
		for _, c := range cats {
			if !c.IsSensitive {
				categoryName = c.NameTh
				break
			}
		}
		subs, err := e.org.ListMaster(ctx, "data_subject_types")
		if err != nil {
			return err
		}
		subjectName = subs[0].NameTh
		return nil
	})
	if categoryName == "" || subjectName == "" {
		t.Fatal("expected at least one non-sensitive data category and one subject type seeded (ORG-07)")
	}

	imp := e.svc.ActivityImport()
	row := importer.Row{
		"code": "IMP-01", "name": "การสมัครงาน", "legal_entity": "บริษัท นำเข้า จำกัด", "org_unit": "ฝ่ายบุคคล",
		"role": "controller", "purpose": "รับสมัครพนักงาน", "lawful_basis": basis,
		"data_category": categoryName, "subject_type": subjectName,
		"retention_months": "24", "retention_basis": "ตามกฎหมายแรงงาน", "trigger_event": "สิ้นสุดการจ้าง",
		"disposal_method": "destroy",
	}

	e.in(t, func(ctx context.Context) error {
		if errs := imp.Validate(ctx, 2, row); len(errs) != 0 {
			t.Fatalf("Validate: unexpected errors %+v", errs)
		}
		if err := imp.Apply(ctx, 2, row); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		acts, _, err := e.svc.ListActivities(ctx, ropaservice.ActivityFilter{Limit: 10})
		if err != nil {
			return err
		}
		var found *ropaservice.Activity
		for i := range acts {
			if acts[i].Code == "IMP-01" {
				found = &acts[i]
			}
		}
		if found == nil {
			t.Fatal("imported activity not found")
		}
		purposes, err := e.svc.ListActivityPurposes(ctx, found.ID)
		if err != nil {
			return err
		}
		if len(purposes) != 1 || purposes[0].PurposeText != "รับสมัครพนักงาน" {
			t.Errorf("purposes: %+v", purposes)
		}
		data, err := e.svc.ListActivityData(ctx, found.ID)
		if err != nil {
			return err
		}
		if len(data) != 1 {
			t.Errorf("data: %+v", data)
		}
		retention, err := e.svc.ListRetentionRules(ctx, found.ID)
		if err != nil {
			return err
		}
		if len(retention) != 1 || retention[0].DisposalMethod != "destroy" {
			t.Errorf("retention: %+v", retention)
		}
		return nil
	})
}

func TestActivityImport_FuzzyMatchesSlightlyMisspelledNames(t *testing.T) {
	e := setup(t, "ropaimportfuzzy")
	var basis, categoryName, subjectName string
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ฟัซซี่ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		if _, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "FIN", NameTh: "ฝ่ายการเงิน", UnitType: "department"}); err != nil {
			return err
		}
		basis = lawfulBasisCode(t, ctx, e.org)
		cats, err := e.org.ListMaster(ctx, "data_categories")
		if err != nil {
			return err
		}
		for _, c := range cats {
			if !c.IsSensitive {
				categoryName = c.NameTh
				break
			}
		}
		subs, err := e.org.ListMaster(ctx, "data_subject_types")
		if err != nil {
			return err
		}
		subjectName = subs[0].NameTh
		return nil
	})

	imp := e.svc.ActivityImport()
	// Slightly different spelling / spacing of the legal entity and department than what was saved —
	// the acceptance criterion's own "จับคู่ master data แบบ fuzzy".
	row := importer.Row{
		"code": "IMP-02", "name": "บัญชี", "legal_entity": "บริษัทฟัซซี่จำกัด", "org_unit": "ฝ่ายการเงิน ",
		"role": "controller", "purpose": "จัดทำบัญชี", "lawful_basis": basis,
		"data_category": categoryName, "subject_type": subjectName,
		"retention_months": "60", "retention_basis": "กฎหมายบัญชี", "trigger_event": "สิ้นปีบัญชี",
		"disposal_method": "delete",
	}

	e.in(t, func(ctx context.Context) error {
		if errs := imp.Validate(ctx, 2, row); len(errs) != 0 {
			t.Fatalf("Validate: expected fuzzy match to succeed, got errors %+v", errs)
		}
		return nil
	})
}

func TestActivityImport_ReportsUnknownReferencesAndBadValues(t *testing.T) {
	e := setup(t, "ropaimportbad")
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตรวจสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		_, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "OPS", NameTh: "ฝ่ายปฏิบัติการ", UnitType: "department"})
		return err
	})

	imp := e.svc.ActivityImport()
	row := importer.Row{
		"code": "IMP-03", "name": "x", "legal_entity": "ไม่มีบริษัทนี้", "org_unit": "ไม่มีหน่วยงานนี้",
		"role": "bogus_role", "purpose": "x", "lawful_basis": "ไม่มีฐานนี้",
		"data_category": "ไม่มีหมวดนี้", "subject_type": "ไม่มีกลุ่มนี้",
		"retention_months": "not-a-number", "retention_basis": "x", "trigger_event": "x",
		"disposal_method": "burn_it",
	}

	e.in(t, func(ctx context.Context) error {
		errs := imp.Validate(ctx, 2, row)
		want := map[string]string{
			"role": "invalid_role", "legal_entity": "unknown_legal_entity", "lawful_basis": "unknown_lawful_basis",
			"data_category": "unknown_data_category", "subject_type": "unknown_subject_type",
			"retention_months": "invalid_retention_months", "disposal_method": "invalid_disposal_method",
		}
		got := map[string]string{}
		for _, fe := range errs {
			got[fe.Column] = fe.Message
		}
		for col, msg := range want {
			if got[col] != msg {
				t.Errorf("column %q: got %q, want %q (all errors: %+v)", col, got[col], msg, errs)
			}
		}
		return nil
	})
}

// ROPA-06: a lawful basis requiring consent evidence has no column in this single-row import to
// link a real consent purpose, so it's flagged rather than silently dropping that requirement.
func TestActivityImport_ConsentRequiringBasisIsFlagged(t *testing.T) {
	e := setup(t, "ropaimportconsent")
	var consentBasis, categoryName, subjectName string
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ยินยอม จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		if _, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "MKT", NameTh: "ฝ่ายการตลาด", UnitType: "department"}); err != nil {
			return err
		}
		consentBasis = consentLawfulBasisCode(t, ctx, e.org)
		cats, err := e.org.ListMaster(ctx, "data_categories")
		if err != nil {
			return err
		}
		for _, c := range cats {
			if !c.IsSensitive {
				categoryName = c.NameTh
				break
			}
		}
		subs, err := e.org.ListMaster(ctx, "data_subject_types")
		if err != nil {
			return err
		}
		subjectName = subs[0].NameTh
		return nil
	})

	imp := e.svc.ActivityImport()
	row := importer.Row{
		"code": "IMP-04", "name": "การตลาด", "legal_entity": "บริษัท ยินยอม จำกัด", "org_unit": "ฝ่ายการตลาด",
		"role": "controller", "purpose": "การตลาดทางตรง", "lawful_basis": consentBasis,
		"data_category": categoryName, "subject_type": subjectName,
		"retention_months": "12", "retention_basis": "x", "trigger_event": "x", "disposal_method": "delete",
	}

	e.in(t, func(ctx context.Context) error {
		errs := imp.Validate(ctx, 2, row)
		found := false
		for _, fe := range errs {
			if fe.Column == "lawful_basis" && fe.Message == "requires_consent_link" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected requires_consent_link on lawful_basis, got %+v", errs)
		}
		return nil
	})
}
