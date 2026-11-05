package noticehttp

import (
	"context"
	"strconv"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	noticeservice "pdpa-platform/internal/notice/service"
	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) NoticeListIndirectCollections(ctx context.Context, req NoticeListIndirectCollectionsRequestObject) (NoticeListIndirectCollectionsResponseObject, error) {
	f := noticeservice.IndirectCollectionFilter{}
	if req.Params.Status != nil {
		s := string(*req.Params.Status)
		f.Status = &s
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		at, id, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &noticeservice.IndirectCollectionCursor{DueAt: at, ID: id}
	}
	list, next, err := h.svc.ListCollections(ctx, f)
	if err != nil {
		return nil, problem(err)
	}
	resp := NoticeListIndirectCollections200JSONResponse{Data: make([]IndirectCollection, 0, len(list))}
	for _, ic := range list {
		resp.Data = append(resp.Data, toIndirectCollectionWire(ic))
	}
	if next != nil {
		c := encodeCursor(next.DueAt, next.ID)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) NoticeRegisterIndirectCollection(ctx context.Context, req NoticeRegisterIndirectCollectionRequestObject) (NoticeRegisterIndirectCollectionResponseObject, error) {
	b := *req.Body
	in := noticeservice.IndirectCollection{SourcePartyID: b.SourcePartyId, ActivityID: b.ActivityId, ObtainedAt: b.ObtainedAt.Time}
	if b.SubjectCount != nil {
		in.SubjectCount = b.SubjectCount
	}
	ic, err := h.svc.RegisterCollection(ctx, in)
	if err != nil {
		return nil, problem(err)
	}
	return NoticeRegisterIndirectCollection201JSONResponse{Body: toIndirectCollectionWire(ic),
		Headers: NoticeRegisterIndirectCollection201ResponseHeaders{ETag: etag(ic.RowVersion)}}, nil
}

func (h *Strict) NoticeGetIndirectCollection(ctx context.Context, req NoticeGetIndirectCollectionRequestObject) (NoticeGetIndirectCollectionResponseObject, error) {
	ic, err := h.svc.GetCollection(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return NoticeGetIndirectCollection200JSONResponse{Body: toIndirectCollectionWire(ic),
		Headers: NoticeGetIndirectCollection200ResponseHeaders{ETag: etag(ic.RowVersion)}}, nil
}

func (h *Strict) NoticeRecordIndirectNotice(ctx context.Context, req NoticeRecordIndirectNoticeRequestObject) (NoticeRecordIndirectNoticeResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := *req.Body
	ic, err := h.svc.RecordNotice(ctx, req.Id, v, string(b.Method), b.EvidenceFileId)
	if err != nil {
		return nil, problem(err)
	}
	return NoticeRecordIndirectNotice200JSONResponse{Body: toIndirectCollectionWire(ic),
		Headers: NoticeRecordIndirectNotice200ResponseHeaders{ETag: etag(ic.RowVersion)}}, nil
}

func toIndirectCollectionWire(ic noticeservice.IndirectCollection) IndirectCollection {
	w := IndirectCollection{Id: ic.ID, SourcePartyId: ic.SourcePartyID, ActivityId: ic.ActivityID,
		ObtainedAt: openapi_types.Date{Time: ic.ObtainedAt}, NotifyDueAt: openapi_types.Date{Time: ic.NotifyDueAt},
		EvidenceFileId: ic.EvidenceFileID, Status: IndirectCollectionStatus(ic.Status), RowVersion: int(ic.RowVersion),
		SubjectCount: ic.SubjectCount}
	if ic.Method != "" {
		m := IndirectCollectionMethod(ic.Method)
		w.Method = &m
	}
	if ic.NotifiedAt != nil {
		t := ic.NotifiedAt.UTC()
		w.NotifiedAt = &t
	}
	return w
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}
