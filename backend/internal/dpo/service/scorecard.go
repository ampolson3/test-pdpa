// DPO-10 (compliance scorecard): a per-org-unit summary combining what every module already computes —
// RoPA completeness (ROPA-03), legal-gap readiness (RRA-13), DPIA screening/approval status (DPIA-01/02/10)
// and training completion (DPX-01) — read live every call (the acceptance criterion's own "คะแนนคำนวณซ้ำ
// ได้": reproducible, never a stale cached number) with every component named (the acceptance criterion's
// own "อธิบายที่มาได้": explainable, never a bare overall figure). "คำขอเกินกำหนด" (overdue requests) from
// the module description is deliberately left out: dsar.requests has no org-unit link in this schema (a
// request is tied to a legal entity and a data subject, not a department), so there is nothing to score
// per unit — the tenant-wide picture is already DPO-05's own Deadlines, not duplicated here.
package service

import (
	"context"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// ScorecardComponent is one named factor behind the overall score — the acceptance criterion's own
// "อธิบายที่มาได้."
type ScorecardComponent struct {
	Name   string // ropa_completeness | legal_readiness | dpia_readiness | training_completion
	Score  float64
	Detail string
}

// Scorecard is one org unit's own PDPA-readiness summary.
type Scorecard struct {
	OrgUnitID  uuid.UUID
	Overall    float64
	Components []ScorecardComponent
}

// OrgUnitScorecard is the acceptance criterion itself: a reproducible, explained score — recomputed from
// live data on every call, never persisted.
func (s *Service) OrgUnitScorecard(ctx context.Context, orgUnitID uuid.UUID) (Scorecard, error) {
	out := Scorecard{OrgUnitID: orgUnitID}

	var activities []ropaservice.Activity
	if s.Ropa != nil {
		var err error
		activities, err = s.listAllActivities(ctx, orgUnitID)
		if err != nil {
			return Scorecard{}, err
		}
	}

	if c, ok := ropaCompletenessComponent(activities); ok {
		out.Components = append(out.Components, c)
	}

	if s.Risk != nil {
		ou, err := s.Risk.OrgUnitComplianceScore(ctx, orgUnitID)
		if err != nil {
			return Scorecard{}, err
		}
		detail := "no activities to score"
		if len(ou.Activities) > 0 {
			detail = "average legal-gap score across activities (RRA-13)"
		}
		out.Components = append(out.Components, ScorecardComponent{Name: "legal_readiness", Score: ou.AverageScore, Detail: detail})
	}

	if s.Dpia != nil && len(activities) > 0 {
		c, err := s.dpiaReadinessComponent(ctx, activities)
		if err != nil {
			return Scorecard{}, err
		}
		out.Components = append(out.Components, c)
	}

	if s.Gov != nil {
		total, completed, err := s.Gov.OrgUnitTrainingCompletion(ctx, orgUnitID)
		if err != nil {
			return Scorecard{}, err
		}
		score := 100.0
		detail := "no training assigned to this unit"
		if total > 0 {
			score = float64(completed) / float64(total) * 100
			detail = "passed attempts for assignments targeted directly at this unit"
		}
		out.Components = append(out.Components, ScorecardComponent{Name: "training_completion", Score: score, Detail: detail})
	}

	if len(out.Components) > 0 {
		var sum float64
		for _, c := range out.Components {
			sum += c.Score
		}
		out.Overall = sum / float64(len(out.Components))
	}
	return out, nil
}

// listAllActivities drains every page of this org unit's own activities.
func (s *Service) listAllActivities(ctx context.Context, orgUnitID uuid.UUID) ([]ropaservice.Activity, error) {
	var out []ropaservice.Activity
	f := ropaservice.ActivityFilter{OrgUnitID: &orgUnitID}
	for {
		page, cursor, err := s.Ropa.ListActivities(ctx, f)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if cursor == nil {
			return out, nil
		}
		f.After = cursor
	}
}

func ropaCompletenessComponent(activities []ropaservice.Activity) (ScorecardComponent, bool) {
	if len(activities) == 0 {
		return ScorecardComponent{}, false
	}
	var sum float64
	for _, a := range activities {
		sum += float64(a.Completeness)
	}
	return ScorecardComponent{
		Name: "ropa_completeness", Score: sum / float64(len(activities)),
		Detail: "average ROPA-03 completeness across this unit's own activities",
	}, true
}

// dpiaReadinessComponent is the fraction of activities that are DPIA-compliant: either their latest
// screening round reads "not_required," or it has reached "approved" — anything else (never screened,
// still in progress, rejected, needs_review) counts against the unit.
func (s *Service) dpiaReadinessComponent(ctx context.Context, activities []ropaservice.Activity) (ScorecardComponent, error) {
	var compliant int
	for _, a := range activities {
		rounds, _, err := s.Dpia.ListAssessments(ctx, dpiaservice.AssessmentFilter{ActivityID: &a.ID, Limit: 1})
		if err != nil {
			return ScorecardComponent{}, err
		}
		if len(rounds) == 0 {
			continue
		}
		latest := rounds[0]
		if latest.ScreeningResult == "not_required" || latest.Status == "approved" {
			compliant++
		}
	}
	return ScorecardComponent{
		Name: "dpia_readiness", Score: float64(compliant) / float64(len(activities)) * 100,
		Detail: "activities whose latest DPIA round is not_required or approved",
	}, nil
}
