package ropahttp

import (
	"context"
)

// RopaListActivityRejections is ROPA-10's read side: the DSAR rejections a subscriber (internal/wiring.Events)
// has logged against this activity from the `dsar.rejected` event — no write endpoint, since this module never
// creates these rows itself (rule 9).
func (h *Strict) RopaListActivityRejections(ctx context.Context, req RopaListActivityRejectionsRequestObject) (RopaListActivityRejectionsResponseObject, error) {
	list, err := h.svc.ListActivityRejections(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListActivityRejections200JSONResponse{Data: make([]ActivityRejection, 0, len(list))}
	for _, r := range list {
		resp.Data = append(resp.Data, ActivityRejection{Id: r.ID, ActivityId: r.ActivityID, DsarRequestId: r.DsarRequestID, ReasonCode: r.ReasonCode, RejectedAt: r.RejectedAt})
	}
	return resp, nil
}
