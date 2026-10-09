package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
)

// requiredDsaClauseCodes are the eight ม.27/28-29/37(2)/37(4) clauses migration 00061 seeds for "dsa" —
// every one but cross_border_transfer always applies; that one applies only to an agreement that actually
// has a cross-border transfer on record (ROPA-08), the same shape requiredDpaClauseCodes already uses.
var requiredDsaClauseCodes = []string{
	"dsa.purpose_limitation", "dsa.no_excess_use", "dsa.security_measures", "dsa.breach_notification",
	"dsa.dsar_support", "dsa.retention_and_destruction", "dsa.onward_disclosure", "dsa.termination",
}

func createDsaAgreement(t *testing.T, e env, leID, counterpartyID uuid.UUID, activityIDs []uuid.UUID) agreementservice.Agreement {
	t.Helper()
	var a agreementservice.Agreement
	e.in(t, func(ctx context.Context) error {
		var err error
		a, err = e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dsa", OurRole: "controller", CounterpartyPartyID: counterpartyID, CounterpartyRole: "receiving",
			LegalEntityID: leID, ActivityIDs: activityIDs, Title: "DSA กับหน่วยงานพันธมิตร",
		})
		return err
	})
	return a
}

// TestMissingMandatoryClauses_DsaBlocksUntilAttached is DSA-05's own acceptance criterion directly: a
// fresh "dsa" agreement (no cross-border transfer) is missing every one of the eight unconditional
// ม.27/37(2)/37(4) clauses migration 00061 seeds, attaching all eight clears the list and lets the
// document submit for approval — proving the agreement_type-agnostic gate DPA-03 already built (and
// cmd/api/main.go's new docsSvc.SetSubmitValidate("dsa", ...) line) works unchanged for "dsa".
func TestMissingMandatoryClauses_DsaBlocksUntilAttached(t *testing.T) {
	e := setup(t, "dsaClauses")
	var leID, activityID, counterpartyID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _, activityID = fixture(t, ctx, e)
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "หน่วยงานพันธมิตร", CountryCode: "TH"}, 0)
		counterpartyID = party.ID
		return err
	})
	a := createDsaAgreement(t, e, leID, counterpartyID, []uuid.UUID{activityID})

	e.in(t, func(ctx context.Context) error {
		missing, err := e.svc.MissingMandatoryClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(missing) != len(requiredDsaClauseCodes) {
			t.Fatalf("missing = %d, want %d (no transfer on record, so cross_border_transfer doesn't apply)", len(missing), len(requiredDsaClauseCodes))
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
		for _, code := range requiredDsaClauseCodes {
			if _, err := e.svc.AddClause(ctx, a.ID, agreementservice.AddClauseInput{ClauseID: clauseID(t, ctx, e, code)}); err != nil {
				t.Fatalf("attach %s: %v", code, err)
			}
		}
		missing, err := e.svc.MissingMandatoryClauses(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(missing) != 0 {
			t.Fatalf("missing after attaching all = %d, want 0: %+v", len(missing), missing)
		}
		if err := e.svc.CheckSubmittable(ctx, a.DocumentID); err != nil {
			t.Fatalf("CheckSubmittable after attaching all: %v, want nil", err)
		}
		return nil
	})
}
