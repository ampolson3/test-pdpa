package dpiahttp

import (
	"context"

	dpiaservice "pdpa-platform/internal/dpia/service"
	"pdpa-platform/internal/platform/forms"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DpiaGetScreeningRules(ctx context.Context, req DpiaGetScreeningRulesRequestObject) (DpiaGetScreeningRulesResponseObject, error) {
	r, err := h.svc.Rules(ctx)
	if err != nil {
		return nil, err
	}
	return DpiaGetScreeningRules200JSONResponse(toRuleWire(r)), nil
}

func (h *Strict) DpiaSaveScreeningRules(ctx context.Context, req DpiaSaveScreeningRulesRequestObject) (DpiaSaveScreeningRulesResponseObject, error) {
	var minScore *float64
	if req.Body.MinScore != nil {
		v := float64(*req.Body.MinScore)
		minScore = &v
	}
	r, err := h.svc.SaveRules(ctx, req.Body.MinFactors, minScore)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaSaveScreeningRules200JSONResponse(toRuleWire(r)), nil
}

func (h *Strict) DpiaScreenActivity(ctx context.Context, req DpiaScreenActivityRequestObject) (DpiaScreenActivityResponseObject, error) {
	a, err := h.svc.Screen(ctx, req.Id, forms.Answers(req.Body.Answers))
	if err != nil {
		return nil, problem(err)
	}
	return DpiaScreenActivity201JSONResponse(toAssessmentWire(a)), nil
}

func (h *Strict) DpiaListAssessments(ctx context.Context, req DpiaListAssessmentsRequestObject) (DpiaListAssessmentsResponseObject, error) {
	f := dpiaservice.AssessmentFilter{}
	if req.Params.ActivityId != nil {
		f.ActivityID = req.Params.ActivityId
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		c, err := decodeAssessmentCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &c
	}
	list, next, err := h.svc.ListAssessments(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := DpiaListAssessments200JSONResponse{Data: make([]DpiaAssessment, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toAssessmentWire(a))
	}
	if next != nil {
		c := encodeAssessmentCursor(*next)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) DpiaGetAssessment(ctx context.Context, req DpiaGetAssessmentRequestObject) (DpiaGetAssessmentResponseObject, error) {
	a, err := h.svc.GetAssessment(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaGetAssessment200JSONResponse(toAssessmentWire(a)), nil
}

func toRuleWire(r dpiaservice.ScreeningRule) DpiaScreeningRule {
	w := DpiaScreeningRule{MinFactors: r.MinFactors}
	if r.MinScore != nil {
		v := float32(*r.MinScore)
		w.MinScore = &v
	}
	return w
}

func toAssessmentWire(a dpiaservice.Assessment) DpiaAssessment {
	w := DpiaAssessment{Id: a.ID, ActivityId: a.ActivityID, RoundNo: a.RoundNo, PreviousId: a.PreviousID,
		Title: ptr(a.Title), Status: DpiaAssessmentStatus(a.Status), ScreeningResult: DpiaAssessmentScreeningResult(a.ScreeningResult),
		ScreeningReason: a.ScreeningReason, Score: float32(a.Score), RowVersion: int(a.RowVersion), CreatedAt: a.CreatedAt.UTC()}
	for _, f := range a.Factors {
		w.Factors = append(w.Factors, struct {
			Answer   interface{} `json:"answer"`
			Points   float32     `json:"points"`
			Question string      `json:"question"`
		}{Answer: f.Answer, Points: float32(f.Points), Question: f.Question})
	}
	return w
}
