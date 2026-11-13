package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
)

// submitAgreement attaches every mandatory clause (so the submit-time gate passes) and submits the
// agreement's document for review — the author's own step.
func submitAgreement(t *testing.T, e env, a agreementservice.Agreement) {
	t.Helper()
	e.in(t, func(ctx context.Context) error {
		for _, code := range requiredDpaClauseCodes {
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

// approveAgreement decides the agreement's own open approval as the DPO user — a real second user, since
// PLT-08's maker-checker refuses the same user as both author and approver.
func approveAgreement(t *testing.T, e env, a agreementservice.Agreement) {
	t.Helper()
	dpoPerms := []string{"agreement.dpa.read", "agreement.dpa.update", "agreement.dpa.approve"}
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

// TestReviewLifecycle_MirrorsAgreementStatus is DPA-07's own acceptance criterion: the agreement's own
// ST-04 status (agreement.agreements.status) follows the underlying PLT-08 review lifecycle — draft while
// unsubmitted, in_review once submitted, approved once the DPO decides — and SendForSignature, the one
// action this feature gates, succeeds only from approved.
func TestReviewLifecycle_MirrorsAgreementStatus(t *testing.T) {
	e := setup(t, "dpa07lifecycle")
	var leID, vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, nil)
	if a.Status != "draft" {
		t.Fatalf("fresh agreement status = %q, want draft", a.Status)
	}

	submitAgreement(t, e, a)
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

	approveAgreement(t, e, a)
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

	// A stale row_version is refused distinctly from "not approved yet".
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.SendForSignature(ctx, a.ID, approved.RowVersion+1); !errors.Is(err, agreementservice.ErrVersionMismatch) {
			t.Fatalf("stale row_version: err = %v, want ErrVersionMismatch", err)
		}
		return nil
	})

	// Now the acceptance criterion's own success path: approved -> out_for_signature.
	e.in(t, func(ctx context.Context) error {
		out, err := e.svc.SendForSignature(ctx, a.ID, approved.RowVersion)
		if err != nil {
			return err
		}
		if out.Status != "out_for_signature" {
			t.Fatalf("after send-for-signature, status = %q, want out_for_signature", out.Status)
		}
		// And it can't be sent twice from the same state.
		if _, err := e.svc.SendForSignature(ctx, a.ID, out.RowVersion); !errors.Is(err, agreementservice.ErrInvalidTransition) {
			t.Fatalf("second send-for-signature: err = %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

// TestReviewLifecycle_ReturnGoesBackToDraft proves the other half of the mirrored lifecycle: a returned
// (not approved) version sends the agreement itself back to draft, not stuck at in_review.
func TestReviewLifecycle_ReturnGoesBackToDraft(t *testing.T) {
	e := setup(t, "dpa07return")
	var leID, vendorID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, vendorID, _ = fixture(t, ctx, e)
		return nil
	})
	a := createAgreement(t, e, leID, vendorID, nil)
	submitAgreement(t, e, a)

	dpoPerms := []string{"agreement.dpa.read", "agreement.dpa.update", "agreement.dpa.approve"}
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
				_, err = e.ver.Decide(ctx, it.ID, it.RowVersion, "returned", "ขอให้แก้ไขรายละเอียด")
				return err
			}
		}
		t.Fatal("submitted version not in the DPO's inbox")
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetAgreement(ctx, a.ID)
		if err != nil {
			return err
		}
		if got.Status != "draft" {
			t.Fatalf("after return, status = %q, want draft", got.Status)
		}
		return nil
	})
}
