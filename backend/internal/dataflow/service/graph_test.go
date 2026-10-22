package service_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	agreementservice "pdpa-platform/internal/agreement/service"
	dataflowservice "pdpa-platform/internal/dataflow/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
	"pdpa-platform/internal/wiring"
)

type env struct {
	app       *pgxpool.Pool
	tenant    dbtest.Tenant
	svc       *dataflowservice.Service
	org       *orgservice.Service
	ropa      *ropaservice.Service
	vendor    *vendorservice.Service
	agreement *agreementservice.Service
}

var perms = []string{"ropa.dataflow.read", "ropa.activity.read", "ropa.activity.create", "ropa.activity.update",
	"org.structure.read", "org.structure.create", "org.structure.update", "org.party.read", "org.party.create",
	"vendor.vendor.read", "vendor.vendor.create", "agreement.dpa.read", "agreement.dpa.create", "agreement.dpa.update",
	"agreement.dsa.read", "agreement.dsa.create", "agreement.dsa.update"}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM agreement.clauses`, `DELETE FROM agreement.agreement_activities`, `DELETE FROM agreement.parties`,
				`DELETE FROM agreement.agreements`, `DELETE FROM platform.approvals`, `DELETE FROM platform.record_versions`,
				`DELETE FROM platform.document_downloads`, `DELETE FROM platform.document_versions`, `DELETE FROM platform.documents`,
				`DELETE FROM vendor.vendors`, `DELETE FROM ropa.processing_activities`,
				`DELETE FROM org.external_parties`, `DELETE FROM org.org_units`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	org := &orgservice.Service{Audit: audit.New()}
	ropa := &ropaservice.Service{Audit: audit.New(), Org: org}
	vendor := &vendorservice.Service{Audit: audit.New(), Org: org}
	versioningSvc := wiring.Versioning(nil, audit.New())
	docsSvc := wiring.Docs(versioningSvc, nil, nil, audit.New(), nil)
	docsSvc.RegisterVersioning()
	agreement := &agreementservice.Service{Docs: docsSvc, Org: org, Vendor: vendor, Ropa: ropa, Audit: audit.New()}
	docsSvc.SetSubmitValidate("dpa", agreement.CheckSubmittable)
	svc := &dataflowservice.Service{Ropa: ropa, Org: org, Vendor: vendor, Agreement: agreement}
	return env{app: app, tenant: tenant, svc: svc, org: org, ropa: ropa, vendor: vendor, agreement: agreement}
}

func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: perms}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

func findEdge(edges []dataflowservice.Edge, typ, source, target string) (dataflowservice.Edge, bool) {
	for _, e := range edges {
		if e.Type == typ && e.Source == source && e.Target == target {
			return e, true
		}
	}
	return dataflowservice.Edge{}, false
}

func findNode(nodes []dataflowservice.Node, id string) (dataflowservice.Node, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return dataflowservice.Node{}, false
}

// TestGraph_SwimlanesActivitiesAndDisposal is the core of DFG-01's acceptance criterion: one org_unit
// swimlane per department, one activity node nested under it, and a disposal node reachable by a "destroy"
// edge — all composed live, matching what was actually saved on the activity.
func TestGraph_SwimlanesActivitiesAndDisposal(t *testing.T) {
	e := setup(t, "dfggraph")
	var le orgservice.LegalEntity
	var u1, u2 orgservice.OrgUnit
	var a1, a2 ropaservice.Activity

	e.in(t, func(ctx context.Context) error {
		var err error
		if le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if u1, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "ฝ่ายบุคคล", UnitType: "department"}); err != nil {
			return err
		}
		if u2, err = e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "MKT", NameTh: "ฝ่ายการตลาด", UnitType: "department"}); err != nil {
			return err
		}
		if a1, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: u1.ID, Code: "HR-01", Name: "กิจกรรมฝ่ายบุคคล", Role: "controller"}, 0); err != nil {
			return err
		}
		if a2, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: u2.ID, Code: "MKT-01", Name: "กิจกรรมฝ่ายการตลาด", Role: "controller"}, 0); err != nil {
			return err
		}
		if _, err = e.ropa.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: a1.ID, RetentionBasis: "นโยบายบริษัท", TriggerEvent: "สิ้นสุดสัญญา", DisposalMethod: "delete"}); err != nil {
			return err
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil)
		if err != nil {
			return err
		}
		if g.ActivityCount != 2 {
			t.Errorf("activity_count = %d, want 2", g.ActivityCount)
		}
		if _, ok := findNode(g.Nodes, "org_unit:"+u1.ID.String()); !ok {
			t.Error("missing org_unit node for u1")
		}
		if _, ok := findNode(g.Nodes, "org_unit:"+u2.ID.String()); !ok {
			t.Error("missing org_unit node for u2")
		}
		actNode, ok := findNode(g.Nodes, "activity:"+a1.ID.String())
		if !ok {
			t.Fatal("missing activity node for a1")
		}
		if actNode.ParentID != "org_unit:"+u1.ID.String() {
			t.Errorf("a1 parent = %q, want org_unit:%s", actNode.ParentID, u1.ID)
		}
		if actNode.ActivityCode != "HR-01" || actNode.Role != "controller" {
			t.Errorf("a1 node fields: %+v", actNode)
		}
		if len(actNode.DisposalMethods) != 1 || actNode.DisposalMethods[0] != "delete" {
			t.Errorf("a1 disposal methods = %v, want [delete]", actNode.DisposalMethods)
		}
		disposalID := "disposal:" + a1.ID.String() + ":delete"
		if _, ok := findNode(g.Nodes, disposalID); !ok {
			t.Error("missing disposal node for a1")
		}
		if _, ok := findEdge(g.Edges, "destroy", "activity:"+a1.ID.String(), disposalID); !ok {
			t.Error("missing destroy edge from a1 to its disposal node")
		}
		// a2 has no retention rule yet — no disposal node for it.
		if _, ok := findNode(g.Nodes, "activity:"+a2.ID.String()); !ok {
			t.Fatal("missing activity node for a2")
		}
		return nil
	})
}

// TestGraph_ForeignTransferWarning is DFG-03's own acceptance criterion: a foreign recipient with no
// logged ROPA-08 transfer is drawn as a warning edge; logging the transfer clears the warning and adds the
// real transfer edge instead. A domestic (TH) recipient never gets a country node or a warning at all.
func TestGraph_ForeignTransferWarning(t *testing.T) {
	e := setup(t, "dfgwarn")
	var a ropaservice.Activity
	var foreignParty, domesticParty orgservice.ExternalParty
	var recipient ropaservice.ActivityRecipient

	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "IT-01", Name: "ระบบคลาวด์", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		foreignParty, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการต่างประเทศ", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		domesticParty, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการในไทย", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		recipient, err = e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: a.ID, PartyID: foreignParty.ID, RecipientRole: "processor"})
		if err != nil {
			return err
		}
		_, err = e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: a.ID, PartyID: domesticParty.ID, RecipientRole: "processor"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil)
		if err != nil {
			return err
		}
		edge, ok := findEdge(g.Edges, "transfer", "recipient:"+foreignParty.ID.String(), "country:US")
		if !ok || !edge.Warning {
			t.Errorf("expected a warning transfer edge to country:US, got edges=%+v", g.Edges)
		}
		if _, ok := findNode(g.Nodes, "country:TH"); ok {
			t.Error("a domestic (TH) recipient should never get a country node")
		}
		if _, ok := findEdge(g.Edges, "send", "activity:"+a.ID.String(), "recipient:"+domesticParty.ID.String()); !ok {
			t.Error("missing send edge to the domestic recipient")
		}
		return nil
	})

	// Logging the transfer clears the warning and adds the real, non-warning transfer edge.
	e.in(t, func(ctx context.Context) error {
		_, err := e.ropa.AddActivityTransfer(ctx, ropaservice.ActivityTransfer{
			ActivityID: a.ID, RecipientID: &recipient.ID, CountryCode: "US", TransferBasis: "standard_clauses",
		})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil)
		if err != nil {
			return err
		}
		if _, ok := findEdge(g.Edges, "transfer", "recipient:"+foreignParty.ID.String(), "country:US"); ok {
			t.Error("the no-basis warning edge should be gone once a real transfer is logged")
		}
		edge, ok := findEdge(g.Edges, "transfer", "recipient:"+recipient.ID.String(), "country:US")
		if !ok || edge.Warning || edge.Label != "standard_clauses" {
			t.Errorf("expected a real, non-warning transfer edge labelled standard_clauses, got %+v (ok=%v)", edge, ok)
		}
		return nil
	})
}

// TestGraph_LegalEntityFilter narrows the diagram to one legal entity's own activities.
func TestGraph_LegalEntityFilter(t *testing.T) {
	e := setup(t, "dfgfilter")
	var le1, le2 orgservice.LegalEntity
	var a1, a2 ropaservice.Activity

	e.in(t, func(ctx context.Context) error {
		var err error
		if le1, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท หนึ่ง จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		if le2, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท สอง จำกัด", IsController: true}, 0); err != nil {
			return err
		}
		u1, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le1.ID, Code: "U1", NameTh: "หน่วย 1", UnitType: "department"})
		if err != nil {
			return err
		}
		u2, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le2.ID, Code: "U2", NameTh: "หน่วย 2", UnitType: "department"})
		if err != nil {
			return err
		}
		if a1, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le1.ID, OrgUnitID: u1.ID, Code: "A1", Name: "A1", Role: "controller"}, 0); err != nil {
			return err
		}
		if a2, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le2.ID, OrgUnitID: u2.ID, Code: "A2", Name: "A2", Role: "controller"}, 0); err != nil {
			return err
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, &le1.ID)
		if err != nil {
			return err
		}
		if g.ActivityCount != 1 {
			t.Errorf("activity_count = %d, want 1", g.ActivityCount)
		}
		if _, ok := findNode(g.Nodes, "activity:"+a1.ID.String()); !ok {
			t.Error("missing a1 under its own legal entity filter")
		}
		if _, ok := findNode(g.Nodes, "activity:"+a2.ID.String()); ok {
			t.Error("a2 should be filtered out by legal_entity_id")
		}
		return nil
	})
}

// TestGraph_TwoTenantIsolation proves tenant B's graph never includes tenant A's org units or activities.
func TestGraph_TwoTenantIsolation(t *testing.T) {
	e1 := setup(t, "dfgisoA")
	e2 := setup(t, "dfgisoB")

	var a1 ropaservice.Activity
	e1.in(t, func(ctx context.Context) error {
		le, err := e1.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		u, err := e1.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "A", NameTh: "A", UnitType: "department"})
		if err != nil {
			return err
		}
		a1, err = e1.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: u.ID, Code: "A-01", Name: "A-01", Role: "controller"}, 0)
		return err
	})

	e2.in(t, func(ctx context.Context) error {
		le, err := e2.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท บี จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		u, err := e2.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "B", NameTh: "B", UnitType: "department"})
		if err != nil {
			return err
		}
		_, err = e2.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: u.ID, Code: "B-01", Name: "B-01", Role: "controller"}, 0)
		return err
	})

	e2.in(t, func(ctx context.Context) error {
		g, err := e2.svc.Graph(ctx, nil)
		if err != nil {
			return err
		}
		if g.ActivityCount != 1 {
			t.Errorf("tenant B activity_count = %d, want 1", g.ActivityCount)
		}
		if _, ok := findNode(g.Nodes, "activity:"+a1.ID.String()); ok {
			t.Error("tenant B's graph must never contain tenant A's activity")
		}
		return nil
	})
}
