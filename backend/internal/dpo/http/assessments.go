package dpohttp

import (
	"context"

	dposervice "pdpa-platform/internal/dpo/service"
	"pdpa-platform/internal/platform/forms"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DpoListAssessments(ctx context.Context, req DpoListAssessmentsRequestObject) (DpoListAssessmentsResponseObject, error) {
	f := dposervice.AssessmentFilter{}
	if req.Params.LegalEntityId != nil {
		f.LegalEntityID = req.Params.LegalEntityId
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
	resp := DpoListAssessments200JSONResponse{Data: make([]DpoSecurityAssessment, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toAssessmentWire(a))
	}
	if next != nil {
		c := encodeAssessmentCursor(*next)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) DpoRecordAssessment(ctx context.Context, req DpoRecordAssessmentRequestObject) (DpoRecordAssessmentResponseObject, error) {
	a, err := h.svc.Assess(ctx, req.Body.LegalEntityId, req.Body.FormId, forms.Answers(req.Body.Answers))
	if err != nil {
		return nil, problem(err)
	}
	return DpoRecordAssessment201JSONResponse(toAssessmentWire(a)), nil
}

func (h *Strict) DpoGetAssessment(ctx context.Context, req DpoGetAssessmentRequestObject) (DpoGetAssessmentResponseObject, error) {
	a, err := h.svc.GetAssessment(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DpoGetAssessment200JSONResponse(toAssessmentWire(a)), nil
}

func toAssessmentWire(a dposervice.Assessment) DpoSecurityAssessment {
	w := DpoSecurityAssessment{Id: a.ID, LegalEntityId: a.LegalEntityID, FormSubmissionId: a.FormSubmissionID,
		Score: float32(a.Score), Result: a.Result, AssessedBy: a.AssessedBy, AssessedAt: a.AssessedAt.UTC()}
	for _, f := range a.Factors {
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
	if a.Tasks != nil {
		tasks := make([]DpoRemediationTask, 0, len(a.Tasks))
		for _, t := range a.Tasks {
			tasks = append(tasks, DpoRemediationTask{Id: t.ID, TaskNo: t.TaskNo, Title: t.Title, Description: ptr(t.Description),
				Status: DpoRemediationTaskStatus(t.Status), Priority: DpoRemediationTaskPriority(t.Priority), CreatedAt: t.CreatedAt.UTC()})
		}
		w.Tasks = &tasks
	}
	return w
}

func encodeAssessmentCursor(c dposervice.AssessmentCursor) string { return encodeCursor(c.AssessedAt, c.ID) }

func decodeAssessmentCursor(s string) (dposervice.AssessmentCursor, error) {
	t, u, err := decodeCursor(s)
	return dposervice.AssessmentCursor{AssessedAt: t, ID: u}, err
}
