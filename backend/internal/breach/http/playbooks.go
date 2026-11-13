package breachhttp

import (
	"context"

	"github.com/google/uuid"

	breach "pdpa-platform/internal/breach/service"
	"pdpa-platform/internal/pkg/httpx"
)

func playbookStepsFromWire(in []BreachPlaybookStep) []breach.PlaybookStep {
	out := make([]breach.PlaybookStep, 0, len(in))
	for _, s := range in {
		out = append(out, breach.PlaybookStep{Code: s.Code, Title: s.Title})
	}
	return out
}

func playbookWire(p breach.Playbook) BreachPlaybook {
	steps := make([]BreachPlaybookStep, 0, len(p.Steps))
	for _, s := range p.Steps {
		steps = append(steps, BreachPlaybookStep{Code: s.Code, Title: s.Title})
	}
	return BreachPlaybook{
		Id: p.ID, IncidentType: p.IncidentType, Name: p.Name, Steps: steps, VersionNo: int(p.VersionNo),
		RowVersion: int(p.RowVersion), CreatedAt: p.CreatedAt.UTC(), UpdatedAt: p.UpdatedAt.UTC(),
	}
}

func responseTaskWire(t breach.ResponseTask) BreachResponseTask {
	out := BreachResponseTask{
		Id: t.ID, IncidentId: t.IncidentID, Title: t.Title, Status: BreachResponseTaskStatus(t.Status),
		RowVersion: int(t.RowVersion), CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
	}
	if t.PlaybookID != nil {
		out.PlaybookId = t.PlaybookID
	}
	if t.StepCode != "" {
		c := t.StepCode
		out.StepCode = &c
	}
	if t.AssigneeUserID != nil {
		out.AssigneeUserId = t.AssigneeUserID
	}
	if t.AssigneeName != "" {
		n := t.AssigneeName
		out.AssigneeName = &n
	}
	if t.AssigneeGroupID != nil {
		out.AssigneeGroupId = t.AssigneeGroupID
	}
	if t.DueAt != nil {
		d := *t.DueAt
		out.DueAt = &d
	}
	if t.CompletedAt != nil {
		c := *t.CompletedAt
		out.CompletedAt = &c
	}
	return out
}

func (h *Strict) BreachListPlaybooks(ctx context.Context, req BreachListPlaybooksRequestObject) (BreachListPlaybooksResponseObject, error) {
	list, err := h.svc.ListPlaybooks(ctx, req.Params.IncidentType)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachListPlaybooks200JSONResponse{Data: make([]BreachPlaybook, 0, len(list))}
	for _, p := range list {
		out.Data = append(out.Data, playbookWire(p))
	}
	return out, nil
}

func (h *Strict) BreachCreatePlaybook(ctx context.Context, req BreachCreatePlaybookRequestObject) (BreachCreatePlaybookResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("incident_type")
	}
	p, err := h.svc.CreatePlaybook(ctx, req.Body.IncidentType, req.Body.Name, playbookStepsFromWire(req.Body.Steps))
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachCreatePlaybook201JSONResponse{Body: playbookWire(p), Headers: BreachCreatePlaybook201ResponseHeaders{ETag: etag(p.RowVersion)}}, nil
}

func (h *Strict) BreachGetPlaybook(ctx context.Context, req BreachGetPlaybookRequestObject) (BreachGetPlaybookResponseObject, error) {
	p, err := h.svc.GetPlaybook(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachGetPlaybook200JSONResponse{Body: playbookWire(p), Headers: BreachGetPlaybook200ResponseHeaders{ETag: etag(p.RowVersion)}}, nil
}

func (h *Strict) BreachUpdatePlaybook(ctx context.Context, req BreachUpdatePlaybookRequestObject) (BreachUpdatePlaybookResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("name")
	}
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	p, err := h.svc.UpdatePlaybook(ctx, req.Id, v, req.Body.Name, playbookStepsFromWire(req.Body.Steps))
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachUpdatePlaybook200JSONResponse{Body: playbookWire(p), Headers: BreachUpdatePlaybook200ResponseHeaders{ETag: etag(p.RowVersion)}}, nil
}

func (h *Strict) BreachListResponseTasks(ctx context.Context, req BreachListResponseTasksRequestObject) (BreachListResponseTasksResponseObject, error) {
	list, err := h.svc.ListResponseTasks(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachListResponseTasks200JSONResponse{Data: make([]BreachResponseTask, 0, len(list))}
	for _, t := range list {
		out.Data = append(out.Data, responseTaskWire(t))
	}
	return out, nil
}

func (h *Strict) BreachApplyPlaybook(ctx context.Context, req BreachApplyPlaybookRequestObject) (BreachApplyPlaybookResponseObject, error) {
	var playbookID *uuid.UUID
	if req.Body != nil {
		playbookID = req.Body.PlaybookId
	}
	list, err := h.svc.ApplyPlaybook(ctx, req.Id, playbookID)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachApplyPlaybook200JSONResponse{Data: make([]BreachResponseTask, 0, len(list))}
	for _, t := range list {
		out.Data = append(out.Data, responseTaskWire(t))
	}
	return out, nil
}

func (h *Strict) BreachSetResponseTaskStatus(ctx context.Context, req BreachSetResponseTaskStatusRequestObject) (BreachSetResponseTaskStatusResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("status")
	}
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	t, err := h.svc.SetTaskStatus(ctx, req.Id, v, string(req.Body.Status))
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachSetResponseTaskStatus200JSONResponse{Body: responseTaskWire(t), Headers: BreachSetResponseTaskStatus200ResponseHeaders{ETag: etag(t.RowVersion)}}, nil
}

func (h *Strict) BreachAssignResponseTask(ctx context.Context, req BreachAssignResponseTaskRequestObject) (BreachAssignResponseTaskResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	var userID, groupID *uuid.UUID
	if req.Body != nil {
		userID = req.Body.AssigneeUserId
		groupID = req.Body.AssigneeGroupId
	}
	t, err := h.svc.AssignTask(ctx, req.Id, v, userID, groupID)
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachAssignResponseTask200JSONResponse{Body: responseTaskWire(t), Headers: BreachAssignResponseTask200ResponseHeaders{ETag: etag(t.RowVersion)}}, nil
}
