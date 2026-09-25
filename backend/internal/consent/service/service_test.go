package service_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"pdpa-platform/internal/consent/consenttest"
	consent "pdpa-platform/internal/consent/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/publickeys"
	"pdpa-platform/internal/platform/versioning"
)

// Acceptance CON-12: every version is kept and it can be traced which version each person consented to; a new
// version goes live only after someone other than its consenttest.Maker approves it.
func TestPurposeVersions_TraceWhichTextEachPersonAccepted(t *testing.T) {
	f := consenttest.Setup(t)
	p := f.LivePurpose(t, "NEWSLETTER", consenttest.Content("จดหมายข่าว", "ยินยอมรับข่าวสารทางอีเมล"))
	if p.Status != "active" || p.CurrentVersion == nil || p.CurrentVersion.No != 1 || p.IsSensitive || p.LawfulBasis != "CONSENT" {
		t.Fatalf("published v1: %+v", p)
	}
	// Maker-checker: the author can't approve their own draft.
	f.As(t, f.A, f.Alice, []string{"DPO"}, consenttest.Maker, func(ctx context.Context) error {
		if _, err := f.Svc.SavePurposeDraft(ctx, p.ID, consenttest.Content("จดหมายข่าว", "ข้อความใหม่")); err != nil {
			return err
		}
		vs, _ := f.Ver.List(ctx, consent.PurposeType, p.ID)
		v, err := f.Ver.Submit(ctx, vs[0].ID, vs[0].RowVersion)
		if err != nil {
			return err
		}
		if v, err = f.Ver.Get(ctx, v.ID); err != nil {
			return err
		}
		if _, err := f.Ver.Decide(ctx, v.Approvals[0].ID, v.Approvals[0].RowVersion, "approved", ""); !errors.Is(err, versioning.ErrSelfApproval) {
			t.Errorf("author approving: %v", err)
		}
		return nil
	})
	cp := f.LiveCP(t, "SIGNUP", consent.CPPurposeInput{PurposeID: p.ID})
	if _, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("somchai@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "NEWSLETTER", PurposeVersionNo: 1, Decision: "CONSENTED"}}}); err != nil {
		t.Fatal(err)
	}
	// Version 2: a material change asking for re-consent (the v2 draft is already in review).
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		return nil
	})
	f.As(t, f.A, f.DPO, []string{"DPO"}, []string{"consent.purpose.read"}, func(ctx context.Context) error {
		inbox, err := f.Ver.Inbox(ctx)
		if err != nil || len(inbox) != 1 {
			t.Fatalf("inbox: %+v %v", inbox, err)
		}
		_, err = f.Ver.Decide(ctx, inbox[0].ID, inbox[0].RowVersion, "returned", "ให้ระบุเป็นการเปลี่ยนแปลงสาระสำคัญ")
		return err
	})
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		c := consenttest.Content("จดหมายข่าว", "ยินยอมรับข่าวสารและข้อเสนอจากพันธมิตร")
		c.ChangeType, c.RequiresReconsent = "material", true
		_, err := f.Svc.SavePurposeDraft(ctx, p.ID, c)
		return err
	})
	if err := f.ApproveAndPublish(t, p.ID); err != nil {
		t.Fatal(err)
	}
	// A stale form (showing v1) is refused; the current one records against v2.
	var de *consent.DecisionError
	if _, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("somying@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "NEWSLETTER", PurposeVersionNo: 1, Decision: "CONSENTED"}}}); !errors.As(err, &de) || de.Fields[0].Code != "stale_version" {
		t.Errorf("stale version: %v", err)
	}
	if _, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("somying@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "NEWSLETTER", PurposeVersionNo: 2, Decision: "CONSENTED"}}}); err != nil {
		t.Fatal(err)
	}
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		p, _ = f.Svc.GetPurpose(ctx, p.ID)
		if len(p.Versions) != 2 || p.Versions[0].No != 2 || p.Versions[0].ChangeType != "material" || !p.Versions[0].RequiresReconsent || p.Versions[1].ConsentText.Th != "ยินยอมรับข่าวสารทางอีเมล" {
			t.Errorf("versions kept: %+v", p.Versions)
		}
		for _, c := range []struct {
			email     string
			version   int32
			reconsent bool
		}{{"somchai@example.com", 1, true}, {"SomYing@Example.com", 2, false}} {
			subs, err := f.Svc.Subjects(ctx, "email", c.email)
			if err != nil || len(subs) != 1 {
				t.Fatalf("find %s: %+v %v", c.email, subs, err)
			}
			prof, err := f.Svc.GetProfile(ctx, subs[0].ID)
			if err != nil {
				return err
			}
			if prof.Statuses[0].VersionNo != c.version || prof.History[0].VersionNo != c.version || prof.Statuses[0].NeedsReconsent != c.reconsent {
				t.Errorf("%s: status %+v history %+v", c.email, prof.Statuses[0], prof.History[0])
			}
		}
		return nil
	})
}

