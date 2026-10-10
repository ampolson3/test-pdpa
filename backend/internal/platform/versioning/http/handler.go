// Package versioninghttp holds the PLT-08 versioning & approval endpoints. Types in versioning.gen.go are
// generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package versioninghttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/versioning"
)

type Strict struct {
	svc *versioning.Service
}

func NewStrict(svc *versioning.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) PlatformListRecordVersions(ctx context.Context, req PlatformListRecordVersionsRequestObject) (PlatformListRecordVersionsResponseObject, error) {
	list, err := h.svc.List(ctx, req.EntityType, req.EntityId)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformListRecordVersions200JSONResponse{Data: make([]RecordVersion, 0, len(list))}
	for _, v := range list {
		w, err := versionWire(v)
		if err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) PlatformGetRecordVersion(ctx context.Context, req PlatformGetRecordVersionRequestObject) (PlatformGetRecordVersionResponseObject, error) {
	v, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	w, err := versionWire(v)
	if err != nil {
		return nil, err
	}
	return PlatformGetRecordVersion200JSONResponse{Body: w, Headers: PlatformGetRecordVersion200ResponseHeaders{ETag: etag(v.RowVersion)}}, nil
}

func (h *Strict) PlatformCompareRecordVersions(ctx context.Context, req PlatformCompareRecordVersionsRequestObject) (PlatformCompareRecordVersionsResponseObject, error) {
	changes, err := h.svc.Compare(ctx, req.Id, req.Params.With)
	if err != nil {
		return nil, problem(err)
	}
	var out struct {
		Changes []VersionChange `json:"changes"`
	}
	if err := convert(map[string]any{"changes": nonNil(changes)}, &out); err != nil {
		return nil, err
	}
	return PlatformCompareRecordVersions200JSONResponse{Changes: out.Changes}, nil
}

func (h *Strict) PlatformSubmitRecordVersion(ctx context.Context, req PlatformSubmitRecordVersionRequestObject) (PlatformSubmitRecordVersionResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	if _, err := h.svc.Submit(ctx, req.Id, ver); err != nil {
		return nil, problem(err)
	}
	w, v, err := h.reload(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return PlatformSubmitRecordVersion200JSONResponse{Body: w, Headers: PlatformSubmitRecordVersion200ResponseHeaders{ETag: etag(v)}}, nil
}

func (h *Strict) PlatformPublishRecordVersion(ctx context.Context, req PlatformPublishRecordVersionRequestObject) (PlatformPublishRecordVersionResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	if _, err := h.svc.Publish(ctx, req.Id, ver); err != nil {
		return nil, problem(err)
	}
	w, v, err := h.reload(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return PlatformPublishRecordVersion200JSONResponse{Body: w, Headers: PlatformPublishRecordVersion200ResponseHeaders{ETag: etag(v)}}, nil
}

func (h *Strict) PlatformDecideApproval(ctx context.Context, req PlatformDecideApprovalRequestObject) (PlatformDecideApprovalResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	reason := ""
	if req.Body.Reason != nil {
		reason = *req.Body.Reason
	}
	v, err := h.svc.Decide(ctx, req.Id, ver, string(req.Body.Decision), reason)
	if err != nil {
		return nil, problem(err)
	}
	w, rv, err := h.reload(ctx, v.ID)
	if errors.Is(err, versioning.ErrNotFound) { // an approver with a role but no read permission
		w, err = versionWire(v)
		rv = v.RowVersion
	}
	if err != nil {
		return nil, err
	}
	return PlatformDecideApproval200JSONResponse{Body: w, Headers: PlatformDecideApproval200ResponseHeaders{ETag: etag(rv)}}, nil
}

func (h *Strict) PlatformListMyApprovals(ctx context.Context, _ PlatformListMyApprovalsRequestObject) (PlatformListMyApprovalsResponseObject, error) {
	items, err := h.svc.Inbox(ctx)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformListMyApprovals200JSONResponse{Data: make([]ApprovalInboxItem, 0, len(items))}
	for _, it := range items {
		m := approvalMap(it.Approval)
		m["version_id"], m["entity_type"], m["entity_id"], m["version"] = it.VersionID, it.EntityType, it.EntityID, it.VersionNo
		if it.Title != "" {
			m["title"] = it.Title
		}
		if it.AuthorName != "" {
			m["author_name"] = it.AuthorName
		}
		var w ApprovalInboxItem
		if err := convert(m, &w); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) reload(ctx context.Context, id Uuid) (RecordVersion, int32, error) {
	v, err := h.svc.Get(ctx, id)
	if err != nil {
		return RecordVersion{}, 0, err
	}
	w, err := versionWire(v)
	return w, v.RowVersion, err
}

func versionWire(v versioning.Version) (RecordVersion, error) {
	approvals := make([]map[string]any, 0, len(v.Approvals))
	for _, a := range v.Approvals {
		approvals = append(approvals, approvalMap(a))
	}
	m := map[string]any{"id": v.ID, "entity_type": v.EntityType, "entity_id": v.EntityID, "version": v.No, "snapshot": json.RawMessage(v.Snapshot),
		"diff": nonNil(v.Diff), "status": v.Status, "created_at": v.CreatedAt.UTC(), "updated_at": v.UpdatedAt.UTC(), "row_version": v.RowVersion,
		"approvals": approvals}
	if v.AuthorID != nil {
		m["author_id"] = v.AuthorID
		if v.AuthorName != "" {
			m["author_name"] = v.AuthorName
		}
	}
	var w RecordVersion
	return w, convert(m, &w)
}

func approvalMap(a versioning.Approval) map[string]any {
	m := map[string]any{"id": a.ID, "step": a.Step, "role": a.Role, "requested_by": a.RequestedBy, "decision": a.Decision, "row_version": a.RowVersion}
	if a.Requester != "" {
		m["requester_name"] = a.Requester
	}
	if a.ApproverID != nil {
		m["approver_id"] = a.ApproverID
		if a.ApproverName != "" {
			m["approver_name"] = a.ApproverName
		}
	}
	if a.Reason != "" {
		m["reason"] = a.Reason
	}
	if a.DecidedAt != nil {
		m["decided_at"] = a.DecidedAt.UTC().Format(time.RFC3339Nano)
	}
	return m
}

func nonNil(c []versioning.Change) []versioning.Change {
	if c == nil {
		return []versioning.Change{}
	}
	return c
}

func convert(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func etag(v int32) *string {
	s := fmt.Sprintf("%q", strconv.Itoa(int(v)))
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func problem(err error) error {
	switch {
	case errors.Is(err, versioning.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, versioning.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, versioning.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, versioning.ErrInvalidTransition):
		return httpx.Problem{Status: http.StatusConflict, Code: "versioning.invalid_transition", Title: "Invalid state transition"}
	case errors.Is(err, versioning.ErrLocked):
		return httpx.Problem{Status: http.StatusConflict, Code: "versioning.locked", Title: "Version locked"}
	case errors.Is(err, versioning.ErrSelfApproval):
		return httpx.Problem{Status: http.StatusForbidden, Code: "versioning.self_approval", Title: "Maker-checker"}
	case errors.Is(err, versioning.ErrInvalidRequest):
		return httpx.UnprocessableEntity("versioning.invalid_request", err.Error())
	}
	return err
}
