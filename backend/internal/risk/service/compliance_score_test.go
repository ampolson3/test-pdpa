package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestActivityComplianceScore_ExplainsDeductions is RRA-13's own acceptance criterion directly
// ("คะแนนอธิบายได้ว่าหักจากช่องว่างใด"): a bare activity scores 100 minus no_lawful_basis (high, -20) and
// no_retention (medium, -10) — each deduction naming the exact gap it came from — then rises once the
// lawful-basis gap is actually fixed.
func TestActivityComplianceScore_ExplainsDeductions(t *testing.T) {
	e := gapSetup(t, "rra13score", true) // notice already covers it, same fixture gap_analysis_test.go uses
	var activityID uuid.UUID
	var basis string
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "SCORE-01",
			Name: "กิจกรรมทดสอบคะแนน", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		basis = nonConsentLawfulBasis(t, ctx, e.org)
		if _, err := e.svc.AnalyzeActivity(ctx, activityID); err != nil {
			return err
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		cs, err := e.svc.ActivityComplianceScore(ctx, activityID)
		if err != nil {
			return err
		}
		if cs.Score != 70 {
			t.Errorf("score = %v, want 70 (100 - 20 no_lawful_basis - 10 no_retention)", cs.Score)
		}
		if len(cs.Deductions) != 2 {
			t.Fatalf("expected 2 deductions, got %d: %+v", len(cs.Deductions), cs.Deductions)
		}
		codes := map[string]float64{}
		for _, d := range cs.Deductions {
			codes[d.RuleCode] = d.Points
		}
		if codes["no_lawful_basis"] != 20 {
			t.Errorf("no_lawful_basis deduction = %v, want 20", codes["no_lawful_basis"])
		}
		if codes["no_retention"] != 10 {
			t.Errorf("no_retention deduction = %v, want 10", codes["no_retention"])
		}
		return nil
	})

	// Fix the lawful-basis gap — the score rises and that deduction disappears from the explanation.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.ropa.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activityID, PurposeText: "ทดสอบ", LawfulBasisCode: basis}); err != nil {
			return err
		}
		if _, err := e.svc.AnalyzeActivity(ctx, activityID); err != nil {
			return err
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		cs, err := e.svc.ActivityComplianceScore(ctx, activityID)
		if err != nil {
			return err
		}
		if cs.Score != 90 {
			t.Errorf("score after fixing lawful basis = %v, want 90", cs.Score)
		}
		if len(cs.Deductions) != 1 || cs.Deductions[0].RuleCode != "no_retention" {
			t.Errorf("expected only no_retention left, got %+v", cs.Deductions)
		}
		return nil
	})
}

func TestActivityComplianceScore_PerfectScoreHasNoDeductions(t *testing.T) {
	e := gapSetup(t, "rra13perfect", true)
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "SCORE-PERFECT",
			Name: "กิจกรรมสมบูรณ์", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = a.ID
		basis := nonConsentLawfulBasis(t, ctx, e.org)
		if _, err := e.ropa.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activityID, PurposeText: "ทดสอบ", LawfulBasisCode: basis}); err != nil {
			return err
		}
		if _, err := e.ropa.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: activityID, RetentionMonths: intPtr(12),
			RetentionBasis: "ทดสอบ", TriggerEvent: "สิ้นสุดสัญญา", DisposalMethod: "delete"}); err != nil {
			return err
		}
		if _, err := e.svc.AnalyzeActivity(ctx, activityID); err != nil {
			return err
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		cs, err := e.svc.ActivityComplianceScore(ctx, activityID)
		if err != nil {
			return err
		}
		if cs.Score != 100 || len(cs.Deductions) != 0 {
			t.Errorf("score = %v, deductions = %+v, want 100 and none", cs.Score, cs.Deductions)
		}
		return nil
	})
}

// TestOrgUnitComplianceScore_AveragesItsActivities is the module description's own rollup: every
// activity under one org unit, averaged.
func TestOrgUnitComplianceScore_AveragesItsActivities(t *testing.T) {
	e := gapSetup(t, "rra13rollup", true)
	var unitID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		unitID = unit.ID
		basis := nonConsentLawfulBasis(t, ctx, e.org)

		// Activity 1: perfect (score 100).
		a1, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "ROLLUP-01",
			Name: "กิจกรรม 1", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: a1.ID, PurposeText: "ทดสอบ", LawfulBasisCode: basis}); err != nil {
			return err
		}
		if _, err := e.ropa.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: a1.ID, RetentionMonths: intPtr(12),
			RetentionBasis: "ทดสอบ", TriggerEvent: "สิ้นสุดสัญญา", DisposalMethod: "delete"}); err != nil {
			return err
		}
		if _, err := e.svc.AnalyzeActivity(ctx, a1.ID); err != nil {
			return err
		}

		// Activity 2: bare (score 70: -20 no_lawful_basis, -10 no_retention).
		a2, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "ROLLUP-02",
			Name: "กิจกรรม 2", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		if _, err := e.svc.AnalyzeActivity(ctx, a2.ID); err != nil {
			return err
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		ou, err := e.svc.OrgUnitComplianceScore(ctx, unitID)
		if err != nil {
			return err
		}
		if len(ou.Activities) != 2 {
			t.Fatalf("expected 2 activities, got %d", len(ou.Activities))
		}
		if ou.AverageScore != 85 {
			t.Errorf("average score = %v, want 85 ((100+70)/2)", ou.AverageScore)
		}
		return nil
	})

	// The tenant-wide report includes this org unit too.
	e.in(t, func(ctx context.Context) error {
		report, err := e.svc.ComplianceScoreReport(ctx)
		if err != nil {
			return err
		}
		var found bool
		for _, ou := range report {
			if ou.OrgUnitID == unitID {
				found = true
				if ou.AverageScore != 85 {
					t.Errorf("report average score = %v, want 85", ou.AverageScore)
				}
			}
		}
		if !found {
			t.Error("expected the org unit in the tenant-wide report")
		}
		return nil
	})
}

// TestComplianceScore_TwoTenantIsolation: RLS already scopes both the activity score and the report.
func TestComplianceScore_TwoTenantIsolation(t *testing.T) {
	a := gapSetup(t, "rra13isoa", true)
	b := gapSetup(t, "rra13isob", true)
	var activityID, unitID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท A จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		unitID = unit.ID
		act, err := a.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "ISO-01",
			Name: "กิจกรรม A", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = act.ID
		_, err = a.svc.AnalyzeActivity(ctx, activityID)
		return err
	})

	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.ActivityComplianceScore(ctx, activityID); err == nil {
			t.Error("tenant B computed a score for tenant A's activity")
		}
		ou, err := b.svc.OrgUnitComplianceScore(ctx, unitID)
		if err != nil {
			return err
		}
		if len(ou.Activities) != 0 {
			t.Error("tenant B's org-unit rollup saw tenant A's activity")
		}
		report, err := b.svc.ComplianceScoreReport(ctx)
		if err != nil {
			return err
		}
		for _, r := range report {
			if r.OrgUnitID == unitID {
				t.Error("tenant B's report saw tenant A's org unit")
			}
		}
		return nil
	})
}

func intPtr(v int) *int { return &v }
