// RRA-13 (compliance score per activity and org unit): a plain function of RRA-04's own live gap
// findings — 100 minus a per-severity point deduction for each still-open finding, so the score always
// explains exactly which gap(s) it was deducted for (the acceptance criterion's own "คะแนนอธิบายได้ว่าหัก
// จากช่องว่างใด"). Read-only: this never re-runs the rule engine itself (that stays AnalyzeActivity's own
// job, already on the activity page) — it reads whatever risk.gap_findings currently holds, the same
// "list is for scanning, not for triggering writes" convention this module already uses elsewhere.
package service

import (
	"context"

	"github.com/google/uuid"
)

// severityDeduction is a plain tunable (CLAUDE.md's "implement the listed default as config," not a
// docs/decisions.md question): how many points a still-open gap of each severity costs.
var severityDeduction = map[string]float64{"low": 5, "medium": 10, "high": 20}

// ScoreDeduction is one still-open gap finding counted against the score — the acceptance criterion's own
// explanation, not a summary number alone.
type ScoreDeduction struct {
	RuleCode string
	RuleName string
	Severity string
	Points   float64
}

// ComplianceScore is one activity's own compliance score (0-100) with every deduction named.
type ComplianceScore struct {
	ActivityID uuid.UUID
	Score      float64
	Deductions []ScoreDeduction
}

// ActivityComplianceScore computes one activity's score from its current risk.gap_findings — every rule
// that no longer applies has already been auto-resolved by the last AnalyzeActivity run (RRA-04), so an
// "open" finding here is always a real, still-applicable gap.
func (s *Service) ActivityComplianceScore(ctx context.Context, activityID uuid.UUID) (ComplianceScore, error) {
	findings, err := s.ListGapFindingsForActivity(ctx, activityID)
	if err != nil {
		return ComplianceScore{}, err
	}
	rules, err := s.ListGapRules(ctx)
	if err != nil {
		return ComplianceScore{}, err
	}
	byCode := make(map[string]GapRule, len(rules))
	for _, r := range rules {
		byCode[r.Code] = r
	}
	score := 100.0
	var deductions []ScoreDeduction
	for _, f := range findings {
		if f.Status != "open" {
			continue
		}
		rule, ok := byCode[f.RuleCode]
		if !ok {
			continue
		}
		pts := severityDeduction[rule.Severity]
		score -= pts
		deductions = append(deductions, ScoreDeduction{RuleCode: rule.Code, RuleName: rule.Name, Severity: rule.Severity, Points: pts})
	}
	if score < 0 {
		score = 0
	}
	return ComplianceScore{ActivityID: activityID, Score: score, Deductions: deductions}, nil
}

// OrgUnitComplianceScore is the rollup the module description calls for: every activity under one org
// unit, averaged, each with its own explained score.
type OrgUnitComplianceScore struct {
	OrgUnitID    uuid.UUID
	AverageScore float64
	Activities   []ComplianceScore
}

func (s *Service) OrgUnitComplianceScore(ctx context.Context, orgUnitID uuid.UUID) (OrgUnitComplianceScore, error) {
	ids, err := s.Ropa.ListActivityIDsByOrgUnit(ctx, orgUnitID)
	if err != nil {
		return OrgUnitComplianceScore{}, err
	}
	out := OrgUnitComplianceScore{OrgUnitID: orgUnitID, Activities: make([]ComplianceScore, 0, len(ids))}
	var total float64
	for _, id := range ids {
		cs, err := s.ActivityComplianceScore(ctx, id)
		if err != nil {
			return OrgUnitComplianceScore{}, err
		}
		out.Activities = append(out.Activities, cs)
		total += cs.Score
	}
	if len(out.Activities) > 0 {
		out.AverageScore = total / float64(len(out.Activities))
	}
	return out, nil
}

// ComplianceScoreReport is the module description's own "ตารางคะแนนรายหน่วยงาน": every org unit in the
// tenant, each with its own averaged, explained score — for comparing and tracking progress across
// departments.
func (s *Service) ComplianceScoreReport(ctx context.Context) ([]OrgUnitComplianceScore, error) {
	unitIDs, err := s.Ropa.ListOrgUnitIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]OrgUnitComplianceScore, 0, len(unitIDs))
	for _, id := range unitIDs {
		ou, err := s.OrgUnitComplianceScore(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, ou)
	}
	return out, nil
}
