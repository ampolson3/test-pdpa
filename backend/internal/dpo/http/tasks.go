package dpohttp

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	dposervice "pdpa-platform/internal/dpo/service"
	"pdpa-platform/internal/pkg/httpx"
)

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
		AssigneeUserId: t.AssigneeUserID, RowVersion: int(t.RowVersion), CreatedAt: t.CreatedAt.UTC()}
	if t.DueAt != nil {
		w.DueAt = &openapi_types.Date{Time: *t.DueAt}
	}
	if t.CompletedAt != nil {
		c := t.CompletedAt.UTC()
		w.CompletedAt = &c
	}
	return w
}
