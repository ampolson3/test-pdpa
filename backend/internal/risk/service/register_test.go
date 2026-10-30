package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// fakeVendor is RRA-09's own test double for the Vendor interface (rule 9 — no real vendormgmt.Service
// import needed to prove the FK-visibility check).
type fakeVendor struct{ known uuid.UUID }

func (f fakeVendor) VendorVisible(_ context.Context, id uuid.UUID) (bool, error) {
	return id == f.known, nil
}

// TestListRisks_AllSourceTypesInOneRegister is RRA-09's own acceptance criterion directly: a dpia-sourced,
// an activity-sourced (RoPA) and a manual risk all land in the same ListRisks register, and the
// source_type filter narrows to exactly the one asked for.
func TestListRisks_AllSourceTypesInOneRegister(t *testing.T) {
	e := scoreSetup(t, "rra09all")
	var activityID uuid.UUID
	var dpiaID, activityRiskID, manualID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		activityID = seedActivity(t, ctx, e)
		dpiaID = mustIdentify(t, ctx, e.svc, riskservice.Risk{SourceType: "dpia", Title: "dpia risk", Likelihood: 2, Impact: 2}).ID
		activityRiskID = mustIdentify(t, ctx, e.svc, riskservice.Risk{SourceType: "activity", Title: "ropa risk", ActivityID: &activityID, Likelihood: 2, Impact: 2}).ID
		manualID = mustIdentify(t, ctx, e.svc, riskservice.Risk{SourceType: "manual", Title: "manual risk", Likelihood: 1, Impact: 1}).ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListRisks(ctx, riskservice.RiskFilter{})
		if err != nil {
			return err
		}
		seen := map[uuid.UUID]bool{}
		for _, r := range list {
			seen[r.ID] = true
		}
		for _, id := range []uuid.UUID{dpiaID, activityRiskID, manualID} {
			if !seen[id] {
				t.Errorf("register missing risk %s", id)
			}
		}

		activityOnly, _, err := e.svc.ListRisks(ctx, riskservice.RiskFilter{SourceType: "activity"})
		if err != nil {
			return err
		}
		if len(activityOnly) != 1 || activityOnly[0].ID != activityRiskID {
			t.Errorf("source_type=activity filter = %+v, want exactly the one RoPA-sourced risk", activityOnly)
		}
		return nil
	})
}

