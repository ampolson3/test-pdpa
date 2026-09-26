package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
	docsservice "pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/versioning"
)

func heading(topic, text string) render.Node {
	attrs := map[string]any{"level": float64(2)}
	if topic != "" {
		attrs["topic"] = topic
	}
	return render.Node{Type: "heading", Attrs: attrs, Content: []render.Node{{Type: "text", Text: text}}}
}

func para(text string) render.Node {
	return render.Node{Type: "paragraph", Content: []render.Node{{Type: "text", Text: text}}}
}

// TestChecklist_PureFunction covers PNG-02's completeness rule directly, without a database: a topic is
// complete only when its heading carries the topic code and the text under it isn't empty or a bracketed
// placeholder; data_retention needs both its halves; a document with no topic-coded headings at all (built
// outside the wizard) reads as every topic missing.
func TestChecklist_PureFunction(t *testing.T) {
	complete := render.Content{"th": {Type: "doc", Content: []render.Node{
		heading(noticeservice.TopicPurposeBasis, "วัตถุประสงค์"), para("จ่ายเงินเดือน (ฐาน: สัญญา)"),
		heading(noticeservice.TopicConsequence, "ผลกระทบ"), para("ท่านจะไม่ได้รับเงินเดือน"),
		heading(noticeservice.TopicData, "ข้อมูล"), para("ข้อมูลเงินเดือน"),
		heading(noticeservice.TopicRetention, "ระยะเวลา"), para("10 ปี"),
		heading(noticeservice.TopicRecipients, "ผู้รับ"), para("กรมสรรพากร"),
		heading(noticeservice.TopicContact, "ติดต่อ"), para("บริษัท ก. อีเมล a@b.com"),
		heading(noticeservice.TopicRights, "สิทธิ"), para("ท่านมีสิทธิ..."),
	}}}
	items := noticeservice.Checklist(complete)
	if len(items) != 6 {
		t.Fatalf("expected 6 checklist items, got %d", len(items))
	}
	for _, it := range items {
		if !it.Complete {
			t.Errorf("%s: expected complete", it.Code)
		}
	}

	t.Run("placeholder blocks its topic", func(t *testing.T) {
		content := render.Content{"th": {Type: "doc", Content: []render.Node{
			heading(noticeservice.TopicPurposeBasis, "วัตถุประสงค์"), para("[โปรดระบุวัตถุประสงค์และฐานทางกฎหมาย]"),
			heading(noticeservice.TopicConsequence, "ผลกระทบ"), para("ท่านจะไม่ได้รับเงินเดือน"),
			heading(noticeservice.TopicData, "ข้อมูล"), para("ข้อมูลเงินเดือน"),
			heading(noticeservice.TopicRetention, "ระยะเวลา"), para("10 ปี"),
			heading(noticeservice.TopicRecipients, "ผู้รับ"), para("กรมสรรพากร"),
			heading(noticeservice.TopicContact, "ติดต่อ"), para("บริษัท ก."),
			heading(noticeservice.TopicRights, "สิทธิ"), para("ท่านมีสิทธิ..."),
		}}}
		items := noticeservice.Checklist(content)
		for _, it := range items {
			want := it.Code != "purpose_basis"
			if it.Complete != want {
				t.Errorf("%s: complete=%v, want %v", it.Code, it.Complete, want)
			}
		}
	})

	t.Run("data_retention needs both halves", func(t *testing.T) {
		content := render.Content{"th": {Type: "doc", Content: []render.Node{
			heading(noticeservice.TopicData, "ข้อมูล"), para("ข้อมูลเงินเดือน"),
			heading(noticeservice.TopicRetention, "ระยะเวลา"), para("[โปรดระบุระยะเวลาการเก็บรักษาข้อมูล]"),
		}}}
		items := noticeservice.Checklist(content)
		for _, it := range items {
			if it.Code == "data_retention" && it.Complete {
				t.Error("data_retention should be incomplete when retention is still a placeholder")
			}
		}
	})

	t.Run("no topic-coded headings at all: everything missing", func(t *testing.T) {
		content := render.Content{"th": {Type: "doc", Content: []render.Node{
			heading("", "หัวข้อที่เขียนเอง"), para("เนื้อหาที่เขียนเอง"),
		}}}
		for _, it := range noticeservice.Checklist(content) {
			if it.Complete {
				t.Errorf("%s: expected incomplete for a document with no wizard topic codes", it.Code)
			}
		}
	})
}

