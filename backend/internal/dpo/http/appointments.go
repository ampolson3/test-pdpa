package dpohttp

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	dposervice "pdpa-platform/internal/dpo/service"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DpoListAppointments(ctx context.Context, req DpoListAppointmentsRequestObject) (DpoListAppointmentsResponseObject, error) {
	f := dposervice.AppointmentFilter{}
	if req.Params.LegalEntityId != nil {
		f.LegalEntityID = req.Params.LegalEntityId
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		c, err := decodeAppointmentCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &c
	}
	list, next, err := h.svc.ListAppointments(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := DpoListAppointments200JSONResponse{Data: make([]DpoAppointment, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toAppointmentWire(a))
	}
	if next != nil {
		c := encodeAppointmentCursor(*next)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) DpoCreateAppointment(ctx context.Context, req DpoCreateAppointmentRequestObject) (DpoCreateAppointmentResponseObject, error) {
	a, err := h.svc.SaveAppointment(ctx, toAppointmentInput(*req.Body), 0)
	if err != nil {
		return nil, problem(err)
	}
	return DpoCreateAppointment201JSONResponse{Body: toAppointmentWire(a), Headers: DpoCreateAppointment201ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) DpoGetAppointment(ctx context.Context, req DpoGetAppointmentRequestObject) (DpoGetAppointmentResponseObject, error) {
	a, err := h.svc.GetAppointment(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DpoGetAppointment200JSONResponse{Body: toAppointmentWire(a), Headers: DpoGetAppointment200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) DpoUpdateAppointment(ctx context.Context, req DpoUpdateAppointmentRequestObject) (DpoUpdateAppointmentResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := toAppointmentInput(*req.Body)
	in.ID = req.Id
	a, err := h.svc.SaveAppointment(ctx, in, v)
	if err != nil {
		return nil, problem(err)
	}
	return DpoUpdateAppointment200JSONResponse{Body: toAppointmentWire(a), Headers: DpoUpdateAppointment200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func toAppointmentInput(b DpoAppointmentInput) dposervice.Appointment {
	a := dposervice.Appointment{LegalEntityID: b.LegalEntityId, DPOType: string(b.DpoType), UserID: b.UserId,
		ExternalName: str(b.ExternalName), ExternalCompany: str(b.ExternalCompany), ContactEmail: string(b.ContactEmail),
		ContactPhone: str(b.ContactPhone), AppointedAt: b.AppointedAt.Time, AppointmentFileID: b.AppointmentFileId,
		PDPCEvidenceFileID: b.PdpcEvidenceFileId}
	if b.PdpcNotifiedAt != nil {
		a.PDPCNotifiedAt = &b.PdpcNotifiedAt.Time
	}
	if b.EndedAt != nil {
		a.EndedAt = &b.EndedAt.Time
	}
	return a
}

func toAppointmentWire(a dposervice.Appointment) DpoAppointment {
	w := DpoAppointment{Id: a.ID, LegalEntityId: a.LegalEntityID, DpoType: DpoType(a.DPOType), UserId: a.UserID,
		ExternalName: ptr(a.ExternalName), ExternalCompany: ptr(a.ExternalCompany), ContactEmail: a.ContactEmail,
		ContactPhone: ptr(a.ContactPhone), AppointedAt: openapi_types.Date{Time: a.AppointedAt}, AppointmentFileId: a.AppointmentFileID,
		PdpcEvidenceFileId: a.PDPCEvidenceFileID, RowVersion: int(a.RowVersion), UpdatedAt: a.UpdatedAt.UTC()}
	if a.PDPCNotifiedAt != nil {
		w.PdpcNotifiedAt = &openapi_types.Date{Time: *a.PDPCNotifiedAt}
	}
	if a.EndedAt != nil {
		w.EndedAt = &openapi_types.Date{Time: *a.EndedAt}
	}
	return w
}
