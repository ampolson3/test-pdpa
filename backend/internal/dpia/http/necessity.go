package dpiahttp

import (
	"context"

	dpiaservice "pdpa-platform/internal/dpia/service"
	"pdpa-platform/internal/platform/forms"
)

// DpiaAssessNecessity is DPIA-05: answer (or re-answer) the necessity/proportionality checklist for a DPIA
// assessment.
func (h *Strict) DpiaAssessNecessity(ctx context.Context, req DpiaAssessNecessityRequestObject) (DpiaAssessNecessityResponseObject, error) {
	n, err := h.svc.AssessNecessity(ctx, req.Id, forms.Answers(req.Body.Answers))
	if err != nil {
		return nil, problem(err)
	}
	return DpiaAssessNecessity200JSONResponse(toNecessityWire(n)), nil
}

// DpiaGetNecessity reads back the assessment's necessity checklist.
func (h *Strict) DpiaGetNecessity(ctx context.Context, req DpiaGetNecessityRequestObject) (DpiaGetNecessityResponseObject, error) {
	n, err := h.svc.GetNecessity(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaGetNecessity200JSONResponse(toNecessityWire(n)), nil
}

func toNecessityWire(n dpiaservice.NecessityAssessment) DpiaNecessity {
	w := DpiaNecessity{
		AssessmentId: n.AssessmentID,
		Result:       DpiaNecessityResult(n.Result),
		Missing:      n.Missing,
		SubmittedAt:  n.SubmittedAt.UTC(),
	}
	for _, a := range n.Answers {
		w.Answers = append(w.Answers, struct {
			Answer   interface{} `json:"answer"`
			Question string      `json:"question"`
		}{Question: a.Question, Answer: a.Answer})
	}
	return w
}
