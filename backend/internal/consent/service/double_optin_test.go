package service_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"testing"
	"time"

	"pdpa-platform/internal/consent/consenttest"
	consent "pdpa-platform/internal/consent/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// lastDoubleOptInToken decrypts the most recently queued consent.double_opt_in notification's own "link"
// variable and returns its token query parameter — a white-box read standing in for "the subject clicks
// the link in their inbox" (the token itself is never otherwise exposed by the service, by design).
func lastDoubleOptInToken(t *testing.T, f *consenttest.Fixture) string {
	t.Helper()
	var token string
	err := f.Public(t, func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		var payloadJSON []byte
		if err := tx.QueryRow(ctx, `SELECT n.payload FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id
			WHERE t.code = 'consent.double_opt_in' ORDER BY n.created_at DESC LIMIT 1`).Scan(&payloadJSON); err != nil {
			return err
		}
		var p struct {
			VarsEnc string `json:"vars_enc"`
		}
		if err := json.Unmarshal(payloadJSON, &p); err != nil {
			return err
		}
		varsEnc, err := base64.StdEncoding.DecodeString(p.VarsEnc)
		if err != nil {
			return err
		}
		plain, err := f.Svc.Keyring.Decrypt(ctx, "notification", "platform.notifications.payload.vars_enc", varsEnc)
		if err != nil {
			return err
		}
		var vars map[string]any
		if err := json.Unmarshal(plain, &vars); err != nil {
			return err
		}
		link, _ := vars["link"].(string)
		u, err := url.Parse(link)
		if err != nil {
			return err
		}
		token = u.Query().Get("token")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// liveDoubleOptInCP is LiveCP (consenttest) with CollectionPointInput.DoubleOptIn set — the fixture's own
// helper has no slot for it, since no other feature needed one before CON-23.
func liveDoubleOptInCP(t *testing.T, f *consenttest.Fixture, code string, purposes ...consent.CPPurposeInput) consent.CollectionPoint {
	t.Helper()
	var cp consent.CollectionPoint
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		var err error
		if cp, err = f.Svc.CreateCollectionPoint(ctx, consent.CollectionPointInput{Code: code, Name: "สมัครสมาชิก", Channel: "web",
			LegalEntityID: f.EntityA, DoubleOptIn: true, Purposes: purposes}); err != nil {
			return err
		}
		cp, err = f.Svc.PublishCollectionPoint(ctx, cp.ID, cp.RowVersion, consenttest.FullChecklist)
		return err
	})
	return cp
}

