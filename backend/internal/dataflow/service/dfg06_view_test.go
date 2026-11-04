package service_test

import (
	"context"
	"testing"

	dataflowservice "pdpa-platform/internal/dataflow/service"
	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// nonSwimlaneNodeIDs is DFG-06's own acceptance criterion check: every node that isn't a swimlane itself
// (org_unit/legal_entity/data_category) — activities, recipients, countries, disposal methods — the part
// of the diagram that must read identically whichever view it was requested under ("สลับมุมมองได้โดย
// ข้อมูลตรงกัน").
func nonSwimlaneNodeIDs(nodes []dataflowservice.Node) map[string]bool {
	out := map[string]bool{}
	for _, n := range nodes {
		if n.Type == "org_unit" || n.Type == "legal_entity" || n.Type == "data_category" {
			continue
		}
		out[n.ID] = true
	}
	return out
}

func sameKeys(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// TestGraph_SwitchingViewsKeepsTheSameUnderlyingData is DFG-06's own literal acceptance criterion: the
// department (default), company and data_category views all return the exact same activity/recipient/
// country/disposal nodes and edges — only each activity's own swimlane (org_unit vs legal_entity vs
// data_category) differs.
func TestGraph_SwitchingViewsKeepsTheSameUnderlyingData(t *testing.T) {
	e := setup(t, "dfg06same")
	var le orgservice.LegalEntity
	var unit orgservice.OrgUnit
	var activity ropaservice.Activity

	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "ฝ่ายบุคคล", UnitType: "department"}); err != nil {
			return err
		}
		if activity, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-01", Name: "กิจกรรมฝ่ายบุคคล", Role: "controller"}, 0); err != nil {
			return err
		}
		_, err = e.ropa.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: activity.ID, RetentionBasis: "นโยบายบริษัท", TriggerEvent: "สิ้นสุดสัญญา", DisposalMethod: "delete"})
		return err
	})

	var deptIDs, companyIDs, categoryIDs map[string]bool
	var deptEdges, companyEdges, categoryEdges int
	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil, dataflowservice.ViewDepartment)
		if err != nil {
			return err
		}
		deptIDs, deptEdges = nonSwimlaneNodeIDs(g.Nodes), len(g.Edges)
		if n, ok := findNode(g.Nodes, "activity:"+activity.ID.String()); !ok || n.ParentID == "" {
			t.Fatal("expected the activity's own parent_id under the department view")
		} else if n.ParentID != "org_unit:"+unit.ID.String() {
			t.Errorf("department view parent_id = %q, want the org unit's", n.ParentID)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil, dataflowservice.ViewCompany)
		if err != nil {
			return err
		}
		companyIDs, companyEdges = nonSwimlaneNodeIDs(g.Nodes), len(g.Edges)
		if n, ok := findNode(g.Nodes, "activity:"+activity.ID.String()); !ok {
			t.Fatal("expected the activity node under the company view")
		} else if n.ParentID != "legal_entity:"+le.ID.String() {
			t.Errorf("company view parent_id = %q, want the legal entity's", n.ParentID)
		}
		if _, ok := findNode(g.Nodes, "legal_entity:"+le.ID.String()); !ok {
			t.Error("expected a legal_entity swimlane node")
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil, dataflowservice.ViewDataCategory)
		if err != nil {
			return err
		}
		categoryIDs, categoryEdges = nonSwimlaneNodeIDs(g.Nodes), len(g.Edges)
		// no data category was ever linked to this activity, so it falls under the "uncategorized" swimlane.
		if n, ok := findNode(g.Nodes, "activity:"+activity.ID.String()); !ok {
			t.Fatal("expected the activity node under the data_category view")
		} else if n.ParentID != "data_category:uncategorized" {
			t.Errorf("data_category view parent_id = %q, want the uncategorized swimlane", n.ParentID)
		}
		return nil
	})

	if !sameKeys(deptIDs, companyIDs) || !sameKeys(deptIDs, categoryIDs) {
		t.Errorf("non-swimlane node sets differ across views: department=%v company=%v data_category=%v", deptIDs, companyIDs, categoryIDs)
	}
	if deptEdges != companyEdges || deptEdges != categoryEdges {
		t.Errorf("edge counts differ across views: department=%d company=%d data_category=%d", deptEdges, companyEdges, categoryEdges)
	}
}