// TestCreateWizard_ChecklistReflectsPlaceholders is GetChecklist's own read path: with no linked activity,
// only the topics compose.go has no real fallback text for (purposes, consequence, data_retention) still read
// as missing — recipients defaults to a real "no third-party disclosure" statement and contact/rights are
// always filled in, so those three are complete even with nothing linked.
func TestCreateWizard_ChecklistReflectsPlaceholders(t *testing.T) {
	e := setup(t, "pngchecklist")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		return err
	})
	var n noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		n, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "x", Slug: "checklist-empty"})
		return err
	})
	stillMissing := map[string]bool{"purpose_basis": true, "consequence": true, "data_retention": true}
	e.in(t, func(ctx context.Context) error {
		items, err := e.svc.GetChecklist(ctx, n.ID)
		if err != nil {
			return err
		}
		for _, it := range items {
			if it.Complete == stillMissing[it.Code] {
				t.Errorf("%s: complete=%v, want %v", it.Code, it.Complete, !stillMissing[it.Code])
			}
		}
		return nil
	})
}

// TestPublish_BlockedThenAllowed is PNG-02's acceptance criterion end to end, through the real PLT-08
// submit → DPO approve → publish flow: a notice missing ม.23 topics can't be published; a notice whose content
// is complete before submission (BP-04 step t3, editing the draft, happens before t4/g1 in the real flow)
// publishes normally. Two separate notices, not one edited in place after a blocked publish — once a PLT-08
// version reaches "approved" it's locked against further edits (SaveDraft refuses anything but the open draft),
// so recovering from a blocked publish is itself a return-to-draft action outside this feature's scope; PNG-02
// only needs to prove the gate fires, which either notice's own first attempt already shows.
func TestPublish_BlockedThenAllowed(t *testing.T) {
	e := setup(t, "pngpublish")
	var le orgservice.LegalEntity
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", NameEn: "Test Co., Ltd.",
			ContactEmail: "dpo@test.example", ContactPhone: "02-000-0000",
			Address:      orgservice.Address{Line1: "1 ถนนทดสอบ", District: "เขตทดสอบ", Province: "กรุงเทพมหานคร", PostalCode: "10110"},
			IsController: true}, 0)
		return err
	})

	authorPerms := append(append([]string{}, noticePermissions...), "notice.document.publish")
	dpoPerms := []string{"notice.document.read", "notice.document.update", "notice.document.approve", "notice.document.publish"}

	// submitApprovePublish drives one document all the way through PLT-08 (submit → DPO decide → publish) and
	// returns whatever error, if any, the final Publish call produced.
	submitApprovePublish := func(documentID uuid.UUID) error {
		var d struct {
			id      uuid.UUID
			version int32
		}
		if err := e.inAs(e.tenant.UserID, authorPerms, nil, func(ctx context.Context) error {
			doc, err := e.docs.Get(ctx, documentID)
			if err != nil {
				return err
			}
			d.id, d.version = doc.Latest.ID, doc.Latest.RowVersion
			return nil
		}); err != nil {
			return err
		}
		var v versioning.Version
		if err := e.inAs(e.tenant.UserID, authorPerms, nil, func(ctx context.Context) error {
			var err error
			v, err = e.ver.Submit(ctx, d.id, d.version)
			return err
		}); err != nil {
			return err
		}
		if err := e.inAs(e.dpo, dpoPerms, []string{"DPO"}, func(ctx context.Context) error {
			inbox, err := e.ver.Inbox(ctx)
			if err != nil {
				return err
			}
			for _, it := range inbox {
				if it.VersionID == v.ID {
					v, err = e.ver.Decide(ctx, it.ID, it.RowVersion, "approved", "")
					return err
				}
			}
			t.Fatal("submitted version not in the DPO's inbox")
			return nil
		}); err != nil {
			return err
		}
		return e.inAs(e.dpo, dpoPerms, []string{"DPO"}, func(ctx context.Context) error {
			_, err := e.ver.Publish(ctx, v.ID, v.RowVersion)
			return err
		})
	}

	var incomplete noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		incomplete, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "ประกาศที่ยังไม่ครบ", Slug: "publish-blocked"})
		return err
	})
	err := submitApprovePublish(incomplete.DocumentID)
	var inc *noticeservice.ErrChecklistIncomplete
	if !errors.As(err, &inc) || !errors.Is(err, versioning.ErrInvalidRequest) {
		t.Fatalf("publish with placeholders left: err = %v, want ErrChecklistIncomplete", err)
	}
	if len(inc.Missing) == 0 {
		t.Error("expected a non-empty Missing list")
	}

	var complete noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		complete, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "ประกาศที่ครบแล้ว", Slug: "publish-allowed"})
		return err
	})
	// Fill in every section before submitting (BP-04 t3, ahead of t4's checklist gate in the real flow).
	content := render.Content{"th": {Type: "doc", Content: []render.Node{
		heading(noticeservice.TopicPurposeBasis, "วัตถุประสงค์"), para("จ่ายเงินเดือน (ฐาน: สัญญา)"),
		heading(noticeservice.TopicConsequence, "ผลกระทบ"), para("ท่านจะไม่ได้รับเงินเดือน"),
		heading(noticeservice.TopicData, "ข้อมูล"), para("ข้อมูลเงินเดือน"),
		heading(noticeservice.TopicRetention, "ระยะเวลา"), para("10 ปี"),
		heading(noticeservice.TopicRecipients, "ผู้รับ"), para("กรมสรรพากร"),
		heading(noticeservice.TopicContact, "ติดต่อ"), para("บริษัท ทดสอบ จำกัด อีเมล dpo@test.example"),
		heading(noticeservice.TopicRights, "สิทธิ"), para("ท่านมีสิทธิขอเข้าถึงข้อมูลของท่าน"),
	}}}
	if err := e.inAs(e.tenant.UserID, authorPerms, nil, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, complete.DocumentID)
		if err != nil {
			return err
		}
		_, err = e.docs.SaveDraft(ctx, complete.DocumentID, doc.RowVersion, docsservice.Draft{Title: doc.Title, LegalEntityID: &le.ID, Content: content})
		return err
	}); err != nil {
		t.Fatalf("save complete draft: %v", err)
	}
	if err := submitApprovePublish(complete.DocumentID); err != nil {
		t.Fatalf("publish (complete): %v", err)
	}
}

// TestCheckPublishable_ConfigurableOff: EnforceChecklist=false (the tunable's non-default) skips the gate
// entirely — publish succeeds even with placeholders left. Exercises CheckPublishable directly rather than
// the full submit/approve/publish chain, since that's all this particular behaviour touches.
func TestCheckPublishable_ConfigurableOff(t *testing.T) {
	e := setup(t, "pngchecklistoff")
	e.svc.EnforceChecklist = false
	incomplete := render.Content{"th": {Type: "doc", Content: []render.Node{
		heading(noticeservice.TopicPurposeBasis, "วัตถุประสงค์"), para("[โปรดระบุวัตถุประสงค์และฐานทางกฎหมาย]"),
	}}}
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.CheckPublishable(ctx, uuid.New(), docsservice.Draft{Content: incomplete}); err != nil {
			t.Errorf("expected no error with EnforceChecklist=false, got %v", err)
		}
		return nil
	})
}
