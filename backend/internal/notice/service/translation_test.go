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

func bilingualContent(purposeTh, purposeEn string) render.Content {
	sections := []struct{ topic, th, en string }{
		{noticeservice.TopicPurposeBasis, purposeTh, purposeEn},
		{noticeservice.TopicConsequence, "ท่านจะไม่ได้รับเงินเดือน", "You will not be paid"},
		{noticeservice.TopicData, "ข้อมูลเงินเดือน", "Payroll data"},
		{noticeservice.TopicRetention, "10 ปี", "10 years"},
		{noticeservice.TopicRecipients, "กรมสรรพากร", "Revenue Department"},
		{noticeservice.TopicContact, "บริษัท ทดสอบ จำกัด อีเมล dpo@test.example", "Test Co., Ltd. e-mail dpo@test.example"},
		{noticeservice.TopicRights, "ท่านมีสิทธิขอเข้าถึงข้อมูลของท่าน", "You have the right to access your data"},
	}
	var th, en []render.Node
	for _, s := range sections {
		th = append(th, heading(s.topic, s.th), para(s.th))
		en = append(en, heading(s.topic, s.en), para(s.en))
	}
	return render.Content{"th": {Type: "doc", Content: th}, "en": {Type: "doc", Content: en}}
}

// TestStaleTranslation_PureFunction covers PNG-05's rule directly: stale only when Thai changed but English
// did not; adding or removing English entirely is never itself a staleness signal.
func TestStaleTranslation_PureFunction(t *testing.T) {
	base := bilingualContent("จ่ายเงินเดือน", "Pay salary")
	changedTh := bilingualContent("จ่ายเงินเดือนและโบนัส", "Pay salary")
	changedBoth := bilingualContent("จ่ายเงินเดือนและโบนัส", "Pay salary and bonus")
	thOnly := render.Content{"th": base["th"]}

	cases := []struct {
		name              string
		current, previous render.Content
		want              bool
	}{
		{"th changed, en unchanged: stale", changedTh, base, true},
		{"both changed: not stale", changedBoth, base, false},
		{"neither changed: not stale", base, base, false},
		{"english newly added: not stale", base, thOnly, false},
		{"english removed: not stale", thOnly, base, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := noticeservice.StaleTranslation(c.current, c.previous); got != c.want {
				t.Errorf("StaleTranslation() = %v, want %v", got, c.want)
			}
		})
	}
}

// saveDraft replaces a document's open draft with new content, as the author.
func saveDraft(t *testing.T, e env, authorPerms []string, documentID uuid.UUID, legalEntity uuid.UUID, content render.Content) {
	t.Helper()
	if err := e.inAs(e.tenant.UserID, authorPerms, nil, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, documentID)
		if err != nil {
			return err
		}
		_, err = e.docs.SaveDraft(ctx, documentID, doc.RowVersion, docsservice.Draft{Title: doc.Title, LegalEntityID: &legalEntity, Content: content})
		return err
	}); err != nil {
		t.Fatalf("save draft: %v", err)
	}
}

// runSubmitApprovePublish drives one document through the real PLT-08 submit → DPO approve → publish chain
// and returns the final Publish error, if any.
func runSubmitApprovePublish(t *testing.T, e env, documentID uuid.UUID, authorPerms, dpoPerms []string) error {
	t.Helper()
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
		t.Fatalf("get: %v", err)
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
		t.Fatalf("decide: %v", err)
	}
	return e.inAs(e.dpo, dpoPerms, []string{"DPO"}, func(ctx context.Context) error {
		_, err := e.ver.Publish(ctx, v.ID, v.RowVersion)
		return err
	})
}

// TestPublish_TranslationSync is PNG-05's acceptance criterion end to end, through the real PLT-08 submit → DPO
// approve → publish chain: publishing a second version with an edited Thai section but an untouched English one
// is blocked; publishing a second version where both languages were updated together succeeds. Two independent
// notices (as in PNG-02's own publish test), since a version blocked at publish stays locked at "approved" —
// recovering it isn't in scope here, only proving each gate fires.
func TestPublish_TranslationSync(t *testing.T) {
	e := setup(t, "pngtranslation")
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

	// --- Blocked: Thai edited, English left as-is. ---
	var blocked noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		blocked, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "ประกาศทดสอบ 1", Slug: "translation-blocked"})
		return err
	})
	saveDraft(t, e, authorPerms, blocked.DocumentID, le.ID, bilingualContent("จ่ายเงินเดือน", "Pay salary"))
	if err := runSubmitApprovePublish(t, e, blocked.DocumentID, authorPerms, dpoPerms); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	saveDraft(t, e, authorPerms, blocked.DocumentID, le.ID, bilingualContent("จ่ายเงินเดือนและโบนัส", "Pay salary"))
	err := runSubmitApprovePublish(t, e, blocked.DocumentID, authorPerms, dpoPerms)
	var stale *noticeservice.ErrTranslationStale
	if !errors.As(err, &stale) || !errors.Is(err, versioning.ErrInvalidRequest) {
		t.Fatalf("publish with stale translation: err = %v, want ErrTranslationStale", err)
	}

	// --- Allowed: both languages updated together. ---
	var allowed noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		allowed, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "ประกาศทดสอบ 2", Slug: "translation-allowed"})
		return err
	})
	saveDraft(t, e, authorPerms, allowed.DocumentID, le.ID, bilingualContent("จ่ายเงินเดือน", "Pay salary"))
	if err := runSubmitApprovePublish(t, e, allowed.DocumentID, authorPerms, dpoPerms); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	saveDraft(t, e, authorPerms, allowed.DocumentID, le.ID, bilingualContent("จ่ายเงินเดือนและโบนัส", "Pay salary and bonus"))
	if err := runSubmitApprovePublish(t, e, allowed.DocumentID, authorPerms, dpoPerms); err != nil {
		t.Fatalf("second publish (both languages updated): %v", err)
	}
}

// TestGetTranslationStatus is the read-side check GetChecklist's own read path mirrors: before any publish it's
// always false (nothing to compare against yet); after a Thai-only edit against a published version it's true.
func TestGetTranslationStatus(t *testing.T) {
	e := setup(t, "pngtranslationstatus")
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

	var n noticeservice.Notice
	e.in(t, func(ctx context.Context) error {
		var err error
		n, err = e.svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "x", Slug: "translation-status"})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		stale, err := e.svc.GetTranslationStatus(ctx, n.ID)
		if err != nil {
			return err
		}
		if stale {
			t.Error("expected not stale before any publish")
		}
		return nil
	})

	saveDraft(t, e, authorPerms, n.DocumentID, le.ID, bilingualContent("จ่ายเงินเดือน", "Pay salary"))
	if err := runSubmitApprovePublish(t, e, n.DocumentID, authorPerms, dpoPerms); err != nil {
		t.Fatalf("publish: %v", err)
	}
	saveDraft(t, e, authorPerms, n.DocumentID, le.ID, bilingualContent("จ่ายเงินเดือนและโบนัส", "Pay salary"))
	e.in(t, func(ctx context.Context) error {
		stale, err := e.svc.GetTranslationStatus(ctx, n.ID)
		if err != nil {
			return err
		}
		if !stale {
			t.Error("expected stale after a Thai-only edit against the published version")
		}
		return nil
	})
}
