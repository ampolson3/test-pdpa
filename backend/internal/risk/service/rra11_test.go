package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestListHighRiskActivityScores_FindsOnlyCurrentlyHighOrVeryHigh is RRA-11's own acceptance criterion
// directly: a low-scoring activity never appears; a high-scoring one does; and once that same activity is
// re-scored into a lower band (a control added, lowering likelihood), it drops out again — proving the list
// reflects each activity's own *current* score, not any stale earlier high computation.
func TestListHighRiskActivityScores_FindsOnlyCurrentlyHighOrVeryHigh(t *testing.T) {
	e := scoreSetup(t, "rra11")

	var lowID, highID uuid.UUID
	var controlID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		lowID = seedActivity(t, ctx, e)

		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท สอง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR2", NameTh: "HR2", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-HIGH", Name: "กิจกรรมเสี่ยงสูง", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		highID = a.ID

		cat, err := e.org.CreateMaster(ctx, orgservice.KindDataCategories, orgservice.MasterItem{Code: "rra11sens", NameTh: "ข้อมูลอ่อนไหว", IsSensitive: true})
		if err != nil {
			return err
		}
		subj, err := e.org.CreateMaster(ctx, orgservice.KindSubjectTypes, orgservice.MasterItem{Code: "rra11vuln", NameTh: "กลุ่มเปราะบาง", IsVulnerable: true})
		if err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: highID, DataCategoryID: *cat.ID, SubjectTypeID: *subj.ID, Source: "direct", IsSensitive: true, VolumeBand: "gt_100k"}); err != nil {
			return err
		}
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้รับข้อมูล", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: highID, PartyID: party.ID, RecipientRole: "processor"}); err != nil {
			return err
		}

		controls, err := e.svc.ListControls(ctx)
		if err != nil {
			return err
		}
		if len(controls) == 0 {
			t.Fatal("expected the seeded global security-control catalog (ROPA-09) to be non-empty")
		}
		controlID = controls[0].ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.Score(ctx, lowID); err != nil {
			return err
		}
		high, err := e.svc.Score(ctx, highID)
		if err != nil {
			return err
		}
		if high.Level != "high" && high.Level != "very_high" {
			t.Fatalf("expected the seeded activity to score high/very_high, got level=%s score=%v", high.Level, high.Score)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListHighRiskActivityScores(ctx)
		if err != nil {
			return err
		}
		foundHigh, foundLow := false, false
		for _, s := range list {
			if s.ActivityID == highID {
				foundHigh = true
			}
			if s.ActivityID == lowID {
				foundLow = true
			}
		}
		if !foundHigh {
			t.Errorf("expected the high-scoring activity in ListHighRiskActivityScores, got %+v", list)
		}
		if foundLow {
			t.Errorf("the low-scoring activity must not appear in ListHighRiskActivityScores, got %+v", list)
		}
		return nil
	})

	// Adding a control removes the "no_controls" likelihood factor, dropping the score below the high
	// threshold — the list must reflect this current, lower score, not the earlier high one.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.ropa.AddActivityControl(ctx, ropaservice.ActivityControl{ActivityID: highID, ControlID: controlID}); err != nil {
			return err
		}
		after, err := e.svc.Score(ctx, highID)
		if err != nil {
			return err
		}
		if after.Level == "high" || after.Level == "very_high" {
			t.Fatalf("expected adding a control to lower the score below high, got level=%s score=%v", after.Level, after.Score)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListHighRiskActivityScores(ctx)
		if err != nil {
			return err
		}
		for _, s := range list {
			if s.ActivityID == highID {
				t.Errorf("the now-lower-scoring activity must no longer appear in ListHighRiskActivityScores, got %+v", list)
			}
		}
		return nil
	})
}

// TestListHighRiskActivityScores_TwoTenantIsolation proves RLS: tenant B never sees tenant A's own
// high-risk activity.
func TestListHighRiskActivityScores_TwoTenantIsolation(t *testing.T) {
	a := scoreSetup(t, "rra11isoA")
	b := scoreSetup(t, "rra11isoB")

	var activityID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, a.svc)
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "A", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "A", NameTh: "A", UnitType: "department"})
		if err != nil {
			return err
		}
		act, err := a.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "ISO-01", Name: "A", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		activityID = act.ID
		cat, err := a.org.CreateMaster(ctx, orgservice.KindDataCategories, orgservice.MasterItem{Code: "iso11sens", NameTh: "ข้อมูลอ่อนไหว", IsSensitive: true})
		if err != nil {
			return err
		}
		subj, err := a.org.CreateMaster(ctx, orgservice.KindSubjectTypes, orgservice.MasterItem{Code: "iso11vuln", NameTh: "เปราะบาง", IsVulnerable: true})
		if err != nil {
			return err
		}
		if _, err := a.ropa.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: activityID, DataCategoryID: *cat.ID, SubjectTypeID: *subj.ID, Source: "direct", IsSensitive: true, VolumeBand: "gt_100k"}); err != nil {
			return err
		}
		party, err := a.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้รับข้อมูล", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		if _, err := a.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: activityID, PartyID: party.ID, RecipientRole: "processor"}); err != nil {
			return err
		}
		_, err = a.svc.Score(ctx, activityID)
		return err
	})

	b.in(t, func(ctx context.Context) error {
		list, err := b.svc.ListHighRiskActivityScores(ctx)
		if err != nil {
			return err
		}
		for _, s := range list {
			if s.ActivityID == activityID {
				t.Error("tenant B must never see tenant A's own high-risk activity")
			}
		}
		return nil
	})
}
