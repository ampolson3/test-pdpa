package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"

	dataflowservice "pdpa-platform/internal/dataflow/service"
)

// TestLayout_NeverSaved reads back the empty-defaults shape ("ยังไม่เคยบันทึก") — row_version 0, so the
// very first SaveLayout call can carry If-Match: "0", the same convention ORG-20's own org_settings uses.
func TestLayout_NeverSaved(t *testing.T) {
	e := setup(t, "dfg05never")
	e.in(t, func(ctx context.Context) error {
		l, err := e.svc.GetLayout(ctx, nil, "")
		if err != nil {
			return err
		}
		if l.RowVersion != 0 {
			t.Errorf("got row_version=%d, want 0", l.RowVersion)
		}
		if len(l.Positions) != 0 || len(l.Notes) != 0 {
			t.Errorf("got non-empty defaults: %+v", l)
		}
		return nil
	})
}

// TestLayout_SavePersistsAcrossGraphRecompute is the acceptance criterion itself ("ตำแหน่งที่จัดเองยังอยู่
// หลังข้อมูลอัปเดต"): a saved position for an activity node stays exactly where it was dragged after a
// real RoPA data update (a second activity added, which recomputes the live Graph completely) — the
// overlay is read back unchanged because it's keyed by the node's own stable id, never derived from the
// graph itself.
func TestLayout_SavePersistsAcrossGraphRecompute(t *testing.T) {
	e := setup(t, "dfg05persist")
	var nodeID string

	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "L-01", Name: "กิจกรรม 1", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		nodeID = "activity:" + a.ID.String()
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		l, err := e.svc.SaveLayout(ctx, nil, "", map[string]dataflowservice.NodePosition{nodeID: {X: 120, Y: 340}}, map[string]string{nodeID: "ย้ายมาตรงนี้"}, 0)
		if err != nil {
			return err
		}
		if l.RowVersion != 1 {
			t.Errorf("got row_version=%d, want 1", l.RowVersion)
		}
		return nil
	})

	// a real data update — the acceptance criterion's own "ข้อมูลอัปเดต" — recomputes the live Graph
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท สอง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		_, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "L-02", Name: "กิจกรรม 2", Role: "controller"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil, "")
		if err != nil {
			return err
		}
		if g.ActivityCount != 2 {
			t.Errorf("got activity_count=%d, want 2 (data really changed)", g.ActivityCount)
		}
		l, err := e.svc.GetLayout(ctx, nil, "")
		if err != nil {
			return err
		}
		if l.RowVersion != 1 {
			t.Errorf("got row_version=%d, want 1 (unchanged)", l.RowVersion)
		}
		pos, ok := l.Positions[nodeID]
		if !ok || pos.X != 120 || pos.Y != 340 {
			t.Errorf("got positions=%+v, want %q to still be at (120, 340)", l.Positions, nodeID)
		}
		if l.Notes[nodeID] != "ย้ายมาตรงนี้" {
			t.Errorf("got note=%q, want the saved annotation", l.Notes[nodeID])
		}
		return nil
	})
}

// TestLayout_SaveReplacesWholeOverlay proves a second SaveLayout is a full replace, not a merge — a
// position/note left out of the second call no longer reads back.
func TestLayout_SaveReplacesWholeOverlay(t *testing.T) {
	e := setup(t, "dfg05replace")

	var v int32
	e.in(t, func(ctx context.Context) error {
		l, err := e.svc.SaveLayout(ctx, nil, "", map[string]dataflowservice.NodePosition{"a": {X: 1, Y: 1}, "b": {X: 2, Y: 2}}, map[string]string{"a": "note a"}, 0)
		if err != nil {
			return err
		}
		v = l.RowVersion
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		l, err := e.svc.SaveLayout(ctx, nil, "", map[string]dataflowservice.NodePosition{"a": {X: 9, Y: 9}}, nil, v)
		if err != nil {
			return err
		}
		if _, ok := l.Positions["b"]; ok {
			t.Error("node b should have been dropped by the full-replace save")
		}
		if l.Positions["a"].X != 9 {
			t.Errorf("got a.X=%v, want 9", l.Positions["a"].X)
		}
		if len(l.Notes) != 0 {
			t.Errorf("got notes=%+v, want empty (the second save sent none)", l.Notes)
		}
		return nil
	})
}

