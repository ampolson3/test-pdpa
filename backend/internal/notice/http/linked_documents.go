package noticehttp

import (
	"context"

	noticeservice "pdpa-platform/internal/notice/service"
	"pdpa-platform/internal/pkg/httpx"
)

func toLinkedDocumentWire(ld noticeservice.LinkedDocument) NoticeLinkedDocument {
	return NoticeLinkedDocument{
		Id: ld.ID, NoticeId: ld.NoticeID, Label: ld.Label, Url: ld.URL, FileId: ld.FileID,
		DisplayMode: NoticeLinkedDocumentDisplayMode(ld.DisplayMode), RowVersion: int(ld.RowVersion), CreatedAt: ld.CreatedAt.UTC(),
	}
}

// NoticeListLinkedDocuments is PNG-13's own read side — every reference document linked from this notice.
func (h *Strict) NoticeListLinkedDocuments(ctx context.Context, req NoticeListLinkedDocumentsRequestObject) (NoticeListLinkedDocumentsResponseObject, error) {
	list, err := h.svc.ListLinkedDocuments(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := NoticeListLinkedDocuments200JSONResponse{Data: make([]NoticeLinkedDocument, 0, len(list))}
	for _, ld := range list {
		resp.Data = append(resp.Data, toLinkedDocumentWire(ld))
	}
	return resp, nil
}

// NoticeAddLinkedDocument links a reference document — a plain url or the caller's own clean upload — to a
// notice, with how it should be shown (PNG-13's own acceptance criterion: inline / popup / link).
func (h *Strict) NoticeAddLinkedDocument(ctx context.Context, req NoticeAddLinkedDocumentRequestObject) (NoticeAddLinkedDocumentResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.RequestInvalid("label, display_mode")
	}
	ld, err := h.svc.AddLinkedDocument(ctx, req.Id, noticeservice.LinkedDocumentInput{
		Label: req.Body.Label, URL: req.Body.Url, FileID: req.Body.FileId, DisplayMode: string(req.Body.DisplayMode),
	})
	if err != nil {
		return nil, problem(err)
	}
	return NoticeAddLinkedDocument201JSONResponse(toLinkedDocumentWire(ld)), nil
}

// NoticeRemoveLinkedDocument unlinks a reference document from a notice.
func (h *Strict) NoticeRemoveLinkedDocument(ctx context.Context, req NoticeRemoveLinkedDocumentRequestObject) (NoticeRemoveLinkedDocumentResponseObject, error) {
	if err := h.svc.RemoveLinkedDocument(ctx, req.Id, req.LinkedDocumentId); err != nil {
		return nil, problem(err)
	}
	return NoticeRemoveLinkedDocument204Response{}, nil
}
