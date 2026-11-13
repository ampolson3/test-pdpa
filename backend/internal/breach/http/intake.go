package breachhttp

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	breach "pdpa-platform/internal/breach/service"
	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) BreachListReportForms(ctx context.Context, req BreachListReportFormsRequestObject) (BreachListReportFormsResponseObject, error) {
	status := ""
	if req.Params.Status != nil {
		status = string(*req.Params.Status)
	}
	var cursorAt *time.Time
	var cursorID *uuid.UUID
	if req.Params.Cursor != nil {
		at, id, ok := strings.Cut(*req.Params.Cursor, "|")
		t, err1 := time.Parse(time.RFC3339Nano, at)
		u, err2 := uuid.Parse(id)
		if !ok || err1 != nil || err2 != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		cursorAt, cursorID = &t, &u
	}
	limit := int32(20)
	if req.Params.Limit != nil {
		limit = int32(*req.Params.Limit)
	}
	forms, err := h.svc.ListReportForms(ctx, status, cursorAt, cursorID, limit)
	if err != nil {
		return nil, ToProblem(err)
	}
	resp := BreachListReportForms200JSONResponse{Data: make([]BreachReportForm, 0, len(forms))}
	for _, f := range forms {
		resp.Data = append(resp.Data, toReportFormWire(f))
	}
	if len(forms) > 0 {
		last := forms[len(forms)-1]
		c := last.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID.String()
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) BreachCreateReportForm(ctx context.Context, req BreachCreateReportFormRequestObject) (BreachCreateReportFormResponseObject, error) {
	b := *req.Body
	f, err := h.svc.CreateReportForm(ctx, breach.CreateReportFormInput{LegalEntityID: b.LegalEntityId, Name: b.Name})
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachCreateReportForm201JSONResponse{Body: toReportFormWire(f), Headers: BreachCreateReportForm201ResponseHeaders{ETag: etag(f.RowVersion)}}, nil
}

func (h *Strict) BreachGetReportForm(ctx context.Context, req BreachGetReportFormRequestObject) (BreachGetReportFormResponseObject, error) {
	f, err := h.svc.GetReportForm(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachGetReportForm200JSONResponse{Body: toReportFormWire(f), Headers: BreachGetReportForm200ResponseHeaders{ETag: etag(f.RowVersion)}}, nil
}

func (h *Strict) BreachPublishReportForm(ctx context.Context, req BreachPublishReportFormRequestObject) (BreachPublishReportFormResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	f, err := h.svc.PublishReportForm(ctx, req.Id, v)
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachPublishReportForm200JSONResponse{Body: toReportFormWire(f), Headers: BreachPublishReportForm200ResponseHeaders{ETag: etag(f.RowVersion)}}, nil
}

func (h *Strict) BreachRetireReportForm(ctx context.Context, req BreachRetireReportFormRequestObject) (BreachRetireReportFormResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	f, err := h.svc.RetireReportForm(ctx, req.Id, v)
	if err != nil {
		return nil, ToProblem(err)
	}
	return BreachRetireReportForm200JSONResponse{Body: toReportFormWire(f), Headers: BreachRetireReportForm200ResponseHeaders{ETag: etag(f.RowVersion)}}, nil
}

func toReportFormWire(f breach.ReportForm) BreachReportForm {
	return BreachReportForm{Id: f.ID, LegalEntityId: f.LegalEntityID, Name: f.Name, Status: BreachReportFormStatus(f.Status),
		PublicKey: f.PublicKey, RowVersion: int(f.RowVersion), UpdatedAt: f.UpdatedAt.UTC()}
}
