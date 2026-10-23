package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

// TestGraph_ProcessorWithoutDPAWarning is DFG-04's own acceptance criterion
// ("ผู้ประมวลผลที่ไม่มี DPA แสดงเตือนบนแผนผัง"): a processor recipient tracked as a VEN-01 vendor with no
// agreement.agreements row of type "dpa" is flagged on its node; creating the DPA clears the flag and the
// node carries the agreement id to link through to. A processor recipient never onboarded as a vendor at
// all is left unflagged — there's nothing to check or click through to.
func TestGraph_ProcessorWithoutDPAWarning(t *testing.T) {
	e := setup(t, "dfgdpa")
	var a ropaservice.Activity
	var vendorParty, untrackedParty orgservice.ExternalParty
	var vendor vendorservice.Vendor

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
		vendorParty, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการคลาวด์", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		vendor, err = e.vendor.SaveVendor(ctx, vendorservice.Vendor{PartyID: vendorParty.ID, ServiceDescription: "ประมวลผลข้อมูลบนคลาวด์", IsProcessor: true}, 0)
		if err != nil {
			return err
		}
		untrackedParty, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการที่ยังไม่ขึ้นทะเบียน", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: a.ID, PartyID: vendorParty.ID, RecipientRole: "processor"}); err != nil {
			return err
		}
		_, err = e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: a.ID, PartyID: untrackedParty.ID, RecipientRole: "processor"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		g, err := e.svc.Graph(ctx, nil)
		if err != nil {
			return err
		}
		vn, ok := findNode(g.Nodes, "recipient:"+vendorParty.ID.String())
		if !ok || !vn.Warning || vn.VendorID == nil || *vn.VendorID != vendor.ID || len(vn.AgreementIDs) != 0 {
			t.Errorf("before a DPA exists: node=%+v ok=%v", vn, ok)
		}
		un, ok := findNode(g.Nodes, "recipient:"+untrackedParty.ID.String())
		if !ok || un.Warning || un.VendorID != nil {
			t.Errorf("a processor never onboarded as a vendor should never be flagged: node=%+v ok=%v", un, ok)
		}
		return nil
	})

	// Draft a DPA for the tracked vendor — the warning must clear and the agreement id must appear.
	var agreementID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		ag, err := e.agreement.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", VendorID: vendor.ID,
			LegalEntityID: a.LegalEntityID, ActivityIDs: []uuid.UUID{a.ID}, Title: "DPA กับผู้ให้บริการคลาวด์",
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
		vn, ok := findNode(g.Nodes, "recipient:"+vendorParty.ID.String())
		if !ok || vn.Warning || len(vn.AgreementIDs) != 1 || vn.AgreementIDs[0] != agreementID {
			t.Errorf("after a DPA exists: node=%+v ok=%v, want AgreementIDs=[%s]", vn, ok, agreementID)
		}
		return nil
	})
}