// Acceptance CON-10: a purpose using sensitive data can't be published without its own explicit statement, and a
// collection point can't make it a condition (it is always its own, unticked checkbox).
func TestSensitivePurpose_NeedsItsOwnExplicitConsent(t *testing.T) {
	f := consenttest.Setup(t)
	var p consent.Purpose
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		var err error
		p, err = f.Svc.CreatePurpose(ctx, "HEALTH-PROGRAM", f.EntityA, consenttest.Content("โปรแกรมสุขภาพ", "ยินยอมให้ใช้ข้อมูลสุขภาพ", "health", "contact"))
		return err
	})
	var ce *consent.CheckError
	if err := f.ApproveAndPublish(t, p.ID); !errors.As(err, &ce) || !slices.Contains(ce.Failed, "explicit_text_required") {
		t.Fatalf("publish without explicit text: %v", err)
	}
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		vs, _ := f.Ver.List(ctx, consent.PurposeType, p.ID)
		if vs[0].Status != "approved" {
			t.Errorf("failed publish left %s", vs[0].Status)
		}
		return nil
	})
	f.As(t, f.A, f.DPO, []string{"DPO"}, consenttest.Maker, func(ctx context.Context) error { return nil })
	p2 := f.LivePurpose(t, "HEALTH-2", func() consent.PurposeContent {
		c := consenttest.Content("โปรแกรมสุขภาพ", "ยินยอมให้ใช้ข้อมูลสุขภาพ", "health")
		c.ExplicitText = consent.Text{Th: "ข้าพเจ้ายินยอมโดยชัดแจ้งให้ใช้ข้อมูลสุขภาพ (ข้อมูลอ่อนไหว ม.26)"}
		return c
	}())
	if !p2.IsSensitive || !p2.RequiresExplicit || p2.LawfulBasis != "EXPLICIT_CONSENT" {
		t.Errorf("sensitive purpose: %+v", p2)
	}
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		cp, err := f.Svc.CreateCollectionPoint(ctx, consent.CollectionPointInput{Code: "CLINIC", Name: "คลินิก", Channel: "web", LegalEntityID: f.EntityA,
			Purposes: []consent.CPPurposeInput{{PurposeID: p2.ID, Required: true}}})
		if err != nil {
			return err
		}
		if _, err := f.Svc.PublishCollectionPoint(ctx, cp.ID, cp.RowVersion, consenttest.FullChecklist); !errors.As(err, &ce) || !slices.Contains(ce.Failed, "sensitive_required") {
			t.Errorf("sensitive purpose as a condition: %v", err)
		}
		return nil
	})
}

