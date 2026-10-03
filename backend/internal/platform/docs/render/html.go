package render

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"html"
	"strings"
)

//go:embed fonts/*.woff2
var fonts embed.FS

// fontFaces embeds Sarabun (SIL OFL, fonts/OFL.txt) — Thai and Latin subsets, regular and bold — so a PDF renders the
// same Thai glyphs wherever it is produced, with no font installed on the renderer.
func fontFaces() string {
	var b strings.Builder
	for _, f := range []struct {
		file   string
		weight int
		rng    string
	}{
		{"sarabun-thai-400-normal.woff2", 400, "U+02D7, U+0303, U+0331, U+0E01-0E5B, U+200C-200D, U+25CC"},
		{"sarabun-thai-700-normal.woff2", 700, "U+02D7, U+0303, U+0331, U+0E01-0E5B, U+200C-200D, U+25CC"},
		{"sarabun-latin-400-normal.woff2", 400, "U+0000-00FF, U+0131, U+0152-0153, U+02BB-02BC, U+02C6, U+02DA, U+02DC, U+0304, U+0308, U+0329, U+2000-206F, U+20AC, U+2122, U+2191, U+2193, U+2212, U+2215, U+FEFF, U+FFFD"},
		{"sarabun-latin-700-normal.woff2", 700, "U+0000-00FF, U+0131, U+0152-0153, U+02BB-02BC, U+02C6, U+02DA, U+02DC, U+0304, U+0308, U+0329, U+2000-206F, U+20AC, U+2122, U+2191, U+2193, U+2212, U+2215, U+FEFF, U+FFFD"},
	} {
		data, err := fonts.ReadFile("fonts/" + f.file)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "@font-face{font-family:'Sarabun';font-style:normal;font-weight:%d;src:url(data:font/woff2;base64,%s) format('woff2');unicode-range:%s}\n",
			f.weight, base64.StdEncoding.EncodeToString(data), f.rng)
	}
	return b.String()
}

const pageCSS = `
@page{size:A4;margin:20mm 18mm}
body{font-family:'Sarabun','Noto Sans Thai','Loma',sans-serif;font-size:15pt;line-height:1.5;color:#111}
h1{font-size:20pt;margin:0 0 8pt}h2{font-size:17pt;margin:14pt 0 6pt}h3{font-size:15pt;margin:12pt 0 4pt}
p{margin:0 0 6pt}ul,ol{margin:0 0 6pt 18pt;padding:0}blockquote{margin:6pt 0 6pt 12pt;padding-left:8pt;border-left:2pt solid #999}
hr{border:0;border-top:1px solid #999;margin:10pt 0}.clause{margin:8pt 0}.clause>.clause-title{font-weight:700;margin-bottom:4pt}
.missing{background:#fde68a;padding:0 2pt}.doc-title{font-size:22pt;font-weight:700;margin-bottom:12pt}.draft{border:2pt solid #b91c1c;color:#b91c1c;font-weight:700;text-align:center;padding:4pt;margin-bottom:12pt}
`

// HTML renders a complete, self-contained page (fonts embedded) — what Gotenberg or Chromium prints to PDF and what
// the preview shows. Every text is escaped; only the known node types produce markup.
func HTML(in Input) []byte {
	var b bytes.Buffer
	lang := in.Language
	if lang == "" {
		lang = "th"
	}
	fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title><style>%s%s</style></head><body>`,
		html.EscapeString(lang), html.EscapeString(in.Title), fontFaces(), pageCSS)
	if in.Draft {
		fmt.Fprintf(&b, `<div class="draft">%s</div>`, html.EscapeString(DraftLabel(lang)))
	}
	if in.Title != "" {
		fmt.Fprintf(&b, `<div class="doc-title">%s</div>`, html.EscapeString(in.Title))
	}
	w := &htmlWriter{b: &b, fields: in.Fields}
	for _, c := range in.expanded().Content {
		w.node(c)
	}
	b.WriteString("</body></html>")
	return b.Bytes()
}

type htmlWriter struct {
	b      *bytes.Buffer
	fields map[string]string
}

func (w *htmlWriter) children(n Node) {
	for _, c := range n.Content {
		w.node(c)
	}
}

func (w *htmlWriter) node(n Node) {
	b := w.b
	switch n.Type {
	case "paragraph":
		b.WriteString("<p>")
		w.children(n)
		b.WriteString("</p>")
	case "heading":
		l := Level(n)
		fmt.Fprintf(b, "<h%d>", l)
		w.children(n)
		fmt.Fprintf(b, "</h%d>", l)
	case "bulletList":
		b.WriteString("<ul>")
		w.children(n)
		b.WriteString("</ul>")
	case "orderedList":
		b.WriteString("<ol>")
		w.children(n)
		b.WriteString("</ol>")
	case "listItem":
		b.WriteString("<li>")
		w.children(n)
		b.WriteString("</li>")
	case "blockquote":
		b.WriteString("<blockquote>")
		w.children(n)
		b.WriteString("</blockquote>")
	case "horizontalRule":
		b.WriteString("<hr>")
	case "clauseBlock":
		fmt.Fprintf(b, `<div class="clause" data-clause="%s">`, html.EscapeString(attr(n, "code")))
		if t := attr(n, "title"); t != "" {
			fmt.Fprintf(b, `<div class="clause-title">%s</div>`, html.EscapeString(t))
		}
		w.children(n)
		b.WriteString("</div>")
	case "clause": // not resolved: a visible placeholder
		fmt.Fprintf(b, `<p><span class="missing">[%s@%d]</span></p>`, html.EscapeString(attr(n, "code")), ClauseVersion(n))
	case "hardBreak":
		b.WriteString("<br>")
	case "mergeField":
		k := attr(n, "key")
		if v, ok := w.fields[k]; ok {
			b.WriteString(html.EscapeString(v))
		} else {
			fmt.Fprintf(b, `<span class="missing">[%s]</span>`, html.EscapeString(k))
		}
	case "text":
		open, close := marks(n.Marks)
		b.WriteString(open)
		b.WriteString(html.EscapeString(n.Text))
		b.WriteString(close)
	}
}

func marks(ms []Mark) (string, string) {
	tags := map[string]string{"bold": "strong", "italic": "em", "underline": "u", "strike": "s"}
	var open, close strings.Builder
	for i, m := range ms {
		if t, ok := tags[m.Type]; ok {
			open.WriteString("<" + t + ">")
			_ = i
		}
	}
	for i := len(ms) - 1; i >= 0; i-- {
		if t, ok := tags[ms[i].Type]; ok {
			close.WriteString("</" + t + ">")
		}
	}
	return open.String(), close.String()
}
