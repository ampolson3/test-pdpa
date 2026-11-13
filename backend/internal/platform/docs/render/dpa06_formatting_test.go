package render

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// formattingSample is a document using every DPA-06 formatting capability: paragraph/heading text alignment, a
// font-size mark, and a table of contents that must reflect the document's own headings.
func formattingSample(t *testing.T) Input {
	return Input{Title: "เอกสารทดสอบการจัดรูปแบบ", Language: "th",
		Body: doc(t, `{"type":"doc","content":[
			{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"หัวข้อที่หนึ่ง"}]},
			{"type":"tableOfContents"},
			{"type":"paragraph","attrs":{"textAlign":"center"},"content":[{"type":"text","text":"กึ่งกลาง"}]},
			{"type":"paragraph","attrs":{"textAlign":"right"},"content":[{"type":"text","text":"ชิดขวา"}]},
			{"type":"paragraph","attrs":{"textAlign":"justify"},"content":[{"type":"text","text":"กระจายขอบ"}]},
			{"type":"paragraph","content":[{"type":"text","text":"ใหญ่","marks":[{"type":"fontSize","attrs":{"size":"large"}}]}]},
			{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"หัวข้อย่อย"}]}]}`)}
}

// Acceptance DPA-06: reformatting (text alignment, font size) and inserting a table of contents shows up correctly
// in the exported file — HTML (what the PDF renderer prints from) and DOCX both match what the editor produced.
func TestFormatting_HTMLMatchesWhatWasSet(t *testing.T) {
	h := string(HTML(formattingSample(t)))
	for _, want := range []string{
		`<p style="text-align:center">กึ่งกลาง</p>`,
		`<p style="text-align:right">ชิดขวา</p>`,
		`<p style="text-align:justify">กระจายขอบ</p>`,
		`<span style="font-size:130%">ใหญ่</span>`,
		`<nav class="toc">`,
		`<p class="toc-l1">หัวข้อที่หนึ่ง</p>`,
		`<p class="toc-l2">หัวข้อย่อย</p>`,
	} {
		if !strings.Contains(h, want) {
			t.Errorf("html lacks %q\n%s", want, h)
		}
	}
	// A left-aligned (default) paragraph carries no inline style at all.
	if strings.Contains(h, `<h1 style`) {
		t.Error("default-aligned heading should not carry a style attribute")
	}
}

func TestFormatting_DOCXMatchesWhatWasSet(t *testing.T) {
	b, err := DOCX(formattingSample(t))
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var doc string
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			r, _ := f.Open()
			data, _ := io.ReadAll(r)
			r.Close()
			doc = string(data)
		}
	}
	for _, want := range []string{
		`<w:jc w:val="center"/>`,
		`<w:jc w:val="right"/>`,
		`<w:jc w:val="both"/>`, // "justify" has no literal w:jc value; Word's own is "both"
		`<w:sz w:val="40"/><w:szCs w:val="40"/>`,
		"หัวข้อที่หนึ่ง",
		"หัวข้อย่อย",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document.xml lacks %q\n%s", want, doc)
		}
	}
}

// The table of contents is computed live from the body every render, never stored — editing a heading after
// inserting the TOC changes what the TOC shows on the very next export, with nothing to go stale.
func TestFormatting_TableOfContentsReflectsCurrentHeadings(t *testing.T) {
	in := Input{Language: "th", Body: doc(t, `{"type":"doc","content":[
		{"type":"tableOfContents"},
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"เดิม"}]}]}`)}
	if !strings.Contains(string(HTML(in)), "เดิม") {
		t.Fatal("toc should list the original heading")
	}
	in.Body.Content[1].Content[0].Text = "ใหม่"
	h := string(HTML(in))
	if strings.Contains(h, "เดิม") || !strings.Contains(h, "ใหม่") {
		t.Errorf("toc did not follow the edited heading: %s", h)
	}
}

func TestFormatting_ValidatesTextAlignAndFontSize(t *testing.T) {
	for name, c := range map[string]Content{
		"bad paragraph align": {"th": doc(t, `{"type":"doc","content":[{"type":"paragraph","attrs":{"textAlign":"diagonal"}}]}`)},
		"bad heading align":   {"th": doc(t, `{"type":"doc","content":[{"type":"heading","attrs":{"level":1,"textAlign":"diagonal"}}]}`)},
		"bad font size": {"th": doc(t, `{"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"x","marks":[{"type":"fontSize","attrs":{"size":"huge"}}]}]}]}`)},
	} {
		if err := c.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	ok := Content{"th": doc(t, `{"type":"doc","content":[
		{"type":"tableOfContents"},
		{"type":"paragraph","attrs":{"textAlign":"center"},"content":[{"type":"text","text":"x","marks":[{"type":"fontSize","attrs":{"size":"small"}}]}]}]}`)}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid formatting refused: %v", err)
	}
}