// Acceptance CON-09: only a published form with the s.19 checklist is live; it takes a decision per purpose and
// issues a receipt every time.
func TestCollectionPoint_PublishedFormRecordsPerPurposeWithReceipts(t *testing.T) {
	f := consenttest.Setup(t)
	news := f.LivePurpose(t, "NEWS", consenttest.Content("ข่าวสาร", "รับข่าวสาร"))
	prof := f.LivePurpose(t, "PROFILING", consenttest.Content("วิเคราะห์ความสนใจ", "ให้วิเคราะห์ความสนใจ"))
	var cp consent.CollectionPoint
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		var err error
		cp, err = f.Svc.CreateCollectionPoint(ctx, consent.CollectionPointInput{Code: "WEB-SIGNUP", Name: "สมัครสมาชิก", Channel: "web", LegalEntityID: f.EntityA,
			AllowedOrigins: []string{"https://shop.example"}, Purposes: []consent.CPPurposeInput{{PurposeID: news.ID}, {PurposeID: prof.ID}}})
		if err != nil {
			return err
		}
		var ce *consent.CheckError
		if _, err := f.Svc.PublishCollectionPoint(ctx, cp.ID, cp.RowVersion, consent.Checklist{SeparateText: true}); !errors.As(err, &ce) ||
			!slices.Equal(ce.Failed, []string{"checklist_not_bundled", "checklist_plain_language", "checklist_withdrawal_info"}) {
			t.Errorf("incomplete checklist: %v", err)
		}
		cp, err = f.Svc.PublishCollectionPoint(ctx, cp.ID, cp.RowVersion, consenttest.FullChecklist)
		return err
	})
	if cp.Status != "active" || len(cp.PublicKey) < 22 {
		t.Fatalf("published: %+v", cp)
	}
	k, ok, err := publickeys.Resolve(context.Background(), f.App, cp.PublicKey)
	if err != nil || !ok || k.TenantID != f.A.ID || k.EntityID != cp.ID || !slices.Equal(k.AllowedOrigins, []string{"https://shop.example"}) {
		t.Fatalf("key: %+v %v %v", k, ok, err)
	}
	if err := f.Public(t, func(ctx context.Context) error {
		v, err := f.Svc.PublicView(ctx, cp.ID, "en")
		if err != nil || len(v.Purposes) != 2 || v.Purposes[0].Code != "NEWS" || v.Purposes[0].Text != "รับข่าวสาร (en)" || v.Purposes[0].VersionNo != 1 {
			t.Errorf("public view: %+v %v", v, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A web form decides every purpose it shows (nothing is pre-ticked or silently skipped).
	_, err = f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("a@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "NEWS", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	var de *consent.DecisionError
	if !errors.As(err, &de) || de.Fields[0] != (consent.FieldError{Field: "PROFILING", Code: "missing"}) {
		t.Errorf("undecided purpose: %v", err)
	}
	for i := range 2 {
		r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("a@example.com"), Source: "web", Public: true, Language: "th",
			Decisions: []consent.Decision{{PurposeCode: "NEWS", PurposeVersionNo: 1, Decision: "CONSENTED"}, {PurposeCode: "PROFILING", PurposeVersionNo: 1, Decision: "NOT_CONSENTED"}}})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"CONSENTED:ACTIVE", "NOT_CONSENTED:NOT_GIVEN"}
		if i == 1 {
			want[0] = "EXTENDED:ACTIVE"
		}
		var got []string
		for _, tx := range r.Transactions {
			got = append(got, tx.Type+":"+tx.Status)
		}
		if !strings.HasPrefix(r.No, "CR-") || !slices.Equal(got, want) {
			t.Errorf("submission %d: %s %v", i, r.No, got)
		}
	}
	counts := count(f, t, f.A, `SELECT (SELECT count(*) FROM consent.consent_receipts)::text || '/' || (SELECT count(*) FROM consent.consent_transactions)::text || '/' ||
		(SELECT count(*) FROM platform.outbox_events WHERE event_type IN ('consent.granted','consent.denied'))::text || '/' ||
		(SELECT count(*) FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id WHERE t.code = 'consent.receipt')::text`)
	if counts != "2/4/4/2" {
		t.Errorf("receipts/transactions/events/receipt e-mails = %s, want 2/4/4/2", counts)
	}
	// Retiring revokes the key.
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		cp, _ = f.Svc.GetCollectionPoint(ctx, cp.ID)
		_, err := f.Svc.RetireCollectionPoint(ctx, cp.ID, cp.RowVersion)
		return err
	})
	if _, ok, _ := publickeys.Resolve(context.Background(), f.App, cp.PublicKey); ok {
		t.Error("retired point's key still resolves")
	}
}

