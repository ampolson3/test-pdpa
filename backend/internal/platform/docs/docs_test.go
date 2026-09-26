package docs_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/docstest"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/versioning"
)

var (
	F         = docstest.Field
	P         = docstest.Para
	Doc       = docstest.Doc
	H         = docstest.Heading
	ClauseRef = docstest.ClauseRef
)

// clause creates and publishes a library clause (as the lawyer) and returns it.
func clause(t *testing.T, f *docstest.Fixture, code, title string, body ...render.Node) docs.Clause {
	t.Helper()
	var c docs.Clause
	f.As(t, f.Law, nil, docstest.Legal, func(ctx context.Context) error {
		var err error
		if c, err = f.Svc.CreateClause(ctx, docs.ClauseInput{Code: code, Category: "retention", AppliesTo: []string{"notice", "dpa"},
			Body: map[string]docs.ClauseBody{"th": {Title: title, Doc: Doc(body...)}}}); err != nil {
			return err
		}
		c, err = f.Svc.PublishClause(ctx, c.ID, c.RowVersion)
		return err
	})
	return c
}

func docxXML(t *testing.T, b []byte) string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	for _, zf := range z.File {
		if zf.Name == "word/document.xml" {
			r, _ := zf.Open()
			defer r.Close()
			x, _ := io.ReadAll(r)
			return string(x)
		}
	}
	t.Fatal("no document.xml")
	return ""
}

