package service_test

import (
	"context"
	"testing"
	"time"

	"pdpa-platform/internal/consent/consenttest"
	consent "pdpa-platform/internal/consent/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// livePurposeWithLifespan is consenttest.Content plus a lifespan (CON-20's own "ตั้งอายุใน Purpose"),
// which the shared fixture helper has no slot for since no earlier feature needed one.
func livePurposeWithLifespan(t *testing.T, f *consenttest.Fixture, code string, days int) consent.Purpose {
	t.Helper()
	c := consenttest.Content(code, "ยินยอมที่มีอายุ")
	c.LifespanDays = &days
	return f.LivePurpose(t, code, c)
}

// TestExpiry_PastDueBecomesExpired is CON-20's own acceptance criterion: an ACTIVE consent past its own
// expires_at moves to EXPIRED with a new chained transaction and a published consent.expired event.
func TestExpiry_PastDueBecomesExpired(t *testing.T) {
	f := consenttest.Setup(t)
	p := livePurposeWithLifespan(t, f, "EXP-10", 10)
	cp := f.LiveCP(t, "EXP-CP", consent.CPPurposeInput{PurposeID: p.ID})

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.Svc.Now = func() time.Time { return start }
	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("exp@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "EXP-10", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if err != nil {
		t.Fatal(err)
	}

	// Before the lifespan elapses, a check finds nothing due.
	_ = f.Public(t, func(ctx context.Context) error {
		n, err := f.Svc.CheckDueExpirations(ctx)
		if err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("before due: CheckDueExpirations = %d, want 0", n)
		}
		return nil
	})

	// Past the 10-day lifespan: the acceptance criterion itself.
	f.Svc.Now = func() time.Time { return start.AddDate(0, 0, 11) }
	_ = f.Public(t, func(ctx context.Context) error {
		n, err := f.Svc.CheckDueExpirations(ctx)
		if err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("past due: CheckDueExpirations = %d, want 1", n)
		}
		return nil
	})
	_ = f.Public(t, func(ctx context.Context) error {
		prof, err := f.Svc.GetProfile(ctx, r.SubjectID)
		if err != nil {
			return err
		}
		if len(prof.Statuses) != 1 || prof.Statuses[0].Status != consent.StatusExpired {
			t.Fatalf("after expiry: %+v", prof.Statuses)
		}
		if len(prof.History) == 0 || prof.History[0].Type != consent.TxExpired || prof.History[0].Source != "system" {
			t.Errorf("history head = %+v, want an EXPIRED/system entry", prof.History)
		}
		return nil
	})
	if got := count(f, t, f.A, `SELECT count(*)::text FROM platform.outbox_events WHERE event_type = 'consent.expired' AND payload->'data'->>'subject_ref' = '`+r.SubjectID.String()+`'`); got != "1" {
		t.Errorf("consent.expired outbox rows = %s, want 1", got)
	}

	// A second check is a harmless no-op — the row no longer matches ACTIVE.
	_ = f.Public(t, func(ctx context.Context) error {
		n, err := f.Svc.CheckDueExpirations(ctx)
		if err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("repeat check: CheckDueExpirations = %d, want 0", n)
		}
		return nil
	})
}

// TestExpiry_ListNearExpiry is CON-20's own frontend note: the near-expiry window finds a consent due
// within it and excludes one further out.
func TestExpiry_ListNearExpiry(t *testing.T) {
	f := consenttest.Setup(t)
	p := livePurposeWithLifespan(t, f, "EXP-NEAR", 10)
	cp := f.LiveCP(t, "EXP-NEAR-CP", consent.CPPurposeInput{PurposeID: p.ID})

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.Svc.Now = func() time.Time { return start }
	if _, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("near@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "EXP-NEAR", PurposeVersionNo: 1, Decision: "CONSENTED"}}}); err != nil {
		t.Fatal(err)
	}

	_ = f.Public(t, func(ctx context.Context) error {
		list, err := f.Svc.ListNearExpiry(ctx, 15)
		if err != nil {
			return err
		}
		found := false
		for _, n := range list {
			if n.PurposeCode == "EXP-NEAR" {
				found = true
			}
		}
		if !found {
			t.Errorf("within 15 days: expected EXP-NEAR present, got %+v", list)
		}
		list, err = f.Svc.ListNearExpiry(ctx, 5)
		if err != nil {
			return err
		}
		for _, n := range list {
			if n.PurposeCode == "EXP-NEAR" {
				t.Errorf("within 5 days: expected EXP-NEAR absent, got %+v", list)
			}
		}
		return nil
	})
}

// TestExpiry_TwoTenantIsolation proves tenant B's own check never touches tenant A's due consent.
func TestExpiry_TwoTenantIsolation(t *testing.T) {
	f := consenttest.Setup(t)
	p := livePurposeWithLifespan(t, f, "EXP-ISO", 10)
	cp := f.LiveCP(t, "EXP-ISO-CP", consent.CPPurposeInput{PurposeID: p.ID})

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.Svc.Now = func() time.Time { return start }
	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("iso-exp@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "EXP-ISO", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if err != nil {
		t.Fatal(err)
	}

	f.Svc.Now = func() time.Time { return start.AddDate(0, 0, 11) }
	err = pdb.WithTenantTx(context.Background(), f.App, f.B.ID.String(), "", func(ctx context.Context) error {
		_, err := f.Svc.CheckDueExpirations(authz.WithGrants(ctx, authz.Grants{TenantID: f.B.ID.String()}))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Public(t, func(ctx context.Context) error {
		prof, err := f.Svc.GetProfile(ctx, r.SubjectID)
		if err != nil {
			return err
		}
		if len(prof.Statuses) != 1 || prof.Statuses[0].Status != consent.StatusActive {
			t.Errorf("tenant B's check must not touch tenant A's due consent: %+v", prof.Statuses)
		}
		return nil
	})
}
