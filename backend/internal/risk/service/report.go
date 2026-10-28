package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/platform/docs/render"
)

// riskReportLabels is this report's own fixed bilingual strings — an internal operational summary for
// EXEC, not legal wording shown to a data subject or the PDPC, so rule 8 doesn't gate it (the same
// reasoning dataflow's own report.go already used for DFG-08).
var riskReportLabels = map[string]map[string]string{
	"th": {
		"title": "รายงานความเสี่ยงสำหรับผู้บริหาร", "generatedAt": "สร้างเมื่อ %s",
		"summary": "ความเสี่ยงระดับสูง/สูงมากที่ยังเปิดอยู่ %d รายการ", "noRisks": "ไม่มีความเสี่ยงระดับสูง/สูงมากที่ยังเปิดอยู่",
		"level": "ระดับ", "score": "คะแนน", "owner": "ผู้รับผิดชอบ", "noOwner": "ยังไม่ระบุ",
		"status": "สถานะความเสี่ยง", "remediation": "แผนแก้ไข/ความคืบหน้า", "noTask": "ยังไม่มีงานแก้ไข",
	},
	"en": {
		"title": "Executive Risk Report", "generatedAt": "Generated %s",
		"summary": "%d open high/very-high risks", "noRisks": "No open high/very-high risks",
		"level": "Level", "score": "Score", "owner": "Owner", "noOwner": "Unassigned",
		"status": "Risk status", "remediation": "Remediation / progress", "noTask": "No remediation task yet",
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

// ExecutiveReport is RRA-14's own acceptance criterion: "รายงานสร้างได้ในคลิกเดียวและตรงกับทะเบียน" — built
// live from the exact same risk register RRA-09's own ListRisks already serves (never a stale, separately
// maintained copy), narrowed to every still-open risk whose *effective* (residual, once any control is
// linked — RRA-09's own EffectiveLevel) level currently reads high/very_high, each with its owner and its
// own remediation task's current status (DpoTasks.TaskStatusForRisk, nil-means-skip the same way every
// other optional cross-cutting field on this Service already works).
func (s *Service) ExecutiveReport(ctx context.Context, lang, format string) ([]byte, string, error) {
	if lang != "th" && lang != "en" {
		return nil, "", fmt.Errorf("%w: language", ErrInvalid)
	}
	l := riskReportLabels[lang]

	var risks []Risk
	var after *RiskCursor
	for {
		page, next, err := s.ListRisks(ctx, RiskFilter{After: after})
		if err != nil {
			return nil, "", err
		}
		for _, r := range page {
			if r.Status == "closed" {
				continue
			}
			if lvl := r.EffectiveLevel(); lvl == "high" || lvl == "very_high" {
				risks = append(risks, r)
			}
		}
		if next == nil {
			break
		}
		after = next
	}

	var ownerIDs []uuid.UUID
	for _, r := range risks {
		if r.OwnerUserID != nil {
			ownerIDs = append(ownerIDs, *r.OwnerUserID)
		}
	}
	names, err := iamservice.Names(ctx, ownerIDs)
	if err != nil {
		return nil, "", err
	}

	title := l["title"]
	var content []render.Node
	content = append(content, rHeading(1, title))
	content = append(content, rParagraph(fmt.Sprintf(l["generatedAt"], s.now().Format("2006-01-02 15:04 UTC"))))
	content = append(content, rParagraph(fmt.Sprintf(l["summary"], len(risks))))

	if len(risks) == 0 {
		content = append(content, rParagraph(l["noRisks"]))
	}
	for _, r := range risks {
		content = append(content, rHeading(2, r.Title))
		owner := l["noOwner"]
		if r.OwnerUserID != nil {
			if n, ok := names[*r.OwnerUserID]; ok {
				owner = n
			}
		}
		remediation := l["noTask"]
		if s.Dpo != nil {
			st, found, err := s.Dpo.TaskStatusForRisk(ctx, r.ID)
			if err != nil {
				return nil, "", err
			}
			if found {
				remediation = st
			}
		}
		content = append(content, rBulletList([]string{
			fmt.Sprintf("%s: %s", l["level"], r.EffectiveLevel()),
			fmt.Sprintf("%s: %.1f", l["score"], r.InherentScore),
			fmt.Sprintf("%s: %s", l["owner"], owner),
			fmt.Sprintf("%s: %s", l["status"], r.Status),
			fmt.Sprintf("%s: %s", l["remediation"], remediation),
		}))
	}

	in := render.Input{Title: title, Language: lang, Body: render.Node{Type: "doc", Content: content}}
	name := "risk-executive-report-" + lang
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
