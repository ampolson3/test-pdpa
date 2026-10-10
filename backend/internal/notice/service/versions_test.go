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

// completeContent is the same fully ม.23-compliant draft checklist_test.go's own publish test uses, reused
// here since PNG-06's own acceptance criterion needs a notice that actually reaches "published", not one
// blocked by PNG-02's gate.
func completeContent() render.Content {
	return render.Content{"th": {Type: "doc", Content: []render.Node{
		heading(noticeservice.TopicPurposeBasis, "วัตถุประสงค์"), para("จ่ายเงินเดือน (ฐาน: สัญญา)"),
		heading(noticeservice.TopicConsequence, "ผลกระทบ"), para("ท่านจะไม่ได้รับเงินเดือน"),
		heading(noticeservice.TopicData, "ข้อมูล"), para("ข้อมูลเงินเดือน"),
		heading(noticeservice.TopicRetention, "ระยะเวลา"), para("10 ปี"),
		heading(noticeservice.TopicRecipients, "ผู้รับ"), para("กรมสรรพากร"),
		heading(noticeservice.TopicContact, "ติดต่อ"), para("บริษัท ทดสอบ จำกัด อีเมล dpo@test.example"),
		heading(noticeservice.TopicRights, "สิทธิ"), para("ท่านมีสิทธิขอเข้าถึงข้อมูลของท่าน"),
	}}}
}

// publishNotice drives one notice's document all the way through PLT-08 (submit → DPO approve → publish) —
// the same chain checklist_test.go's own TestPublish_BlockedThenAllowed already established, factored out
// here so this file can publish (and, for the second-version test, re-publish) without duplicating that gate.
func publishNotice(t *testing.T, e env, n noticeservice.Notice, content render.Content) {
	t.Helper()
	authorPerms := append(append([]string{}, noticePermissions...), "notice.document.publish")
	dpoPerms := []string{"notice.document.read", "notice.document.update", "notice.document.approve", "notice.document.publish"}

	if err := e.inAs(e.tenant.UserID, authorPerms, nil, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, n.DocumentID)
		if err != nil {
			return err
		}
		_, err = e.docs.SaveDraft(ctx, n.DocumentID, doc.RowVersion, docsservice.Draft{Title: doc.Title, LegalEntityID: &n.LegalEntityID, Content: content})
		return err
	}); err != nil {
		t.Fatalf("save draft: %v", err)
	}

	var d struct {
		id      uuid.UUID
		version int32
	}
	if err := e.inAs(e.tenant.UserID, authorPerms, nil, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, n.DocumentID)
		if err != nil {
			return err
		}
		d.id, d.version = doc.Latest.ID, doc.Latest.RowVersion
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var v versioning.Version
	if err := e.inAs(e.tenant.UserID, authorPerms, nil, func(ctx context.Context) error {
		var err error
		v, err = e.ver.Submit(ctx, d.id, d.version)
		return err
	}); err != nil {
		t.Fatalf("submit: %v", err)
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
		t.Fatalf("approve: %v", err)
	}
	if err := e.inAs(e.dpo, dpoPerms, []string{"DPO"}, func(ctx context.Context) error {
		_, err := e.ver.Publish(ctx, v.ID, v.RowVersion)
		return err
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

// TestOnDocumentPublished_RecordsVersionAndIssuesKey is PNG-06's own acceptance criterion: publishing a
// notice's document writes one notice.notice_versions row carrying the ม.23 checklist it passed, flips the
// notice to published, and issues a public key the public page can resolve. A second publish keeps the same
// key (a bookmarked public URL must keep working) and adds a second version to the history.
func TestOnDocumentPublished_RecordsVersionAndIssuesKey(t *testing.T) {
	e := setup(t, "png06publish")
	var le orgservice.LegalEntity
	var n noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", NameEn: "Test Co., Ltd.",
			ContactEmail: "dpo@test.example", ContactPhone: "02-000-0000",
			Address:      orgservice.Address{Line1: "1 ถนนทดสอบ", District: "เขตทดสอบ", Province: "กรุงเทพมหานคร", PostalCode: "10110"},
			IsController: true}, 0)
		if err != nil {
			return err
		}
		n, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "ประกาศ PNG-06", Slug: "png06-publish"})
		return err
	})

	publishNotice(t, e, n, completeContent())

	var firstKey string
	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetNotice(ctx, n.ID)
		if err != nil {
			return err
		}
		if got.Status != "published" {
			t.Errorf("status = %q, want published", got.Status)
		}
		if got.PublicKey == nil {
			t.Fatal("expected a public key to be issued on first publish")
		}
		firstKey = *got.PublicKey

		versions, err := e.svc.ListNoticeVersions(ctx, n.ID)
		if err != nil {
			return err
		}
		if len(versions) != 1 {
			t.Fatalf("expected 1 version, got %d", len(versions))
		}
		v := versions[0]
		if v.VersionNo != 1 {
			t.Errorf("version_no = %d, want 1", v.VersionNo)
		}
		if len(v.Checklist) != 6 {
			t.Errorf("expected the 6-item ม.23 checklist snapshot, got %d", len(v.Checklist))
		}
		for _, it := range v.Checklist {
			if !it.Complete {
				t.Errorf("%s: expected complete in the snapshot", it.Code)
			}
		}
		if v.EffectiveFrom.IsZero() {
			t.Error("expected a non-zero effective_from (defaults to the publish moment)")
		}
		return nil
	})

	// Re-publish (a plain content edit + submit/approve/publish again): the key stays the same, a second
	// version is added.
	publishNotice(t, e, n, completeContent())
	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetNotice(ctx, n.ID)
		if err != nil {
			return err
		}
		if got.PublicKey == nil || *got.PublicKey != firstKey {
			t.Errorf("public key changed across publishes: got %v, want %q", got.PublicKey, firstKey)
		}
		versions, err := e.svc.ListNoticeVersions(ctx, n.ID)
		if err != nil {
			return err
		}
		if len(versions) != 2 {
			t.Fatalf("expected 2 versions after a second publish, got %d", len(versions))
		}
		if versions[0].VersionNo != 2 { // newest first
			t.Errorf("newest version_no = %d, want 2", versions[0].VersionNo)
		}
		return nil
	})
}

