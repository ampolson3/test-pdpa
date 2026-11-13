package dpiahttp

import (
	"context"

	dpiaservice "pdpa-platform/internal/dpia/service"
)

// DpiaGetAssessmentDiff is DPIA-14: which screening answers changed since the round this one supersedes.
func (h *Strict) DpiaGetAssessmentDiff(ctx context.Context, req DpiaGetAssessmentDiffRequestObject) (DpiaGetAssessmentDiffResponseObject, error) {
	d, err := h.svc.CompareToPrevious(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaGetAssessmentDiff200JSONResponse(toDiffWire(d)), nil
}

func toDiffWire(d dpiaservice.AssessmentDiff) DpiaAssessmentDiff {
	w := DpiaAssessmentDiff{AssessmentId: d.AssessmentID, PreviousId: d.PreviousID}
	for _, c := range d.Changes {
		w.Changes = append(w.Changes, struct {
			After    interface{} `json:"after,omitempty"`
			Before   interface{} `json:"before,omitempty"`
			Question string      `json:"question"`
		}{Question: c.Question, Before: c.Before, After: c.After})
	}
	return w
}