// TestLayout_StaleRowVersionRefused proves the ETag guard: a save against an already-superseded
// row_version is refused rather than silently overwriting a concurrent change.
func TestLayout_StaleRowVersionRefused(t *testing.T) {
	e := setup(t, "dfg05stale")

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveLayout(ctx, nil, "", map[string]dataflowservice.NodePosition{"a": {X: 1, Y: 1}}, nil, 0); err != nil {
			return err
		}
		_, err := e.svc.SaveLayout(ctx, nil, "", map[string]dataflowservice.NodePosition{"a": {X: 2, Y: 2}}, nil, 0)
		if err == nil {
			t.Fatal("want ErrVersionMismatch against the already-advanced row_version 1")
		}
		if err != dataflowservice.ErrVersionMismatch {
			t.Errorf("got %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

// TestLayout_Reset is "คืนค่าเดิม": discarding a saved overlay reverts GetLayout to the empty defaults;
// resetting again (nothing saved) is a harmless no-op.
func TestLayout_Reset(t *testing.T) {
	e := setup(t, "dfg05reset")

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SaveLayout(ctx, nil, "", map[string]dataflowservice.NodePosition{"a": {X: 1, Y: 1}}, nil, 0); err != nil {
			return err
		}
		if err := e.svc.ResetLayout(ctx, nil, ""); err != nil {
			return err
		}
		l, err := e.svc.GetLayout(ctx, nil, "")
		if err != nil {
			return err
		}
		if l.RowVersion != 0 || len(l.Positions) != 0 {
			t.Errorf("got %+v, want the empty defaults after reset", l)
		}
		// a second reset with nothing saved is a harmless no-op
		return e.svc.ResetLayout(ctx, nil, "")
	})
}

// TestLayout_ScopedPerLegalEntityAndView proves one (legal_entity, view) diagram's own overlay never
// leaks into a different legal_entity or a different view's own overlay.
func TestLayout_ScopedPerLegalEntityAndView(t *testing.T) {
	e := setup(t, "dfg05scope")

	var le1, le2 string
	e.in(t, func(ctx context.Context) error {
		a, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "A", IsController: true}, 0)
		if err != nil {
			return err
		}
		b, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "B", IsController: true}, 0)
		if err != nil {
			return err
		}
		le1, le2 = a.ID.String(), b.ID.String()
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		a, err := uuid.Parse(le1)
		if err != nil {
			return err
		}
		b, err := uuid.Parse(le2)
		if err != nil {
			return err
		}
		if _, err := e.svc.SaveLayout(ctx, &a, dataflowservice.ViewDepartment, map[string]dataflowservice.NodePosition{"x": {X: 1, Y: 1}}, nil, 0); err != nil {
			return err
		}
		if _, err := e.svc.SaveLayout(ctx, &a, dataflowservice.ViewCompany, map[string]dataflowservice.NodePosition{"x": {X: 2, Y: 2}}, nil, 0); err != nil {
			return err
		}

		deptOverlay, err := e.svc.GetLayout(ctx, &a, dataflowservice.ViewDepartment)
		if err != nil {
			return err
		}
		if deptOverlay.Positions["x"].X != 1 {
			t.Errorf("department view: got x=%v, want 1", deptOverlay.Positions["x"].X)
		}
		companyOverlay, err := e.svc.GetLayout(ctx, &a, dataflowservice.ViewCompany)
		if err != nil {
			return err
		}
		if companyOverlay.Positions["x"].X != 2 {
			t.Errorf("company view: got x=%v, want 2 (must not share the department view's own overlay)", companyOverlay.Positions["x"].X)
		}
		otherEntity, err := e.svc.GetLayout(ctx, &b, dataflowservice.ViewDepartment)
		if err != nil {
			return err
		}
		if len(otherEntity.Positions) != 0 {
			t.Errorf("legal entity B's own overlay must stay empty, got %+v", otherEntity.Positions)
		}
		return nil
	})
}

// TestLayout_TwoTenantIsolation proves RLS: tenant B never sees tenant A's own saved overlay.
func TestLayout_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dfg05isoA")
	b := setup(t, "dfg05isoB")

	a.in(t, func(ctx context.Context) error {
		_, err := a.svc.SaveLayout(ctx, nil, "", map[string]dataflowservice.NodePosition{"a": {X: 1, Y: 1}}, map[string]string{"a": "tenant A's own note"}, 0)
		return err
	})

	b.in(t, func(ctx context.Context) error {
		l, err := b.svc.GetLayout(ctx, nil, "")
		if err != nil {
			return err
		}
		if len(l.Positions) != 0 || l.RowVersion != 0 {
			t.Errorf("tenant B must never see tenant A's own saved overlay, got %+v", l)
		}
		return nil
	})
}
