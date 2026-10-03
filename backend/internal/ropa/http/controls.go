package ropahttp

import (
	"context"

	riskservice "pdpa-platform/internal/risk/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

func (h *Strict) RopaListSecurityControls(ctx context.Context, req RopaListSecurityControlsRequestObject) (RopaListSecurityControlsResponseObject, error) {
	list, err := h.svc.ListControls(ctx)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListSecurityControls200JSONResponse{Data: make([]SecurityControl, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toSecurityControlWire(c))
	}
	return resp, nil
}

func (h *Strict) RopaListActivityControls(ctx context.Context, req RopaListActivityControlsRequestObject) (RopaListActivityControlsResponseObject, error) {
	list, err := h.svc.ListActivityControls(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListActivityControls200JSONResponse{Data: make([]ActivityControl, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toActivityControlWire(c))
	}
	return resp, nil
}

func (h *Strict) RopaAddActivityControl(ctx context.Context, req RopaAddActivityControlRequestObject) (RopaAddActivityControlResponseObject, error) {
	b := *req.Body
	c := ropaservice.ActivityControl{ActivityID: req.Id, ControlID: b.ControlId}
	if b.Description != nil {
		c.Description = *b.Description
	}
	out, err := h.svc.AddActivityControl(ctx, c)
	if err != nil {
		return nil, problem(err)
	}
	return RopaAddActivityControl201JSONResponse(toActivityControlWire(out)), nil
}

func (h *Strict) RopaDeleteActivityControl(ctx context.Context, req RopaDeleteActivityControlRequestObject) (RopaDeleteActivityControlResponseObject, error) {
	if err := h.svc.DeleteActivityControl(ctx, req.Id, req.ControlId); err != nil {
		return nil, problem(err)
	}
	return RopaDeleteActivityControl204Response{}, nil
}

func toSecurityControlWire(c riskservice.Control) SecurityControl {
	return SecurityControl{Id: c.ID, Code: c.Code, Name: c.Name, Category: SecurityControlCategory(c.Category),
		Description: ptr(c.Description), FrameworkRefs: c.FrameworkRefs}
}

func toActivityControlWire(c ropaservice.ActivityControl) ActivityControl {
	return ActivityControl{ActivityId: c.ActivityID, ControlId: c.ControlID, Description: ptr(c.Description)}
}
