package noticehttp

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	noticeservice "pdpa-platform/internal/notice/service"
	"pdpa-platform/internal/pkg/httpx"
)

func toAcknowledgementWire(a noticeservice.Acknowledgement) NoticeAcknowledgement {
	w := NoticeAcknowledgement{Id: a.ID, NoticeVersionId: a.NoticeVersionID, Channel: NoticeAckChannel(a.Channel), OccurredAt: a.OccurredAt.Time.UTC()}
	if a.UserID != nil {
		w.UserId = a.UserID
		name := a.UserName
		w.UserName = &name
	}
	if a.IP != nil {
		s := a.IP.String()
		w.Ip = &s
	}
	return w
}

// NoticeListAcknowledgements is PNG-09's own acceptance criterion read-side.
func (h *Strict) NoticeListAcknowledgements(ctx context.Context, req NoticeListAcknowledgementsRequestObject) (NoticeListAcknowledgementsResponseObject, error) {
	var before *pgtype.Timestamptz
	if req.Params.Before != nil {
		before = &pgtype.Timestamptz{Time: *req.Params.Before, Valid: true}
	}
	var limit int32
	if req.Params.Limit != nil {
		limit = int32(*req.Params.Limit)
	}
	list, total, err := h.svc.ListAcknowledgements(ctx, req.Id, int32(req.VersionNo), before, limit)
	if err != nil {
		return nil, problem(err)
	}
	resp := NoticeListAcknowledgements200JSONResponse{Data: make([]NoticeAcknowledgement, 0, len(list)), Total: int(total)}
	for _, a := range list {
		resp.Data = append(resp.Data, toAcknowledgementWire(a))
	}
	return resp, nil
}

// NoticeRecordAcknowledgement is the staff/API-side write — a named user acknowledged this version.
func (h *Strict) NoticeRecordAcknowledgement(ctx context.Context, req NoticeRecordAcknowledgementRequestObject) (NoticeRecordAcknowledgementResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("channel, user_id")
	}
	a, err := h.svc.RecordAcknowledgement(ctx, req.Id, int32(req.VersionNo), req.Body.UserId, string(req.Body.Channel))
	if err != nil {
		return nil, problem(err)
	}
	return NoticeRecordAcknowledgement201JSONResponse(toAcknowledgementWire(a)), nil
}
