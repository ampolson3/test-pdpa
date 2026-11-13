package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestActivityControls_MissingUntilReferenced is the acceptance criterion: every activity must
// reference a security measure under ม.37(1); adding and removing one flips security_controls in
// and out of the missing-item list.
func TestActivityControls_MissingUntilReferenced(t *testing.T) {
	e := setup(t, "ropacontrol")
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
		activity, err = e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "IT-02", Name: "ระบบสำรองข้อมูล", Role: "controller"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range got.MissingItems {
			if m == "security_controls" {
				found = true
			}
		}
		if !found {
			t.Errorf("a brand-new activity should flag security_controls, got %v", got.MissingItems)
		}
		return nil
	})

	var controls []riskControl
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListControls(ctx)
		if err != nil {
			return err
		}
		if len(list) < 2 {
			t.Fatal("expected at least two seeded security controls (migration 00040)")
		}
		for _, c := range list {
			controls = append(controls, riskControl{id: c.ID, category: c.Category})
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddActivityControl(ctx, ropaservice.ActivityControl{ActivityID: activity.ID, ControlID: uuid.New()}); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("unknown control_id: %v, want ErrInvalid", err)
		}
		return nil
	})

	var linked ropaservice.ActivityControl
	e.in(t, func(ctx context.Context) error {
		var err error
		linked, err = e.svc.AddActivityControl(ctx, ropaservice.ActivityControl{ActivityID: activity.ID, ControlID: controls[0].id, Description: "เข้ารหัสฐานข้อมูลด้วย AES-256"})
		if err != nil {
			return err
		}
		if linked.Description != "เข้ารหัสฐานข้อมูลด้วย AES-256" {
			t.Errorf("description did not stick: %+v", linked)
		}
		// Duplicate link is refused, not a silent no-op.
		if _, err := e.svc.AddActivityControl(ctx, ropaservice.ActivityControl{ActivityID: activity.ID, ControlID: controls[0].id}); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("duplicate control link: %v, want ErrInvalid", err)
		}
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		for _, m := range got.MissingItems {
			if m == "security_controls" {
				t.Errorf("security_controls should clear once a measure is referenced: %v", got.MissingItems)
			}
		}
		list, err := e.svc.ListActivityControls(ctx, activity.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Errorf("expected one linked control, got %+v", list)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if err := e.svc.DeleteActivityControl(ctx, activity.ID, linked.ControlID); err != nil {
			return err
		}
		got, err := e.svc.GetActivity(ctx, activity.ID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range got.MissingItems {
			if m == "security_controls" {
				found = true
			}
		}
		if !found {
			t.Errorf("deleting the only control should bring security_controls back: %v", got.MissingItems)
		}
		return nil
	})
}

type riskControl struct {
	id       uuid.UUID
	category string
}

func TestListControls_CoversAllMandatoryCategories(t *testing.T) {
	e := setup(t, "ropacontrolcat")
	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListControls(ctx)
		if err != nil {
			return err
		}
		want := map[string]bool{"organizational": true, "technical": true, "physical": true, "access_control": true}
		for _, c := range list {
			delete(want, c.Category)
		}
		if len(want) != 0 {
			t.Errorf("expected the seeded catalog to cover %v, still missing %v", []string{"organizational", "technical", "physical", "access_control"}, want)
		}
		return nil
	})
}

func TestActivityControls_Isolation(t *testing.T) {
	a := setup(t, "ropacontrola")
	b := setup(t, "ropacontrolb")
	var activity ropaservice.Activity
	var control ropaservice.ActivityControl
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "เอ", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := a.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "U", NameTh: "U", UnitType: "department"})
		if err != nil {
			return err
		}
		activity, err = a.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "A-01", Name: "เอ", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		list, err := a.svc.ListControls(ctx)
		if err != nil {
			return err
		}
		control, err = a.svc.AddActivityControl(ctx, ropaservice.ActivityControl{ActivityID: activity.ID, ControlID: list[0].ID})
		return err
	})
	b.in(t, func(ctx context.Context) error {
		list, err := b.svc.ListActivityControls(ctx, activity.ID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B should not see tenant A's control link, got %+v", list)
		}
		if err := b.svc.DeleteActivityControl(ctx, activity.ID, control.ControlID); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("tenant B deleting tenant A's control link: %v, want ErrNotFound", err)
		}
		return nil
	})
}
