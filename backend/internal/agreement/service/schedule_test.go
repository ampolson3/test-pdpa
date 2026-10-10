package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestProcessingSchedule_MatchesRoPA is DPA-04's acceptance criterion directly: the annex's purposes, data,
// retention and security measures for a linked activity are exactly what that activity's own RoPA rows say
// — resolved to display names through org/ropa's own exported services (rule 9), not re-derived.
func TestProcessingSchedule_MatchesRoPA(t *testing.T) {
	e := setup(t, "dpaSchedule")
	var leID, vendorID, activityID uuid.UUID
	var basisCode string
	var categoryID, subjectTypeID, controlID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)

		bases, err := e.org.ListMaster(ctx, orgservice.KindLawfulBases)
		if err != nil {
			return err
		}
		for _, b := range bases {
			if !b.RequiresConsent {
				basisCode = b.Code
				break
			}
		}
		if basisCode == "" {
			t.Fatal("expected at least one non-consent lawful basis to be seeded (ORG-07)")
		}

		cats, err := e.org.ListMaster(ctx, orgservice.KindDataCategories)
		if err != nil {
			return err
		}
		categoryID = *cats[0].ID

		subjects, err := e.org.ListMaster(ctx, orgservice.KindSubjectTypes)
		if err != nil {
			return err
		}
		subjectTypeID = *subjects[0].ID

		controls, err := e.ropa.ListControls(ctx)
		if err != nil {
			return err
		}
		controlID = controls[0].ID

		if _, err := e.ropa.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{
			ActivityID: activityID, PurposeText: "จ่ายเงินเดือน", LawfulBasisCode: basisCode,
		}); err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityData(ctx, ropaservice.ActivityData{
			ActivityID: activityID, DataCategoryID: categoryID, SubjectTypeID: subjectTypeID, Source: "direct",
		}); err != nil {
			return err
		}
		months := 60
		if _, err := e.ropa.AddRetentionRule(ctx, ropaservice.RetentionRule{
			ActivityID: activityID, DataCategoryID: &categoryID, RetentionMonths: &months,
			RetentionBasis: "ตามกฎหมายแรงงาน", TriggerEvent: "สิ้นสุดสัญญาจ้าง", DisposalMethod: "destroy",
		}); err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityControl(ctx, ropaservice.ActivityControl{
			ActivityID: activityID, ControlID: controlID, Description: "เข้ารหัสฐานข้อมูล",
		}); err != nil {
			return err
		}
		return nil
	})

	a := createAgreement(t, e, leID, vendorID, []uuid.UUID{activityID})

	e.in(t, func(ctx context.Context) error {
		sched, err := e.svc.ProcessingSchedule(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(sched.Activities) != 1 {
			t.Fatalf("activities = %d, want 1", len(sched.Activities))
		}
		sa := sched.Activities[0]
		if sa.ActivityID != activityID {
			t.Errorf("activity_id = %s, want %s", sa.ActivityID, activityID)
		}
		if len(sa.Purposes) != 1 || sa.Purposes[0].Text != "จ่ายเงินเดือน" || sa.Purposes[0].LawfulBasisCode != basisCode {
			t.Errorf("purposes = %+v", sa.Purposes)
		}
		if len(sa.Data) != 1 || !sa.Data[0].IsSensitive && sa.Data[0].CategoryNameTh == "" {
			t.Errorf("data = %+v", sa.Data)
		}
		if len(sa.Retention) != 1 || sa.Retention[0].TriggerEvent != "สิ้นสุดสัญญาจ้าง" || *sa.Retention[0].RetentionMonths != 60 {
			t.Errorf("retention = %+v", sa.Retention)
		}
		if len(sa.SecurityMeasures) != 1 || sa.SecurityMeasures[0].Description != "เข้ารหัสฐานข้อมูล" {
			t.Errorf("security measures = %+v", sa.SecurityMeasures)
		}
		return nil
	})
}

// TestProcessingSchedule_NoActivities: an agreement with no linked activities has an empty schedule, not
// an error.
func TestProcessingSchedule_NoActivities(t *testing.T) {
	e := setup(t, "dpaScheduleEmpty")
	var leID, vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, nil)

	e.in(t, func(ctx context.Context) error {
		sched, err := e.svc.ProcessingSchedule(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(sched.Activities) != 0 {
			t.Errorf("activities = %d, want 0", len(sched.Activities))
		}
		return nil
	})
}

// TestProcessingSchedule_TwoTenantIsolation: tenant B cannot read tenant A's processing schedule by
// agreement id.
func TestProcessingSchedule_TwoTenantIsolation(t *testing.T) {
	eA := setup(t, "dpaScheduleIsoA")
	var leID, vendorID uuid.UUID
	eA.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, eA)
		return nil
	})
	a := createAgreement(t, eA, leID, vendorID, nil)

	eB := setup(t, "dpaScheduleIsoB")
	eB.in(t, func(ctx context.Context) error {
		if _, err := eB.svc.ProcessingSchedule(ctx, a.ID); err != agreementservice.ErrNotFound {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
		return nil
	})
}