// TestDoubleOptIn_PendingUntilConfirmed is CON-23's acceptance criterion in full: a brand-new subject's
// first CONSENTED decision at a double-opt-in-gated collection point has no effect until the subject
// confirms the link sent to their own identifier.
func TestDoubleOptIn_PendingUntilConfirmed(t *testing.T) {
	f := consenttest.Setup(t)
	p := f.LivePurpose(t, "MKT-DOI", consenttest.Content("MKT-DOI", "ยินยอมรับข่าวสารการตลาด"))
	cp := liveDoubleOptInCP(t, f, "DOI-SIGNUP", consent.CPPurposeInput{PurposeID: p.ID})

	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("doi@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "MKT-DOI", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Transactions) != 1 || r.Transactions[0].Type != consent.TxPending || r.Transactions[0].Status != consent.StatusPending {
		t.Fatalf("expected a PENDING transaction, got %+v", r.Transactions)
	}
	if len(r.DoubleOptInRequests) != 1 || r.DoubleOptInRequests[0].Channel != "email" {
		t.Fatalf("expected one double opt-in request over e-mail, got %+v", r.DoubleOptInRequests)
	}

	// Still pending before confirmation.
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

	// The acceptance criterion itself: confirming the link activates the purpose, not before.
	token := lastDoubleOptInToken(t, f)
	var purposeCode string
	_ = f.Public(t, func(ctx context.Context) error {
		var err error
		purposeCode, err = f.Svc.ConfirmDoubleOptIn(ctx, token)
		return err
	})
	if purposeCode != "MKT-DOI" {
		t.Errorf("confirmed purpose code = %q, want MKT-DOI", purposeCode)
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

	// A second confirm with the same token (redelivery / retry) is a harmless no-op, not a duplicate
	// activation.
	_ = f.Public(t, func(ctx context.Context) error {
		_, err := f.Svc.ConfirmDoubleOptIn(ctx, token)
		if !errors.Is(err, consent.ErrInvalidTransition) {
			t.Errorf("second confirm: err = %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

// TestDoubleOptIn_GuardianTakesPriority proves that when both the guardian gate and the double opt-in
// gate would apply to the very same first-ever decision, only the guardian's own flow is used — no
// separate double opt-in link is ever sent for it.
func TestDoubleOptIn_GuardianTakesPriority(t *testing.T) {
	f := consenttest.Setup(t)
	age := 13
	c := consenttest.Content("MKT-DOI-MINOR", "ยินยอมรับข่าวสารการตลาด")
	c.MinAge = &age
	p := f.LivePurpose(t, "MKT-DOI-MINOR", c)
	cp := liveDoubleOptInCP(t, f, "DOI-MINOR-SIGNUP", consent.CPPurposeInput{PurposeID: p.ID})

	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("minor-doi@example.com"), Source: "web", Public: true,
		IsMinor: true, Guardian: &consent.Guardian{Identifiers: []consent.Identifier{{Type: "email", Value: "parent-doi@example.com"}}, Relationship: "parent"},
		Decisions: []consent.Decision{{PurposeCode: "MKT-DOI-MINOR", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.GuardianApprovals) != 1 {
		t.Fatalf("expected one guardian approval, got %+v", r.GuardianApprovals)
	}
	if len(r.DoubleOptInRequests) != 0 {
		t.Fatalf("expected no double opt-in request when guardian consent also applies, got %+v", r.DoubleOptInRequests)
	}
}

// TestDoubleOptIn_UnknownTokenRefused proves a token that doesn't match any sent request is refused
// rather than silently confirming something.
func TestDoubleOptIn_UnknownTokenRefused(t *testing.T) {
	f := consenttest.Setup(t)
	_ = f.Public(t, func(ctx context.Context) error {
		if _, err := f.Svc.ConfirmDoubleOptIn(ctx, "not-a-real-token"); !errors.Is(err, consent.ErrNotFound) {
			t.Errorf("unknown token: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestDoubleOptIn_SweepCancelsUnconfirmedLink is CON-23's own expiry path: a confirmation link nobody
// ever clicked within its 48-hour window never takes effect — the sweep cancels it to NOT_GIVEN, not
// left dangling as PENDING forever.
func TestDoubleOptIn_SweepCancelsUnconfirmedLink(t *testing.T) {
	f := consenttest.Setup(t)
	p := f.LivePurpose(t, "MKT-DOI-EXP", consenttest.Content("MKT-DOI-EXP", "ยินยอมรับข่าวสารการตลาด"))
	cp := liveDoubleOptInCP(t, f, "DOI-EXP-SIGNUP", consent.CPPurposeInput{PurposeID: p.ID})

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.Svc.Now = func() time.Time { return start }
	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("exp-doi@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "MKT-DOI-EXP", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if err != nil {
		t.Fatal(err)
	}

	// Still within the 48h window: a sweep right now touches nothing.
	_ = f.Public(t, func(ctx context.Context) error { return f.Svc.SweepDueDoubleOptInRequests(ctx) })
	_ = f.Public(t, func(ctx context.Context) error {
		prof, err := f.Svc.GetProfile(ctx, r.SubjectID)
		if err != nil {
			return err
		}
		if len(prof.Statuses) != 1 || prof.Statuses[0].Status != consent.StatusPending {
			t.Errorf("before expiry: %+v", prof.Statuses)
		}
		return nil
	})

	// Past the window: the sweep cancels the PENDING decision to NOT_GIVEN.
	f.Svc.Now = func() time.Time { return start.Add(49 * time.Hour) }
	_ = f.Public(t, func(ctx context.Context) error { return f.Svc.SweepDueDoubleOptInRequests(ctx) })
	_ = f.Public(t, func(ctx context.Context) error {
		prof, err := f.Svc.GetProfile(ctx, r.SubjectID)
		if err != nil {
			return err
		}
		if len(prof.Statuses) != 1 || prof.Statuses[0].Status != consent.StatusNotGiven {
			t.Errorf("after expiry: %+v", prof.Statuses)
		}
		return nil
	})

	// A second sweep is a harmless no-op (already expired/confirmed by another path).
	_ = f.Public(t, func(ctx context.Context) error { return f.Svc.SweepDueDoubleOptInRequests(ctx) })
}

// TestDoubleOptIn_TwoTenantIsolation proves the sweep for tenant A never touches tenant B's own requests
// (consent.double_optin_requests has no RLS filter in ListDueDoubleOptInRequests beyond the tx's own
// tenant — this proves RLS itself does the scoping).
func TestDoubleOptIn_TwoTenantIsolation(t *testing.T) {
	f := consenttest.Setup(t)
	p := f.LivePurpose(t, "MKT-DOI-ISO", consenttest.Content("MKT-DOI-ISO", "ยินยอมรับข่าวสารการตลาด"))
	cp := liveDoubleOptInCP(t, f, "DOI-ISO-SIGNUP", consent.CPPurposeInput{PurposeID: p.ID})

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.Svc.Now = func() time.Time { return start }
	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("iso-doi@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "MKT-DOI-ISO", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if err != nil {
		t.Fatal(err)
	}

	f.Svc.Now = func() time.Time { return start.Add(49 * time.Hour) }
	// Sweeping as tenant B must not see or touch tenant A's due request.
	err = pdb.WithTenantTx(context.Background(), f.App, f.B.ID.String(), "", func(ctx context.Context) error {
		return f.Svc.SweepDueDoubleOptInRequests(authz.WithGrants(ctx, authz.Grants{TenantID: f.B.ID.String()}))
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Public(t, func(ctx context.Context) error {
		prof, err := f.Svc.GetProfile(ctx, r.SubjectID)
		if err != nil {
			return err
		}
		if len(prof.Statuses) != 1 || prof.Statuses[0].Status != consent.StatusPending {
			t.Errorf("tenant B's sweep must not touch tenant A's request: %+v", prof.Statuses)
		}
		return nil
	})
}
