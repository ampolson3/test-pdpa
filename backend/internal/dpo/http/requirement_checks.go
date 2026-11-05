package dpohttp

import (
	"context"

	dposervice "pdpa-platform/internal/dpo/service"
	"pdpa-platform/internal/platform/forms"
)

func (h *Strict) DpoListRequirementChecks(ctx context.Context, req DpoListRequirementChecksRequestObject) (DpoListRequirementChecksResponseObject, error) {
	list, err := h.svc.ListRequirementChecks(ctx, req.Params.LegalEntityId)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpoListRequirementChecks200JSONResponse{Data: make([]DpoRequirementCheck, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toRequirementCheckWire(c))
	}
	return resp, nil
}

func (h *Strict) DpoAssessRequirementCheck(ctx context.Context, req DpoAssessRequirementCheckRequestObject) (DpoAssessRequirementCheckResponseObject, error) {
	answers := forms.Answers(req.Body.Answers)
	c, err := h.svc.AssessDpoRequirement(ctx, req.Body.LegalEntityId, answers)
	if err != nil {
		return nil, problem(err)
	}
	return DpoAssessRequirementCheck201JSONResponse(toRequirementCheckWire(c)), nil
}

func toRequirementCheckWire(c dposervice.RequirementCheck) DpoRequirementCheck {
	return DpoRequirementCheck{
		Id: c.ID, LegalEntityId: c.LegalEntityID, AssessmentId: c.AssessmentID,
		Result: DpoRequirementCheckResult(c.Result), Basis: c.Basis, AssessedAt: c.AssessedAt.UTC(),
		RowVersion: int(c.RowVersion), CreatedAt: c.CreatedAt.UTC(),
	}
}
