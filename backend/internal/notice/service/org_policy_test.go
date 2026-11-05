package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	cookieservice "pdpa-platform/internal/cookie/service"
	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
)

// fakeCookie is PNG-16's own test double for the Cookie interface — no real cookie module is wired into
// this package's test env, so this stands in for it the same way other modules' tests stub a small
// cross-module read (rule 9).
type fakeCookie struct{ domains []cookieservice.Domain }

func (f fakeCookie) ListDomains(ctx context.Context) ([]cookieservice.Domain, error) {
	return f.domains, nil
}

func pubKey(s string) *string { return &s }

// TestCreateOrgPolicyWizard_PrivacyPolicy_CitesPublishedNoticeAtLatestVersion is PNG-16's own acceptance
// criterion: the generated policy cites a published notice, by title and version, at its own always-current
// public link — and never a still-unpublished one.
func TestCreateOrgPolicyWizard_PrivacyPolicy_CitesPublishedNoticeAtLatestVersion(t *testing.T) {
	e := setup(t, "png16pub")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", NameEn: "Test Co., Ltd.",
			ContactEmail: "dpo@test.example", ContactPhone: "02-000-0000",
			Address:      orgservice.Address{Line1: "1 ถนนทดสอบ", District: "เขตทดสอบ", Province: "กรุงเทพมหานคร", PostalCode: "10110"},
			IsController: true}, 0)
		return err
	})

	var published noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		published, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "ประกาศลูกค้า", Slug: "png16-customer"})
		return err
	})
	publishNotice(t, e, published, completeContent())

	var draftOnly noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		draftOnly, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "employee", Title: "ประกาศพนักงาน ยังไม่เผยแพร่", Slug: "png16-employee-draft"})
		return err
	})
	if draftOnly.Status != "draft" {
		t.Fatalf("expected the second notice to stay draft, got %q", draftOnly.Status)
	}

	var policy noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		policy, err = e.svc.CreateOrgPolicyWizard(ctx, noticeservice.OrgPolicyWizardInput{
			LegalEntityID: le.ID, PolicyType: "privacy_policy", Title: "นโยบายความเป็นส่วนตัวระดับองค์กร", Slug: "png16-privacy-policy",
		})
		return err
	})
	if policy.NoticeType != "privacy_policy" || policy.Status != "draft" {
		t.Fatalf("got %+v, want notice_type=privacy_policy status=draft", policy)
	}

	e.in(t, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, policy.DocumentID)
		if err != nil {
			return err
		}
		var sb strings.Builder
		plainText(doc.Draft.Content["th"], &sb)
		text := sb.String()
		if !strings.Contains(text, "ประกาศลูกค้า") {
			t.Errorf("expected the published notice's own title cited, got: %s", text)
		}
		if !strings.Contains(text, "v1") {
			t.Errorf("expected the published notice's own version number (v1) cited, got: %s", text)
		}
		if !strings.Contains(text, "/th/n/") {
			t.Errorf("expected the published notice's own live public link cited, got: %s", text)
		}
		if strings.Contains(text, "ประกาศพนักงาน ยังไม่เผยแพร่") {
			t.Error("an unpublished (draft-only) notice must never be cited")
		}
		return nil
	})
}

// TestCreateOrgPolicyWizard_PrivacyPolicy_NoPublishedNoticeIsPlaceholder: a legal entity with nothing
// published yet gets a bracketed placeholder, the same convention PNG-01's own compose() uses elsewhere —
// never a silently empty section.
func TestCreateOrgPolicyWizard_PrivacyPolicy_NoPublishedNoticeIsPlaceholder(t *testing.T) {
	e := setup(t, "png16empty")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		return err
	})

	var policy noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		policy, err = e.svc.CreateOrgPolicyWizard(ctx, noticeservice.OrgPolicyWizardInput{
			LegalEntityID: le.ID, PolicyType: "privacy_policy", Title: "นโยบาย", Slug: "png16-empty",
		})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, policy.DocumentID)
		if err != nil {
			return err
		}
		var sb strings.Builder
		plainText(doc.Draft.Content["th"], &sb)
		if !strings.Contains(sb.String(), "[") {
			t.Errorf("expected a bracketed placeholder, got: %s", sb.String())
		}
		return nil
	})
}