func mustIdentify(t *testing.T, ctx context.Context, svc *riskservice.Service, r riskservice.Risk) riskservice.Risk {
	t.Helper()
	out, err := svc.IdentifyRisk(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestIdentifyRisk_AssetAndVendorLinks is the module doc's own "ผูกกิจกรรม / ระบบ / คู่ค้า": a real
// ropa.assets row and a real (faked) vendor both link and round-trip; an unknown id of either is refused.
func TestIdentifyRisk_AssetAndVendorLinks(t *testing.T) {
	e := scoreSetup(t, "rra09link")
	vendorID := uuid.Must(uuid.NewV7())
	e.svc.Vendor = fakeVendor{known: vendorID}
	var assetID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		a, err := e.ropa.SaveAsset(ctx, ropaservice.Asset{Name: "ระบบทดสอบ", AssetType: "application"}, 0)
		if err != nil {
			return err
		}
		assetID = a.ID

		r, err := e.svc.IdentifyRisk(ctx, riskservice.Risk{SourceType: "manual", Title: "linked", AssetID: &assetID, VendorID: &vendorID, Likelihood: 1, Impact: 1})
		if err != nil {
			return err
		}
		if r.AssetID == nil || *r.AssetID != assetID {
			t.Errorf("AssetID = %v, want %s", r.AssetID, assetID)
		}
		if r.VendorID == nil || *r.VendorID != vendorID {
			t.Errorf("VendorID = %v, want %s", r.VendorID, vendorID)
		}

		unknownAsset := uuid.Must(uuid.NewV7())
		if _, err := e.svc.IdentifyRisk(ctx, riskservice.Risk{SourceType: "manual", Title: "x", AssetID: &unknownAsset, Likelihood: 1, Impact: 1}); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("unknown asset_id: err = %v, want ErrInvalid", err)
		}
		unknownVendor := uuid.Must(uuid.NewV7())
		if _, err := e.svc.IdentifyRisk(ctx, riskservice.Risk{SourceType: "manual", Title: "x", VendorID: &unknownVendor, Likelihood: 1, Impact: 1}); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("unknown vendor_id: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestIdentifyRisk_VendorRefusedWithoutAdapter proves a vendor_id is refused outright when no module has
// wired riskSvc.Vendor yet, rather than silently accepting an unverifiable link.
func TestIdentifyRisk_VendorRefusedWithoutAdapter(t *testing.T) {
	e := scoreSetup(t, "rra09novendor")
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		vendorID := uuid.Must(uuid.NewV7())
		if _, err := e.svc.IdentifyRisk(ctx, riskservice.Risk{SourceType: "manual", Title: "x", VendorID: &vendorID, Likelihood: 1, Impact: 1}); !errors.Is(err, riskservice.ErrInvalid) {
			t.Errorf("vendor_id with no Vendor adapter: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestUpdateRisk_NeverMovesLinks guards the real regression this feature's own UpdateRisk query comment
// warns about: editing a risk's title/status must never clear the activity/asset/vendor link it was
// identified with, since DPIA-06's own UpdateRisk caller never supplies those fields at all.
func TestUpdateRisk_NeverMovesLinks(t *testing.T) {
	e := scoreSetup(t, "rra09noclobber")
	var activityID, riskID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, e.svc)
		activityID = seedActivity(t, ctx, e)
		r := mustIdentify(t, ctx, e.svc, riskservice.Risk{SourceType: "activity", Title: "ropa risk", ActivityID: &activityID, Likelihood: 2, Impact: 2})
		riskID = r.ID

		updated, err := e.svc.UpdateRisk(ctx, riskID, riskservice.Risk{Title: "ropa risk (renamed)", Likelihood: 2, Impact: 2, Status: "in_treatment"}, r.RowVersion)
		if err != nil {
			return err
		}
		if updated.ActivityID == nil || *updated.ActivityID != activityID {
			t.Errorf("UpdateRisk cleared ActivityID: %v, want %s", updated.ActivityID, activityID)
		}
		if updated.Title != "ropa risk (renamed)" || updated.Status != "in_treatment" {
			t.Errorf("UpdateRisk did not apply the edit: %+v", updated)
		}
		return nil
	})
}

// TestListRisks_TwoTenantIsolation proves tenant B's own register never shows tenant A's risk, and a
// cross-tenant GetRisk is ErrNotFound.
func TestListRisks_TwoTenantIsolation(t *testing.T) {
	a := scoreSetup(t, "rra09isoA")
	b := scoreSetup(t, "rra09isoB")
	var riskID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, a.svc)
		riskID = mustIdentify(t, ctx, a.svc, riskservice.Risk{SourceType: "manual", Title: "tenant a's own risk", Likelihood: 1, Impact: 1}).ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		defaultMatrix3x3(t, ctx, b.svc)
		mustIdentify(t, ctx, b.svc, riskservice.Risk{SourceType: "manual", Title: "tenant b's own risk", Likelihood: 1, Impact: 1})

		list, _, err := b.svc.ListRisks(ctx, riskservice.RiskFilter{})
		if err != nil {
			return err
		}
		for _, r := range list {
			if r.ID == riskID {
				t.Errorf("tenant B's register leaked tenant A's risk %s", riskID)
			}
		}
		if _, err := b.svc.GetRisk(ctx, riskID); !errors.Is(err, riskservice.ErrNotFound) {
			t.Errorf("cross-tenant GetRisk: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}
