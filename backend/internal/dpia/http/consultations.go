package dpiahttp

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	dpiaservice "pdpa-platform/internal/dpia/service"
)

func toConsultationWire(c dpiaservice.Consultation) DpiaConsultation {
	w := DpiaConsultation{
		Id: c.ID, AssessmentId: c.AssessmentID, Stakeholder: c.Stakeholder,
		StakeholderType: DpiaConsultationStakeholderType(c.StakeholderType),
		ConsultedAt:     openapi_types.Date{Time: c.ConsultedAt},
		Summary:         c.Summary, RowVersion: int(c.RowVersion), CreatedAt: c.CreatedAt.UTC(),
	}
	if c.Response != "" {
		w.Response = &c.Response
	}
	return w
}

func (h *Strict) DpiaListConsultations(ctx context.Context, req DpiaListConsultationsRequestObject) (DpiaListConsultationsResponseObject, error) {
	list, err := h.svc.ListConsultations(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpiaListConsultations200JSONResponse{Data: make([]DpiaConsultation, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toConsultationWire(c))
	}
	return resp, nil
}

func (h *Strict) DpiaAddConsultation(ctx context.Context, req DpiaAddConsultationRequestObject) (DpiaAddConsultationResponseObject, error) {
	var response string
	if req.Body.Response != nil {
		response = *req.Body.Response
	}
	c, err := h.svc.AddConsultation(ctx, req.Id, req.Body.Stakeholder, string(req.Body.StakeholderType), req.Body.ConsultedAt.Time, req.Body.Summary, response)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaAddConsultation201JSONResponse(toConsultationWire(c)), nil
}

func (h *Strict) DpiaRemoveConsultation(ctx context.Context, req DpiaRemoveConsultationRequestObject) (DpiaRemoveConsultationResponseObject, error) {
	if err := h.svc.RemoveConsultation(ctx, req.Id, req.ConsultationId); err != nil {
		return nil, problem(err)
	}
	return DpiaRemoveConsultation204Response{}, nil
}
