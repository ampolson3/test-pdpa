// DPIA-13: linking an incident to the RoPA processing activity it affected.
package breachhttp

import (
	"context"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) BreachListIncidentActivities(ctx context.Context, req BreachListIncidentActivitiesRequestObject) (BreachListIncidentActivitiesResponseObject, error) {
	list, err := h.svc.ListIncidentActivities(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachListIncidentActivities200JSONResponse{Data: make([]BreachIncidentActivity, 0, len(list))}
	for _, a := range list {
		out.Data = append(out.Data, BreachIncidentActivity{Id: a.ID, IncidentId: a.IncidentID, ActivityId: a.ActivityID})
	}
	return out, nil
}

func (h *Strict) BreachLinkIncidentActivity(ctx context.Context, req BreachLinkIncidentActivityRequestObject) (BreachLinkIncidentActivityResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("activity_id")
	}
	a, err := h.svc.LinkActivity(ctx, req.Id, req.Body.ActivityId)
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachLinkIncidentActivity201JSONResponse{Id: a.ID, IncidentId: a.IncidentID, ActivityId: a.ActivityID}, nil
}
