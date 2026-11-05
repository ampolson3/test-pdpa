package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
)

// submitDsaAgreement is submitAgreement's own "dsa" twin: attaches every mandatory DSA clause (DSA-05)
// so the submit-time gate passes, then submits the agreement's document for review.
func submitDsaAgreement(t *testing.T, e env, a agreementservice.Agreement) {
	t.Helper()
	e.in(t, func(ctx context.Context) error {
		for _, code := range requiredDsaClauseCodes {
			if _, err := e.svc.AddClause(ctx, a.ID, agreementservice.AddClauseInput{ClauseID: clauseID(t, ctx, e, code)}); err != nil {
				return err
			}
		}
		doc, err := e.svc.Docs.Get(ctx, a.DocumentID)
		if err != nil {
			return err
		}
		_, err = e.ver.Submit(ctx, doc.Latest.ID, doc.Latest.RowVersion)
		return err
	})
}

// approveDsaAgreement is approveAgreement's own "dsa" twin: decides the agreement's own open approval as
// the DPO user, granted the "dsa" permission codes (distinct from "dpa"'s own) a real DSA reviewer holds.
func approveDsaAgreement(t *testing.T, e env, a agreementservice.Agreement) {
	t.Helper()
	dpoPerms := []string{"agreement.dsa.read", "agreement.dsa.update", "agreement.dsa.approve"}
	if err := e.inAs(e.dpo, dpoPerms, []string{"DPO"}, func(ctx context.Context) error {
		doc, err := e.svc.Docs.Get(ctx, a.DocumentID)
		if err != nil {
			return err
		}
		inbox, err := e.ver.Inbox(ctx)
		if err != nil {
			return err
		}
		for _, it := range inbox {
			if it.VersionID == doc.Latest.ID {
				_, err = e.ver.Decide(ctx, it.ID, it.RowVersion, "approved", "")
				return err
			}
		}
		t.Fatal("submitted version not in the DPO's inbox")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestReviewLifecycle_WorksForDsaAgreementsToo is DSA-08's own acceptance criterion directly: the
// ST-04 status mirroring and the SendForSignature gate DPA-07 built never branch on agreement_type — the
// same OnDocumentTransition hook is wired for "dsa" exactly as it is for "dpa" (cmd/api/main.go's own
// SetOnTransition calls for both types) — so a "dsa" agreement walks draft -> in_review -> approved ->
// out_for_signature identically, with send-for-signature refused both before approval and a second time
// after it, with no new service code needed.
func TestReviewLifecycle_WorksForDsaAgreementsToo(t *testing.T) {
	e := setup(t, "dsa08lifecycle")
	var leID, activityID, counterpartyID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _, activityID = fixture(t, ctx, e)
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "หน่วยงานพันธมิตร DSA-08", CountryCode: "TH"}, 0)
		counterpartyID = party.ID
		return err
	})
	a := createDsaAgreement(t, e, leID, counterpartyID, []uuid.UUID{activityID})
	if a.Status != "draft" {
		t.Fatalf("fresh dsa agreement status = %q, want draft", a.Status)
	}

	submitDsaAgreement(t, e, a)
	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetAgreement(ctx, a.ID)
		if err != nil {
			return err
		}
		if got.Status != "in_review" {
			t.Fatalf("after submit, status = %q, want in_review", got.Status)
		}
		return nil
	})

	// Refused before approved — the acceptance criterion's own gate.
	e.in(t, func(ctx context.Context) error {
		cur, err := e.svc.GetAgreement(ctx, a.ID)
		if err != nil {
			return err
		}
		if _, err := e.svc.SendForSignature(ctx, a.ID, cur.RowVersion); !errors.Is(err, agreementservice.ErrInvalidTransition) {
			t.Fatalf("send-for-signature while in_review: err = %v, want ErrInvalidTransition", err)
		}
		return nil
	})

	approveDsaAgreement(t, e, a)
	var approved agreementservice.Agreement
	e.in(t, func(ctx context.Context) error {
		var err error
		approved, err = e.svc.GetAgreement(ctx, a.ID)
		if err != nil {
			return err
		}
		if approved.Status != "approved" {
			t.Fatalf("after DPO approval, status = %q, want approved", approved.Status)
		}
		return nil
	})

	// The acceptance criterion's own success path: approved -> out_for_signature, refused a second time.
	e.in(t, func(ctx context.Context) error {
		out, err := e.svc.SendForSignature(ctx, a.ID, approved.RowVersion)
		if err != nil {
			return err
		}
		if out.Status != "out_for_signature" {
			t.Fatalf("after send-for-signature, status = %q, want out_for_signature", out.Status)
		}
		if _, err := e.svc.SendForSignature(ctx, a.ID, out.RowVersion); !errors.Is(err, agreementservice.ErrInvalidTransition) {
			t.Fatalf("second send-for-signature: err = %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}
