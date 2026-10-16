package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"pdpa-platform/internal/platform/docs/render"
)

// reportLabels is this report's own fixed bilingual strings — an internal operational summary of the
// diagram, not legal wording shown to a data subject or the PDPC, so rule 8 doesn't gate it (the same
// reasoning DPIA-15's own report.go already used).
var dataflowReportLabels = map[string]map[string]string{
	"th": {
		"title": "รายงานแผนผังการไหลของข้อมูล", "activities": "กิจกรรม %d รายการ", "noActivities": "ไม่พบกิจกรรมที่ตรงกับเงื่อนไข",
		"data": "ประเภทข้อมูล", "recipients": "ผู้รับข้อมูล", "noRecipients": "ไม่มีผู้รับข้อมูล",
		"transfers": "การโอนไปต่างประเทศ", "noTransfers": "ไม่มีการโอนไปต่างประเทศ", "disposal": "วิธีทำลาย/ลบข้อมูล",
		"noDisposal": "ยังไม่ได้กำหนดวิธีทำลาย/ลบข้อมูล", "missingDpa": "ไม่มี DPA", "missingBasis": "ไม่มีฐานการโอน (คำเตือน)",
	},
	"en": {
		"title": "Data Flow Diagram Report", "activities": "%d processing activities", "noActivities": "No activities match the filter",
		"data": "Data categories", "recipients": "Recipients", "noRecipients": "No recipients",
		"transfers": "Cross-border transfers", "noTransfers": "No cross-border transfers", "disposal": "Disposal method",
		"noDisposal": "No disposal method set yet", "missingDpa": "no DPA on file", "missingBasis": "no transfer basis logged (warning)",
	},
}

func rHeading(level int, text string) render.Node {
	return render.Node{Type: "heading", Attrs: map[string]any{"level": level}, Content: []render.Node{{Type: "text", Text: text}}}
}

func rParagraph(text string) render.Node {
	return render.Node{Type: "paragraph", Content: []render.Node{{Type: "text", Text: text}}}
}

func rBulletList(items []string) render.Node {
	var li []render.Node
	for _, it := range items {
		li = append(li, render.Node{Type: "listItem", Content: []render.Node{rParagraph(it)}})
	}
	return render.Node{Type: "bulletList", Content: li}
}

// reportInput builds the written companion ("รายงานประกอบ") to the exported diagram image — built from the
// exact same live Graph() both the image export and this report share, so the two are always consistent with
// each other and with RoPA. One section per org-unit lane, one subsection per activity: its data categories,
// recipients (flagging a processor with no DPA on file, the same DFG-04 warning the image itself colours
// red), logged transfers (flagging one with no basis), and disposal methods.
func reportInput(lang string, g Graph) render.Input {
	l := dataflowReportLabels[lang]
	title := l["title"]

	byID := map[string]Node{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	childrenByParent := map[string][]Node{}
	for _, n := range g.Nodes {
		if n.ParentID != "" {
			childrenByParent[n.ParentID] = append(childrenByParent[n.ParentID], n)
		}
	}
	edgesBySource := map[string][]Edge{}
	for _, e := range g.Edges {
		edgesBySource[e.Source] = append(edgesBySource[e.Source], e)
	}

	var content []render.Node
	content = append(content, rHeading(1, title))
	content = append(content, rParagraph(fmt.Sprintf(l["activities"], g.ActivityCount)))

	if g.ActivityCount == 0 {
		content = append(content, rParagraph(l["noActivities"]))
		return render.Input{Title: title, Language: lang, Body: render.Node{Type: "doc", Content: content}}
	}

	for _, n := range g.Nodes {
		if n.Type != "org_unit" {
			continue
		}
		content = append(content, rHeading(2, n.Label))
		for _, act := range childrenByParent[n.ID] {
			if act.Type != "activity" {
				continue
			}
			sub := act.Label
			if act.ActivityCode != "" {
				sub = fmt.Sprintf("%s (%s)", act.Label, act.ActivityCode)
			}
			content = append(content, rHeading(3, sub))

			if len(act.DataCategories) > 0 {
				content = append(content, rParagraph(l["data"]+": "+joinComma(act.DataCategories)))
			}

			var recipients []string
			for _, e := range edgesBySource[act.ID] {
				if e.Type != "send" {
					continue
				}
				rn, ok := byID[e.Target]
				if !ok {
					continue
				}
				line := fmt.Sprintf("%s (%s)", rn.Label, rn.RecipientRole)
				if rn.Warning {
					line += " — " + l["missingDpa"]
				}
				recipients = append(recipients, line)
			}
			content = append(content, rHeading(4, l["recipients"]))
			if len(recipients) == 0 {
				content = append(content, rParagraph(l["noRecipients"]))
			} else {
				content = append(content, rBulletList(recipients))
			}

			// A transfer edge's source is either the activity itself (an implicit, never-logged transfer —
			// graph.go draws those straight from the activity) or one of its recipients (a real,
			// ROPA-08-logged transfer, drawn from the recipient) — collect both.
			var transfers []string
			for _, e := range edgesBySource[act.ID] {
				if e.Type == "transfer" {
					transfers = append(transfers, transferLine(byID[e.Target], e, l))
				}
			}
			for _, e := range edgesBySource[act.ID] {
				if e.Type != "send" {
					continue
				}
				for _, te := range edgesBySource[e.Target] {
					if te.Type == "transfer" {
						transfers = append(transfers, transferLine(byID[te.Target], te, l))
					}
				}
			}
			content = append(content, rHeading(4, l["transfers"]))
			if len(transfers) == 0 {
				content = append(content, rParagraph(l["noTransfers"]))
			} else {
				content = append(content, rBulletList(dedupe(transfers)))
			}

			content = append(content, rHeading(4, l["disposal"]))
			if len(act.DisposalMethods) == 0 {
				content = append(content, rParagraph(l["noDisposal"]))
			} else {
				content = append(content, rBulletList(act.DisposalMethods))
			}
		}
	}

	return render.Input{Title: title, Language: lang, Body: render.Node{Type: "doc", Content: content}}
}

func transferLine(country Node, e Edge, l map[string]string) string {
	line := country.Label
	if e.Warning {
		line += " — " + l["missingBasis"]
	} else if e.Label != "" {
		line += " — " + e.Label
	}
	return line
}

func joinComma(items []string) string {
	out := ""
	for i, it := range items {
		if i > 0 {
			out += ", "
		}
		out += it
	}
	return out
}

func dedupe(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		if seen[it] {
			continue
		}
		seen[it] = true
		out = append(out, it)
	}
	return out
}

// Report is DFG-08's own "รายงานประกอบ" (accompanying report): the written companion to ExportDiagram, built
// from the exact same live Graph() so it never drifts from the exported image.
func (s *Service) Report(ctx context.Context, legalEntityID *uuid.UUID, lang, format string) ([]byte, string, error) {
	if lang != "th" && lang != "en" {
		return nil, "", fmt.Errorf("%w: language", ErrInvalid)
	}
	g, err := s.Graph(ctx, legalEntityID)
	if err != nil {
		return nil, "", err
	}
	in := reportInput(lang, g)
	name := "dataflow-report-" + lang
	switch format {
	case "pdf":
		if s.PDF == nil {
			return nil, "", render.ErrNoRenderer
		}
		b, err := s.PDF.PDF(ctx, render.HTML(in))
		return b, name + ".pdf", err
	case "docx":
		b, err := render.DOCX(in)
		return b, name + ".docx", err
	case "html":
		return render.HTML(in), name + ".html", nil
	}
	return nil, "", fmt.Errorf("%w: format", ErrInvalid)
}
