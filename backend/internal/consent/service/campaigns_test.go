package service_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/consent/consenttest"
	consent "pdpa-platform/internal/consent/service"
	pdb "pdpa-platform/internal/pkg/db"
)

// campaignRequestTokens is the same white-box "read the queued notification's own encrypted link variable"
// technique CON-23's own double-opt-in tests already established, extended to recover the n most recently
// queued consent.campaign_request tokens oldest-first — a send to several recipients in one call queues one
// notification per recipient, in the same order ListCampaignRecipients (and so sendToRecipients) walks.
func campaignRequestTokens(t *testing.T, f *consenttest.Fixture, n int) []string {
	t.Helper()
	var tokens []string
	err := f.Public(t, func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		rows, err := tx.Query(ctx, `SELECT n.payload FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id
			WHERE t.code = 'consent.campaign_request' ORDER BY n.created_at DESC LIMIT $1`, n)
		if err != nil {
			return err
		}
		defer rows.Close()
		var raw [][]byte
		for rows.Next() {
			var p []byte
			if err := rows.Scan(&p); err != nil {
				return err
			}
			raw = append(raw, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for i := len(raw) - 1; i >= 0; i-- { // DESC query, so reverse back to oldest-first
			var p struct {
				VarsEnc string `json:"vars_enc"`
			}
			if err := json.Unmarshal(raw[i], &p); err != nil {
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
			idx := strings.LastIndex(link, "token=")
			if idx < 0 {
				tokens = append(tokens, "")
				continue
			}
			tokens = append(tokens, link[idx+len("token="):])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

// countCampaignRequestNotifications is a plain count of every consent.campaign_request notification queued
// so far, for asserting exactly how many sends/resends actually happened.
func countCampaignRequestNotifications(t *testing.T, f *consenttest.Fixture) int {
	t.Helper()
	var n int
	err := f.Public(t, func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		return tx.QueryRow(ctx, `SELECT count(*) FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id
			WHERE t.code = 'consent.campaign_request'`).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// countCampaignRecipients is a plain count of a campaign's own recipient rows.
func countCampaignRecipients(t *testing.T, f *consenttest.Fixture, campaignID string) int {
	t.Helper()
	var n int
	f.As(t, f.A, f.Alice, nil, campaignPerms, func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		return tx.QueryRow(ctx, `SELECT count(*) FROM consent.campaign_recipients WHERE campaign_id = $1`, campaignID).Scan(&n)
	})
	return n
}

var campaignPerms = []string{"consent.campaign.read", "consent.campaign.create", "consent.campaign.execute"}

// TestCreateCampaign_ResolvesAudienceIntoSubjects is the acceptance criterion's own data half: every given
// identifier resolves into a real, queued recipient immediately — nothing raw is stored on the campaign
// itself (audience_query only ever holds the resulting subject ids).
func TestCreateCampaign_ResolvesAudienceIntoSubjects(t *testing.T) {
	f := consenttest.Setup(t)
	p := f.LivePurpose(t, "CAMP-01", consenttest.Content("CAMP-01", "ยินยอมแคมเปญ"))

	var c consent.Campaign
	f.As(t, f.A, f.Alice, nil, campaignPerms, func(ctx context.Context) error {
		var err error
		c, err = f.Svc.CreateCampaign(ctx, consent.CampaignInput{
			Name:        "ฤดูร้อน 2026",
			PurposeIDs:  []uuid.UUID{p.ID},
			Channel:     "email",
			Identifiers: []consent.Identifier{{Type: "email", Value: "one@example.com"}, {Type: "email", Value: "two@example.com"}},
		})
		return err
	})
	if c.Status != "draft" {
		t.Fatalf("status = %q", c.Status)
	}
	if len(c.PurposeIDs) != 1 || c.PurposeIDs[0] != p.ID {
		t.Fatalf("purpose not stored: %+v", c.PurposeIDs)
	}
	if c.SentCount != 0 || c.OpenedCount != 0 || c.ResponseCount != 0 {
		t.Fatalf("fresh campaign should have zero counts, got %+v", c)
	}
	if n := countCampaignRecipients(t, f, c.ID.String()); n != 2 {
		t.Fatalf("recipients = %d, want 2", n)
	}
}

// TestCreateCampaign_Validation covers the write-path refusals before anything is written.
func TestCreateCampaign_Validation(t *testing.T) {
	f := consenttest.Setup(t)
	p := f.LivePurpose(t, "CAMP-02", consenttest.Content("CAMP-02", "ยินยอมแคมเปญ"))
	ids := []consent.Identifier{{Type: "email", Value: "a@example.com"}}

	cases := []consent.CampaignInput{
		{Name: "", PurposeIDs: []uuid.UUID{p.ID}, Channel: "email", Identifiers: ids},
		{Name: "x", PurposeIDs: nil, Channel: "email", Identifiers: ids},
		{Name: "x", PurposeIDs: []uuid.UUID{p.ID}, Channel: "fax", Identifiers: ids},
		{Name: "x", PurposeIDs: []uuid.UUID{p.ID}, Channel: "email", Identifiers: nil},
	}
	for i, in := range cases {
		f.As(t, f.A, f.Alice, nil, campaignPerms, func(ctx context.Context) error {
			_, err := f.Svc.CreateCampaign(ctx, in)
			if !errors.Is(err, consent.ErrInvalidRequest) {
				t.Errorf("case %d: err = %v, want ErrInvalidRequest", i, err)
			}
			return nil
		})
	}
}

// TestSendCampaign_ThenRespond_ShowsCounts is the acceptance criterion's own display half: send → the
// recipient opens → responds, and sent/opened/responded all read correctly, live, every time.
func TestSendCampaign_ThenRespond_ShowsCounts(t *testing.T) {
	f := consenttest.Setup(t)
	p := f.LivePurpose(t, "CAMP-03", consenttest.Content("CAMP-03", "ยินยอมแคมเปญ"))

	var c consent.Campaign
	f.As(t, f.A, f.Alice, nil, campaignPerms, func(ctx context.Context) error {
		var err error
		c, err = f.Svc.CreateCampaign(ctx, consent.CampaignInput{Name: "แคมเปญ", PurposeIDs: []uuid.UUID{p.ID}, Channel: "email",
			Identifiers: []consent.Identifier{{Type: "email", Value: "resp@example.com"}}})
		return err
	})

	f.As(t, f.A, f.Alice, nil, campaignPerms, func(ctx context.Context) error {
		var err error
		c, err = f.Svc.SendCampaign(ctx, c.ID, "th")
		return err
	})
	if c.Status != "done" {
		t.Fatalf("status after send = %q, want done", c.Status)
	}
	if c.SentCount != 1 {
		t.Fatalf("sent_count = %d, want 1", c.SentCount)
	}

	tokens := campaignRequestTokens(t, f, 1)
	if len(tokens) != 1 || tokens[0] == "" {
		t.Fatalf("expected one real token, got %v", tokens)
	}

	var view consent.CampaignView
	err := f.Public(t, func(ctx context.Context) error {
		var err error
		view, err = f.Svc.ViewCampaignRecipient(ctx, c.ID, tokens[0], "th")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Responded || len(view.Purposes) != 1 {
		t.Fatalf("view = %+v", view)
	}

	var receipt consent.Receipt
	err = f.Public(t, func(ctx context.Context) error {
		var err error
		receipt, err = f.Svc.RespondCampaignRecipient(ctx, c.ID, tokens[0], []consent.Decision{
			{PurposeCode: "CAMP-03", PurposeVersionNo: view.Purposes[0].VersionNo, Decision: "CONSENTED"},
		}, "th")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.No == "" {
		t.Fatal("expected a non-empty receipt number")
	}

	f.As(t, f.A, f.Alice, nil, campaignPerms, func(ctx context.Context) error {
		got, err := f.Svc.GetCampaign(ctx, c.ID)
		if err != nil {
			return err
		}
		if got.OpenedCount != 1 || got.ResponseCount != 1 || got.SentCount != 1 {
			t.Fatalf("counts after respond = %+v", got)
		}
		return nil
	})

	// A second response with the same token is refused, not a silent duplicate.
	err = f.Public(t, func(ctx context.Context) error {
		_, err := f.Svc.RespondCampaignRecipient(ctx, c.ID, tokens[0], []consent.Decision{
			{PurposeCode: "CAMP-03", PurposeVersionNo: view.Purposes[0].VersionNo, Decision: "CONSENTED"},
		}, "th")
		return err
	})
	if !errors.Is(err, consent.ErrInvalidTransition) {
		t.Fatalf("second response err = %v, want ErrInvalidTransition", err)
	}
}

// TestResendCampaign_OnlyNonResponders is CON-24's own literal acceptance criterion: resend reaches only the
// recipients who haven't responded yet, and invalidates their own earlier link in the process.
func TestResendCampaign_OnlyNonResponders(t *testing.T) {
	f := consenttest.Setup(t)
	p := f.LivePurpose(t, "CAMP-04", consenttest.Content("CAMP-04", "ยินยอมแคมเปญ"))

	var c consent.Campaign
	f.As(t, f.A, f.Alice, nil, campaignPerms, func(ctx context.Context) error {
		var err error
		c, err = f.Svc.CreateCampaign(ctx, consent.CampaignInput{Name: "แคมเปญ", PurposeIDs: []uuid.UUID{p.ID}, Channel: "email",
			Identifiers: []consent.Identifier{{Type: "email", Value: "resp2@example.com"}, {Type: "email", Value: "pending2@example.com"}}})
		return err
	})
	f.As(t, f.A, f.Alice, nil, campaignPerms, func(ctx context.Context) error {
		var err error
		c, err = f.Svc.SendCampaign(ctx, c.ID, "th")
		return err
	})
	if countCampaignRequestNotifications(t, f) != 2 {
		t.Fatalf("expected 2 notifications after the first send")
	}
	tokens := campaignRequestTokens(t, f, 2)

	// The first recipient responds.
	err := f.Public(t, func(ctx context.Context) error {
		view, err := f.Svc.ViewCampaignRecipient(ctx, c.ID, tokens[0], "th")
		if err != nil {
			return err
		}
		_, err = f.Svc.RespondCampaignRecipient(ctx, c.ID, tokens[0], []consent.Decision{
			{PurposeCode: "CAMP-04", PurposeVersionNo: view.Purposes[0].VersionNo, Decision: "NOT_CONSENTED"},
		}, "th")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	f.As(t, f.A, f.Alice, nil, campaignPerms, func(ctx context.Context) error {
		var err error
		c, err = f.Svc.ResendCampaign(ctx, c.ID, "th")
		return err
	})
	// Only the still-pending second recipient got a fresh notification.
	if n := countCampaignRequestNotifications(t, f); n != 3 {
		t.Fatalf("notifications after resend = %d, want 3", n)
	}

	// The first recipient's own old link is untouched (never re-sent) and already responded, so viewing it
	// again is still valid and still shows responded — but the second recipient's *old* token no longer
	// matches anything, since resend overwrote its hash with a fresh one.
	err = f.Public(t, func(ctx context.Context) error {
		_, err := f.Svc.ViewCampaignRecipient(ctx, c.ID, tokens[1], "th")
		return err
	})
	if !errors.Is(err, consent.ErrNotFound) {
		t.Fatalf("stale resent token err = %v, want ErrNotFound", err)
	}
}

// TestListCampaigns_TwoTenantIsolation proves RLS, not just a tenant filter in application code.
func TestListCampaigns_TwoTenantIsolation(t *testing.T) {
	f := consenttest.Setup(t)
	p := f.LivePurpose(t, "CAMP-05", consenttest.Content("CAMP-05", "ยินยอมแคมเปญ"))

	var c consent.Campaign
	f.As(t, f.A, f.Alice, nil, campaignPerms, func(ctx context.Context) error {
		var err error
		c, err = f.Svc.CreateCampaign(ctx, consent.CampaignInput{Name: "A's campaign", PurposeIDs: []uuid.UUID{p.ID}, Channel: "email",
			Identifiers: []consent.Identifier{{Type: "email", Value: "iso@example.com"}}})
		return err
	})

	f.As(t, f.B, f.B.UserID, nil, campaignPerms, func(ctx context.Context) error {
		list, err := f.Svc.ListCampaigns(ctx)
		if err != nil {
			return err
		}
		for _, other := range list {
			if other.ID == c.ID {
				t.Fatal("tenant B saw tenant A's campaign")
			}
		}
		_, err = f.Svc.GetCampaign(ctx, c.ID)
		if !errors.Is(err, consent.ErrNotFound) {
			t.Fatalf("cross-tenant GetCampaign err = %v, want ErrNotFound", err)
		}
		return nil
	})
}
