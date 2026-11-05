package dpohttp

import (
	"context"

	dposervice "pdpa-platform/internal/dpo/service"
	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DpoListIndependenceDeclarations(ctx context.Context, req DpoListIndependenceDeclarationsRequestObject) (DpoListIndependenceDeclarationsResponseObject, error) {
	list, err := h.svc.ListIndependenceDeclarations(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpoListIndependenceDeclarations200JSONResponse{Data: make([]DpoIndependenceDeclaration, 0, len(list))}
	for _, d := range list {
		resp.Data = append(resp.Data, toIndependenceDeclarationWire(d))
	}
	return resp, nil
}

func (h *Strict) DpoRecordIndependenceDeclaration(ctx context.Context, req DpoRecordIndependenceDeclarationRequestObject) (DpoRecordIndependenceDeclarationResponseObject, error) {
	d, err := h.svc.RecordDeclaration(ctx, req.Id, req.Body.Year, str(req.Body.OtherDuties), boolVal(req.Body.HasConflict))
	if err != nil {
		return nil, problem(err)
	}
	return DpoRecordIndependenceDeclaration201JSONResponse{
		Body:    toIndependenceDeclarationWire(d),
		Headers: DpoRecordIndependenceDeclaration201ResponseHeaders{ETag: etag(d.RowVersion)},
	}, nil
}

func (h *Strict) DpoSignIndependenceDeclaration(ctx context.Context, req DpoSignIndependenceDeclarationRequestObject) (DpoSignIndependenceDeclarationResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	var fileID *Uuid
	if req.Body != nil {
		fileID = req.Body.FileId
	}
	d, err := h.svc.SignDeclaration(ctx, req.Id, v, fileID)
	if err != nil {
		return nil, problem(err)
	}
	return DpoSignIndependenceDeclaration200JSONResponse{
		Body:    toIndependenceDeclarationWire(d),
		Headers: DpoSignIndependenceDeclaration200ResponseHeaders{ETag: etag(d.RowVersion)},
	}, nil
}

func toIndependenceDeclarationWire(d dposervice.IndependenceDeclaration) DpoIndependenceDeclaration {
	w := DpoIndependenceDeclaration{
		Id: d.ID, AppointmentId: d.AppointmentID, Year: d.Year, OtherDuties: ptr(d.OtherDuties),
		HasConflict: d.HasConflict, FileId: d.FileID, RowVersion: int(d.RowVersion),
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
	if d.SignedAt != nil {
		t := d.SignedAt.UTC()
		w.SignedAt = &t
	}
	return w
}

func boolVal(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
