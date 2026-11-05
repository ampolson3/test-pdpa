package riskhttp

import (
	"context"

	riskservice "pdpa-platform/internal/risk/service"

	"pdpa-platform/internal/platform/forms"
)

// RiskListActivityChecklists is RRA-05's own read path: every checklist run against an activity,
// newest first.
func (h *Strict) RiskListActivityChecklists(ctx context.Context, req RiskListActivityChecklistsRequestObject) (RiskListActivityChecklistsResponseObject, error) {
	list, err := h.svc.ListActivityChecklists(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := RiskListActivityChecklists200JSONResponse{Data: make([]RiskActivityChecklist, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toChecklistWire(c))
	}
	return resp, nil
}

// RiskAnswerActivityChecklist is RRA-05's own acceptance criterion: a completed run of a published
// checklist form, its answers landing against the activity itself.
func (h *Strict) RiskAnswerActivityChecklist(ctx context.Context, req RiskAnswerActivityChecklistRequestObject) (RiskAnswerActivityChecklistResponseObject, error) {
	c, err := h.svc.AnswerChecklist(ctx, req.Id, req.Body.FormId, forms.Answers(req.Body.Answers))
	if err != nil {
		return nil, problem(err)
	}
	return RiskAnswerActivityChecklist201JSONResponse(toChecklistWire(c)), nil
}

// RiskGetActivityChecklist is one checklist run.
func (h *Strict) RiskGetActivityChecklist(ctx context.Context, req RiskGetActivityChecklistRequestObject) (RiskGetActivityChecklistResponseObject, error) {
	c, err := h.svc.GetActivityChecklist(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return RiskGetActivityChecklist200JSONResponse(toChecklistWire(c)), nil
}

func toChecklistWire(c riskservice.ActivityChecklist) RiskActivityChecklist {
	w := RiskActivityChecklist{Id: c.ID, ActivityId: c.ActivityID, FormSubmissionId: c.FormSubmissionID,
		Score: float32(c.Score), Result: c.Result, CompletedBy: c.CompletedBy, CompletedAt: c.CompletedAt.UTC(),
		RowVersion: int(c.RowVersion)}
	for _, f := range c.Factors {
		item := struct {
			Answer       interface{}          `json:"answer,omitempty"`
			AnswerLabels *[]map[string]string `json:"answer_labels,omitempty"`
			Label        map[string]string    `json:"label"`
			Points       float32              `json:"points"`
			Question     string               `json:"question"`
		}{Answer: f.Answer, Label: f.Label, Points: float32(f.Points), Question: f.Question}
		if len(f.AnswerLabels) > 0 {
			labels := make([]map[string]string, 0, len(f.AnswerLabels))
			for _, l := range f.AnswerLabels {
				labels = append(labels, l)
			}
			item.AnswerLabels = &labels
		}
		w.Factors = append(w.Factors, item)
	}
	return w
}
