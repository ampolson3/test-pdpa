package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	noticestore "pdpa-platform/internal/notice/store"
	pdb "pdpa-platform/internal/pkg/db"
	docsservice "pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/versioning"
)

// Topic codes for compose.go's headings — PNG-02's ม.23 checklist keys off these, not the (editable) heading
// text, so rewording a heading in the document editor doesn't itself make the topic count as missing.
const (
	TopicPurposeBasis = "purpose_basis"
	TopicConsequence  = "consequence"
	TopicData         = "data"
	TopicRetention    = "retention"
	TopicRecipients   = "recipients"
	TopicContact      = "contact"
	TopicRights       = "rights"
)

// ChecklistItem is one of ม.23's six mandatory notice topics.
type ChecklistItem struct {
	Code     string // purpose_basis | consequence | data_retention | recipients | contact | rights
	Complete bool
}

// checklistItems are ม.23's six mandatory topics, in the order the law states them (PNG-02's description).
// data_retention is complete only when both the data and retention sections are (they're composed as two
// headings, ม.23 counts them as one item: "ข้อมูลที่เก็บและระยะเวลา").
var checklistItems = []string{"purpose_basis", "consequence", "data_retention", "recipients", "contact", "rights"}

// placeholderPrefix is compose.go's marker for a topic the wizard couldn't derive real content for.
const placeholderPrefix = "["

// Checklist is PNG-02's mandatory-content check: a pure function over the document's Thai content (BP-04 rule
// 5 — a notice needs Thai at minimum, so that's the language checked) — it never touches the database. A topic
// is complete when a heading carries that topic code (attrs.topic, set by compose.go) and the text under it,
// before the next heading, is non-empty and isn't one of the wizard's own bracketed placeholders. A document
// with no topic-coded headings at all (created outside the wizard, e.g. directly through PLT-16's generic
// document endpoints) reads as every topic missing — the checklist only recognizes what PNG-01's wizard
// composes; free-form authoring outside it isn't reworded automatically. Section headings not read (the
// consequence text lacking a `[` prefix but not literally the wizard's own placeholder) still count as
// filled in, so a user rewriting the placeholder in their own words works.
func Checklist(content render.Content) []ChecklistItem {
	doc, ok := content["th"]
	if !ok {
		return nil
	}
	present := map[string]bool{}
	var topic string
	for _, n := range doc.Content {
		if n.Type == "heading" {
			topic, _ = n.Attrs["topic"].(string)
			continue
		}
		if topic == "" || present[topic] {
			continue
		}
		if t := plainText(n); t != "" && !strings.HasPrefix(t, placeholderPrefix) {
			present[topic] = true
		}
	}
	out := make([]ChecklistItem, len(checklistItems))
	for i, code := range checklistItems {
		complete := present[code]
		if code == "data_retention" {
			complete = present[TopicData] && present[TopicRetention]
		}
		out[i] = ChecklistItem{Code: code, Complete: complete}
	}
	return out
}

func plainText(n render.Node) string {
	var sb strings.Builder
	var walk func(render.Node)
	walk = func(n render.Node) {
		if n.Type == "text" {
			sb.WriteString(n.Text)
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// ErrChecklistIncomplete blocks a notice's document from publishing while a ม.23 topic is still missing (PNG-02's
// acceptance criterion). Unwrap reports it as an invalid request through PLT-08's publish endpoint (422),
// following docs.IncompleteError's own pattern for the platform's generic completeness check.
type ErrChecklistIncomplete struct{ Missing []string }

func (e *ErrChecklistIncomplete) Error() string {
	return fmt.Sprintf("notice: missing ม.23 topics: %s", strings.Join(e.Missing, ", "))
}
func (e *ErrChecklistIncomplete) Unwrap() error { return versioning.ErrInvalidRequest }

// GetChecklist returns a notice's current checklist, reading its document's open draft (or, once published,
// its published content — either way, whatever docs.Service.Get resolves as the newest version).
func (s *Service) GetChecklist(ctx context.Context, noticeID uuid.UUID) ([]ChecklistItem, error) {
	n, err := s.GetNotice(ctx, noticeID)
	if err != nil {
		return nil, err
	}
	doc, err := s.Docs.Get(ctx, n.DocumentID)
	if err != nil {
		return nil, err
	}
	if doc.Draft == nil {
		return nil, nil
	}
	return Checklist(doc.Draft.Content), nil
}

// CheckPublishable is docs.Service's per-type publish gate for "notice" documents (wired with SetValidate in
// cmd/api/main.go, once both docs.Service and this service exist) — PNG-02's actual enforcement point: it
// blocks PLT-08's publish transition, in the same request transaction, whenever a ม.23 topic is still missing.
// Configurable per CLAUDE.md's "implement the listed default as configuration" (no decisions.md entry exists
// for this — it's a tunable, not a legally-relevant behaviour): EnforceChecklist, default true.
func (s *Service) CheckPublishable(ctx context.Context, documentID uuid.UUID, d docsservice.Draft) error {
	if !s.EnforceChecklist {
		return nil
	}
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	_, err := q.GetNoticeByDocumentID(ctx, documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // not a notice-backed document (e.g. a future "policy" type) — nothing for PNG-02 to check
	}
	if err != nil {
		return err
	}
	items := Checklist(d.Content)
	var missing []string
	for _, it := range items {
		if !it.Complete {
			missing = append(missing, it.Code)
		}
	}
	if len(missing) > 0 {
		return &ErrChecklistIncomplete{Missing: missing}
	}
	return nil
}