// TestGraph_CompanyView_SeparatesLegalEntitiesIntoSwimlanes is the company view's own distinguishing
// behaviour: two legal entities' activities land under two separate legal_entity swimlanes, never merged
// under one, and never under an org_unit swimlane the way the department view would.
func TestGraph_CompanyView_SeparatesLegalEntitiesIntoSwimlanes(t *testing.T) {
	e := setup(t, "dfg06company")
	var leA, leB orgservice.LegalEntity
	var unitA, unitB orgservice.OrgUnit
	var actA, actB ropaservice.Activity

	e.in(t, func(ctx context.Context) error {
		var err error
		if leA, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอ จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if leB, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท บี จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unitA, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: leA.ID, Code: "HR", NameTh: "ฝ่ายบุคคล เอ", UnitType: "department"}); err != nil {
			return err
		}
		if unitB, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: leB.ID, Code: "HR", NameTh: "ฝ่ายบุคคล บี", UnitType: "department"}); err != nil {
			return err
		}
		if actA, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: leA.ID, OrgUnitID: unitA.ID, Code: "A-01", Name: "กิจกรรม เอ", Role: "controller"}, 0); err != nil {
			return err
		}
		actB, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: leB.ID, OrgUnitID: unitB.ID, Code: "B-01", Name: "กิจกรรม บี", Role: "controller"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil, dataflowservice.ViewCompany)
		if err != nil {
			return err
		}
		nA, ok := findNode(g.Nodes, "activity:"+actA.ID.String())
		if !ok {
			t.Fatal("missing activity A")
		}
		nB, ok := findNode(g.Nodes, "activity:"+actB.ID.String())
		if !ok {
			t.Fatal("missing activity B")
		}
		if nA.ParentID == nB.ParentID {
			t.Errorf("expected two separate legal_entity swimlanes, both activities landed under %q", nA.ParentID)
		}
		if nA.ParentID != "legal_entity:"+leA.ID.String() || nB.ParentID != "legal_entity:"+leB.ID.String() {
			t.Errorf("parent_id = %q/%q, want each activity's own legal entity", nA.ParentID, nB.ParentID)
		}
		return nil
	})
}

// TestGraph_DataCategoryView_GroupsByPrimaryCategory proves the data_category view's own swimlane
// assignment: an activity with a logged category lands under that category's own swimlane (named for it),
// never the uncategorized one.
func TestGraph_DataCategoryView_GroupsByPrimaryCategory(t *testing.T) {
	e := setup(t, "dfg06cat")
	var le orgservice.LegalEntity
	var unit orgservice.OrgUnit
	var activity ropaservice.Activity
	var category orgservice.MasterItem

	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if unit, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "ฝ่ายบุคคล", UnitType: "department"}); err != nil {
			return err
		}
		cats, err := e.org.ListMaster(ctx, "data_categories")
		if err != nil {
			return err
		}
		if len(cats) == 0 {
			t.Fatal("expected ORG-07's own seeded default data categories")
		}
		category = cats[0]
		subjects, err := e.org.ListMaster(ctx, "data_subject_types")
		if err != nil {
			return err
		}
		if len(subjects) == 0 {
			t.Fatal("expected ORG-07's own seeded default data subject types")
		}
		if activity, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "HR-02", Name: "กิจกรรมฝ่ายบุคคล 2", Role: "controller"}, 0); err != nil {
			return err
		}
		_, err = e.ropa.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: activity.ID, DataCategoryID: *category.ID, SubjectTypeID: *subjects[0].ID, Source: "direct"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil, dataflowservice.ViewDataCategory)
		if err != nil {
			return err
		}
		n, ok := findNode(g.Nodes, "activity:"+activity.ID.String())
		if !ok {
			t.Fatal("missing activity node")
		}
		wantParent := "data_category:" + category.ID.String()
		if n.ParentID != wantParent {
			t.Errorf("parent_id = %q, want %q", n.ParentID, wantParent)
		}
		swimlane, ok := findNode(g.Nodes, wantParent)
		if !ok || swimlane.Label != category.NameTh {
			t.Errorf("swimlane node = %+v, want label %q", swimlane, category.NameTh)
		}
		if _, ok := findNode(g.Nodes, "data_category:uncategorized"); ok {
			t.Error("did not expect an uncategorized swimlane when every activity has a category")
		}
		return nil
	})
}
