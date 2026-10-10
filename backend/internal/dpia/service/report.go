package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/platform/docs/render"
)

// ReportMeasure is one ม.37(1) security measure linked to the assessment's own RoPA activity (ROPA-09) —
// DPIA-15's own "มาตรการ" (measures): the closest concrete security-measures concept this codebase has
// built so far, since DPIA-06/07's own risk-scoring and mitigation-measure features aren't built yet.
type ReportMeasure struct {
	Code        string
	Name        string
	Category    string
	Description string
}

// ReportApproval is one DPO opinion recorded on the assessment (DPIA-10) — the report's "ผู้อนุมัติ".
type ReportApproval struct {
	DpoName        string
	Opinion        string
	Recommendation string
	CreatedAt      string // RFC3339, kept as a plain string here — the report is text, not an API response
}

// ReportData is everything DPIA-15's acceptance criterion asks the report to show, computed live (the same
// "never a stale copy" discipline DPIA-04/12 already established) from the assessment's own round, its RoPA
// activity's linked security measures, and every DPO opinion recorded on it.
type ReportData struct {
	ActivityCode    string
	ActivityName    string
	RoundNo         int
	Status          string
	ScreeningResult string
	ScreeningReason string
	Score           float64
	Measures        []ReportMeasure
	Approvals       []ReportApproval
}

func (s *Service) reportData(ctx context.Context, assessmentID uuid.UUID) (ReportData, error) {
	a, err := s.GetAssessment(ctx, assessmentID)
	if err != nil {
		return ReportData{}, err
	}
	act, err := s.Ropa.GetActivity(ctx, a.ActivityID)
	if err != nil {
		return ReportData{}, err
	}
	out := ReportData{
		ActivityCode: act.Code, ActivityName: act.Name, RoundNo: a.RoundNo, Status: a.Status,
		ScreeningResult: a.ScreeningResult, ScreeningReason: a.ScreeningReason, Score: a.Score,
	}

	links, err := s.Ropa.ListActivityControls(ctx, act.ID)
	if err != nil {
		return ReportData{}, err
	}
	if len(links) > 0 {
		catalog, err := s.Ropa.ListControls(ctx)
		if err != nil {
			return ReportData{}, err
		}
		byID := make(map[uuid.UUID]int, len(catalog))
		for i, c := range catalog {
			byID[c.ID] = i
		}
		for _, l := range links {
			i, ok := byID[l.ControlID]
			if !ok {
				continue
			}
			c := catalog[i]
			out.Measures = append(out.Measures, ReportMeasure{Code: c.Code, Name: c.Name, Category: c.Category, Description: l.Description})
		}
	}

	opinions, err := s.ListOpinions(ctx, assessmentID)
	if err != nil {
		return ReportData{}, err
	}
	if len(opinions) > 0 {
		ids := make([]uuid.UUID, len(opinions))
		for i, o := range opinions {
			ids[i] = o.DpoUserID
		}
		names, err := iamservice.AllNames(ctx, ids)
		if err != nil {
			return ReportData{}, err
		}
		for _, o := range opinions {
			name := names[o.DpoUserID]
			if name == "" {
				name = o.DpoUserID.String()
			}
			out.Approvals = append(out.Approvals, ReportApproval{
				DpoName: name, Opinion: o.Opinion, Recommendation: o.Recommendation, CreatedAt: o.CreatedAt.Format("2006-01-02 15:04"),
			})
		}
	}
	return out, nil
}

