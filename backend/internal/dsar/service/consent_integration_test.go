package service_test

import (
	"context"
	"testing"

	consentservice "pdpa-platform/internal/consent/service"
	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/platform/crypto"

	"github.com/google/uuid"
)

func requestTypeIDByCode(t *testing.T, e env, code string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	e.in(t, func(ctx context.Context) error {
		types, err := e.svc.ListRequestTypes(ctx)
		if err != nil {
			return err
		}
		for _, rt := range types {
			if rt.Code == code {
				id = rt.ID
				return nil
			}
		}
		t.Fatalf("no request type with code %q", code)
		return nil
	})
	return id
}

// TestWithdrawConsentRequest_UpdatesConsentAutomatically is DSAR-21's own acceptance criterion: fulfilling a
// "withdraw_consent" (ม.19) request withdraws the requester's own active purposes in CON, with no separate
// staff action, and the receipt it produced is attached to the request.
func TestWithdrawConsentRequest_UpdatesConsentAutomatically(t *testing.T) {
	e, _, fc := exportEnv(t, "dsarconsentwd")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ถอนความยินยอม จำกัด")
	rt := requestTypeIDByCode(t, e, "withdraw_consent")

	const contact = "withdraw-me@example.com"
	var req dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		req, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{
			RequestTypeID: rt, LegalEntityID: legalEntity, Channel: "web",
			RequesterName: "ถอน ยินยอม", RequesterContact: contact, ContactKind: crypto.KindEmail,
		})
		return err
	})
	subjectID := fc.addSubject(contact, []consentservice.PurposeStatus{
		{PurposeCode: "marketing", PurposeName: consentservice.Text{Th: "การตลาด"}, Status: consentservice.StatusActive, VersionNo: 1},
		{PurposeCode: "analytics", PurposeName: consentservice.Text{Th: "วิเคราะห์"}, Status: consentservice.StatusWithdrawn, VersionNo: 1},
	})

	outcome := "fulfilled"
	e.in(t, func(ctx context.Context) error {
		r, _, err := e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "verifying"})
		if err != nil {
			return err
		}
		req = r
		r, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "in_review"})
		if err != nil {
			return err
		}
		req = r
		r, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "in_progress"})
		if err != nil {
			return err
		}
		req = r
		r, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &outcome})
		if err != nil {
			return err
		}
		req = r
		return nil
	})

	if len(fc.withdrawn) != 1 || fc.withdrawn[0] != subjectID {
		t.Fatalf("WithdrawAllActive was not called for the matching subject: withdrawn = %v", fc.withdrawn)
	}
	profile, err := fc.GetProfile(context.Background(), subjectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range profile.Statuses {
		if st.PurposeCode == "marketing" && st.Status != consentservice.StatusWithdrawn {
			t.Errorf("marketing purpose should now be withdrawn, got %s", st.Status)
		}
	}

	if req.ConsentReceiptNo == nil || *req.ConsentReceiptNo == "" {
		t.Fatalf("expected the request to carry the consent receipt number, got %+v", req)
	}

	// Re-reading the request shows the same receipt number was persisted, not just returned once.
	e.in(t, func(ctx context.Context) error {
		reread, err := e.svc.GetRequest(ctx, req.ID)
		if err != nil {
			return err
		}
		if reread.ConsentReceiptNo == nil || *reread.ConsentReceiptNo != *req.ConsentReceiptNo {
			t.Errorf("consent_receipt_no not persisted: reread = %+v", reread)
		}
		return nil
	})
}

// TestWithdrawConsentRequest_NoMatchingSubjectIsHarmless: completing a withdraw_consent request for a
// requester with no consent subject on record is a no-op, not an error.
func TestWithdrawConsentRequest_NoMatchingSubjectIsHarmless(t *testing.T) {
	e, _, fc := exportEnv(t, "dsarconsentwdnone")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ไม่มีผู้สมัคร จำกัด")
	rt := requestTypeIDByCode(t, e, "withdraw_consent")

	var req dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		req, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{
			RequestTypeID: rt, LegalEntityID: legalEntity, Channel: "web",
			RequesterName: "ไม่มี ผู้สมัคร", RequesterContact: "nobody-here@example.com", ContactKind: crypto.KindEmail,
		})
		return err
	})

	outcome := "fulfilled"
	e.in(t, func(ctx context.Context) error {
		r, _, err := e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "verifying"})
		if err != nil {
			return err
		}
		req = r
		r, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "in_review"})
		if err != nil {
			return err
		}
		req = r
		r, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "in_progress"})
		if err != nil {
			return err
		}
		req = r
		r, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &outcome})
		if err != nil {
			return err
		}
		req = r
		return nil
	})

	if len(fc.withdrawn) != 0 {
		t.Errorf("expected no withdrawal for a requester with no consent subject, got %v", fc.withdrawn)
	}
	if req.ConsentReceiptNo != nil {
		t.Errorf("expected no consent receipt, got %v", *req.ConsentReceiptNo)
	}
}