// Acceptance PLT-16: a Thai document exports to PDF and Word with the right characters, and two versions compare.
func TestAcceptance_ThaiExportAndCompare(t *testing.T) {
	f := docstest.Setup(t)
	if !f.HasPDF {
		t.Skip("needs a local Chromium for the PDF")
	}
	ret := clause(t, f, "retention.default", "ระยะเวลาการเก็บรักษา", P("บริษัทเก็บข้อมูลไม่เกิน ๕ ปี นับแต่วันที่สิ้นสุดความสัมพันธ์"))

	var doc docs.Document
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		var err error
		if doc, err = f.Svc.Create(ctx, docs.CreateInput{DocType: "notice", Title: "ประกาศความเป็นส่วนตัว", LegalEntityID: &f.EntityA}); err != nil {
			return err
		}
		doc, err = f.Svc.SaveDraft(ctx, doc.ID, doc.RowVersion, docs.Draft{Title: "ประกาศความเป็นส่วนตัว", LegalEntityID: &f.EntityA,
			EffectiveFrom: "2026-10-01",
			Content: render.Content{"th": Doc(H(1, "ข้อ ๑ ผู้ควบคุมข้อมูลส่วนบุคคล"),
				P(F("org_name_th"), " (ทะเบียนเลขที่ ", F("org_registration_no"), ") ติดต่อ ", F("org_email")),
				ClauseRef("retention.default", int(ret.VersionNo)),
				P("มีผลตั้งแต่ ", F("doc_effective_date"))),
				"en": Doc(H(1, "1. Data controller"), P(F("org_name_en"), " — effective ", F("doc_effective_date")))}})
		return err
	})
	if doc.Missing != nil {
		t.Fatalf("missing %+v", doc.Missing)
	}

	// A draft exports with the DRAFT banner.
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		b, name, err := f.Svc.Export(ctx, doc.ID, nil, "th", "docx")
		if err != nil {
			return err
		}
		if x := docxXML(t, b); !strings.Contains(x, render.DraftLabel("th")) || !strings.HasSuffix(name, ".docx") {
			t.Errorf("draft export lacks the banner (%s)", name)
		}
		return nil
	})

	if err := f.Approve(t, doc, f.Priv, docstest.Privacy); err != nil {
		t.Fatal(err)
	}
	var v1 docs.PublishedVersion
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		vs, err := f.Svc.Versions(ctx, doc.ID)
		if err != nil || len(vs) != 1 {
			t.Fatalf("versions %v %v", vs, err)
		}
		v1 = vs[0]
		return nil
	})
	if v1.RenderStatus != "pending" || v1.VersionNo != 1 || len(v1.Languages) != 2 {
		t.Fatalf("published version %+v", v1)
	}
	if err := f.Render(t, v1.ID); err != nil {
		t.Fatal(err)
	}
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		vs, err := f.Svc.Versions(ctx, doc.ID)
		v1 = vs[0]
		return err
	})
	if v1.RenderStatus != "done" || len(v1.Files["th"]) != 2 || len(v1.Files["en"]) != 2 {
		t.Fatalf("rendered version %+v", v1)
	}

	// Word: the exact Thai text, merge fields filled from the legal entity, the clause expanded, a Buddhist-era date.
	x := docxXML(t, f.ReadFile(t, v1.Files["th"]["docx"]))
	for _, want := range []string{"ข้อ ๑ ผู้ควบคุมข้อมูลส่วนบุคคล", "บริษัท ตัวอย่าง จำกัด (มหาชน)", "0107537000254", "dpo@example.co.th",
		"ระยะเวลาการเก็บรักษา", "บริษัทเก็บข้อมูลไม่เกิน ๕ ปี นับแต่วันที่สิ้นสุดความสัมพันธ์", "1 ตุลาคม 2569"} {
		if !strings.Contains(x, want) {
			t.Errorf("docx lacks %q", want)
		}
	}
	if strings.Contains(x, render.DraftLabel("th")) {
		t.Error("published docx carries the draft banner")
	}
	if en := docxXML(t, f.ReadFile(t, v1.Files["en"]["docx"])); !strings.Contains(en, "Example PCL") || !strings.Contains(en, "1 October 2026") {
		t.Error("english docx")
	}
	// PDF: a real PDF with the Thai font embedded (the E2E test extracts its text).
	pdf := f.ReadFile(t, v1.Files["th"]["pdf"])
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf, []byte("Sarabun")) {
		t.Errorf("pdf: %d bytes", len(pdf))
	}

	// The published version is frozen: later changes to the organization don't alter it.
	f.As(t, f.DPOUser, nil, nil, func(ctx context.Context) error {
		_, err := pdbExec(ctx, `UPDATE org.legal_entities SET name_th = 'ชื่อใหม่' WHERE id = $1`, f.EntityA)
		return err
	})
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		b, _, err := f.Svc.Export(ctx, doc.ID, nil, "th", "html")
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), "บริษัท ตัวอย่าง จำกัด (มหาชน)") {
			t.Error("the published version changed with the organization")
		}
		return nil
	})

	// Version 2 changes a paragraph and adds one; compare 1 → 2.
	var d2 docs.Document
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		cur, err := f.Svc.Get(ctx, doc.ID)
		if err != nil {
			return err
		}
		c := cur.Draft.Content
		th := c["th"]
		th.Content = append(th.Content[:3], P("ท่านมีสิทธิถอนความยินยอมได้ทุกเมื่อ"), P("มีผลตั้งแต่ ", F("doc_effective_date"), " เป็นต้นไป"))
		c["th"] = th
		d2, err = f.Svc.SaveDraft(ctx, doc.ID, cur.RowVersion, docs.Draft{Title: cur.Draft.Title, LegalEntityID: &f.EntityA, EffectiveFrom: "2026-10-01", Content: c})
		return err
	})
	if d2.Latest == nil || d2.Latest.No != 2 || d2.Latest.Status != "draft" {
		t.Fatalf("second draft %+v", d2.Latest)
	}
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		vs, err := f.Ver.List(ctx, docs.EntityType("notice"), doc.ID)
		if err != nil {
			return err
		}
		cmp, err := f.Svc.Compare(ctx, doc.ID, vs[1].ID, vs[0].ID, "th")
		if err != nil {
			return err
		}
		// Version 2 is a draft, so it shows today's organization name (renamed above) against version 1's frozen one.
		if cmp.Summary["insert"] != 1 || cmp.Summary["change"] != 2 || cmp.Summary["delete"] != 0 {
			t.Errorf("summary %v: %+v", cmp.Summary, cmp.Changes)
		}
		var changes []render.Change
		for _, ch := range cmp.Changes {
			if ch.Op == "change" {
				changes = append(changes, ch)
			}
			if ch.Op == "insert" && ch.After != "ท่านมีสิทธิถอนความยินยอมได้ทุกเมื่อ" {
				t.Errorf("insert %+v", ch)
			}
		}
		if len(changes) == 2 {
			if s := changes[0].Segments; s[0] != (render.Segment{Op: "delete", Text: "บริษัท ตัวอย่าง จำกัด (มหาชน)"}) || s[1] != (render.Segment{Op: "insert", Text: "ชื่อใหม่"}) {
				t.Errorf("org segments %+v", s)
			}
			if s := changes[1].Segments; s[len(s)-1] != (render.Segment{Op: "insert", Text: " เป็นต้นไป"}) {
				t.Errorf("date segments %+v", s)
			}
		}
		return nil
	})
}

