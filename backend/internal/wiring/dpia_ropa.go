package wiring

import (
	"context"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	"pdpa-platform/internal/platform/forms"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// RopaDpia adapts the real dpia service to ropa/service's own local Dpia interface (DPX-06) — a direct
// import the other way is impossible, since internal/dpia/service already imports internal/ropa/service
// for its own Ropa interface's return types (rule 9: no cycle). The same pattern RiskRopa already uses
// for risk/service.
type RopaDpia struct {
	Dpia *dpiaservice.Service
}

func (a RopaDpia) ListTemplates(ctx context.Context, assessmentType string) ([]ropaservice.DpiaTemplate, error) {
	ts, err := a.Dpia.ListTemplates(ctx, assessmentType)
	if err != nil {
		return nil, err
	}
	out := make([]ropaservice.DpiaTemplate, len(ts))
	for i, t := range ts {
		out[i] = ropaservice.DpiaTemplate{ID: t.ID, Code: t.Code, Status: t.Status, Name: t.Name}
	}
	return out, nil
}

func (a RopaDpia) RecordSubjectAssessment(ctx context.Context, templateID uuid.UUID, subjectType string, subjectID uuid.UUID, title string, answers forms.Answers) (ropaservice.TiaResult, error) {
	res, err := a.Dpia.RecordSubjectAssessment(ctx, templateID, subjectType, subjectID, title, answers)
	if err != nil {
		return ropaservice.TiaResult{}, err
	}
	return ropaservice.TiaResult{ID: res.ID, Score: res.Score}, nil
}

// HasApprovedLIA forwards DPIA-16's own completeness check.
func (a RopaDpia) HasApprovedLIA(ctx context.Context, activityID uuid.UUID) (bool, error) {
	return a.Dpia.HasApprovedLIA(ctx, activityID)
}