func count(f *consenttest.Fixture, t *testing.T, tenant dbtest.Tenant, sql string) string {
	t.Helper()
	var s string
	if err := pdb.WithTenantTx(context.Background(), f.Owner, tenant.ID.String(), "", func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, sql).Scan(&s)
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

// Acceptance CON-15: transactions can't be edited or deleted, and it can be checked afterwards which version and
// channel a consent came from — the receipt chain exposes any change made out of band.
func TestReceipts_AppendOnlyAndTamperEvident(t *testing.T) {
	f := consenttest.Setup(t)
	news := f.LivePurpose(t, "NEWS", consenttest.Content("ข่าวสาร", "รับข่าวสาร"))
	cp := f.LiveCP(t, "APP", consent.CPPurposeInput{PurposeID: news.ID})
	for _, d := range []string{"CONSENTED", "NOT_CONSENTED", "CONSENTED"} {
		if _, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: []consent.Identifier{{Type: "phone", Value: "081-234-5678"}}, Source: "app", Public: true,
			Decisions: []consent.Decision{{PurposeCode: "NEWS", PurposeVersionNo: 1, Decision: d}}}); err != nil {
			t.Fatal(err)
		}
	}
	var subject uuid.UUID
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		subs, err := f.Svc.Subjects(ctx, "phone", "+66812345678") // another spelling of the same number
		if err != nil || len(subs) != 1 || subs[0].Identifiers[0].Masked != "+66*******78" {
			t.Fatalf("lookup: %+v %v", subs, err)
		}
		subject = subs[0].ID
		prof, err := f.Svc.GetProfile(ctx, subject)
		if err != nil {
			return err
		}
		var types []string
		for _, h := range prof.History {
			types = append(types, h.Type+"@"+h.Channel)
		}
		if !slices.Equal(types, []string{"CONSENTED@web", "WITHDRAWN@web", "CONSENTED@web"}) || prof.Statuses[0].Status != "ACTIVE" {
			t.Errorf("history %v status %+v", types, prof.Statuses)
		}
		res, err := f.Svc.VerifySubject(ctx, subject)
		if err != nil || !res.OK || res.Checked != 3 {
			t.Errorf("verify: %+v %v", res, err)
		}
		return nil
	})
	// The app role can't change or remove evidence.
	for _, q := range []string{`UPDATE consent.consent_transactions SET transaction_type = 'WITHDRAWN'`, `DELETE FROM consent.consent_receipts`} {
		err := pdb.WithTenantTx(context.Background(), f.App, f.A.ID.String(), "", func(ctx context.Context) error {
			_, err := pdb.MustTxFromContext(ctx).Exec(ctx, q)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s as app: %v", q, err)
		}
	}
	// The identifier is stored encrypted.
	if n := count(f, t, f.A, `SELECT count(*)::text FROM consent.subject_identifiers WHERE position(convert_to('812345678', 'UTF8') in value_enc) > 0`); n != "0" {
		t.Errorf("plaintext identifier found in %s rows", n)
	}
	verify := func() consent.VerifyResult {
		var r consent.VerifyResult
		f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
			var err error
			r, err = f.Svc.VerifySubject(ctx, subject)
			return err
		})
		return r
	}
	// Someone with database-owner access rewrites history: detected.
	asOwner(f, t, `UPDATE consent.consent_transactions SET transaction_type = 'CONSENTED' WHERE transaction_type = 'WITHDRAWN'`)
	if r := verify(); r.OK || r.Reason != "hash_mismatch" {
		t.Errorf("edited transaction: %+v", r)
	}
	asOwner(f, t, `UPDATE consent.consent_transactions SET transaction_type = 'WITHDRAWN' WHERE id = (SELECT id FROM consent.consent_transactions ORDER BY occurred_at LIMIT 1 OFFSET 1)`)
	if r := verify(); !r.OK {
		t.Fatalf("restored: %+v", r)
	}
	asOwner(f, t, `DELETE FROM consent.consent_transactions WHERE receipt_id = (SELECT id FROM consent.consent_receipts ORDER BY occurred_at LIMIT 1)`)
	asOwner(f, t, `DELETE FROM consent.consent_receipts WHERE id = (SELECT id FROM consent.consent_receipts ORDER BY occurred_at LIMIT 1)`)
	if r := verify(); r.OK || r.Reason != "prev_hash_mismatch" {
		t.Errorf("removed receipt: %+v", r)
	}
}

func asOwner(f *consenttest.Fixture, t *testing.T, sql string) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), f.Owner, f.A.ID.String(), "", func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, sql)
		return err
	}); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// Acceptance CON-13: a withdrawal takes effect at once, publishes consent.withdrawn for downstream systems, and
