package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestRunExemptionCheck_MatchesByAssetAndByDataCategory is DSAR-10's own acceptance criterion directly
// (ม.33): an asset named by a hold, and a different asset whose own ROPA-01 data inventory carries a
// category named by a (no-asset) hold, are both found held with the right law cited; an unrelated asset is
// not, and the aggregate result reflects whichever mix of assets was actually checked.
func TestRunExemptionCheck_MatchesByAssetAndByDataCategory(t *testing.T) {
	e := setup(t, "dsarexempt")
	var leID, erasureTypeID uuid.UUID
	var assetHeld, assetCategoryHeld, assetFree uuid.UUID
	var holdA, holdB dsarservice.LegalHold
	e.in(t, func(ctx context.Context) error {
		leID, _ = seedLegalEntityAndType(t, ctx, e)
		erasureTypeID = typeByCode(t, ctx, e, "erasure")
		assetHeld = seedAsset(t, ctx, e, "ระบบ A (hold ตรง)")
		assetCategoryHeld = seedAsset(t, ctx, e, "ระบบ B (hold ตามหมวดข้อมูล)")
		assetFree = seedAsset(t, ctx, e, "ระบบ C (ไม่มี hold)")

		var err error
		holdA, err = e.svc.CreateLegalHold(ctx, dsarservice.LegalHold{AssetID: assetHeld, LawRef: "กฎหมาย A มาตรา 1"})
		if err != nil {
			return err
		}

		cats, err := e.org.ListMaster(ctx, "data_categories")
		if err != nil || len(cats) == 0 {
			t.Fatal(err, "expected seeded data categories (ORG-07)")
		}
		cat := cats[0]
		if _, err := e.ropa.SaveDataInventoryItem(ctx, ropaservice.DataInventoryItem{AssetID: assetCategoryHeld, DataCategoryID: *cat.ID}, 0); err != nil {
			return err
		}
		holdB, err = e.svc.CreateLegalHold(ctx, dsarservice.LegalHold{DataCategoryID: cat.ID, LawRef: "กฎหมาย B มาตรา 2"})
		return err
	})

	// Both held assets together with the free one: partial.
	e.in(t, func(ctx context.Context) error {
		c, err := e.svc.RunExemptionCheck(ctx, erasureRequestID(t, ctx, e, erasureTypeID, leID), []uuid.UUID{assetHeld, assetCategoryHeld, assetFree})
		if err != nil {
			return err
		}
		if c.Result != "partial" {
			t.Errorf("result = %q, want partial", c.Result)
		}
		if !strings.Contains(c.Reason, holdA.LawRef) || !strings.Contains(c.Reason, holdB.LawRef) {
			t.Errorf("reason = %q, want both law refs cited", c.Reason)
		}
		if c.LegalHoldID == nil {
			t.Error("expected a legal_hold_id reference on a held result")
		}
		return nil
	})

	// Only the directly-held asset: must_retain.
	e.in(t, func(ctx context.Context) error {
		c, err := e.svc.RunExemptionCheck(ctx, erasureRequestID(t, ctx, e, erasureTypeID, leID), []uuid.UUID{assetHeld})
		if err != nil {
			return err
		}
		if c.Result != "must_retain" {
			t.Errorf("result = %q, want must_retain", c.Result)
		}
		return nil
	})

	// Only the free asset: can_delete.
	e.in(t, func(ctx context.Context) error {
		c, err := e.svc.RunExemptionCheck(ctx, erasureRequestID(t, ctx, e, erasureTypeID, leID), []uuid.UUID{assetFree})
		if err != nil {
			return err
		}
		if c.Result != "can_delete" {
			t.Errorf("result = %q, want can_delete", c.Result)
		}
		return nil
	})
}

// erasureRequestID creates a fresh erasure request for a single RunExemptionCheck call — each test case
// gets its own request rather than reusing one, since exemption checks accumulate on a request.
func erasureRequestID(t *testing.T, ctx context.Context, e env, typeID, leID uuid.UUID) uuid.UUID {
	t.Helper()
	r := createAndAdvanceToInReview(t, ctx, e, typeID, leID, nil)
	return r.ID
}

