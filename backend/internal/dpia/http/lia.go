package dpiahttp

import (
	"context"

	dpiaservice "pdpa-platform/internal/dpia/service"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/forms"
)

// DpiaListLiaForActivity is DPIA-16's own read path: every legitimate-interest assessment round against an
// activity, newest first.
func (h *Strict) DpiaListLiaForActivity(ctx context.Context, req DpiaListLiaForActivityRequestObject) (DpiaListLiaForActivityResponseObject, error) {
	list, err := h.svc.ListLIAForActivity(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpiaListLiaForActivity200JSONResponse{Data: make([]DpiaLiaAssessment, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toLiaWire(a))
	}
	return resp, nil
}

// DpiaAssessLia answers the published LIA test against an activity, opening a new round pending DPO
// approval.
func (h *Strict) DpiaAssessLia(ctx context.Context, req DpiaAssessLiaRequestObject) (DpiaAssessLiaResponseObject, error) {
	a, err := h.svc.AssessLIA(ctx, req.Id, forms.Answers(req.Body.Answers))
	if err != nil {
		return nil, problem(err)
	}
	return DpiaAssessLia201JSONResponse(toLiaWire(a)), nil
}

// DpiaDecideLia is the DPO's own approve/reject decision on an answered LIA round.
func (h *Strict) DpiaDecideLia(ctx context.Context, req DpiaDecideLiaRequestObject) (DpiaDecideLiaResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.PreconditionRequired("If-Match")
	}
	var reason string
	if req.Body.Reason != nil {
		reason = *req.Body.Reason
	}
	a, err := h.svc.DecideLIA(ctx, req.Id, rv, string(req.Body.To), reason)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaDecideLia200JSONResponse(toLiaWire(a)), nil
}

func toLiaWire(a dpiaservice.Assessment) DpiaLiaAssessment {
	w := DpiaLiaAssessment{Id: a.ID, ActivityId: a.ActivityID, RoundNo: a.RoundNo, PreviousId: a.PreviousID,
		Title: ptr(a.Title), Status: DpiaLiaAssessmentStatus(a.Status), Score: float32(a.Score),
		RowVersion: int(a.RowVersion), CreatedAt: a.CreatedAt.UTC()}
	for _, f := range a.Factors {
		w.Factors = append(w.Factors, struct {
			Answer   interface{} `json:"answer"`
			Points   float32     `json:"points"`
			Question string      `json:"question"`
		}{Answer: f.Answer, Points: float32(f.Points), Question: f.Question})
	}
	return w
}
