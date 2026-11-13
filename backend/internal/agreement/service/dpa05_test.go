package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestCreateWizard_AutoAttachesCrossBorderClauseWhenTransferAlreadyOnRecord is DPA-05's own acceptance
// criterion directly: an agreement whose linked activity already shows a cross-border transfer gets the
// standard ม.28-29 clause attached automatically at creation, not merely flagged missing on submit.
func TestCreateWizard_AutoAttachesCrossBorderClauseWhenTransferAlreadyOnRecord(t *testing.T) {
	e := setup(t, "dpa05")
	var leID, vendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)
		_, err := e.ropa.AddActivityTransfer(ctx, ropaservice.ActivityTransfer{
			ActivityID: activityID, CountryCode: "SG", TransferBasis: "adequacy",
		})
		return err
	})

	a := createAgreement(t, e, leID, vendorID, []uuid.UUID{activityID})

	e.in(t, func(ctx context.Context) error {
		clauses, err := e.svc.ListClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		found := false
		for _, c := range clauses {
			if c.ClauseCode == "dpa.cross_border_transfer" {
				found = true
			}
		}
		if !found {
			t.Fatal("expected dpa.cross_border_transfer to be attached automatically")
		}
		missing, err := e.svc.MissingMandatoryClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		for _, m := range missing {
			if m.Code == "dpa.cross_border_transfer" {
				t.Fatal("dpa.cross_border_transfer should already be satisfied, not still missing")
			}
		}
		return nil
	})
}

// TestCreateWizard_NoTransferNoAutoAttach: an agreement with no cross-border transfer on record at
// creation time gets nothing auto-attached — the existing TestMissingMandatoryClauses_CrossBorderConditional
// already proves the manual path still works once a transfer is added afterward.
func TestCreateWizard_NoTransferNoAutoAttach(t *testing.T) {
	e := setup(t, "dpa05none")
	var leID, vendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)
		return nil
	})

	a := createAgreement(t, e, leID, vendorID, []uuid.UUID{activityID})

	e.in(t, func(ctx context.Context) error {
		clauses, err := e.svc.ListClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(clauses) != 0 {
			t.Fatalf("expected no clauses auto-attached with no transfer on record, got %+v", clauses)
		}
		return nil
	})
}