// records who took it and why; a web form can't withdraw (that needs the verified preference centre).
func TestWithdrawal_ImmediateWithEvent(t *testing.T) {
	f := consenttest.Setup(t)
	news := f.LivePurpose(t, "NEWS", consenttest.Content("ข่าวสาร", "รับข่าวสาร"))
	cp := f.LiveCP(t, "BRANCH", consent.CPPurposeInput{PurposeID: news.ID})
	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("b@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "NEWS", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if err != nil {
		t.Fatal(err)
	}
	var de *consent.DecisionError
	if _, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("b@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "NEWS", PurposeVersionNo: 1, Decision: "WITHDRAWN"}}}); !errors.As(err, &de) || de.Fields[0].Code != "withdraw_not_allowed" {
		t.Errorf("public withdrawal: %v", err)
	}
	subject := r.SubjectID
	staff := func(reason string) error {
		var err error
		f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
			err = pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := f.Svc.Record(ctx, consent.Submission{CollectionPointID: cp.ID, SubjectID: &subject, Source: "staff", CapturedBy: &f.Alice,
					Decisions: []consent.Decision{{PurposeCode: "NEWS", PurposeVersionNo: 1, Decision: "WITHDRAWN", ReasonCode: reason}}})
				return err
			})
			return nil
		})
		return err
	}
	if err := staff("bogus"); !errors.As(err, &de) || de.Fields[0].Code != "invalid_reason" {
		t.Errorf("unknown reason: %v", err)
	}
	if err := staff("too_many_messages"); err != nil {
		t.Fatal(err)
	}
	if err := staff("other"); !errors.Is(err, consent.ErrInvalidTransition) {
		t.Errorf("withdrawing twice: %v", err)
	}
	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		prof, err := f.Svc.GetProfile(ctx, subject)
		if err != nil {
			return err
		}
		if prof.Statuses[0].Status != "WITHDRAWN" || prof.History[0].Type != "WITHDRAWN" || prof.History[0].ReasonCode != "too_many_messages" ||
			prof.History[0].CapturedByName != "Alice" || prof.History[0].Source != "staff" {
			t.Errorf("after withdrawal: %+v / %+v", prof.Statuses[0], prof.History[0])
		}
		return nil
	})
	if n := count(f, t, f.A, `SELECT count(*)::text FROM platform.outbox_events WHERE event_type = 'consent.withdrawn' AND payload->'data'->>'subject_ref' = '`+subject.String()+`'`); n != "1" {
		t.Errorf("consent.withdrawn events: %s", n)
	}
	if n := count(f, t, f.A, `SELECT count(*)::text FROM platform.audit_log WHERE action = 'consent.record.on_behalf'`); n != "1" {
		t.Errorf("staff action audited: %s", n)
	}
}

// Tenant B sees nothing of tenant A's purposes, collection points or data subjects; the same e-mail is a
// different subject in each tenant.
func TestTwoTenantIsolation(t *testing.T) {
	f := consenttest.Setup(t)
	news := f.LivePurpose(t, "NEWS", consenttest.Content("ข่าวสาร", "รับข่าวสาร"))
	cp := f.LiveCP(t, "WEB", consent.CPPurposeInput{PurposeID: news.ID})
	r, err := f.Record(t, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("same@example.com"), Source: "web", Public: true,
		Decisions: []consent.Decision{{PurposeCode: "NEWS", PurposeVersionNo: 1, Decision: "CONSENTED"}}})
	if err != nil {
		t.Fatal(err)
	}
	f.As(t, f.B, f.B.UserID, nil, consenttest.Maker, func(ctx context.Context) error {
		if ps, _ := f.Svc.ListPurposes(ctx); len(ps) != 0 {
			t.Errorf("purposes: %d", len(ps))
		}
		if _, err := f.Svc.GetPurpose(ctx, news.ID); !errors.Is(err, consent.ErrNotFound) {
			t.Errorf("purpose: %v", err)
		}
		if _, err := f.Svc.GetCollectionPoint(ctx, cp.ID); !errors.Is(err, consent.ErrNotFound) {
			t.Errorf("collection point: %v", err)
		}
		if _, err := f.Svc.GetProfile(ctx, r.SubjectID); !errors.Is(err, consent.ErrNotFound) {
			t.Errorf("profile: %v", err)
		}
		if subs, _ := f.Svc.Subjects(ctx, "email", "same@example.com"); len(subs) != 0 {
			t.Errorf("found A's subject from B: %+v", subs)
		}
		if _, err := f.Svc.Record(ctx, consent.Submission{CollectionPointID: cp.ID, Identifiers: consenttest.Email("same@example.com"), Source: "web", Public: true,
			Decisions: []consent.Decision{{PurposeCode: "NEWS", PurposeVersionNo: 1, Decision: "CONSENTED"}}}); !errors.Is(err, consent.ErrNotFound) {
			t.Errorf("recording on A's point from B: %v", err)
		}
		return nil
	})
}