// TestRunExemptionCheck_Validation covers the input-level rules: an unknown asset id, and an empty batch.
func TestRunExemptionCheck_Validation(t *testing.T) {
	e := setup(t, "dsarexemptval")
	var leID, erasureTypeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _ = seedLegalEntityAndType(t, ctx, e)
		erasureTypeID = typeByCode(t, ctx, e, "erasure")
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		reqID := erasureRequestID(t, ctx, e, erasureTypeID, leID)
		if _, err := e.svc.RunExemptionCheck(ctx, reqID, nil); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("empty asset_ids: err = %v, want ErrInvalid", err)
		}
		if _, err := e.svc.RunExemptionCheck(ctx, reqID, []uuid.UUID{uuid.New()}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("unknown asset id: err = %v, want ErrInvalid", err)
		}
		if _, err := e.svc.RunExemptionCheck(ctx, uuid.New(), []uuid.UUID{seedAsset(t, ctx, e, "ระบบ")}); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("unknown request id: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestCreateLegalHold_Validation: law_ref is required, and at least one of data_category_id/asset_id.
func TestCreateLegalHold_Validation(t *testing.T) {
	e := setup(t, "dsarholdval")
	e.in(t, func(ctx context.Context) error {
		asset := seedAsset(t, ctx, e, "ระบบ")
		if _, err := e.svc.CreateLegalHold(ctx, dsarservice.LegalHold{AssetID: asset, LawRef: "  "}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("empty law_ref: err = %v, want ErrInvalid", err)
		}
		if _, err := e.svc.CreateLegalHold(ctx, dsarservice.LegalHold{LawRef: "มาตรา 1"}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("no scope: err = %v, want ErrInvalid", err)
		}
		if _, err := e.svc.CreateLegalHold(ctx, dsarservice.LegalHold{AssetID: asset, LawRef: "มาตรา 1"}); err != nil {
			t.Errorf("valid hold: err = %v", err)
		}
		return nil
	})
}

// TestDeactivateLegalHold_ClearsTheMatch proves the acceptance criterion's other direction: once a hold is
// deactivated, the asset it named is no longer held, and a stale row_version is refused distinctly.
func TestDeactivateLegalHold_ClearsTheMatch(t *testing.T) {
	e := setup(t, "dsarholddeactivate")
	var leID, erasureTypeID, asset uuid.UUID
	var hold dsarservice.LegalHold
	e.in(t, func(ctx context.Context) error {
		leID, _ = seedLegalEntityAndType(t, ctx, e)
		erasureTypeID = typeByCode(t, ctx, e, "erasure")
		asset = seedAsset(t, ctx, e, "ระบบ")
		var err error
		hold, err = e.svc.CreateLegalHold(ctx, dsarservice.LegalHold{AssetID: asset, LawRef: "มาตรา 1"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		c, err := e.svc.RunExemptionCheck(ctx, erasureRequestID(t, ctx, e, erasureTypeID, leID), []uuid.UUID{asset})
		if err != nil {
			return err
		}
		if c.Result != "must_retain" {
			t.Fatalf("before deactivation: result = %q, want must_retain", c.Result)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.DeactivateLegalHold(ctx, hold.ID, hold.RowVersion+99); !errors.Is(err, dsarservice.ErrVersionMismatch) {
			t.Errorf("stale row_version: err = %v, want ErrVersionMismatch", err)
		}
		updated, err := e.svc.DeactivateLegalHold(ctx, hold.ID, hold.RowVersion)
		if err != nil {
			return err
		}
		if updated.IsActive {
			t.Error("expected is_active=false after deactivation")
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		c, err := e.svc.RunExemptionCheck(ctx, erasureRequestID(t, ctx, e, erasureTypeID, leID), []uuid.UUID{asset})
		if err != nil {
			return err
		}
		if c.Result != "can_delete" {
			t.Errorf("after deactivation: result = %q, want can_delete", c.Result)
		}
		return nil
	})
}

// TestTransition_Completed_RequiresExemptionCheckForErasure is DSAR-10's own gate: an erasure request
// cannot close without at least one recorded exemption check, but a result alone (must_retain) never blocks
// closing — only the check's existence does; every other right type is unaffected.
func TestTransition_Completed_RequiresExemptionCheckForErasure(t *testing.T) {
	e := setup(t, "dsarexemptgate")
	var leID, erasureTypeID, accessTypeID, asset uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _ = seedLegalEntityAndType(t, ctx, e)
		erasureTypeID = typeByCode(t, ctx, e, "erasure")
		accessTypeID = typeByCode(t, ctx, e, "access")
		asset = seedAsset(t, ctx, e, "ระบบ")
		return nil
	})

	var erasureReq dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		erasureReq = createAndAdvanceToInReview(t, ctx, e, erasureTypeID, leID, nil)
		var err error
		erasureReq, _, err = e.svc.Transition(ctx, erasureReq.ID, erasureReq.RowVersion, dsarservice.TransitionInput{To: "in_progress", AssetIDs: []uuid.UUID{asset}})
		return err
	})
	// Finish the one auto-opened subtask so only the exemption-check gate is left to prove.
	e.in(t, func(ctx context.Context) error {
		subs, err := e.svc.ListSubtasks(ctx, erasureReq.ID)
		if err != nil {
			return err
		}
		if len(subs) != 1 {
			t.Fatalf("subtasks = %d, want 1", len(subs))
		}
		_, err = e.svc.UpdateSubtaskStatus(ctx, erasureReq.ID, subs[0].ID, subs[0].RowVersion, "done", nil)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		outcome := "fulfilled"
		if _, _, err := e.svc.Transition(ctx, erasureReq.ID, erasureReq.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &outcome}); !errors.Is(err, dsarservice.ErrExemptionCheckRequired) {
			t.Errorf("completing erasure with no exemption check: err = %v, want ErrExemptionCheckRequired", err)
		}
		return nil
	})
	// Running the check — even one that finds nothing held — clears the gate.
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.RunExemptionCheck(ctx, erasureReq.ID, []uuid.UUID{asset})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		outcome := "fulfilled"
		r, _, err := e.svc.Transition(ctx, erasureReq.ID, erasureReq.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &outcome})
		if err != nil {
			return err
		}
		if r.Status != "completed" {
			t.Errorf("status = %q, want completed", r.Status)
		}
		return nil
	})

	// An access request (not erasure) completes with no exemption check at all — the gate is erasure-only.
	var accessReq dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		accessReq = createAndAdvanceToInReview(t, ctx, e, accessTypeID, leID, nil)
		var err error
		accessReq, _, err = e.svc.Transition(ctx, accessReq.ID, accessReq.RowVersion, dsarservice.TransitionInput{To: "in_progress", AssetIDs: []uuid.UUID{asset}})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		subs, err := e.svc.ListSubtasks(ctx, accessReq.ID)
		if err != nil {
			return err
		}
		_, err = e.svc.UpdateSubtaskStatus(ctx, accessReq.ID, subs[0].ID, subs[0].RowVersion, "done", nil)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		outcome := "fulfilled"
		r, _, err := e.svc.Transition(ctx, accessReq.ID, accessReq.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &outcome})
		if err != nil {
			return err
		}
		if r.Status != "completed" {
			t.Errorf("access request status = %q, want completed", r.Status)
		}
		return nil
	})
}

