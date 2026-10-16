package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// twoDeptActivities creates one legal entity with two departments, each owning one RoPA activity — the
// minimal shape DPIA-12's registry needs to prove it aggregates across both activities and departments.
func twoDeptActivities(t *testing.T, ctx context.Context, e env) (le orgservice.LegalEntity, u1, u2 orgservice.OrgUnit, a1, a2 ropaservice.Activity) {
	t.Helper()
	var err error
	le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทะเบียน จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	u1, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "ฝ่ายบุคคล", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	u2, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "MKT", NameTh: "ฝ่ายการตลาด", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	a1, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: u1.ID, Code: "REG-HR", Name: "กิจกรรมฝ่ายบุคคล", Role: "controller"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	a2, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: u2.ID, Code: "REG-MKT", Name: "กิจกรรมฝ่ายการตลาด", Role: "controller"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return le, u1, u2, a1, a2
}

// TestRegistry_ReflectsLiveStatusAcrossActivitiesAndDepartments is DPIA-12's own acceptance criterion
// ("สถานะในทะเบียนตรงกับขั้นตอนจริงของแต่ละ DPIA"): the registry lists every activity's own current round
// with its real status and department, across more than one department, and the filters narrow correctly.
func TestRegistry_ReflectsLiveStatusAcrossActivitiesAndDepartments(t *testing.T) {
	e := setup(t, "dpiareg")
	var le orgservice.LegalEntity
	var u1, u2 orgservice.OrgUnit
	var a1, a2 ropaservice.Activity

	e.in(t, func(ctx context.Context) error {
		le, u1, u2, a1, a2 = twoDeptActivities(t, ctx, e)
		if _, err := e.svc.Screen(ctx, a1.ID, allAnswers("no")); err != nil {
			return err
		}
		answers := allAnswers("no")
		answers["sensitive_data"] = "yes"
		answers["large_scale"] = "yes"
		if _, err := e.svc.Screen(ctx, a2.ID, answers); err != nil {
			return err
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.Registry(ctx, dpiaservice.RegistryFilter{})
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Fatalf("got %d entries, want 2", len(list))
		}
		byActivity := map[uuid.UUID]dpiaservice.RegistryEntry{}
		for _, r := range list {
			byActivity[r.ActivityID] = r
		}
		r1, ok := byActivity[a1.ID]
		if !ok || r1.Status != "not_required" || r1.OrgUnitID != u1.ID || r1.OrgUnitName != u1.NameTh || r1.LegalEntityID != le.ID {
			t.Errorf("activity 1 entry: %+v", r1)
		}
		r2, ok := byActivity[a2.ID]
		if !ok || r2.Status != "in_progress" || r2.ScreeningResult != "required" || r2.OrgUnitID != u2.ID {
			t.Errorf("activity 2 entry: %+v", r2)
		}

		// Department filter narrows to that department's own activity.
		filtered, err := e.svc.Registry(ctx, dpiaservice.RegistryFilter{OrgUnitID: &u1.ID})
		if err != nil {
			return err
		}
		if len(filtered) != 1 || filtered[0].ActivityID != a1.ID {
			t.Errorf("org_unit filter: %+v", filtered)
		}

		// Legal-entity filter keeps both (same entity).
		byEntity, err := e.svc.Registry(ctx, dpiaservice.RegistryFilter{LegalEntityID: &le.ID})
		if err != nil {
			return err
		}
		if len(byEntity) != 2 {
			t.Errorf("legal_entity filter: got %d, want 2", len(byEntity))
		}

		// Status filter keeps only the matching round.
		byStatus, err := e.svc.Registry(ctx, dpiaservice.RegistryFilter{Status: "in_progress"})
		if err != nil {
			return err
		}
		if len(byStatus) != 1 || byStatus[0].ActivityID != a2.ID {
			t.Errorf("status filter: %+v", byStatus)
		}

		// Re-screening activity 1 opens a new round that now requires a DPIA — the registry's one row for
		// that activity must show the new, current status, not the stale first round (the literal
		// acceptance criterion: the register always matches each DPIA's real current state).
		answers := allAnswers("no")
		answers["monitoring"] = "yes"
		answers["new_tech"] = "yes"
		if _, err := e.svc.Screen(ctx, a1.ID, answers); err != nil {
			return err
		}
		after, err := e.svc.Registry(ctx, dpiaservice.RegistryFilter{OrgUnitID: &u1.ID})
		if err != nil {
			return err
		}
		if len(after) != 1 || after[0].Status != "in_progress" || after[0].RoundNo != 2 {
			t.Errorf("after re-screen: %+v", after)
		}
		return nil
	})
}

// TestRegistry_TwoTenantIsolation proves tenant B's registry never includes tenant A's activities/rounds.
func TestRegistry_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpiaregisoA")
	b := setup(t, "dpiaregisoB")

	a.in(t, func(ctx context.Context) error {
		_, _, _, a1, a2 := twoDeptActivities(t, ctx, a)
		if _, err := a.svc.Screen(ctx, a1.ID, allAnswers("no")); err != nil {
			return err
		}
		if _, err := a.svc.Screen(ctx, a2.ID, allAnswers("no")); err != nil {
			return err
		}
		return nil
	})

	b.in(t, func(ctx context.Context) error {
		list, err := b.svc.Registry(ctx, dpiaservice.RegistryFilter{})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B registry: got %d entries, want 0 (tenant A's rows must not leak)", len(list))
		}
		return nil
	})
}
