package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestGraph_ControllerWithoutDSAWarning is DSA-12's own acceptance criterion
// ("เส้นทางส่งข้อมูลให้ผู้ควบคุมรายอื่นที่ไม่มี DSA ถูกเตือน"): a recipient whose recipient_role is
// "controller" or "joint_controller" — another controller this activity shares data with — is flagged on
// its node until an agreement.agreements row of type "dsa" names it as the counterparty; drafting the DSA
// clears the flag and the node carries the agreement id to link through to. Mirrors
// TestGraph_ProcessorWithoutDPAWarning (DFG-04), the processor/DPA twin of this same mechanism.
func TestGraph_ControllerWithoutDSAWarning(t *testing.T) {
	e := setup(t, "dsa12")
	var a ropaservice.Activity
	var controllerParty, jointParty orgservice.ExternalParty

	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "MKT", NameTh: "Marketing", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "MKT-01", Name: "แคมเปญร่วม", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		controllerParty, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "พันธมิตรทางธุรกิจ", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		jointParty, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "ผู้ควบคุมร่วม", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: a.ID, PartyID: controllerParty.ID, RecipientRole: "controller"}); err != nil {
			return err
		}
		_, err = e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: a.ID, PartyID: jointParty.ID, RecipientRole: "joint_controller"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil)
		if err != nil {
			return err
		}
		for _, p := range []orgservice.ExternalParty{controllerParty, jointParty} {
			n, ok := findNode(g.Nodes, "recipient:"+p.ID.String())
			if !ok || !n.Warning || len(n.AgreementIDs) != 0 {
				t.Errorf("before a DSA exists: party=%s node=%+v ok=%v", p.NameTh, n, ok)
			}
		}
		return nil
	})

	// Draft a DSA for the plain-controller recipient only — its own warning must clear while the
	// joint_controller recipient (no DSA of its own) stays flagged.
	var agreementID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		ag, err := e.agreement.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dsa", OurRole: "controller", CounterpartyPartyID: controllerParty.ID, CounterpartyRole: "receiving",
			LegalEntityID: a.LegalEntityID, ActivityIDs: []uuid.UUID{a.ID}, Title: "DSA กับพันธมิตรทางธุรกิจ",
		})
		if err != nil {
			return err
		}
		agreementID = ag.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil)
		if err != nil {
			return err
		}
		cn, ok := findNode(g.Nodes, "recipient:"+controllerParty.ID.String())
		if !ok || cn.Warning || len(cn.AgreementIDs) != 1 || cn.AgreementIDs[0] != agreementID {
			t.Errorf("after a DSA exists: node=%+v ok=%v, want AgreementIDs=[%s]", cn, ok, agreementID)
		}
		jn, ok := findNode(g.Nodes, "recipient:"+jointParty.ID.String())
		if !ok || !jn.Warning || len(jn.AgreementIDs) != 0 {
			t.Errorf("joint_controller recipient with no DSA of its own must stay flagged: node=%+v ok=%v", jn, ok)
		}
		return nil
	})
}