func TestPublishNeedsEveryField(t *testing.T) {
	f := docstest.Setup(t)
	var doc docs.Document
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		var err error
		if doc, err = f.Svc.Create(ctx, docs.CreateInput{DocType: "notice", Title: "ไม่มีนิติบุคคล"}); err != nil {
			return err
		}
		doc, err = f.Svc.SaveDraft(ctx, doc.ID, doc.RowVersion, docs.Draft{Title: "ไม่มีนิติบุคคล", Content: render.Content{"th": Doc(P("ผู้ควบคุม ", F("org_name_th")))}})
		return err
	})
	if doc.Missing == nil || len(doc.Missing.Fields) != 1 || doc.Missing.Fields[0] != "org_name_th" {
		t.Fatalf("missing %+v", doc.Missing)
	}
	err := f.Approve(t, doc, f.Priv, docstest.Privacy)
	var inc *docs.IncompleteError
	if !errors.As(err, &inc) || !errors.Is(err, versioning.ErrInvalidRequest) {
		t.Fatalf("publish: %v", err)
	}
}

func TestDraftValidation(t *testing.T) {
	f := docstest.Setup(t)
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		doc, err := f.Svc.Create(ctx, docs.CreateInput{DocType: "notice", Title: "t"})
		if err != nil {
			return err
		}
		for name, d := range map[string]docs.Draft{
			"unknown field":  {Title: "t", Content: render.Content{"th": Doc(P(F("salary")))}},
			"unknown clause": {Title: "t", Content: render.Content{"th": Doc(ClauseRef("nope", 1))}},
			"script node":    {Title: "t", Content: render.Content{"th": Doc(render.Node{Type: "script"})}},
			"no thai":        {Title: "t", Content: render.Content{"en": Doc(P("x"))}},
			"empty title":    {Title: " ", Content: render.Content{"th": Doc(P("x"))}},
			"bad date":       {Title: "t", EffectiveFrom: "1/10/2569", Content: render.Content{"th": Doc(P("x"))}},
			"other entity":   {Title: "t", LegalEntityID: &f.EntityB, Content: render.Content{"th": Doc(P("x"))}},
		} {
			if _, err := f.Svc.SaveDraft(ctx, doc.ID, doc.RowVersion, d); !errors.Is(err, docs.ErrInvalidRequest) {
				t.Errorf("%s: %v", name, err)
			}
		}
		if _, err := f.Svc.SaveDraft(ctx, doc.ID, doc.RowVersion+5, docs.Draft{Title: "t", Content: render.Content{"th": Doc(P("x"))}}); !errors.Is(err, docs.ErrVersionMismatch) {
			t.Errorf("stale row version: %v", err)
		}
		if _, err := f.Svc.Create(ctx, docs.CreateInput{DocType: "report", Title: "t"}); !errors.Is(err, docs.ErrUnknownType) {
			t.Errorf("unoffered type: %v", err)
		}
		if _, err := f.Svc.Create(ctx, docs.CreateInput{DocType: "dpa", Title: "t"}); !errors.Is(err, docs.ErrForbidden) {
			t.Errorf("dpa without agreement.dpa.create: %v", err)
		}
		return nil
	})
}

