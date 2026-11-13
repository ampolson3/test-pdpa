package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
)

// TestListIndustryTemplates_CoversAllSixIndustries: PNG-11's own acceptance criterion — the seed migration
// (00081) provides a starter template for all 6 fixed industries, in both th/en, which the wizard's industry
// picker lists through this method.
func TestListIndustryTemplates_CoversAllSixIndustries(t *testing.T) {
	e := setup(t, "pngindtpl")
	var industries []string
	e.in(t, func(ctx context.Context) error {
		var err error
		industries, err = e.svc.ListIndustryTemplates(ctx)
		return err
	})
	if len(industries) != len(noticeservice.IndustryTemplates) {
		t.Fatalf("industries = %v, want %d", industries, len(noticeservice.IndustryTemplates))
	}
	for _, want := range noticeservice.IndustryTemplates {
		found := false
		for _, g := range industries {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing industry %q", want)
		}
	}
}

// TestListTemplateGroups_ExcludesIndustries proves the two pickers (PNG-03's data-subject groups and PNG-11's
// industries) stay on separate axes of the same notice.wizard_templates table — neither list leaks into the
// other, even though both are looked up by the identical mechanism (subject_type_code).
func TestListTemplateGroups_ExcludesIndustries(t *testing.T) {
	e := setup(t, "pngindtplaxes")
	var groups, industries []string
	e.in(t, func(ctx context.Context) error {
		var err error
		groups, err = e.svc.ListTemplateGroups(ctx)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		var err error
		industries, err = e.svc.ListIndustryTemplates(ctx)
		return err
	})
	for _, ind := range industries {
		for _, g := range groups {
			if g == ind {
				t.Errorf("industry %q leaked into the data-subject group list", ind)
			}
		}
	}
}

// TestCreateWizard_IndustryTemplate_ImmediateDraft is PNG-11's own wizard-side acceptance criterion: picking
// an industry (through the exact same template_group field PNG-03 already uses) immediately produces a draft
// carrying that industry's own sample text and DRAFT marker in both languages.
func TestCreateWizard_IndustryTemplate_ImmediateDraft(t *testing.T) {
	e := setup(t, "pngindtplwiz")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		return err
	})

	var n noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		n, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice",
			Title: "ประกาศความเป็นส่วนตัวสำหรับผู้ป่วย", Slug: "hospital-template-notice", TemplateGroup: "hospital"})
		return err
	})
	if n.Status != "draft" {
		t.Fatalf("status = %q, want draft", n.Status)
	}
	if n.DocumentID == uuid.Nil {
		t.Fatal("expected a document to be created")
	}

	e.in(t, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, n.DocumentID)
		if err != nil {
			return err
		}
		if doc.Draft == nil {
			t.Fatal("expected a draft")
		}
		var sb strings.Builder
		plainText(doc.Draft.Content["th"], &sb)
		text := sb.String()
		if !strings.Contains(text, "ผู้ป่วย") {
			t.Error("expected the hospital industry's own sample text")
		}
		if !strings.Contains(text, "ร่าง") {
			t.Error("expected the DRAFT marker on template-sourced content")
		}
		var sbEn strings.Builder
		plainText(doc.Draft.Content["en"], &sbEn)
		if !strings.Contains(sbEn.String(), "DRAFT") {
			t.Error("expected the DRAFT marker on the English template content too")
		}
		return nil
	})
}
