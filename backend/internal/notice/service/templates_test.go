package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
)

// TestListTemplateGroups_CoversAllEightGroups: PNG-03 lists all 8 fixed data-subject groups the seed migration
// (00042) provides — the wizard's picker relies on this to know what to offer.
func TestListTemplateGroups_CoversAllEightGroups(t *testing.T) {
	e := setup(t, "pngtplgroups")
	var groups []string
	e.in(t, func(ctx context.Context) error {
		var err error
		groups, err = e.svc.ListTemplateGroups(ctx)
		return err
	})
	if len(groups) != len(noticeservice.TemplateGroups) {
		t.Fatalf("groups = %v, want %d groups", groups, len(noticeservice.TemplateGroups))
	}
	for _, want := range noticeservice.TemplateGroups {
		found := false
		for _, g := range groups {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing group %q", want)
		}
	}
}

// TestCreateWizard_TemplateGroup_ImmediateDraft is PNG-03's acceptance criterion: picking a template group
// immediately produces a draft notice for that group, with no RoPA activity or manual authoring needed.
func TestCreateWizard_TemplateGroup_ImmediateDraft(t *testing.T) {
	e := setup(t, "pngtplwiz")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		return err
	})

	var n noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		n, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "employee",
			Title: "ประกาศความเป็นส่วนตัวสำหรับพนักงาน", Slug: "employee-template-notice", TemplateGroup: "employee"})
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
		if !strings.Contains(text, "พนักงาน") {
			t.Error("expected the employee group's own sample text")
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

func TestCreateWizard_TemplateGroup_Validation(t *testing.T) {
	e := setup(t, "pngtplval")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice",
			Title: "x", Slug: "unknown-group", TemplateGroup: "not_a_real_group"})
		if !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("err = %v, want ErrInvalid", err)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		var activityID = uuid.New()
		_, err := e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice",
			Title: "x", Slug: "both-sources", TemplateGroup: "customer", ActivityIDs: []uuid.UUID{activityID}})
		if !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("err = %v, want ErrInvalid (mutually exclusive)", err)
		}
		return nil
	})
}
