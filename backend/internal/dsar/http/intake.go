package dsarhttp

import (
	"context"

	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DsarListIntakeForms(ctx context.Context, req DsarListIntakeFormsRequestObject) (DsarListIntakeFormsResponseObject, error) {
	status := ""
	if req.Params.Status != nil {
		status = string(*req.Params.Status)
	}
	var cursor *dsarservice.RequestCursor
	if req.Params.Cursor != nil {
		at, id, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		cursor = &dsarservice.RequestCursor{ReceivedAt: at, ID: id}
	}
	limit := int32(20)
	if req.Params.Limit != nil {
		limit = int32(*req.Params.Limit)
	}
	forms, err := h.svc.ListIntakeForms(ctx, status, cursor, limit)
	if err != nil {
		return nil, problem(err)
	}
	resp := DsarListIntakeForms200JSONResponse{Data: make([]DsarIntakeForm, 0, len(forms))}
	for _, f := range forms {
		resp.Data = append(resp.Data, toIntakeFormWire(f))
	}
	if len(forms) > 0 {
		last := forms[len(forms)-1]
		c := encodeCursor(last.UpdatedAt, last.ID)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) DsarCreateIntakeForm(ctx context.Context, req DsarCreateIntakeFormRequestObject) (DsarCreateIntakeFormResponseObject, error) {
	b := *req.Body
	f, err := h.svc.CreateIntakeForm(ctx, dsarservice.CreateIntakeFormInput{LegalEntityID: b.LegalEntityId, Name: b.Name})
	if err != nil {
		return nil, problem(err)
	}
	return DsarCreateIntakeForm201JSONResponse{Body: toIntakeFormWire(f), Headers: DsarCreateIntakeForm201ResponseHeaders{ETag: etag(f.RowVersion)}}, nil
}

func (h *Strict) DsarGetIntakeForm(ctx context.Context, req DsarGetIntakeFormRequestObject) (DsarGetIntakeFormResponseObject, error) {
	f, err := h.svc.GetIntakeForm(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DsarGetIntakeForm200JSONResponse{Body: toIntakeFormWire(f), Headers: DsarGetIntakeForm200ResponseHeaders{ETag: etag(f.RowVersion)}}, nil
}

func (h *Strict) DsarPublishIntakeForm(ctx context.Context, req DsarPublishIntakeFormRequestObject) (DsarPublishIntakeFormResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	f, err := h.svc.PublishIntakeForm(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return DsarPublishIntakeForm200JSONResponse{Body: toIntakeFormWire(f), Headers: DsarPublishIntakeForm200ResponseHeaders{ETag: etag(f.RowVersion)}}, nil
}

func (h *Strict) DsarRetireIntakeForm(ctx context.Context, req DsarRetireIntakeFormRequestObject) (DsarRetireIntakeFormResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	f, err := h.svc.RetireIntakeForm(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return DsarRetireIntakeForm200JSONResponse{Body: toIntakeFormWire(f), Headers: DsarRetireIntakeForm200ResponseHeaders{ETag: etag(f.RowVersion)}}, nil
}

func toIntakeFormWire(f dsarservice.IntakeForm) DsarIntakeForm {
	return DsarIntakeForm{Id: f.ID, LegalEntityId: f.LegalEntityID, Name: f.Name, Status: DsarIntakeFormStatus(f.Status),
		PublicKey: f.PublicKey, RowVersion: int(f.RowVersion), UpdatedAt: f.UpdatedAt.UTC()}
}