func TestAccessAndIsolation(t *testing.T) {
	f := docstest.Setup(t)
	var doc docs.Document
	var cl docs.Clause
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		var err error
		doc, err = f.Svc.Create(ctx, docs.CreateInput{DocType: "notice", Title: "ของ A"})
		return err
	})
	cl = clause(t, f, "a.only", "ข้อความของ A", P("x"))
	f.As(t, f.Law, nil, docstest.Legal, func(ctx context.Context) error {
		// Legal reads notices but can't edit them, and doesn't see a DSAR letter type.
		if _, err := f.Svc.SaveDraft(ctx, doc.ID, doc.RowVersion, docs.Draft{Title: "x", Content: render.Content{"th": Doc(P("x"))}}); !errors.Is(err, docs.ErrForbidden) {
			t.Errorf("legal edits a notice: %v", err)
		}
		for _, a := range f.Svc.Access(ctx) {
			if a.DocType == "dsar_letter" {
				t.Error("legal sees dsar letters")
			}
		}
		return nil
	})
	f.As(t, f.DPOUser, nil, []string{"dsar.request.read"}, func(ctx context.Context) error {
		if _, err := f.Svc.Get(ctx, doc.ID); !errors.Is(err, docs.ErrNotFound) {
			t.Errorf("notice without notice.document.read: %v", err)
		}
		list, _, err := f.Svc.List(ctx, docs.ListFilter{})
		if err != nil || len(list) != 0 {
			t.Errorf("list %v %v", list, err)
		}
		return nil
	})
	// Tenant B sees none of A's documents or clauses, even with every permission.
	all := append(append([]string{}, docstest.DPO...), docstest.Legal...)
	f.As(t, f.B.UserID, []string{"DPO"}, all, func(ctx context.Context) error {
		if _, err := f.Svc.Get(ctx, doc.ID); !errors.Is(err, docs.ErrNotFound) {
			t.Errorf("B reads A's document: %v", err)
		}
		list, _, err := f.Svc.List(ctx, docs.ListFilter{})
		if err != nil || len(list) != 0 {
			t.Errorf("B lists %v %v", list, err)
		}
		if _, _, err := f.Svc.GetClause(ctx, cl.ID); !errors.Is(err, docs.ErrNotFound) {
			t.Errorf("B reads A's clause: %v", err)
		}
		cs, err := f.Svc.ListClauses(ctx, docs.ClauseFilter{})
		if err != nil {
			return err
		}
		for _, c := range cs {
			if c.Code == "a.only" {
				t.Error("B lists A's clause")
			}
		}
		// B may use the same code for its own clause.
		if _, err := f.Svc.CreateClause(ctx, docs.ClauseInput{Code: "a.only", Category: "x", Body: map[string]docs.ClauseBody{"th": {Title: "B", Doc: Doc(P("b"))}}}); err != nil {
			t.Errorf("B's own clause: %v", err)
		}
		// And a B document can't cite A's clause.
		d, err := f.Svc.Create(ctx, docs.CreateInput{DocType: "notice", Title: "B"})
		if err != nil {
			return err
		}
		if _, err := f.Svc.SaveDraft(ctx, d.ID, d.RowVersion, docs.Draft{Title: "B", Content: render.Content{"th": Doc(ClauseRef("a.only", 1))}}); !errors.Is(err, docs.ErrInvalidRequest) {
			t.Errorf("B cites A's clause (or B's own draft clause): %v", err)
		}
		return nil
	})
}

