package breachhttp

import (
	"context"

	"github.com/google/uuid"

	breach "pdpa-platform/internal/breach/service"
	"pdpa-platform/internal/pkg/httpx"
)

func correctiveActionWire(a breach.CorrectiveAction) BreachCorrectiveAction {
	out := BreachCorrectiveAction{Id: a.ID, Description: a.Description, Status: BreachCorrectiveActionStatus(a.Status)}
	if a.AssigneeUserID != nil {
		out.AssigneeUserId = a.AssigneeUserID
	}
	if a.DueAt != nil {
		t := *a.DueAt
		out.DueAt = &t
	}
	if a.CompletedAt != nil {
		t := *a.CompletedAt
		out.CompletedAt = &t
	}
	return out
}

func correctiveActionFromWire(in BreachCorrectiveActionInput) breach.CorrectiveAction {
	out := breach.CorrectiveAction{Description: in.Description, AssigneeUserID: in.AssigneeUserId, DueAt: in.DueAt, Status: "open"}
	if in.Id != nil {
		out.ID = *in.Id
	}
	if in.Status != nil {
		out.Status = string(*in.Status)
	}
	return out
}

func rootCauseWire(r breach.RootCause) BreachRootCause {
	actions := make([]BreachCorrectiveAction, 0, len(r.CorrectiveActions))
	for _, a := range r.CorrectiveActions {
		actions = append(actions, correctiveActionWire(a))
	}
	out := BreachRootCause{
		Id: r.ID, IncidentId: r.IncidentID, CauseCategory: r.CauseCategory, Description: r.Description,
		CorrectiveActions: actions, AllActionsDone: r.AllActionsDone(), RowVersion: int(r.RowVersion),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.RiskID != nil {
		out.RiskId = r.RiskID
	}
	return out
}

func actionsFromWire(in []BreachCorrectiveActionInput) []breach.CorrectiveAction {
	out := make([]breach.CorrectiveAction, 0, len(in))
	for _, a := range in {
		out = append(out, correctiveActionFromWire(a))
	}
	return out
}

func (h *Strict) BreachListRootCauses(ctx context.Context, req BreachListRootCausesRequestObject) (BreachListRootCausesResponseObject, error) {
	list, err := h.svc.ListRootCauses(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachListRootCauses200JSONResponse{Data: make([]BreachRootCause, 0, len(list))}
	for _, rc := range list {
		out.Data = append(out.Data, rootCauseWire(rc))
	}
	return out, nil
}

func (h *Strict) BreachAddRootCause(ctx context.Context, req BreachAddRootCauseRequestObject) (BreachAddRootCauseResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("cause_category")
	}
	b := *req.Body
	var riskID *uuid.UUID
	if b.RiskId != nil {
		riskID = b.RiskId
	}
	rc, err := h.svc.AddRootCause(ctx, req.Id, string(b.CauseCategory), b.Description, actionsFromWire(derefActions(b.CorrectiveActions)), riskID)
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachAddRootCause201JSONResponse{Body: rootCauseWire(rc), Headers: BreachAddRootCause201ResponseHeaders{ETag: etag(rc.RowVersion)}}, nil
}

func (h *Strict) BreachUpdateRootCause(ctx context.Context, req BreachUpdateRootCauseRequestObject) (BreachUpdateRootCauseResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("cause_category")
	}
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := *req.Body
	var riskID *uuid.UUID
	if b.RiskId != nil {
		riskID = b.RiskId
	}
	rc, err := h.svc.UpdateRootCause(ctx, req.Id, v, string(b.CauseCategory), b.Description, actionsFromWire(derefActions(b.CorrectiveActions)), riskID)
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachUpdateRootCause200JSONResponse{Body: rootCauseWire(rc), Headers: BreachUpdateRootCause200ResponseHeaders{ETag: etag(rc.RowVersion)}}, nil
}

func (h *Strict) BreachDeleteRootCause(ctx context.Context, req BreachDeleteRootCauseRequestObject) (BreachDeleteRootCauseResponseObject, error) {
	if err := h.svc.DeleteRootCause(ctx, req.Params.IncidentId, req.Id); err != nil {
		return nil, ToProblem(err)
	}
	return BreachDeleteRootCause204Response{}, nil
}

func (h *Strict) BreachCompleteCorrectiveAction(ctx context.Context, req BreachCompleteCorrectiveActionRequestObject) (BreachCompleteCorrectiveActionResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	rc, err := h.svc.CompleteCorrectiveAction(ctx, req.Id, req.ActionId, v)
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachCompleteCorrectiveAction200JSONResponse{Body: rootCauseWire(rc), Headers: BreachCompleteCorrectiveAction200ResponseHeaders{ETag: etag(rc.RowVersion)}}, nil
}

func derefActions(in *[]BreachCorrectiveActionInput) []BreachCorrectiveActionInput {
	if in == nil {
		return nil
	}
	return *in
}
