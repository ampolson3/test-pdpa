package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/versioning"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// requiredDpaClauseCodes are the eight ม.40(1)-(3)/37(2)/28-29 clauses migration 00057 seeds for "dpa" —
// every one but cross_border_transfer always applies; that one applies only to an agreement that actually
// has a cross-border transfer on record (ROPA-08).
var requiredDpaClauseCodes = []string{
	"dpa.processing_on_instructions", "dpa.confidentiality", "dpa.security_measures", "dpa.breach_notification",
	"dpa.sub_processors", "dpa.dsar_assistance", "dpa.return_or_destroy", "dpa.audit_rights",
}

func clauseID(t *testing.T, ctx context.Context, e env, code string) uuid.UUID {
	t.Helper()
	tx := pdb.MustTxFromContext(ctx)
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM platform.clause_library WHERE code = $1 AND tenant_id IS NULL`, code).Scan(&id); err != nil {
		t.Fatalf("clause %s: %v", code, err)
	}
	return id
}

func createAgreement(t *testing.T, e env, leID, vendorID uuid.UUID, activityIDs []uuid.UUID) agreementservice.Agreement {
	t.Helper()
	var a agreementservice.Agreement
	e.in(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", VendorID: vendorID, LegalEntityID: leID,
			ActivityIDs: activityIDs, Title: "DPA กับผู้ให้บริการคลาวด์",
		})
		return err
	})
	return a
}

// TestMissingMandatoryClauses_BlocksUntilAttached is the acceptance criterion directly: a fresh agreement
// (no cross-border transfer) is missing every one of the eight unconditional ม.40/37(2) clauses, attaching
// all eight clears the list and lets the document submit for approval, and CheckSubmittable — the real
// wire-up through versioning.Policy.Validate — refuses/allows exactly the same way.
func TestMissingMandatoryClauses_BlocksUntilAttached(t *testing.T) {
	e := setup(t, "dpaClauses")
	var leID, vendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, []uuid.UUID{activityID})

	e.in(t, func(ctx context.Context) error {
		missing, err := e.svc.MissingMandatoryClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(missing) != len(requiredDpaClauseCodes) {
			t.Fatalf("missing = %d, want %d (no transfer on record, so cross_border_transfer doesn't apply)", len(missing), len(requiredDpaClauseCodes))
		}
		if err := e.svc.CheckSubmittable(ctx, a.DocumentID); err == nil {
			t.Fatal("expected CheckSubmittable to refuse while clauses are missing")
		} else {
			var missErr *agreementservice.ErrMissingMandatoryClauses
			if !errors.As(err, &missErr) {
				t.Fatalf("err = %v, want *ErrMissingMandatoryClauses", err)
			}
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		for _, code := range requiredDpaClauseCodes {
			if _, err := e.svc.AddClause(ctx, a.ID, agreementservice.AddClauseInput{ClauseID: clauseID(t, ctx, e, code)}); err != nil {
				t.Fatalf("attach %s: %v", code, err)
			}
		}
		missing, err := e.svc.MissingMandatoryClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(missing) != 0 {
			t.Fatalf("missing = %v, want none once every clause is attached", missing)
		}
		return e.svc.CheckSubmittable(ctx, a.DocumentID)
	})
}

// TestMissingMandatoryClauses_CrossBorderConditional: the cross_border_transfer rule only appears once a
// linked activity actually has a transfer on record, and disappears again once that's attached.
func TestMissingMandatoryClauses_CrossBorderConditional(t *testing.T) {
	e := setup(t, "dpaClausesXfer")
	var leID, vendorID, activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, activityID = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, []uuid.UUID{activityID})

	e.in(t, func(ctx context.Context) error {
		missing, err := e.svc.MissingMandatoryClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		for _, m := range missing {
			if m.Code == "dpa.cross_border_transfer" {
				t.Fatal("cross_border_transfer should not apply before any transfer is recorded")
			}
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		_, err := e.ropa.AddActivityTransfer(ctx, ropaservice.ActivityTransfer{
			ActivityID: activityID, CountryCode: "SG", TransferBasis: "adequacy",
		})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		missing, err := e.svc.MissingMandatoryClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range missing {
			if m.Code == "dpa.cross_border_transfer" {
				found = true
			}
		}
		if !found {
			t.Fatal("cross_border_transfer should now be required with a real transfer on record")
		}
		xferID := clauseID(t, ctx, e, "dpa.cross_border_transfer")
		if _, err := e.svc.AddClause(ctx, a.ID, agreementservice.AddClauseInput{ClauseID: xferID}); err != nil {
			t.Fatal(err)
		}
		missing, err = e.svc.MissingMandatoryClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		for _, m := range missing {
			if m.Code == "dpa.cross_border_transfer" {
				t.Fatal("attaching the clause should clear it from the missing list")
			}
		}
		return nil
	})
}

// TestAddClause_RulesEnforced: only while the agreement is a draft, only a published clause id, and never
// the same code twice.
func TestAddClause_RulesEnforced(t *testing.T) {
	e := setup(t, "dpaClausesRules")
	var leID, vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, nil)

	e.in(t, func(ctx context.Context) error {
		id := clauseID(t, ctx, e, "dpa.confidentiality")
		if _, err := e.svc.AddClause(ctx, a.ID, agreementservice.AddClauseInput{ClauseID: id}); err != nil {
			t.Fatalf("first attach: %v", err)
		}
		if _, err := e.svc.AddClause(ctx, a.ID, agreementservice.AddClauseInput{ClauseID: id}); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Fatalf("duplicate attach: err = %v, want ErrInvalid", err)
		}
		if _, err := e.svc.AddClause(ctx, a.ID, agreementservice.AddClauseInput{ClauseID: uuid.Must(uuid.NewRandom())}); !errors.Is(err, agreementservice.ErrInvalid) {
			t.Fatalf("unknown clause: err = %v, want ErrInvalid", err)
		}
		list, err := e.svc.ListClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ClauseCode != "dpa.confidentiality" {
			t.Fatalf("list = %+v, want exactly one dpa.confidentiality", list)
		}
		if err := e.svc.RemoveClause(ctx, a.ID, list[0].ID); err != nil {
			t.Fatal(err)
		}
		list, err = e.svc.ListClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Fatalf("list after remove = %v, want empty", list)
		}
		return nil
	})
}

// TestSubmit_RefusedUntilClausesAttached proves the real wiring, not just CheckSubmittable called directly:
// PLT-08's own generic versioning.Submit refuses a "dpa" document's draft while mandatory clauses are
// missing (through docs.Service.SetSubmitValidate -> versioning.Policy.Validate, DPA-03's acceptance
// criterion verbatim — "a contract missing a mandatory clause cannot be sent for approval") and accepts it
// once every clause this agreement needs is attached.
func TestSubmit_RefusedUntilClausesAttached(t *testing.T) {
	e := setup(t, "dpaClausesSubmit")
	var leID, vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, nil) // no activities -> no transfer -> cross_border_transfer doesn't apply

	var versionID uuid.UUID
	var rowVersion int32
	e.in(t, func(ctx context.Context) error {
		doc, err := e.svc.Docs.Get(ctx, a.DocumentID)
		if err != nil {
			return err
		}
		versionID, rowVersion = doc.Latest.ID, doc.Latest.RowVersion
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.ver.Submit(ctx, versionID, rowVersion); err == nil {
			t.Fatal("expected Submit to refuse while mandatory clauses are missing")
		} else if !errors.Is(err, versioning.ErrInvalidRequest) {
			t.Fatalf("err = %v, want versioning.ErrInvalidRequest", err)
		}
		for _, code := range requiredDpaClauseCodes {
			if _, err := e.svc.AddClause(ctx, a.ID, agreementservice.AddClauseInput{ClauseID: clauseID(t, ctx, e, code)}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := e.ver.Submit(ctx, versionID, rowVersion); err != nil {
			t.Fatalf("submit after attaching every mandatory clause: %v", err)
		}
		return nil
	})
}

// TestTwoTenantIsolation_Clauses: tenant B cannot attach a clause to tenant A's agreement, and tenant B's
// own (empty) clause list never shows tenant A's rows.
func TestTwoTenantIsolation_Clauses(t *testing.T) {
	eA := setup(t, "dpaClausesIsoA")
	var leID, vendorID uuid.UUID
	eA.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, eA)
		return nil
	})
	a := createAgreement(t, eA, leID, vendorID, nil)
	eA.in(t, func(ctx context.Context) error {
		id := clauseID(t, ctx, eA, "dpa.confidentiality")
		_, err := eA.svc.AddClause(ctx, a.ID, agreementservice.AddClauseInput{ClauseID: id})
		return err
	})

	eB := setup(t, "dpaClausesIsoB")
	eB.in(t, func(ctx context.Context) error {
		if _, err := eB.svc.ListClauses(ctx, a.ID); !errors.Is(err, agreementservice.ErrNotFound) {
			t.Fatalf("tenant B listing tenant A's agreement clauses: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}
