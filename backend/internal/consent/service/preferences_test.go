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

// startPreferenceVerification starts a CON-18 OTP against tenant A and returns its id + the code the fake
// notifier captured.
func startPreferenceVerification(t *testing.T, f *consenttest.Fixture, email string) (uuid.UUID, string) {
	t.Helper()
	notifier := f.Svc.Verification.Notify.(*fakeIamNotifier)
	var id uuid.UUID
	_ = f.Public(t, func(ctx context.Context) error {
		ref, err := f.Svc.StartPreferenceVerification(ctx, consent.Identifier{Type: "email", Value: email})
		id = ref.ID
		return err
	})
	return id, notifier.lastCode()
}

// TestPreferenceCenter_GrantAndWithdraw is CON-18's own acceptance criterion: once OTP-verified, a subject
// views every active purpose and can both grant and withdraw, getting a receipt each time.
func TestPreferenceCenter_GrantAndWithdraw(t *testing.T) {
	f := consenttest.Setup(t)
	f.Svc.Verification = &iamservice.Service{Keyring: f.Svc.Keyring, Notify: &fakeIamNotifier{}}

	p := f.LivePurpose(t, "MKT-PC", consenttest.Content("MKT-PC", "ยินยอมรับข่าวสาร"))
	// A real, active collection point so UpdatePreferences has something to attribute the receipt's channel to.
	f.LiveCP(t, "PC-SIGNUP", consent.CPPurposeInput{PurposeID: p.ID})

	verID, code := startPreferenceVerification(t, f, "alice@example.com")

	var pc consent.PreferenceCenter
	var err error
	_ = f.Public(t, func(ctx context.Context) error {
		pc, err = f.Svc.GetPreferenceCenter(ctx, verID, code, consent.Identifier{Type: "email", Value: "alice@example.com"}, "th")
		return nil
	})
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(pc.Purposes) != 1 || pc.Purposes[0].Code != "MKT-PC" || pc.Purposes[0].Decision != "" {
		t.Fatalf("expected one undecided purpose, got %+v", pc.Purposes)
	}

	// Grant it — no code needed again, the verification is already decided "verified" for this identifier.
	var r consent.Receipt
	_ = f.Public(t, func(ctx context.Context) error {
		r, err = f.Svc.UpdatePreferences(ctx, verID, "", consent.Identifier{Type: "email", Value: "alice@example.com"},
			[]consent.Decision{{PurposeCode: "MKT-PC", PurposeVersionNo: 1, Decision: consent.TxConsented}}, "th")
		return nil
	})
	if err != nil || r.No == "" || len(r.Transactions) != 1 || r.Transactions[0].Status != consent.StatusActive {
		t.Fatalf("grant: err=%v receipt=%+v", err, r)
	}

	_ = f.Public(t, func(ctx context.Context) error {
		pc, err = f.Svc.GetPreferenceCenter(ctx, verID, "", consent.Identifier{Type: "email", Value: "alice@example.com"}, "th")
		return nil
	})
	if err != nil || pc.Purposes[0].Decision != consent.StatusActive {
		t.Fatalf("after grant: err=%v purposes=%+v", err, pc.Purposes)
	}

	// Withdraw it — a plain collection-point submission could never do this (CON-19's own "withdraw_not_allowed"),
	// but the preference centre is never a public form submission.
	_ = f.Public(t, func(ctx context.Context) error {
		r, err = f.Svc.UpdatePreferences(ctx, verID, "", consent.Identifier{Type: "email", Value: "alice@example.com"},
			[]consent.Decision{{PurposeCode: "MKT-PC", PurposeVersionNo: 1, Decision: consent.TxWithdrawn}}, "th")
		return nil
	})
	if err != nil || len(r.Transactions) != 1 || r.Transactions[0].Status != consent.StatusWithdrawn {
		t.Fatalf("withdraw: err=%v receipt=%+v", err, r)
	}
}