// DSA-06: editing a clause gives new documents the latest version; documents citing the old one keep it.
func TestClauseVersions(t *testing.T) {
	f := docstest.Setup(t)
	v1 := clause(t, f, "security.measures", "มาตรการรักษาความปลอดภัย", P("เข้ารหัสข้อมูลขณะจัดเก็บ"))
	var v2 docs.Clause
	f.As(t, f.Law, nil, docstest.Legal, func(ctx context.Context) error {
		var err error
		if _, err := f.Svc.CreateClause(ctx, docs.ClauseInput{Code: "security.measures", Category: "x", Body: map[string]docs.ClauseBody{"th": {Title: "x", Doc: Doc(P("x"))}}}); !errors.Is(err, docs.ErrCodeTaken) {
			t.Errorf("duplicate code: %v", err)
		}
		in := docs.ClauseInput{Category: "security", AppliesTo: []string{"dpa"},
			Body: map[string]docs.ClauseBody{"th": {Title: "มาตรการรักษาความปลอดภัย", Doc: Doc(P("เข้ารหัสข้อมูลขณะจัดเก็บและส่งผ่าน"))}}}
		if v2, err = f.Svc.UpdateClause(ctx, v1.ID, v1.RowVersion, in); err != nil {
			return err
		}
		if v2.VersionNo != 2 || v2.Status != "draft" || v2.ID == v1.ID {
			t.Fatalf("v2 %+v", v2)
		}
		if _, err := f.Svc.UpdateClause(ctx, v1.ID, v1.RowVersion, in); !errors.Is(err, docs.ErrInvalidState) {
			t.Errorf("editing a superseded version: %v", err)
		}
		v2, err = f.Svc.PublishClause(ctx, v2.ID, v2.RowVersion)
		return err
	})
	f.As(t, f.Law, nil, docstest.Legal, func(ctx context.Context) error {
		old, versions, err := f.Svc.GetClause(ctx, v1.ID)
		if err != nil {
			return err
		}
		if old.Status != "retired" || len(versions) != 2 || versions[0].VersionNo != 2 {
			t.Errorf("history %+v", versions)
		}
		latest, err := f.Svc.ListClauses(ctx, docs.ClauseFilter{PublishedOnly: true, AppliesTo: "dpa"})
		if err != nil {
			return err
		}
		if len(latest) != 1 || latest[0].VersionNo != 2 {
			t.Errorf("latest %+v", latest)
		}
		// A document may still cite version 1 (e.g. one drafted before the change).
		d, err := f.Svc.Create(ctx, docs.CreateInput{DocType: "dpa", Title: "DPA"})
		if err != nil {
			return err
		}
		d, err = f.Svc.SaveDraft(ctx, d.ID, d.RowVersion, docs.Draft{Title: "DPA", Content: render.Content{"th": Doc(ClauseRef("security.measures", 1))}})
		if err != nil {
			return err
		}
		b, _, err := f.Svc.Export(ctx, d.ID, nil, "th", "html")
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), "เข้ารหัสข้อมูลขณะจัดเก็บ</p>") {
			t.Error("version 1 text not rendered")
		}
		return nil
	})
	f.As(t, f.DPOUser, nil, docstest.DPO, func(ctx context.Context) error {
		if _, err := f.Svc.CreateClause(ctx, docs.ClauseInput{Code: "dpo.clause", Category: "x", Body: map[string]docs.ClauseBody{"th": {Title: "x", Doc: Doc(P("x"))}}}); !errors.Is(err, docs.ErrForbidden) {
			t.Errorf("DPO creates a clause: %v", err)
		}
		return nil
	})
}

func TestTemplates(t *testing.T) {
	f := docstest.Setup(t)
	var tpl docs.Template
	f.As(t, f.DPOUser, nil, docstest.DPO, func(ctx context.Context) error {
		var err error
		tpl, err = f.Svc.CreateTemplate(ctx, docs.TemplateInput{DocType: "notice", Code: "notice.customer", Name: "ประกาศสำหรับลูกค้า",
			Content: render.Content{"th": Doc(H(1, "ประกาศความเป็นส่วนตัว"), P(F("org_name_th")))}})
		return err
	})
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		// A draft template is neither offered nor usable.
		list, err := f.Svc.ListTemplates(ctx, "notice", true)
		if err != nil || len(list) != 0 {
			t.Errorf("draft offered: %v %v", list, err)
		}
		if _, err := f.Svc.Create(ctx, docs.CreateInput{DocType: "notice", Title: "x", TemplateID: &tpl.ID}); !errors.Is(err, docs.ErrInvalidRequest) {
			t.Errorf("created from a draft template: %v", err)
		}
		if _, err := f.Svc.PublishTemplate(ctx, tpl.ID, tpl.RowVersion); !errors.Is(err, docs.ErrForbidden) {
			t.Errorf("privacy publishes a template: %v", err)
		}
		return nil
	})
	f.As(t, f.DPOUser, nil, docstest.DPO, func(ctx context.Context) error {
		var err error
		tpl, err = f.Svc.PublishTemplate(ctx, tpl.ID, tpl.RowVersion)
		return err
	})
	f.As(t, f.Priv, nil, docstest.Privacy, func(ctx context.Context) error {
		d, err := f.Svc.Create(ctx, docs.CreateInput{DocType: "notice", Title: "ประกาศลูกค้า", TemplateID: &tpl.ID, LegalEntityID: &f.EntityA})
		if err != nil {
			return err
		}
		if d.Draft == nil || len(d.Draft.Content["th"].Content) != 2 || d.Missing != nil || d.TemplateID == nil || *d.TemplateID != tpl.ID {
			t.Errorf("document from template %+v", d)
		}
		return nil
	})
}

func pdbExec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pdb.MustTxFromContext(ctx).Exec(ctx, sql, args...)
}
