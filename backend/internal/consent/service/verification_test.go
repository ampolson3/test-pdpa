package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/consent/consenttest"
	consent "pdpa-platform/internal/consent/service"
	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// otpCollectionPoint publishes a collection point whose VerificationMethod is "otp" (CON-19) — LiveCP itself
// has no verification knob, so this builds the same create+publish sequence directly.
func otpCollectionPoint(t *testing.T, f *consenttest.Fixture, code string, purposes ...consent.CPPurposeInput) consent.CollectionPoint {
	t.Helper()
	var cp consent.CollectionPoint
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		var err error
		cp, err = f.Svc.CreateCollectionPoint(ctx, consent.CollectionPointInput{Code: code, Name: "สมัครสมาชิก", Channel: "web",
			LegalEntityID: f.EntityA, VerificationMethod: "otp", Purposes: purposes})
		if err != nil {
			return err
		}
		cp, err = f.Svc.PublishCollectionPoint(ctx, cp.ID, cp.RowVersion, consenttest.FullChecklist)
		return err
	})
	return cp
}

// TestConsentVerification_RequiredBeforeRecording is CON-19's own acceptance criterion: a collection point
// that mandates OTP records a submission only once the subject has verified via that same OTP.
func TestConsentVerification_RequiredBeforeRecording(t *testing.T) {
	f := consenttest.Setup(t)
	notifier := &fakeIamNotifier{}
	f.Svc.Verification = &iamservice.Service{Keyring: f.Svc.Keyring, Notify: notifier}

	p := f.LivePurpose(t, "MKT-OTP", consenttest.Content("MKT-OTP", "ยินยอมรับข่าวสาร"))
	cp := otpCollectionPoint(t, f, "OTP-SIGNUP", consent.CPPurposeInput{PurposeID: p.ID})

	decisions := []consent.Decision{{PurposeCode: "MKT-OTP", PurposeVersionNo: 1, Decision: "CONSENTED"}}

	// No verification at all: refused, nothing written.
	var de *consent.DecisionError
	_, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("otp@example.com"), Source: "web", Public: true, Decisions: decisions})
	if !errors.As(err, &de) || len(de.Fields) != 1 || de.Fields[0].Field != "verification" || de.Fields[0].Code != "required" {
		t.Fatalf("no verification: err = %v, want verification/required", err)
	}

	// Start verification, then submit with the wrong code: refused, and the attempt is consumed.
	var verID1 uuid.UUID
	_ = f.Public(t, func(ctx context.Context) error {
		ref, err := f.Svc.StartConsentVerification(ctx, cp.ID, consent.Identifier{Type: "email", Value: "otp@example.com"})
		if err != nil {
			return err
		}
		verID1 = ref.ID
		if ref.Channel != "email" {
			t.Errorf("expected email channel, got %s", ref.Channel)
		}
		return nil
	})
	if len(notifier.sent) != 1 || notifier.sent[0].TemplateCode != "iam.otp" {
		t.Fatalf("expected one OTP sent, got %+v", notifier.sent)
	}
	_, err = f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("otp@example.com"), Source: "web", Public: true,
		Decisions: decisions, VerificationID: &verID1, VerificationCode: "000000"})
	if !errors.As(err, &de) || de.Fields[0].Field != "verification" || de.Fields[0].Code != "invalid" {
		t.Fatalf("wrong code: err = %v, want verification/invalid", err)
	}

	// Start a fresh verification, confirm with the right code, then submit with an unrelated identifier: refused
	// as a mismatch — someone else's completed OTP can't cover this submission.
	var verID2 uuid.UUID
	_ = f.Public(t, func(ctx context.Context) error {
		ref, err := f.Svc.StartConsentVerification(ctx, cp.ID, consent.Identifier{Type: "email", Value: "otp2@example.com"})
		verID2 = ref.ID
		return err
	})
	code2 := notifier.lastCode()
	_, err = f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("someone-else@example.com"), Source: "web", Public: true,
		Decisions: decisions, VerificationID: &verID2, VerificationCode: code2})
	if !errors.As(err, &de) || de.Fields[0].Field != "verification" || de.Fields[0].Code != "identifier_mismatch" {
		t.Fatalf("mismatched identifier: err = %v, want verification/identifier_mismatch", err)
	}

	// Start a fresh verification, confirm with the right code and the same identifier: recorded.
	var verID3 uuid.UUID
	_ = f.Public(t, func(ctx context.Context) error {
		ref, err := f.Svc.StartConsentVerification(ctx, cp.ID, consent.Identifier{Type: "email", Value: "otp@example.com"})
		verID3 = ref.ID
		return err
	})
	code3 := notifier.lastCode()
	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("otp@example.com"), Source: "web", Public: true,
		Decisions: decisions, VerificationID: &verID3, VerificationCode: code3})
	if err != nil {
		t.Fatalf("verified submission: %v", err)
	}
	if len(r.Transactions) != 1 || r.Transactions[0].Status != consent.StatusActive {
		t.Errorf("expected an active transaction, got %+v", r.Transactions)
	}
}

