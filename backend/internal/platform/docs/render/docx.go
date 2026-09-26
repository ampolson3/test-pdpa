package render

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// DOCX writes a Word document (Office Open XML) of the input: headings, paragraphs, lists, quotes, clauses and merge
// fields, bold / italic / underline / strike. Runs set Sarabun for Latin and complex-script (Thai) text and mark the
// language th-TH, so Word shapes Thai correctly and falls back to a Thai system font when Sarabun isn't installed.
func DOCX(in Input) ([]byte, error) {
	var body bytes.Buffer
	d := &docxWriter{b: &body, fields: in.Fields}
	if in.Draft {
		lang := in.Language
		if lang == "" {
			lang = "th"
		}
		d.paragraph("", "", []run{{text: DraftLabel(lang), bold: true, highlight: true}})
	}
	if in.Title != "" {
		d.paragraph("Title", "", []run{{text: in.Title}})
	}
	for _, c := range in.expanded().Content {
		d.block(c, "")
	}
	lang := "th-TH"
	if in.Language == "en" {
		lang = "en-US"
	}
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/></Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/></Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
		"docProps/core.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + esc(in.Title) + `</dc:title><dc:language>` + lang + `</dc:language></cp:coreProperties>`,
		"word/styles.xml": styles(lang),
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() +
			`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="1021" w:bottom="1134" w:left="1021" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr></w:body></w:document>`,
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "docProps/core.xml", "word/_rels/document.xml.rels", "word/styles.xml", "word/document.xml"} {
		f, err := z.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write([]byte(files[name])); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func styles(lang string) string {
	font := `<w:rFonts w:ascii="Sarabun" w:hAnsi="Sarabun" w:cs="Sarabun" w:eastAsia="Sarabun"/>`
	style := func(id, name string, size int, bold bool) string {
		b := ""
		if bold {
			b = "<w:b/><w:bCs/>"
		}
		return fmt.Sprintf(`<w:style w:type="paragraph" w:styleId="%s"><w:name w:val="%s"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="240" w:after="120"/></w:pPr><w:rPr>%s<w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr></w:style>`, id, name, b, size, size)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr>` + font +
		`<w:sz w:val="30"/><w:szCs w:val="30"/><w:lang w:val="` + lang + `" w:bidi="th-TH"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="300" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` +
		style("Title", "Title", 44, true) + style("Heading1", "heading 1", 40, true) + style("Heading2", "heading 2", 34, true) + style("Heading3", "heading 3", 30, true) +
		`<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="567"/></w:pPr><w:rPr><w:i/><w:iCs/></w:rPr></w:style>` +
		`</w:styles>`
}

type run struct {
	text                            string
	br                              bool
	bold, italic, underline, strike bool
	highlight                       bool
}

type docxWriter struct {
	b      *bytes.Buffer
	fields map[string]string
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (d *docxWriter) paragraph(style, prefix string, runs []run) {
	d.b.WriteString("<w:p>")
	if style != "" {
		fmt.Fprintf(d.b, `<w:pPr><w:pStyle w:val="%s"/></w:pPr>`, style)
	}
	if prefix != "" {
		runs = append([]run{{text: prefix}}, runs...)
	}
	for _, r := range runs {
		d.b.WriteString("<w:r>")
		var rpr strings.Builder
		if r.bold {
			rpr.WriteString("<w:b/><w:bCs/>")
		}
		if r.italic {
			rpr.WriteString("<w:i/><w:iCs/>")
		}
		if r.underline {
			rpr.WriteString(`<w:u w:val="single"/>`)
		}
		if r.strike {
			rpr.WriteString("<w:strike/>")
		}
		if r.highlight {
			rpr.WriteString(`<w:highlight w:val="yellow"/>`)
		}
		if rpr.Len() > 0 {
			d.b.WriteString("<w:rPr>" + rpr.String() + "</w:rPr>")
		}
		if r.br {
			d.b.WriteString("<w:br/>")
		} else {
			fmt.Fprintf(d.b, `<w:t xml:space="preserve">%s</w:t>`, esc(r.text))
		}
		d.b.WriteString("</w:r>")
	}
	d.b.WriteString("</w:p>")
}

func (d *docxWriter) inline(n Node) []run {
	var out []run
	for _, c := range n.Content {
		switch c.Type {
		case "text":
			r := run{text: c.Text}
			for _, m := range c.Marks {
				switch m.Type {
				case "bold":
					r.bold = true
				case "italic":
					r.italic = true
				case "underline":
					r.underline = true
				case "strike":
					r.strike = true
				}
			}
			out = append(out, r)
		case "hardBreak":
			out = append(out, run{br: true})
		case "mergeField":
			k := attr(c, "key")
			if v, ok := d.fields[k]; ok {
				out = append(out, run{text: v})
			} else {
				out = append(out, run{text: "[" + k + "]", highlight: true})
			}
		}
	}
	return out
}

func (d *docxWriter) block(n Node, indent string) {
	switch n.Type {
	case "paragraph":
		d.paragraph("", indent, d.inline(n))
	case "heading":
		d.paragraph(fmt.Sprintf("Heading%d", Level(n)), "", d.inline(n))
	case "blockquote":
		for _, c := range n.Content {
			d.block(c, indent)
		}
	case "bulletList", "orderedList":
		for i, item := range n.Content {
			marker := "• "
			if n.Type == "orderedList" {
				marker = fmt.Sprintf("%d. ", i+1)
			}
			for j, c := range item.Content {
				p := indent + "    "
				if j == 0 {
					p = indent + marker
				}
				if c.Type == "paragraph" {
					d.paragraph("", p, d.inline(c))
				} else {
					d.block(c, indent+"    ")
				}
			}
		}
	case "horizontalRule":
		d.paragraph("", "", []run{{text: "────────────────────"}})
	case "clauseBlock":
		if t := attr(n, "title"); t != "" {
			d.paragraph("Heading3", "", []run{{text: t}})
		}
		for _, c := range n.Content {
			d.block(c, indent)
		}
	case "clause":
		d.paragraph("", "", []run{{text: fmt.Sprintf("[%s@%d]", attr(n, "code"), ClauseVersion(n)), highlight: true}})
	}
}
