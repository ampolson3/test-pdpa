package riskhttp

import (
	"context"

	riskservice "pdpa-platform/internal/risk/service"
)

// RiskListAcceptances is RRA-08's own read path: every acceptance ever recorded for a risk, newest first.
func (h *Strict) RiskListAcceptances(ctx context.Context, req RiskListAcceptancesRequestObject) (RiskListAcceptancesResponseObject, error) {
	list, err := h.svc.ListAcceptances(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := RiskListAcceptances200JSONResponse{Data: make([]RiskAcceptance, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toAcceptanceWire(a))
	}
	return resp, nil
}

// RiskRecordAcceptance is RRA-08's own acceptance criterion: a DPO/EXEC acceptance of the risk's own
// residual, with a reason and a review date — flips the risk to status=accepted and schedules the one
// reminder at expires_at.
func (h *Strict) RiskRecordAcceptance(ctx context.Context, req RiskRecordAcceptanceRequestObject) (RiskRecordAcceptanceResponseObject, error) {
	a, err := h.svc.RecordAcceptance(ctx, req.Id, req.Body.Reason, req.Body.ExpiresAt.Time)
	if err != nil {
		return nil, problem(err)
	}
	return RiskRecordAcceptance201JSONResponse(toAcceptanceWire(a)), nil
}

func toAcceptanceWire(a riskservice.Acceptance) RiskAcceptance {
	w := RiskAcceptance{Id: a.ID, RiskId: a.RiskID, Reason: a.Reason, RowVersion: int(a.RowVersion)}
	w.ExpiresAt.Time = a.ExpiresAt
	if a.ApprovedBy != nil {
		w.ApprovedBy = a.ApprovedBy
	}
	if !a.ApprovedAt.IsZero() {
		t := a.ApprovedAt.UTC()
		w.ApprovedAt = &t
	}
	if !a.CreatedAt.IsZero() {
		w.CreatedAt = a.CreatedAt.UTC()
	}
	return w
}