// TestConsentVerification_StartRefusedWithoutOtpMethod proves StartConsentVerification is scoped to
// collection points that actually require OTP — a "none" collection point refuses to issue a code.
func TestConsentVerification_StartRefusedWithoutOtpMethod(t *testing.T) {
	f := consenttest.Setup(t)
	notifier := &fakeIamNotifier{}
	f.Svc.Verification = &iamservice.Service{Keyring: f.Svc.Keyring, Notify: notifier}

	p := f.LivePurpose(t, "MKT-NONE", consenttest.Content("MKT-NONE", "ยินยอมรับข่าวสาร"))
	cp := f.LiveCP(t, "NONE-SIGNUP", consent.CPPurposeInput{PurposeID: p.ID})

	_ = f.Public(t, func(ctx context.Context) error {
		if _, err := f.Svc.StartConsentVerification(ctx, cp.ID, consent.Identifier{Type: "email", Value: "x@example.com"}); !errors.Is(err, consent.ErrInvalidRequest) {
			t.Errorf("start verification on a none-method point: %v, want ErrInvalidRequest", err)
		}
		return nil
	})
	if len(notifier.sent) != 0 {
		t.Errorf("expected no OTP sent, got %+v", notifier.sent)
	}
}

// TestConsentVerification_CrossTenantIsolation proves a verification id started under one tenant can never
// be presented to satisfy another tenant's OTP-gated collection point (RLS on iam.subject_verifications).
func TestConsentVerification_CrossTenantIsolation(t *testing.T) {
	f := consenttest.Setup(t)
	notifier := &fakeIamNotifier{}
	f.Svc.Verification = &iamservice.Service{Keyring: f.Svc.Keyring, Notify: notifier}

	p := f.LivePurpose(t, "MKT-ISOA", consenttest.Content("MKT-ISOA", "ยินยอมรับข่าวสาร"))
	cp := otpCollectionPoint(t, f, "OTP-ISOA", consent.CPPurposeInput{PurposeID: p.ID})

	_ = f.Public(t, func(ctx context.Context) error {
		_, err := f.Svc.StartConsentVerification(ctx, cp.ID, consent.Identifier{Type: "email", Value: "iso@example.com"})
		return err
	})

	// Tenant B has no such collection point: starting a verification against tenant A's id from tenant B's
	// own transaction (RLS) must fail — a cross-tenant lookup is invisible, not merely denied.
	err := pdb.WithTenantTx(context.Background(), f.App, f.B.ID.String(), "", func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: f.B.ID.String()})
		_, err := f.Svc.StartConsentVerification(ctx, cp.ID, consent.Identifier{Type: "email", Value: "iso@example.com"})
		return err
	})
	if err == nil {
		t.Error("tenant B starting verification against tenant A's collection point should fail")
	}
}
