package dpohttp

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	dposervice "pdpa-platform/internal/dpo/service"
	"pdpa-platform/internal/pkg/httpx"
)

// DpoListTasks is DPO-06's own kanban/list view: every dpo.tasks job, whatever module opened it, filterable
// by status/source_type/priority/assignee.
func (h *Strict) DpoListTasks(ctx context.Context, req DpoListTasksRequestObject) (DpoListTasksResponseObject, error) {
	f := dposervice.TaskFilter{}
	if req.Params.Status != nil {
		f.Status = string(*req.Params.Status)
	}
	if req.Params.SourceType != nil {
		f.SourceType = string(*req.Params.SourceType)
	}
	if req.Params.Priority != nil {
		f.Priority = string(*req.Params.Priority)
	}
	if req.Params.AssigneeUserId != nil {
		f.AssigneeUserID = req.Params.AssigneeUserId
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		at, id, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &dposervice.TaskCursor{CreatedAt: at, ID: id}
	}
	list, next, err := h.svc.ListTasks(ctx, f)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpoListTasks200JSONResponse{Data: make([]DpoRemediationTask, 0, len(list))}
	for _, t := range list {
		resp.Data = append(resp.Data, toTaskWire(t))
	}
	if next != nil {
		c := encodeCursor(next.CreatedAt, next.ID)
		resp.NextCursor = &c
	}
	return resp, nil
}

// DpoAssignTask is DPO-06's own real gap: set or change who is assigned, who reviews, the due date and the
// priority on an already-created task.
func (h *Strict) DpoAssignTask(ctx context.Context, req DpoAssignTaskRequestObject) (DpoAssignTaskResponseObject, error) {
	version, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := dposervice.AssignTaskInput{}
	if req.Body != nil {
		in.AssigneeUserID = req.Body.AssigneeUserId
		in.ReviewerUserID = req.Body.ReviewerUserId
		in.OrgUnitID = req.Body.OrgUnitId
		if req.Body.DueAt != nil {
			in.DueAt = &req.Body.DueAt.Time
		}
		if req.Body.Priority != nil {
			in.Priority = string(*req.Body.Priority)
		}
	}
	t, err := h.svc.AssignTask(ctx, req.Id, in, version)
	if err != nil {
		return nil, problem(err)
	}
	return DpoAssignTask200JSONResponse{Body: toTaskWire(t), Headers: DpoAssignTask200ResponseHeaders{ETag: etag(t.RowVersion)}}, nil
}

// DpoGetTask is RRA-07's own tracking view: one dpo.tasks job by id, whatever module opened it.
func (h *Strict) DpoGetTask(ctx context.Context, req DpoGetTaskRequestObject) (DpoGetTaskResponseObject, error) {
	t, err := h.svc.GetTask(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	w := toTaskWire(t)
	return DpoGetTask200JSONResponse(w), nil
}

// DpoUpdateTaskStatus is RRA-07's own "ติดตามจนปิด": move the task forward, closing a "ropa_gap" one
// re-checks that finding's rule (dposervice.UpdateTaskStatus's own job, not this handler's).
func (h *Strict) DpoUpdateTaskStatus(ctx context.Context, req DpoUpdateTaskStatusRequestObject) (DpoUpdateTaskStatusResponseObject, error) {
	version, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	t, err := h.svc.UpdateTaskStatus(ctx, req.Id, string(req.Body.Status), version)
	if err != nil {
		return nil, problem(err)
	}
	return DpoUpdateTaskStatus200JSONResponse{Body: toTaskWire(t), Headers: DpoUpdateTaskStatus200ResponseHeaders{ETag: etag(t.RowVersion)}}, nil
}

func toTaskWire(t dposervice.Task) DpoRemediationTask {
	w := DpoRemediationTask{Id: t.ID, TaskNo: t.TaskNo, Title: t.Title, Description: ptr(t.Description),
		SourceType: (*DpoRemediationTaskSourceType)(ptr(t.SourceType)), SourceId: t.SourceID,
		Status: DpoRemediationTaskStatus(t.Status), Priority: DpoRemediationTaskPriority(t.Priority),
		AssigneeUserId: t.AssigneeUserID, ReviewerUserId: t.ReviewerUserID, OrgUnitId: t.OrgUnitID,
		RowVersion: int(t.RowVersion), CreatedAt: t.CreatedAt.UTC()}
	if t.DueAt != nil {
		w.DueAt = &openapi_types.Date{Time: *t.DueAt}
	}
	if t.CompletedAt != nil {
		c := t.CompletedAt.UTC()
		w.CompletedAt = &c
	}
	return w
}