// TestListLegalHolds_TwoTenantIsolation proves one tenant's own legal hold never leaks into another's list,
// and an exemption check against another tenant's asset id is refused the same way DSAR-03's own FK checks
// already prove tenant isolation.
func TestListLegalHolds_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dsarholdisoA")
	b := setup(t, "dsarholdisoB")

	var holdID uuid.UUID
	var assetA uuid.UUID
	a.in(t, func(ctx context.Context) error {
		assetA = seedAsset(t, ctx, a, "ระบบ")
		h, err := a.svc.CreateLegalHold(ctx, dsarservice.LegalHold{AssetID: assetA, LawRef: "มาตรา 1"})
		if err != nil {
			return err
		}
		holdID = h.ID
		return nil
	})

	b.in(t, func(ctx context.Context) error {
		holds, err := b.svc.ListLegalHolds(ctx)
		if err != nil {
			return err
		}
		for _, h := range holds {
			if h.ID == holdID {
				t.Fatal("tenant B's own legal-hold list leaked tenant A's own hold")
			}
		}
		leID, erasureTypeID := seedLegalEntityAndType(t, ctx, b)
		erasureTypeID = typeByCode(t, ctx, b, "erasure")
		reqID := erasureRequestID(t, ctx, b, erasureTypeID, leID)
		if _, err := b.svc.RunExemptionCheck(ctx, reqID, []uuid.UUID{assetA}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("tenant B checking tenant A's asset id: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}
