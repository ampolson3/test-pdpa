package dsarhttp

import (
	"context"

	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DsarListSubtasks(ctx context.Context, req DsarListSubtasksRequestObject) (DsarListSubtasksResponseObject, error) {
	list, err := h.svc.ListSubtasks(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DsarListSubtasks200JSONResponse{Data: make([]DsarSubtask, 0, len(list))}
	for _, st := range list {
		resp.Data = append(resp.Data, toSubtaskWire(st))
	}
	return resp, nil
}

func (h *Strict) DsarCreateSubtask(ctx context.Context, req DsarCreateSubtaskRequestObject) (DsarCreateSubtaskResponseObject, error) {
	b := *req.Body
	in := dsarservice.CreateSubtaskInput{Action: string(b.Action), AssigneeUserID: b.AssigneeUserId, AssigneeGroupID: b.AssigneeGroupId}
	if b.DueAt != nil {
		in.DueAt = b.DueAt
	}
	st, err := h.svc.CreateSubtask(ctx, req.Id, in)
	if err != nil {
		return nil, problem(err)
	}
	return DsarCreateSubtask201JSONResponse{Body: toSubtaskWire(st), Headers: DsarCreateSubtask201ResponseHeaders{ETag: etag(st.RowVersion)}}, nil
}

func (h *Strict) DsarUpdateSubtaskStatus(ctx context.Context, req DsarUpdateSubtaskStatusRequestObject) (DsarUpdateSubtaskStatusResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := *req.Body
	st, err := h.svc.UpdateSubtaskStatus(ctx, req.Id, req.SubtaskId, v, string(b.Status), b.EvidenceFileId)
	if err != nil {
		return nil, problem(err)
	}
	return DsarUpdateSubtaskStatus200JSONResponse{Body: toSubtaskWire(st), Headers: DsarUpdateSubtaskStatus200ResponseHeaders{ETag: etag(st.RowVersion)}}, nil
}

func (h *Strict) DsarDeleteSubtask(ctx context.Context, req DsarDeleteSubtaskRequestObject) (DsarDeleteSubtaskResponseObject, error) {
	if err := h.svc.DeleteSubtask(ctx, req.Id, req.SubtaskId); err != nil {
		return nil, problem(err)
	}
	return DsarDeleteSubtask204Response{}, nil
}

func toSubtaskWire(st dsarservice.Subtask) DsarSubtask {
	w := DsarSubtask{Id: st.ID, RequestId: st.RequestID, AssetId: st.AssetID, Action: DsarSubtaskAction(st.Action), Status: DsarSubtaskStatus(st.Status),
		AssigneeUserId: st.AssigneeUserID, AssigneeGroupId: st.AssigneeGroupID, EvidenceFileId: st.EvidenceFileID,
		RowVersion: int(st.RowVersion), CreatedAt: st.CreatedAt.UTC(), UpdatedAt: st.UpdatedAt.UTC()}
	if st.DueAt != nil {
		t := st.DueAt.UTC()
		w.DueAt = &t
	}
	if st.CompletedAt != nil {
		t := st.CompletedAt.UTC()
		w.CompletedAt = &t
	}
	return w
}
