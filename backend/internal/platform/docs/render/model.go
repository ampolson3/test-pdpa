// Package render turns a document's structured content (ProseMirror JSON as the TipTap editor saves it) into HTML,
// PDF (through Gotenberg, or a local headless Chromium in development) and DOCX, and compares two versions (PLT-16).
// It has no database access: callers resolve merge fields and clauses and pass them in.
package render

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Node is a ProseMirror node.
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []Node         `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
}

// Mark is inline formatting on a text node.
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// Content is a document's body per language ("th" required, "en" optional).
type Content map[string]Node

// Node and mark types the editor may produce; anything else is refused (and so never reaches the renderers).
var (
	blockTypes  = map[string]bool{"doc": true, "paragraph": true, "heading": true, "bulletList": true, "orderedList": true, "listItem": true, "blockquote": true, "horizontalRule": true, "clause": true}
	inlineTypes = map[string]bool{"text": true, "hardBreak": true, "mergeField": true}
	markTypes   = map[string]bool{"bold": true, "italic": true, "underline": true, "strike": true}
)

// ErrInvalid wraps every content problem.
var ErrInvalid = errors.New("render: invalid content")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// MaxNodes bounds a document.
const MaxNodes = 20000

// Validate checks a content: "th" present, only known languages, nodes and marks, and sane attributes.
func (c Content) Validate() error {
	if _, ok := c["th"]; !ok {
		return invalid("th is required")
	}
	for lang, doc := range c {
		if lang != "th" && lang != "en" {
			return invalid("language %q", lang)
		}
		if doc.Type != "doc" {
			return invalid("%s: root must be doc", lang)
		}
		n := 0
		if err := validate(doc, 0, &n); err != nil {
			return fmt.Errorf("%s: %w", lang, err)
		}
	}
	return nil
}

func validate(n Node, depth int, count *int) error {
	*count++
	if *count > MaxNodes || depth > 40 {
		return invalid("document too large")
	}
	switch {
	case blockTypes[n.Type], inlineTypes[n.Type]:
	default:
		return invalid("node type %q", n.Type)
	}
	switch n.Type {
	case "heading":
		if l := Level(n); l < 1 || l > 3 {
			return invalid("heading level")
		}
	case "mergeField":
		if !keyRE(attr(n, "key")) {
			return invalid("merge field key")
		}
	case "clause":
		if !keyRE(attr(n, "code")) || ClauseVersion(n) < 1 {
			return invalid("clause reference")
		}
	case "text":
		if n.Text == "" {
			return invalid("empty text node")
		}
		for _, m := range n.Marks {
			if !markTypes[m.Type] {
				return invalid("mark %q", m.Type)
			}
		}
	}
	for _, c := range n.Content {
		if err := validate(c, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

func keyRE(s string) bool {
	if s == "" || len(s) > 80 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-') {
			return false
		}
	}
	return true
}

func attr(n Node, k string) string {
	if v, ok := n.Attrs[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Level is a heading's level.
func Level(n Node) int {
	switch v := n.Attrs["level"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	}
	return 0
}

// ClauseVersion is a clause node's version.
func ClauseVersion(n Node) int {
	switch v := n.Attrs["version"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// ClauseKey is how a clause reference is looked up: "code@version".
func ClauseKey(code string, version int) string { return fmt.Sprintf("%s@%d", code, version) }

// FieldKeys returns the merge fields a node uses, in order of first use.
func FieldKeys(n Node) []string {
	var out []string
	seen := map[string]bool{}
	walk(n, func(x Node) {
		if x.Type == "mergeField" {
			if k := attr(x, "key"); !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	})
	return out
}

// ClauseRefs returns the clause references (code@version) a node uses.
func ClauseRefs(n Node) []string {
	var out []string
	seen := map[string]bool{}
	walk(n, func(x Node) {
		if x.Type == "clause" {
			k := ClauseKey(attr(x, "code"), ClauseVersion(x))
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	})
	return out
}

func walk(n Node, fn func(Node)) {
	fn(n)
	for _, c := range n.Content {
		walk(c, fn)
	}
}

// Input is one language of a document ready to render.
type Input struct {
	Title    string
	Language string // th | en
	Body     Node
	// Fields are merge field values; a field without one shows as [key] (a draft) — publishing requires them all.
	Fields map[string]string
	// Clauses are the clause bodies in this language, by ClauseKey.
	Clauses map[string]Clause
	// Draft marks an unapproved version: exports carry a DRAFT banner (legal text stays a draft until approved).
	Draft bool
}

// DraftLabel is the banner of a draft export, in the document's language.
func DraftLabel(lang string) string {
	if lang == "en" {
		return "DRAFT — not approved"
	}
	return "ร่าง — ยังไม่ได้รับการอนุมัติ"
}

// Clause is a clause's title and body in one language.
type Clause struct {
	Title string
	Body  Node
}

// Missing lists the merge fields and clauses of the input that have no value / body.
func (in Input) Missing() (fields, clauses []string) {
	body := in.expanded()
	for _, k := range FieldKeys(body) {
		if _, ok := in.Fields[k]; !ok {
			fields = append(fields, k)
		}
	}
	for _, k := range ClauseRefs(in.Body) {
		if _, ok := in.Clauses[k]; !ok {
			clauses = append(clauses, k)
		}
	}
	return
}

// expanded replaces clause references by their bodies (one level: clauses can't contain clauses).
func (in Input) expanded() Node {
	return expand(in.Body, in.Clauses)
}

func expand(n Node, clauses map[string]Clause) Node {
	out := n
	out.Content = nil
	for _, c := range n.Content {
		if c.Type == "clause" {
			if cl, ok := clauses[ClauseKey(attr(c, "code"), ClauseVersion(c))]; ok {
				out.Content = append(out.Content, Node{Type: "clauseBlock", Attrs: map[string]any{"title": cl.Title, "code": attr(c, "code")}, Content: cl.Body.Content})
				continue
			}
		}
		out.Content = append(out.Content, expand(c, clauses))
	}
	return out
}

// PlainText is a node's text with fields resolved (for search, diff and tests).
func PlainText(n Node, fields map[string]string) string {
	var b strings.Builder
	var rec func(Node)
	rec = func(x Node) {
		switch x.Type {
		case "text":
			b.WriteString(x.Text)
		case "hardBreak":
			b.WriteString("\n")
		case "mergeField":
			k := attr(x, "key")
			if v, ok := fields[k]; ok {
				b.WriteString(v)
			} else {
				b.WriteString("[" + k + "]")
			}
		}
		for _, c := range x.Content {
			rec(c)
		}
	}
	rec(n)
	return b.String()
}
