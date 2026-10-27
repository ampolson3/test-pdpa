package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/forms"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// fakeDpia is a minimal test double for ropaservice.Dpia — DPIA-16's own completeness check only exercises
// HasApprovedLIA; the other two methods (DPX-06's own TIA path) are never called by these tests.
type fakeDpia struct {
	approved map[uuid.UUID]bool
}

func (f *fakeDpia) ListTemplates(ctx context.Context, assessmentType string) ([]ropaservice.DpiaTemplate, error) {
	return nil, nil
}
func (f *fakeDpia) RecordSubjectAssessment(ctx context.Context, templateID uuid.UUID, subjectType string, subjectID uuid.UUID, title string, answers forms.Answers) (ropaservice.TiaResult, error) {
	return ropaservice.TiaResult{}, nil
}
func (f *fakeDpia) HasApprovedLIA(ctx context.Context, activityID uuid.UUID) (bool, error) {
	return f.approved[activityID], nil
}

// TestActivityCompleteness_LegitimateInterestNeedsApprovedLIA is DPIA-16's own acceptance criterion taken
// literally: a purpose relying on ม.24(5) legitimate interest flags lia_approved until an approved LIA is
// on record for the activity.
func TestActivityCompleteness_LegitimateInterestNeedsApprovedLIA(t *testing.T) {
	e := setup(t, "ropalia")
	dpia := &fakeDpia{approved: map[uuid.UUID]bool{}}
	e.svc.Dpia = dpia

	var activity ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
		if err != nil {
			return err
		}
		activity, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "LIA-RO-01", Name: "วิเคราะห์ความเสี่ยงคู่ค้า", Role: "controller"}, 0)
		return err
	})

	// A purpose on an ordinary lawful basis never needs an LIA at all.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activity.ID, PurposeText: "ปฏิบัติตามสัญญา", LawfulBasisCode: "CONTRACT"}); err != nil {
			return err
		}
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		for _, m := range got.MissingItems {
			if m == "lia_approved" {
				t.Errorf("a contract-basis purpose should never flag lia_approved, got %v", got.MissingItems)
			}
		}
		return nil
	})

	// Adding a legitimate-interest purpose flags lia_approved until an approved LIA exists.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activity.ID, PurposeText: "ประเมินความเสี่ยงคู่ค้า", LawfulBasisCode: "LEGITIMATE_INTEREST"}); err != nil {
			return err
		}
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range got.MissingItems {
			if m == "lia_approved" {
				found = true
			}
		}
		if !found {
			t.Errorf("a legitimate-interest purpose with no approved LIA should flag lia_approved, got %v", got.MissingItems)
		}
		return nil
	})

	// Once the LIA is approved (as the fake's own double would report), the flag clears.
	dpia.approved[activity.ID] = true
	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		for _, m := range got.MissingItems {
			if m == "lia_approved" {
				t.Errorf("an approved LIA should clear lia_approved, got %v", got.MissingItems)
			}
		}
		return nil
	})
}

// TestActivityCompleteness_NoLIAAdapterSkipsCheck proves the nil-means-skip convention: a Service with no
// Dpia wired (e.g. a worker context that never needs this) never flags lia_approved, even for a
// legitimate-interest purpose.
func TestActivityCompleteness_NoLIAAdapterSkipsCheck(t *testing.T) {
	e := setup(t, "ropaliaskip")
	// e.svc.Dpia is nil by default (see setup()).
	var activity ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
		if err != nil {
			return err
		}
		activity, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "LIA-SKIP-01", Name: "ทดสอบ", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		_, err = e.svc.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: activity.ID, PurposeText: "ประเมินความเสี่ยงคู่ค้า", LawfulBasisCode: "LEGITIMATE_INTEREST"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		for _, m := range got.MissingItems {
			if m == "lia_approved" {
				t.Errorf("with no Dpia adapter wired, lia_approved should never be checked, got %v", got.MissingItems)
			}
		}
		return nil
	})
}
