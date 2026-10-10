package render

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const thai = "ผู้ควบคุมข้อมูลส่วนบุคคลจะเก็บรักษาข้อมูลของท่านไม่เกิน ๑๐ ปี ตามมาตรา ๓๗ (๔)"

func doc(t *testing.T, raw string) Node {
	t.Helper()
	var n Node
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func sample(t *testing.T) Input {
	return Input{Title: "ประกาศความเป็นส่วนตัว", Language: "th", Fields: map[string]string{"org_name_th": "บริษัท ทดสอบ จำกัด"},
		Clauses: map[string]Clause{"retention@2": {Title: "ระยะเวลาเก็บรักษา", Body: doc(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"`+thai+`"}]}]}`)}},
		Body: doc(t, `{"type":"doc","content":[
			{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"ข้อ ๑ ผู้ควบคุมข้อมูล"}]},
			{"type":"paragraph","content":[{"type":"mergeField","attrs":{"key":"org_name_th"}},{"type":"text","text":" เป็น"},{"type":"text","text":"ผู้ควบคุมข้อมูล","marks":[{"type":"bold"}]}]},
			{"type":"clause","attrs":{"code":"retention","version":2}},
			{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"สิทธิขอเข้าถึง <script>alert(1)</script>"}]}]}]},
			{"type":"paragraph","content":[{"type":"mergeField","attrs":{"key":"dpo_email"}}]}]}`)}
}

func TestValidate(t *testing.T) {
	ok := Content{"th": doc(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"ก"}]}]}`)}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]Content{
		"no th":      {"en": ok["th"]},
		"language":   {"th": ok["th"], "fr": ok["th"]},
		"html node":  {"th": doc(t, `{"type":"doc","content":[{"type":"html","text":"<b>"}]}`)},
		"code mark":  {"th": doc(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link"}]}]}]}`)},
		"heading 7":  {"th": doc(t, `{"type":"doc","content":[{"type":"heading","attrs":{"level":7}}]}`)},
		"field key":  {"th": doc(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mergeField","attrs":{"key":"<x>"}}]}]}`)},
		"clause ver": {"th": doc(t, `{"type":"doc","content":[{"type":"clause","attrs":{"code":"a","version":0}}]}`)},
		"root":       {"th": doc(t, `{"type":"paragraph"}`)},
	} {
		if err := c.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestHTML_EscapesResolvesAndEmbedsFont(t *testing.T) {
	in := sample(t)
	h := string(HTML(in))
	for _, want := range []string{"บริษัท ทดสอบ จำกัด", "<strong>ผู้ควบคุมข้อมูล</strong>", thai, "ระยะเวลาเก็บรักษา", "&lt;script&gt;", `<span class="missing">[dpo_email]</span>`,
		"font-family:'Sarabun'", "data:font/woff2;base64,", `<html lang="th">`} {
		if !strings.Contains(h, want) {
			t.Errorf("html lacks %q", want)
		}
	}
	if strings.Contains(h, "<script>") {
		t.Error("unescaped script")
	}
	fields, clauses := in.Missing()
	if len(fields) != 1 || fields[0] != "dpo_email" || len(clauses) != 0 {
		t.Errorf("missing %v %v", fields, clauses)
	}
	in.Clauses = nil
	if _, clauses := in.Missing(); len(clauses) != 1 || clauses[0] != "retention@2" {
		t.Errorf("missing clause %v", clauses)
	}
}

// Acceptance PLT-16 (Word half): Thai text comes out character for character, with Thai fonts and language set.
func TestDOCX_ThaiTextExact(t *testing.T) {
	b, err := DOCX(sample(t))
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range z.File {
		r, _ := f.Open()
		data, _ := io.ReadAll(r)
		r.Close()
		files[f.Name] = string(data)
	}
	d := files["word/document.xml"]
	for _, want := range []string{thai, "บริษัท ทดสอบ จำกัด", "ข้อ ๑ ผู้ควบคุมข้อมูล", "<w:b/><w:bCs/>", "&lt;script&gt;", `<w:pStyle w:val="Heading1"/>`, "• ", "[dpo_email]"} {
		if !strings.Contains(d, want) {
			t.Errorf("document.xml lacks %q", want)
		}
	}
	if !strings.Contains(files["word/styles.xml"], `w:cs="Sarabun"`) || !strings.Contains(files["word/styles.xml"], `w:bidi="th-TH"`) {
		t.Error("styles lack the Thai font / language")
	}
	if files["[Content_Types].xml"] == "" || files["_rels/.rels"] == "" {
		t.Error("package parts missing")
	}
}

// Acceptance PLT-16 (comparison): two versions line up block by block, an edited Thai paragraph shows its changes.
func TestCompare(t *testing.T) {
	a := []Block{{"heading1", "ข้อ ๑"}, {"paragraph", "เก็บข้อมูลไม่เกิน ๕ ปี"}, {"paragraph", "ติดต่อ DPO"}}
	b := []Block{{"heading1", "ข้อ ๑"}, {"paragraph", "เก็บข้อมูลไม่เกิน ๑๐ ปี"}, {"paragraph", "สิทธิของท่าน"}, {"paragraph", "ติดต่อ DPO"}}
	cs := Compare(a, b)
	ops := []string{}
	for _, c := range cs {
		ops = append(ops, c.Op)
	}
	if strings.Join(ops, ",") != "equal,change,insert,equal" {
		t.Fatalf("ops %v", ops)
	}
	seg := cs[1].Segments
	var del, ins string
	for _, s := range seg {
		switch s.Op {
		case "delete":
			del += s.Text
		case "insert":
			ins += s.Text
		}
	}
	if del != "๕" || ins != "๑๐" {
		t.Errorf("segments %+v", seg)
	}
	if s := Summary(cs); s["change"] != 1 || s["insert"] != 1 {
		t.Errorf("summary %v", s)
	}
	blocks := Blocks(sample(t))
	if len(blocks) != 6 || blocks[2] != (Block{"clause", "ระยะเวลาเก็บรักษา"}) || !strings.HasPrefix(blocks[4].Text, "• สิทธิขอเข้าถึง") {
		t.Errorf("blocks %+v", blocks)
	}
}

func chromiumPath() string {
	if p := os.Getenv("CHROMIUM_PATH"); p != "" {
		return p
	}
	matches, _ := filepath.Glob("/opt/pw-browsers/chromium-*/chrome-linux/chrome")
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// Acceptance PLT-16 (PDF half): a Thai document prints to PDF with the embedded Sarabun font. (The E2E test extracts
// the text back out of the PDF and compares it.)
func TestChromiumPDF_EmbedsThaiFont(t *testing.T) {
	p := chromiumPath()
	if p == "" {
		t.Skip("no chromium")
	}
	pdf, err := (&Chromium{Path: p}).PDF(context.Background(), HTML(sample(t)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf, []byte("Sarabun")) {
		t.Errorf("pdf: %d bytes, sarabun %v", len(pdf), bytes.Contains(pdf, []byte("Sarabun")))
	}
}

func TestGotenbergClient(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forms/chromium/convert/html" {
			http.NotFound(w, r)
			return
		}
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			if p.FormName() == "files" && p.FileName() == "index.html" {
				got, _ = io.ReadAll(p)
			}
		}
		_, _ = w.Write([]byte("%PDF-1.7 fake"))
	}))
	defer srv.Close()
	pdf, err := (&Gotenberg{URL: srv.URL}).PDF(context.Background(), []byte("<html>ก</html>"))
	if err != nil || string(pdf) != "%PDF-1.7 fake" || string(got) != "<html>ก</html>" {
		t.Errorf("pdf %q err %v sent %q", pdf, err, got)
	}
	_ = multipart.ErrMessageTooLarge
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 503) }))
	defer bad.Close()
	if _, err := (&Gotenberg{URL: bad.URL}).PDF(context.Background(), []byte("x")); err == nil {
		t.Error("503 accepted")
	}
}

