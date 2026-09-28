package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/consent/consenttest"
	consent "pdpa-platform/internal/consent/service"
	iamservice "pdpa-platform/internal/iam/service"
)

// fakeIamNotifier stands in for PLT-04 so the test can read the OTP a StartVerification call would have sent
// the guardian, without a real SMTP/SMS sender — the same technique IAM-05's own tests use.
type fakeIamNotifier struct{ sent []iamservice.NotifyRequest }

func (f *fakeIamNotifier) Send(ctx context.Context, req iamservice.NotifyRequest) (uuid.UUID, error) {
	f.sent = append(f.sent, req)
	return uuid.New(), nil
}

func (f *fakeIamNotifier) lastCode() string {
	if len(f.sent) == 0 {
		return ""
	}
	code, _ := f.sent[len(f.sent)-1].Vars["code"].(string)
	return code
}

func guardianAgeGatedPurpose(t *testing.T, f *consenttest.Fixture, code string) consent.Purpose {
	t.Helper()
	c := consenttest.Content(code, "ยินยอมรับข่าวสารการตลาด")
	age := 13
	c.MinAge = &age
	return f.LivePurpose(t, code, c)
}

// TestGuardianConsent_PendingUntilGuardianVerifies is CON-11's acceptance criterion in full: a minor's
// CONSENTED decision on an age-gated purpose has no effect until the guardian verifies their own OTP.
func TestGuardianConsent_PendingUntilGuardianVerifies(t *testing.T) {
	f := consenttest.Setup(t)
	notifier := &fakeIamNotifier{}
	f.Svc.Verification = &iamservice.Service{Keyring: f.Svc.Keyring, Notify: notifier}

	p := guardianAgeGatedPurpose(t, f, "MKT-MINOR")
	cp := f.LiveCP(t, "MINOR-SIGNUP", consent.CPPurposeInput{PurposeID: p.ID})

	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("minor@example.com"), Source: "web", Public: true,
		IsMinor: true, Guardian: &consent.Guardian{Identifiers: []consent.Identifier{{Type: "email", Value: "parent@example.com"}}, Relationship: "parent"},
		Decisions: []consent.Decision{{PurposeCode: "MKT-MINOR", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Transactions) != 1 || r.Transactions[0].Type != consent.TxPending || r.Transactions[0].Status != consent.StatusPending {
		t.Fatalf("expected a PENDING transaction, got %+v", r.Transactions)
	}
	if len(r.GuardianApprovals) != 1 {
		t.Fatalf("expected one guardian approval, got %+v", r.GuardianApprovals)
	}
	if len(notifier.sent) != 1 || notifier.sent[0].Channel != "email" || notifier.sent[0].TemplateCode != "iam.otp" {
		t.Fatalf("expected one guardian OTP e-mail, got %+v", notifier.sent)
	}
	ga := r.GuardianApprovals[0]

	// Still pending before the guardian verifies.
	_ = f.Public(t, func(ctx context.Context) error {
		prof, err := f.Svc.GetProfile(ctx, r.SubjectID)
		if err != nil {
			return err
		}
		if len(prof.Statuses) != 1 || prof.Statuses[0].Status != consent.StatusPending {
			t.Errorf("before confirm: %+v", prof.Statuses)
		}
		return nil
	})

	code := notifier.lastCode()
	var confirmed []string
	_ = f.Public(t, func(ctx context.Context) error {
		var err error
		confirmed, err = f.Svc.ConfirmGuardianApproval(ctx, ga.ID, ga.VerificationID, code)
		return err
	})
	if len(confirmed) != 1 || confirmed[0] != "MKT-MINOR" {
		t.Errorf("confirmed purposes = %v, want [MKT-MINOR]", confirmed)
	}
	_ = f.Public(t, func(ctx context.Context) error {
		prof, err := f.Svc.GetProfile(ctx, r.SubjectID)
		if err != nil {
			return err
		}
		if len(prof.Statuses) != 1 || prof.Statuses[0].Status != consent.StatusActive {
			t.Errorf("after confirm: %+v", prof.Statuses)
		}
		return nil
	})

	// A second confirm (redelivery / retry) is a harmless no-op, not a duplicate activation.
	_ = f.Public(t, func(ctx context.Context) error {
		_, err := f.Svc.ConfirmGuardianApproval(ctx, ga.ID, ga.VerificationID, code)
		if !errors.Is(err, consent.ErrInvalidTransition) {
			t.Errorf("second confirm: err = %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

// TestGuardianConsent_WrongCodeLeavesConsentPending proves a mismatched OTP never activates the purpose.
func TestGuardianConsent_WrongCodeLeavesConsentPending(t *testing.T) {
	f := consenttest.Setup(t)
	notifier := &fakeIamNotifier{}
	f.Svc.Verification = &iamservice.Service{Keyring: f.Svc.Keyring, Notify: notifier}

	p := guardianAgeGatedPurpose(t, f, "MKT-MINOR2")
	cp := f.LiveCP(t, "MINOR-SIGNUP2", consent.CPPurposeInput{PurposeID: p.ID})
	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("minor2@example.com"), Source: "web", Public: true,
		IsMinor: true, Guardian: &consent.Guardian{Identifiers: []consent.Identifier{{Type: "email", Value: "parent2@example.com"}}, Relationship: "parent"},
		Decisions: []consent.Decision{{PurposeCode: "MKT-MINOR2", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if err != nil {
		t.Fatal(err)
	}
	ga := r.GuardianApprovals[0]
	_ = f.Public(t, func(ctx context.Context) error {
		if _, err := f.Svc.ConfirmGuardianApproval(ctx, ga.ID, ga.VerificationID, "000000"); err == nil {
			t.Error("expected an error for a wrong OTP")
		}
		prof, err := f.Svc.GetProfile(ctx, r.SubjectID)
		if err != nil {
			return err
		}
		if len(prof.Statuses) != 1 || prof.Statuses[0].Status != consent.StatusPending {
			t.Errorf("after wrong code: %+v", prof.Statuses)
		}
		return nil
	})
}

// TestGuardianConsent_RequiresGuardianInfo is the submission-level validation: a minor's CONSENTED decision on
// an age-gated purpose without guardian info is refused, not silently recorded.
func TestGuardianConsent_RequiresGuardianInfo(t *testing.T) {
	f := consenttest.Setup(t)
	p := guardianAgeGatedPurpose(t, f, "MKT-MINOR3")
	cp := f.LiveCP(t, "MINOR-SIGNUP3", consent.CPPurposeInput{PurposeID: p.ID})
	var de *consent.DecisionError
	_, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("minor3@example.com"), Source: "web", Public: true,
		IsMinor:   true,
		Decisions: []consent.Decision{{PurposeCode: "MKT-MINOR3", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if !errors.As(err, &de) || len(de.Fields) != 1 || de.Fields[0].Code != "guardian_required" {
		t.Errorf("missing guardian info: err = %v, want guardian_required", err)
	}
}