// reportLabels is the report's own fixed bilingual strings (operational report text, not legal wording
// shown to a data subject or the PDPC — rule 8 doesn't apply here, the same reasoning DPO-09/BRE's own
// internal assessment text already used).
var reportLabels = map[string]map[string]string{
	"th": {
		"title": "รายงาน DPIA", "screening": "ผลคัดกรองและคะแนน", "round": "รอบที่ %d", "status": "สถานะ: %s",
		"result": "ผล: %s", "score": "คะแนน: %.1f", "reason": "เหตุผล: %s", "measures": "มาตรการความปลอดภัย",
		"noMeasures": "ยังไม่มีมาตรการความปลอดภัยที่เชื่อมกับกิจกรรมนี้", "approvals": "ความเห็นและผู้อนุมัติ",
		"noApprovals": "ยังไม่มีความเห็นที่บันทึกไว้",
	},
	"en": {
		"title": "DPIA Report", "screening": "Screening result and score", "round": "Round %d", "status": "Status: %s",
		"result": "Result: %s", "score": "Score: %.1f", "reason": "Reason: %s", "measures": "Security measures",
		"noMeasures": "No security measures linked to this activity yet", "approvals": "Opinions and approvers",
		"noApprovals": "No opinion recorded yet",
	},
}

func heading(lang string, level int, text string) render.Node {
	return render.Node{Type: "heading", Attrs: map[string]any{"level": level}, Content: []render.Node{{Type: "text", Text: text}}}
}

func paragraph(text string) render.Node {
	return render.Node{Type: "paragraph", Content: []render.Node{{Type: "text", Text: text}}}
}

func bulletList(items []string) render.Node {
	var li []render.Node
	for _, it := range items {
		li = append(li, render.Node{Type: "listItem", Content: []render.Node{paragraph(it)}})
	}
	return render.Node{Type: "bulletList", Content: li}
}

// reportInput builds the render.Input for one language — no merge fields or clauses (this is an internal
// operational report, not a notice/letter; rule 8 doesn't gate it), never a draft banner.
func reportInput(lang string, d ReportData) render.Input {
	l := reportLabels[lang]
	title := fmt.Sprintf("%s: %s (%s)", l["title"], d.ActivityName, d.ActivityCode)
	var content []render.Node
	content = append(content, heading(lang, 1, title))

	content = append(content, heading(lang, 2, l["screening"]))
	content = append(content, paragraph(fmt.Sprintf(l["round"], d.RoundNo)))
	content = append(content, paragraph(fmt.Sprintf(l["status"], d.Status)))
	content = append(content, paragraph(fmt.Sprintf(l["result"], d.ScreeningResult)))
	content = append(content, paragraph(fmt.Sprintf(l["score"], d.Score)))
	if d.ScreeningReason != "" {
		content = append(content, paragraph(fmt.Sprintf(l["reason"], d.ScreeningReason)))
	}

	content = append(content, heading(lang, 2, l["measures"]))
	if len(d.Measures) == 0 {
		content = append(content, paragraph(l["noMeasures"]))
	} else {
		var items []string
		for _, m := range d.Measures {
			line := fmt.Sprintf("%s (%s) [%s]", m.Name, m.Code, m.Category)
			if m.Description != "" {
				line += " — " + m.Description
			}
			items = append(items, line)
		}
		content = append(content, bulletList(items))
	}

	content = append(content, heading(lang, 2, l["approvals"]))
	if len(d.Approvals) == 0 {
		content = append(content, paragraph(l["noApprovals"]))
	} else {
		var items []string
		for _, a := range d.Approvals {
			items = append(items, fmt.Sprintf("%s — %s — %s (%s)", a.DpoName, a.Recommendation, a.Opinion, a.CreatedAt))
		}
		content = append(content, bulletList(items))
	}

	return render.Input{Title: title, Language: lang, Body: render.Node{Type: "doc", Content: content}}
}

// Report is DPIA-15's acceptance criterion: a PDF/Word report, TH or EN, showing the assessment's score,
// linked security measures and every DPO opinion/approver — composed live, the same discipline DPIA-04/12
// already established (no stored document, nothing to go stale).
func (s *Service) Report(ctx context.Context, assessmentID uuid.UUID, lang, format string) ([]byte, string, error) {
	if lang != "th" && lang != "en" {
		return nil, "", fmt.Errorf("%w: language", ErrInvalid)
	}
	d, err := s.reportData(ctx, assessmentID)
	if err != nil {
		return nil, "", err
	}
	in := reportInput(lang, d)
	name := fmt.Sprintf("DPIA-%s-%s", d.ActivityCode, lang)
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
