// Package collabhttp holds the PLT-07 endpoints shared by every module's records: comments, attachments,
// activity and the @mention picker. Types in collab.gen.go are generated from api/openapi/openapi.yaml.
package collabhttp

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/collab"
)

type Strict struct {
	svc *collab.Service
}

func NewStrict(svc *collab.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) PlatformListComments(ctx context.Context, req PlatformListCommentsRequestObject) (PlatformListCommentsResponseObject, error) {
	list, err := h.svc.Comments(ctx, req.EntityType, req.EntityId)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformListComments200JSONResponse{Data: make([]Comment, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toComment(c))
	}
	return resp, nil
}

func (h *Strict) PlatformCreateComment(ctx context.Context, req PlatformCreateCommentRequestObject) (PlatformCreateCommentResponseObject, error) {
	c, err := h.svc.AddComment(ctx, req.EntityType, req.EntityId, req.Body.ParentId, req.Body.Body)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformCreateComment201JSONResponse(toComment(c)), nil
}

func (h *Strict) PlatformUpdateComment(ctx context.Context, req PlatformUpdateCommentRequestObject) (PlatformUpdateCommentResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	c, err := h.svc.EditComment(ctx, req.Id, v, req.Body.Body)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformUpdateComment200JSONResponse(toComment(c)), nil
}

func (h *Strict) PlatformDeleteComment(ctx context.Context, req PlatformDeleteCommentRequestObject) (PlatformDeleteCommentResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	if err := h.svc.DeleteComment(ctx, req.Id, v); err != nil {
		return nil, problem(err)
	}
	return PlatformDeleteComment204Response{}, nil
}

func (h *Strict) PlatformResolveComment(ctx context.Context, req PlatformResolveCommentRequestObject) (PlatformResolveCommentResponseObject, error) {
	c, err := h.svc.SetResolved(ctx, req.Id, req.Body.Resolved)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformResolveComment200JSONResponse(toComment(c)), nil
}

func (h *Strict) PlatformListAttachments(ctx context.Context, req PlatformListAttachmentsRequestObject) (PlatformListAttachmentsResponseObject, error) {
	list, err := h.svc.Attachments(ctx, req.EntityType, req.EntityId)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformListAttachments200JSONResponse{Data: make([]Attachment, 0, len(list))}
	for _, a := range list {
		out := Attachment{Id: a.ID, FileName: a.FileName, MimeType: a.MimeType, SizeBytes: a.SizeBytes,
			AvStatus: AttachmentAvStatus(a.AVStatus), CreatedAt: a.CreatedAt.UTC()}
		if a.UploaderName != "" {
			n := a.UploaderName
			out.UploaderName = &n
		}
		resp.Data = append(resp.Data, out)
	}
	return resp, nil
}

func (h *Strict) PlatformAttachFile(ctx context.Context, req PlatformAttachFileRequestObject) (PlatformAttachFileResponseObject, error) {
	if err := h.svc.Attach(ctx, req.EntityType, req.EntityId, req.Body.FileId); err != nil {
		return nil, problem(err)
	}
	return PlatformAttachFile204Response{}, nil
}

func (h *Strict) PlatformListActivity(ctx context.Context, req PlatformListActivityRequestObject) (PlatformListActivityResponseObject, error) {
	limit := 100
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	list, err := h.svc.Activity(ctx, req.EntityType, req.EntityId, limit)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformListActivity200JSONResponse{Data: make([]Activity, 0, len(list))}
	for _, a := range list {
		out := Activity{Id: a.ID, OccurredAt: a.OccurredAt.UTC(), ActorType: a.ActorType, Action: a.Action}
		if a.ActorName != "" {
			n := a.ActorName
			out.ActorName = &n
		}
		if a.Before != nil {
			b := a.Before
			out.Before = &b
		}
		if a.After != nil {
			v := a.After
			out.After = &v
		}
		resp.Data = append(resp.Data, out)
	}
	return resp, nil
}

func (h *Strict) PlatformSearchMentionableUsers(ctx context.Context, req PlatformSearchMentionableUsersRequestObject) (PlatformSearchMentionableUsersResponseObject, error) {
	users, err := h.svc.SearchMentionable(ctx, req.Params.Q)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformSearchMentionableUsers200JSONResponse{}
	for _, u := range users {
		resp.Data = append(resp.Data, struct {
			DisplayName string `json:"display_name"`
			Id          Uuid   `json:"id"`
		}{DisplayName: u.DisplayName, Id: u.ID})
	}
	if resp.Data == nil {
		resp.Data = []struct {
			DisplayName string `json:"display_name"`
			Id          Uuid   `json:"id"`
		}{}
	}
	return resp, nil
}

func toComment(c collab.Comment) Comment {
	mentions := c.Mentions
	if mentions == nil {
		mentions = []uuid.UUID{}
	}
	return Comment{
		Id: c.ID, ParentId: c.ParentID, AuthorId: c.AuthorID, AuthorName: c.AuthorName, Body: c.Body,
		Mentions: mentions, Resolved: c.Resolved, RowVersion: int(c.RowVersion),
		CreatedAt: c.CreatedAt.UTC(), UpdatedAt: c.UpdatedAt.UTC(),
	}
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func problem(err error) error {
	switch {
	case errors.Is(err, collab.ErrNotFound), errors.Is(err, collab.ErrUnknownType):
		return httpx.NotFound()
	case errors.Is(err, collab.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, collab.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, collab.ErrHasReplies):
		return httpx.Problem{Status: http.StatusConflict, Code: "collab.has_replies", Title: "Comment has replies"}
	case errors.Is(err, collab.ErrInvalid):
		return httpx.UnprocessableEntity("collab.invalid", err.Error())
	}
	return err
}
