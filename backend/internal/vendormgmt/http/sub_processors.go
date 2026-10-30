package vendorhttp

import (
	"context"

	"pdpa-platform/internal/pkg/httpx"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

func (h *Strict) VendorListSubProcessors(ctx context.Context, req VendorListSubProcessorsRequestObject) (VendorListSubProcessorsResponseObject, error) {
	list, err := h.svc.ListSubProcessors(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := VendorListSubProcessors200JSONResponse{Data: make([]VendorSubProcessor, 0, len(list))}
	for _, sp := range list {
		resp.Data = append(resp.Data, toSubProcessorWire(sp))
	}
	return resp, nil
}

func (h *Strict) VendorAddSubProcessor(ctx context.Context, req VendorAddSubProcessorRequestObject) (VendorAddSubProcessorResponseObject, error) {
	b := req.Body
	in := vendorservice.SubProcessorInput{PartyID: b.PartyId, Service: b.Service}
	if b.Countries != nil {
		in.Countries = *b.Countries
	}
	sp, err := h.svc.AddSubProcessor(ctx, req.Id, in)
	if err != nil {
		return nil, problem(err)
	}
	return VendorAddSubProcessor201JSONResponse(toSubProcessorWire(sp)), nil
}

func (h *Strict) VendorDecideSubProcessor(ctx context.Context, req VendorDecideSubProcessorRequestObject) (VendorDecideSubProcessorResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	sp, err := h.svc.DecideSubProcessor(ctx, req.Id, req.SubProcessorId, string(req.Body.Decision), rv)
	if err != nil {
		return nil, problem(err)
	}
	return VendorDecideSubProcessor200JSONResponse(toSubProcessorWire(sp)), nil
}

func toSubProcessorWire(sp vendorservice.SubProcessor) VendorSubProcessor {
	w := VendorSubProcessor{Id: sp.ID, VendorId: sp.VendorID, PartyId: sp.PartyID, Service: sp.Service,
		Countries: sp.Countries, Status: VendorSubProcessorStatus(sp.Status), RowVersion: int(sp.RowVersion),
		CreatedAt: sp.CreatedAt.UTC(), UpdatedAt: sp.UpdatedAt.UTC()}
	if sp.ApprovedBy != nil {
		w.ApprovedBy = sp.ApprovedBy
	}
	if sp.ApprovedAt != nil {
		t := sp.ApprovedAt.UTC()
		w.ApprovedAt = &t
	}
	return w
}