// TestCreateOrgPolicyWizard_CookiePolicy_CitesOwnDomainsWithAPublishedBanner is the Cookie Policy half of
// the acceptance criterion: a composed Cookie Policy cites each of this legal entity's own registered
// domains, only once that domain's own banner has published at least once (CON-01) — never another legal
// entity's domain, and never one with no public key yet.
func TestCreateOrgPolicyWizard_CookiePolicy_CitesOwnDomainsWithAPublishedBanner(t *testing.T) {
	e := setup(t, "png16cookie")
	var leA, leB orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		leA, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		leB, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท บี จำกัด", IsController: true}, 0)
		return err
	})

	e.svc.Cookie = fakeCookie{domains: []cookieservice.Domain{
		{ID: uuid.Must(uuid.NewV7()), Domain: "shop.example.com", LegalEntityID: leA.ID, PublicKey: pubKey("ckkey1")},
		{ID: uuid.Must(uuid.NewV7()), Domain: "unpublished.example.com", LegalEntityID: leA.ID, PublicKey: nil},
		{ID: uuid.Must(uuid.NewV7()), Domain: "other-entity.example.com", LegalEntityID: leB.ID, PublicKey: pubKey("ckkey2")},
	}}

	var policy noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		policy, err = e.svc.CreateOrgPolicyWizard(ctx, noticeservice.OrgPolicyWizardInput{
			LegalEntityID: leA.ID, PolicyType: "cookie_policy", Title: "นโยบายคุกกี้", Slug: "png16-cookie-policy",
		})
		return err
	})
	if policy.NoticeType != "cookie_policy" {
		t.Fatalf("notice_type = %q, want cookie_policy", policy.NoticeType)
	}

	e.in(t, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, policy.DocumentID)
		if err != nil {
			return err
		}
		var sb strings.Builder
		plainText(doc.Draft.Content["th"], &sb)
		text := sb.String()
		if !strings.Contains(text, "shop.example.com") || !strings.Contains(text, "/th/cookie-policy/ckkey1") {
			t.Errorf("expected the published domain's own cookie-policy link cited, got: %s", text)
		}
		if strings.Contains(text, "unpublished.example.com") {
			t.Error("a domain with no published banner yet must never be cited")
		}
		if strings.Contains(text, "other-entity.example.com") {
			t.Error("another legal entity's domain must never be cited")
		}
		return nil
	})
}

// TestCreateOrgPolicyWizard_Validation covers the input checks before any write happens.
func TestCreateOrgPolicyWizard_Validation(t *testing.T) {
	e := setup(t, "png16val")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		return err
	})

	cases := []struct {
		name string
		in   noticeservice.OrgPolicyWizardInput
	}{
		{"empty title", noticeservice.OrgPolicyWizardInput{LegalEntityID: le.ID, PolicyType: "privacy_policy", Title: "", Slug: "x"}},
		{"bad slug", noticeservice.OrgPolicyWizardInput{LegalEntityID: le.ID, PolicyType: "privacy_policy", Title: "x", Slug: "Not A Slug"}},
		{"bad policy_type", noticeservice.OrgPolicyWizardInput{LegalEntityID: le.ID, PolicyType: "privacy_notice", Title: "x", Slug: "png16-badtype"}},
		{"unknown legal entity", noticeservice.OrgPolicyWizardInput{LegalEntityID: uuid.Must(uuid.NewV7()), PolicyType: "privacy_policy", Title: "x", Slug: "png16-badle"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e.in(t, func(ctx context.Context) error {
				if _, err := e.svc.CreateOrgPolicyWizard(ctx, c.in); !errors.Is(err, noticeservice.ErrInvalid) {
					t.Errorf("err = %v, want ErrInvalid", err)
				}
				return nil
			})
		})
	}
}

// TestCreateOrgPolicyWizard_TwoTenantIsolation: tenant B's composed policy never cites tenant A's own
// published notice, even naming the same (coincidentally identical) legal-entity id would be impossible —
// RLS on notice.notices already scopes ListNotices, proven here end to end through the composer itself.
func TestCreateOrgPolicyWizard_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "png16isoa")
	b := setup(t, "png16isob")

	var leA orgservice.LegalEntity
	a.in(t, func(ctx context.Context) error {
		var err error
		leA, err = a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอ จำกัด", IsController: true}, 0)
		return err
	})
	var noticeA noticeservice.Notice
	a.in(t, func(ctx context.Context) error {
		var err error
		noticeA, err = a.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: leA.ID, NoticeType: "privacy_notice", Title: "ประกาศลับของเอ", Slug: "png16-isoa"})
		return err
	})
	publishNotice(t, a, noticeA, completeContent())

	var leB orgservice.LegalEntity
	b.in(t, func(ctx context.Context) error {
		var err error
		leB, err = b.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท บี จำกัด", IsController: true}, 0)
		return err
	})
	var policyB noticeservice.Notice
	b.in(t, func(ctx context.Context) error {
		var err error
		policyB, err = b.svc.CreateOrgPolicyWizard(ctx, noticeservice.OrgPolicyWizardInput{
			LegalEntityID: leB.ID, PolicyType: "privacy_policy", Title: "นโยบาย บี", Slug: "png16-isob",
		})
		return err
	})

	b.in(t, func(ctx context.Context) error {
		doc, err := b.docs.Get(ctx, policyB.DocumentID)
		if err != nil {
			return err
		}
		var sb strings.Builder
		plainText(doc.Draft.Content["th"], &sb)
		if strings.Contains(sb.String(), "ประกาศลับของเอ") {
			t.Error("tenant B's composed policy cited tenant A's own notice")
		}
		return nil
	})
}
