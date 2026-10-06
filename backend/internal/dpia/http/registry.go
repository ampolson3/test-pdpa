package dpiahttp

import (
	"context"

	dpiaservice "pdpa-platform/internal/dpia/service"
)

func (h *Strict) DpiaGetRegistry(ctx context.Context, req DpiaGetRegistryRequestObject) (DpiaGetRegistryResponseObject, error) {
	f := dpiaservice.RegistryFilter{LegalEntityID: req.Params.LegalEntityId, OrgUnitID: req.Params.OrgUnitId}
	if req.Params.Status != nil {
		f.Status = string(*req.Params.Status)
	}
	list, err := h.svc.Registry(ctx, f)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpiaGetRegistry200JSONResponse{Data: make([]DpiaRegistryEntry, 0, len(list))}
	for _, e := range list {
		resp.Data = append(resp.Data, toRegistryEntryWire(e))
	}
	return resp, nil
}

func toRegistryEntryWire(e dpiaservice.RegistryEntry) DpiaRegistryEntry {
	return DpiaRegistryEntry{
		AssessmentId: e.AssessmentID, ActivityId: e.ActivityID, ActivityCode: e.ActivityCode, ActivityName: e.ActivityName,
		LegalEntityId: e.LegalEntityID, LegalEntityName: e.LegalEntityName, OrgUnitId: e.OrgUnitID, OrgUnitName: e.OrgUnitName,
		RoundNo: e.RoundNo, Status: DpiaRegistryEntryStatus(e.Status), ScreeningResult: DpiaRegistryEntryScreeningResult(e.ScreeningResult),
		Score: float32(e.Score), CreatedAt: e.CreatedAt.UTC(),
	}
}