// TestPublicNotice_ServesCurrentAndHistory is the public-page half of PNG-06's acceptance criterion: once
// published, the notice's public key resolves the current version's rendered content and the full history,
// and an unpublished (or unknown) notice is never reachable this way.
func TestPublicNotice_ServesCurrentAndHistory(t *testing.T) {
	e := setup(t, "png06public")
	var le orgservice.LegalEntity
	var n noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", NameEn: "Test Co., Ltd.",
			ContactEmail: "dpo@test.example", ContactPhone: "02-000-0000",
			Address:      orgservice.Address{Line1: "1 ถนนทดสอบ", District: "เขตทดสอบ", Province: "กรุงเทพมหานคร", PostalCode: "10110"},
			IsController: true}, 0)
		if err != nil {
			return err
		}
		n, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "ประกาศสาธารณะ", Slug: "png06-public"})
		return err
	})

	// Not published yet: the public read refuses it even though a caller might already know the notice id.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.PublicNotice(ctx, n.ID); !errors.Is(err, noticeservice.ErrNotFound) {
			t.Errorf("unpublished notice: %v, want ErrNotFound", err)
		}
		return nil
	})

	publishNotice(t, e, n, completeContent())
	publishNotice(t, e, n, completeContent()) // a second version, so the history list has something to show

	e.in(t, func(ctx context.Context) error {
		pub, err := e.svc.PublicNotice(ctx, n.ID)
		if err != nil {
			t.Fatalf("PublicNotice: %v", err)
		}
		if pub.Title != n.Title {
			t.Errorf("title = %q, want %q", pub.Title, n.Title)
		}

		versions, err := e.svc.PublicVersions(ctx, n.ID)
		if err != nil {
			return err
		}
		if len(versions) != 2 {
			t.Fatalf("expected 2 published versions in the public history, got %d", len(versions))
		}

		html, err := e.svc.PublicVersionHTML(ctx, pub, nil, "th")
		if err != nil {
			t.Fatalf("PublicVersionHTML (current): %v", err)
		}
		if len(html) == 0 {
			t.Error("expected non-empty rendered HTML for the current version")
		}

		// The first version is still reachable by number — "ดูประวัติย้อนหลังได้" literally.
		first := versions[1].VersionNo // oldest, since ListNoticeVersions/PublicVersions order newest-first
		htmlFirst, err := e.svc.PublicVersionHTML(ctx, pub, &first, "th")
		if err != nil {
			t.Fatalf("PublicVersionHTML (first): %v", err)
		}
		if len(htmlFirst) == 0 {
			t.Error("expected non-empty rendered HTML for the first version too")
		}
		return nil
	})
}