func TestDraftBanner(t *testing.T) {
	in := Input{Title: "t", Language: "th", Body: Node{Type: "doc", Content: []Node{{Type: "paragraph", Content: []Node{{Type: "text", Text: "x"}}}}}, Draft: true}
	if !strings.Contains(string(HTML(in)), DraftLabel("th")) {
		t.Fatal("html: no draft banner")
	}
	b, err := DOCX(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(docxText(t, b), DraftLabel("th")) {
		t.Fatal("docx: no draft banner")
	}
	in.Draft = false
	if strings.Contains(string(HTML(in)), DraftLabel("th")) {
		t.Fatal("html: banner on an approved version")
	}
}

// docxText is the raw word/document.xml of a DOCX.
func docxText(t *testing.T, b []byte) string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			x, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			return string(x)
		}
	}
	t.Fatal("no word/document.xml")
	return ""
}

func TestCompare_PairsSimilarBlocksAndReadableThai(t *testing.T) {
	a := []Block{{"paragraph", "บริษัท ตัวอย่าง จำกัด (มหาชน) ติดต่อ dpo@example.co.th"}, {"paragraph", "มีผลตั้งแต่ 1 ตุลาคม 2569"}}
	b := []Block{{"paragraph", "ชื่อใหม่ ติดต่อ dpo@example.co.th"}, {"paragraph", "ท่านมีสิทธิถอนความยินยอมได้ทุกเมื่อ"}, {"paragraph", "มีผลตั้งแต่ 1 ตุลาคม 2569 เป็นต้นไป"}}
	cs := Compare(a, b)
	var ops []string
	for _, c := range cs {
		ops = append(ops, c.Op)
	}
	if strings.Join(ops, ",") != "change,insert,change" {
		t.Fatalf("ops %v", ops)
	}
	if got := cs[0].Segments; len(got) != 3 || got[0] != (Segment{"delete", "บริษัท ตัวอย่าง จำกัด (มหาชน)"}) || got[1] != (Segment{"insert", "ชื่อใหม่"}) {
		t.Errorf("segments %+v", got)
	}
	if got := cs[2].Segments; len(got) != 2 || got[1] != (Segment{"insert", " เป็นต้นไป"}) {
		t.Errorf("segments %+v", got)
	}
}
