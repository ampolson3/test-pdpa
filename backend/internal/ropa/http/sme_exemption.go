package ropahttp

import (
	"context"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/forms"
	ropaservice "pdpa-platform/internal/ropa/service"
)

func smeExemptionCheckWire(c ropaservice.SmeExemptionCheck) RopaSmeExemptionCheck {
	return RopaSmeExemptionCheck{
		Id: c.ID, LegalEntityId: c.LegalEntityID, FormSubmissionId: c.FormSubmissionID,
		Result: RopaSmeExemptionResult(c.Result), Basis: c.Basis, AssessedAt: c.AssessedAt.UTC(),
		RowVersion: int(c.RowVersion), CreatedAt: c.CreatedAt.UTC(),
	}
}

func (h *Strict) RopaListSmeExemptionChecks(ctx context.Context, req RopaListSmeExemptionChecksRequestObject) (RopaListSmeExemptionChecksResponseObject, error) {
	list, err := h.svc.ListSmeExemptionChecks(ctx, req.Params.LegalEntityId)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListSmeExemptionChecks200JSONResponse{Data: make([]RopaSmeExemptionCheck, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, smeExemptionCheckWire(c))
	}
	return resp, nil
}

func (h *Strict) RopaAssessSmeExemption(ctx context.Context, req RopaAssessSmeExemptionRequestObject) (RopaAssessSmeExemptionResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("legal_entity_id")
	}
	answers := forms.Answers{}
	for k, v := range req.Body.Answers {
		answers[k] = string(v)
	}
	c, err := h.svc.AssessSmeExemption(ctx, req.Body.LegalEntityId, req.Body.IsProcessor, answers)
	if err != nil {
		return nil, problem(err)
	}
	return RopaAssessSmeExemption201JSONResponse(smeExemptionCheckWire(c)), nil
}
