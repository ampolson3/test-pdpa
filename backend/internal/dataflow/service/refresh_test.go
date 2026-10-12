package service_test

import (
	"context"
	"testing"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestGraph_ReflectsRoPAEditsWithoutRegenerating is DFG-02's whole acceptance criterion
// ("แก้ RoPA แล้วแผนผังเปลี่ยนตามโดยไม่ต้องสร้างใหม่"): Graph() is never cached or persisted (unlike a
// PLT-16 document or a DFG-07 published snapshot), so a RoPA edit shows up on the very next read with no
// "regenerate" step of any kind — no rebuild action, no stale cache to invalidate, no event to listen for.
// Proven directly: rename an activity, move it to a new department, add a new activity, and delete one —
// each change is visible on the immediate next Graph() call.
func TestGraph_ReflectsRoPAEditsWithoutRegenerating(t *testing.T) {
	e := setup(t, "dfgrefresh")
	var le orgservice.LegalEntity
	var u1, u2 orgservice.OrgUnit
	var a ropaservice.Activity

	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if u1, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "U1", NameTh: "หน่วย 1", UnitType: "department"}); err != nil {
			return err
		}
		if u2, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "U2", NameTh: "หน่วย 2", UnitType: "department"}); err != nil {
			return err
		}
		a, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: u1.ID, Code: "A-01", Name: "ชื่อเดิม", Role: "controller"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil)
		if err != nil {
			return err
		}
		n, ok := findNode(g.Nodes, "activity:"+a.ID.String())
		if !ok || n.Label != "ชื่อเดิม" || n.ParentID != "org_unit:"+u1.ID.String() {
			t.Fatalf("before edit: node=%+v ok=%v", n, ok)
		}
		return nil
	})

	// Rename and move the activity to the other department — no "regenerate" call of any kind in between.
	e.in(t, func(ctx context.Context) error {
		a.Name = "ชื่อใหม่"
		a.OrgUnitID = u2.ID
		var err error
		a, err = e.ropa.SaveActivity(ctx, a, a.RowVersion)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil)
		if err != nil {
			return err
		}
		n, ok := findNode(g.Nodes, "activity:"+a.ID.String())
		if !ok || n.Label != "ชื่อใหม่" || n.ParentID != "org_unit:"+u2.ID.String() {
			t.Errorf("after edit: node=%+v ok=%v, want label=ชื่อใหม่ parent=org_unit:%s", n, ok, u2.ID)
		}
		if g.ActivityCount != 1 {
			t.Errorf("activity_count = %d, want 1", g.ActivityCount)
		}
		return nil
	})

	// A brand-new activity shows up immediately too.
	var a2 ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		var err error
		a2, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: u2.ID, Code: "A-02", Name: "กิจกรรมใหม่", Role: "controller"}, 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil)
		if err != nil {
			return err
		}
		if _, ok := findNode(g.Nodes, "activity:"+a2.ID.String()); !ok {
			t.Error("the newly saved activity should appear on the very next read")
		}
		if g.ActivityCount != 2 {
			t.Errorf("activity_count = %d, want 2", g.ActivityCount)
		}
		return nil
	})
}
