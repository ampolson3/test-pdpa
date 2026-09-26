// Package noticehttp holds the notice module's admin endpoints (PNG-01, the wizard-based generator). Types in
// notice.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package noticehttp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/httpx"
	noticeservice "pdpa-platform/internal/notice/service"
)

type Strict struct {
	svc *noticeservice.Service
}

func NewStrict(svc *noticeservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) NoticeListNotices(ctx context.Context, req NoticeListNoticesRequestObject) (NoticeListNoticesResponseObject, error) {
	f := noticeservice.NoticeFilter{}
	if req.Params.NoticeType != nil {
		f.NoticeType = string(*req.Params.NoticeType)
	}
	if req.Params.Status != nil {
		f.Status = string(*req.Params.Status)
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		at, id, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &noticeservice.NoticeCursor{UpdatedAt: at, ID: id}
	}
	list, next, err := h.svc.ListNotices(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := NoticeListNotices200JSONResponse{Data: make([]Notice, 0, len(list))}
	for _, n := range list {
		resp.Data = append(resp.Data, toNoticeWire(n))
	}
	if next != nil {
		c := encodeCursor(next.UpdatedAt, next.ID)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) NoticeCreateNotice(ctx context.Context, req NoticeCreateNoticeRequestObject) (NoticeCreateNoticeResponseObject, error) {
	b := *req.Body
	in := noticeservice.WizardInput{LegalEntityID: b.LegalEntityId, SubjectTypeID: b.SubjectTypeId, NoticeType: string(b.NoticeType), Title: b.Title, Slug: b.Slug}
	if b.ActivityIds != nil {
		in.ActivityIDs = *b.ActivityIds
	}
	n, err := h.svc.CreateWizard(ctx, in)
	if err != nil {
		return nil, problem(err)
	}
	return NoticeCreateNotice201JSONResponse{Body: toNoticeWire(n), Headers: NoticeCreateNotice201ResponseHeaders{ETag: etag(n.RowVersion)}}, nil
}

func (h *Strict) NoticeGetNotice(ctx context.Context, req NoticeGetNoticeRequestObject) (NoticeGetNoticeResponseObject, error) {
	n, err := h.svc.GetNotice(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return NoticeGetNotice200JSONResponse{Body: toNoticeWire(n), Headers: NoticeGetNotice200ResponseHeaders{ETag: etag(n.RowVersion)}}, nil
}

func toNoticeWire(n noticeservice.Notice) Notice {
	w := Notice{Id: n.ID, LegalEntityId: n.LegalEntityID, SubjectTypeId: n.SubjectTypeID, NoticeType: NoticeType(n.NoticeType),
		Title: n.Title, Slug: n.Slug, DocumentId: n.DocumentID, Status: NoticeStatus(n.Status), OwnerUserId: n.OwnerUserID,
		ReviewCycleMonths: n.ReviewCycleMonths, RowVersion: int(n.RowVersion), UpdatedAt: n.UpdatedAt.UTC()}
	if len(n.ActivityIDs) > 0 {
		ids := append([]uuid.UUID(nil), n.ActivityIDs...)
		w.ActivityIds = &ids
	}
	return w
}

func etag(v int32) *string {
	s := fmt.Sprintf("%q", strconv.Itoa(int(v)))
	return &s
}

func encodeCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(s string) (time.Time, uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, uuid.UUID{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return time.Time{}, uuid.UUID{}, errors.New("cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.UUID{}, err
	}
	u, err := uuid.Parse(id)
	return t, u, err
}

func problem(err error) error {
	switch {
	case errors.Is(err, noticeservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, noticeservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, noticeservice.ErrInvalid):
		return httpx.UnprocessableEntity("notice.invalid_input", err.Error())
	}
	return err
}
