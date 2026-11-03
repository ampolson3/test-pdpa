package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	cookieservice "pdpa-platform/internal/cookie/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/docs/render"
)

// Cookie is what PNG-16 reads from the cookie module (rule 9): every registered domain, to cite each one's
// own live, always-current cookie-policy page (CON-05) in a generated org-level Cookie Policy — nil in a
// context that never composes one (the same nil-means-skip convention every other optional cross-cutting
// field on this Service already follows).
type Cookie interface {
	ListDomains(ctx context.Context) ([]cookieservice.Domain, error)
}

// orgPolicyTypes are the two notice_type values PNG-16 composes. Both were already in
// notice.notices.notice_type's own CHECK constraint since the very first baseline migration
// (00007_notice.sql), unused by any code until now.
var orgPolicyTypes = []string{"privacy_policy", "cookie_policy"}

// OrgPolicyWizardInput is PNG-16's own wizard: pick a legal entity and which org-level document to compose.
type OrgPolicyWizardInput struct {
	LegalEntityID uuid.UUID
	PolicyType    string // privacy_policy | cookie_policy
	Title         string
	Slug          string
}

// CreateOrgPolicyWizard composes PNG-16's own first draft and creates it the same way CreateWizard (PNG-01)
// does: a Privacy Policy citing every currently-published notice of this legal entity, or a Cookie Policy
// citing every one of its registered cookie domains' own live cookie-policy page — each as a plain-text
// citation of that page's own always-current public link, never a copy of its content, so the acceptance
// criterion ("อ้างถึงประกาศและตารางคุกกี้เวอร์ชันล่าสุด") holds by construction: the link itself is always
// the latest version, whatever gets republished after this document is composed.
func (s *Service) CreateOrgPolicyWizard(ctx context.Context, in OrgPolicyWizardInput) (Notice, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len([]rune(in.Title)) > 300 {
		return Notice{}, fmt.Errorf("%w: title", ErrInvalid)
	}
	in.Slug = strings.TrimSpace(in.Slug)
	if !slugRE.MatchString(in.Slug) || len(in.Slug) > 120 {
		return Notice{}, fmt.Errorf("%w: slug", ErrInvalid)
	}
	if !contains(orgPolicyTypes, in.PolicyType) {
		return Notice{}, fmt.Errorf("%w: policy_type", ErrInvalid)
	}
	legalEntity, err := s.Org.GetLegalEntity(ctx, in.LegalEntityID)
	if err != nil {
		return Notice{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}

	content, err := s.composeOrgPolicy(ctx, legalEntity, in.PolicyType)
	if err != nil {
		return Notice{}, err
	}
	return s.createNotice(ctx, in.LegalEntityID, nil, in.PolicyType, in.Title, in.Slug, content, nil)
}

// composeOrgPolicy builds the draft content: a contact section (the same merge-field shape PNG-01's own
// compose() already uses for its DPO/contact section) plus one reference section listing every source this
// policy_type covers, by title and live link; anything with nothing to cite yet becomes a bracketed
// placeholder, the same convention compose() already uses elsewhere.
func (s *Service) composeOrgPolicy(ctx context.Context, legalEntity orgservice.LegalEntity, policyType string) (render.Content, error) {
	intro := contactSection(legalEntity)
	intro.topic = "" // PNG-02's checklist only recognizes its own 6 ม.23 topics; an org policy isn't a ม.23 notice
	if policyType == "cookie_policy" {
		intro.headingTh, intro.headingEn = "ข้อมูลติดต่อผู้ควบคุมข้อมูลส่วนบุคคล", "Data controller contact details"
	} else {
		intro.headingTh, intro.headingEn = "ผู้ควบคุมข้อมูลส่วนบุคคลและเจ้าหน้าที่คุ้มครองข้อมูลส่วนบุคคล (DPO)", "Data controller and Data Protection Officer (DPO)"
	}

	var refSec section
	if policyType == "cookie_policy" {
		refSec.headingTh, refSec.headingEn = "นโยบายคุกกี้ของแต่ละเว็บไซต์", "Cookie policy for each website"
		if s.Cookie == nil {
			refSec.bodyTh = []string{"[โปรดระบุลิงก์นโยบายคุกกี้ของแต่ละเว็บไซต์]"}
			refSec.bodyEn = []string{"[Please list each website's own cookie-policy link]"}
		} else {
			domains, err := s.Cookie.ListDomains(ctx)
			if err != nil {
				return nil, err
			}
			for _, d := range domains {
				if d.LegalEntityID != legalEntity.ID || d.PublicKey == nil {
					continue // not this legal entity's own domain, or its banner has never published yet (CON-01)
				}
				refSec.bodyTh = append(refSec.bodyTh, fmt.Sprintf("%s: %s", d.Domain, s.cookiePolicyURL(*d.PublicKey, "th")))
				refSec.bodyEn = append(refSec.bodyEn, fmt.Sprintf("%s: %s", d.Domain, s.cookiePolicyURL(*d.PublicKey, "en")))
			}
		}
		if len(refSec.bodyTh) == 0 {
			refSec.bodyTh = []string{"[ยังไม่มีเว็บไซต์ที่เผยแพร่นโยบายคุกกี้ของนิติบุคคลนี้]"}
			refSec.bodyEn = []string{"[No website of this legal entity has a published cookie banner yet]"}
		}
	} else {
		refSec.headingTh, refSec.headingEn = "ประกาศความเป็นส่วนตัวที่เกี่ยวข้อง", "Related privacy notices"
		notices, _, err := s.ListNotices(ctx, NoticeFilter{Status: "published", LegalEntityID: &legalEntity.ID, Limit: noticePageSize})
		if err != nil {
			return nil, err
		}
		for _, n := range notices {
			if n.NoticeType == policyType || n.PublicKey == nil {
				continue // never cite itself (another already-published org policy), nor an unpublished notice
			}
			versionNo := int32(0)
			if versions, err := s.ListNoticeVersions(ctx, n.ID); err == nil && len(versions) > 0 {
				versionNo = versions[0].VersionNo // newest first, PNG-08's own convention
			}
			refSec.bodyTh = append(refSec.bodyTh, fmt.Sprintf("%s (v%d): %s", n.Title, versionNo, s.noticeURL(*n.PublicKey, "th")))
			refSec.bodyEn = append(refSec.bodyEn, fmt.Sprintf("%s (v%d): %s", n.Title, versionNo, s.noticeURL(*n.PublicKey, "en")))
		}
		if len(refSec.bodyTh) == 0 {
			refSec.bodyTh = []string{"[ยังไม่มีประกาศความเป็นส่วนตัวที่เผยแพร่ของนิติบุคคลนี้]"}
			refSec.bodyEn = []string{"[No privacy notice of this legal entity has been published yet]"}
		}
	}

	sections := []section{intro, refSec, rightsBoilerplate}
	return render.Content{"th": docNode(sections, "th"), "en": docNode(sections, "en")}, nil
}

// noticeURL is PNG-12's own cctv_sign.go link-building, reused here for the same PortalBaseURL config.
func (s *Service) noticeURL(key, lang string) string {
	return strings.TrimRight(s.PortalBaseURL, "/") + "/" + lang + "/n/" + key
}

// cookiePolicyURL is the public cookie-policy page's own path (CON-05, apps/portal's /[locale]/cookie-policy/[key]).
func (s *Service) cookiePolicyURL(key, lang string) string {
	return strings.TrimRight(s.PortalBaseURL, "/") + "/" + lang + "/cookie-policy/" + key
}