// TestPreferenceCenter_RequiresVerification proves every read or write is refused without a correct OTP
// against the exact identifier acting on it.
func TestPreferenceCenter_RequiresVerification(t *testing.T) {
	f := consenttest.Setup(t)
	f.Svc.Verification = &iamservice.Service{Keyring: f.Svc.Keyring, Notify: &fakeIamNotifier{}}
	f.LivePurpose(t, "MKT-PCV", consenttest.Content("MKT-PCV", "ยินยอมรับข่าวสาร"))

	verID, _ := startPreferenceVerification(t, f, "bob@example.com")

	var de *consent.DecisionError
	var err error
	// Wrong code.
	_ = f.Public(t, func(ctx context.Context) error {
		_, err = f.Svc.GetPreferenceCenter(ctx, verID, "000000", consent.Identifier{Type: "email", Value: "bob@example.com"}, "th")
		return nil
	})
	if !errors.As(err, &de) || de.Fields[0].Field != "verification" || de.Fields[0].Code != "invalid" {
		t.Fatalf("wrong code: err=%v, want verification/invalid", err)
	}

	// Right code, wrong identifier.
	verID2, code2 := startPreferenceVerification(t, f, "carol@example.com")
	_ = f.Public(t, func(ctx context.Context) error {
		_, err = f.Svc.GetPreferenceCenter(ctx, verID2, code2, consent.Identifier{Type: "email", Value: "someone-else@example.com"}, "th")
		return nil
	})
	if !errors.As(err, &de) || de.Fields[0].Field != "verification" || de.Fields[0].Code != "invalid" {
		t.Fatalf("mismatched identifier: err=%v, want verification/invalid", err)
	}
}

// TestPreferenceCenter_PublishRetireLink is the admin half: publishing twice keeps the same key, retiring
// revokes it, and a later publish issues a fresh one.
func TestPreferenceCenter_PublishRetireLink(t *testing.T) {
	f := consenttest.Setup(t)

	var key1, key2, key3 string
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		var err error
		if key1, err = f.Svc.PublishPreferenceCenter(ctx, f.EntityA); err != nil {
			return err
		}
		key2, err = f.Svc.PublishPreferenceCenter(ctx, f.EntityA)
		return err
	})
	if key1 == "" || key1 != key2 {
		t.Fatalf("re-publish should keep the same key: %q vs %q", key1, key2)
	}

	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		link, ok, err := f.Svc.GetPreferenceCenterLink(ctx, f.EntityA)
		if err != nil || !ok || link != key1 {
			t.Errorf("GetPreferenceCenterLink: link=%q ok=%v err=%v", link, ok, err)
		}
		return f.Svc.RetirePreferenceCenter(ctx, f.EntityA)
	})

	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		_, ok, err := f.Svc.GetPreferenceCenterLink(ctx, f.EntityA)
		if err != nil || ok {
			t.Errorf("after retire: ok=%v err=%v, want ok=false", ok, err)
		}
		var err2 error
		key3, err2 = f.Svc.PublishPreferenceCenter(ctx, f.EntityA)
		return err2
	})
	if key3 == "" || key3 == key1 {
		t.Fatalf("publish after retire should issue a fresh key, got %q (was %q)", key3, key1)
	}
}

// TestPreferenceCenter_CrossTenantIsolation proves a verification started under tenant A can't be used to
// read or write tenant B's purposes — iam.subject_verifications' own RLS hides it entirely.
func TestPreferenceCenter_CrossTenantIsolation(t *testing.T) {
	f := consenttest.Setup(t)
	f.Svc.Verification = &iamservice.Service{Keyring: f.Svc.Keyring, Notify: &fakeIamNotifier{}}
	f.LivePurpose(t, "MKT-PCISO", consenttest.Content("MKT-PCISO", "ยินยอมรับข่าวสาร"))

	verID, code := startPreferenceVerification(t, f, "iso@example.com")

	err := pdb.WithTenantTx(context.Background(), f.App, f.B.ID.String(), "", func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: f.B.ID.String()})
		_, err := f.Svc.GetPreferenceCenter(ctx, verID, code, consent.Identifier{Type: "email", Value: "iso@example.com"}, "th")
		return err
	})
	if err == nil {
		t.Error("tenant B reading tenant A's verification id should fail")
	}
}
