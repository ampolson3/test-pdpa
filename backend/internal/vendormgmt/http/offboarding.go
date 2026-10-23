package vendorhttp

import (
	"context"

	vendorservice "pdpa-platform/internal/vendormgmt/service"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) VendorGetOffboarding(ctx context.Context, req VendorGetOffboardingRequestObject) (VendorGetOffboardingResponseObject, error) {
	o, err := h.svc.GetOffboarding(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return VendorGetOffboarding200JSONResponse{Body: toVendorOffboardingWire(o), Headers: VendorGetOffboarding200ResponseHeaders{ETag: etag(o.RowVersion)}}, nil
}

func (h *Strict) VendorUpdateOffboarding(ctx context.Context, req VendorUpdateOffboardingRequestObject) (VendorUpdateOffboardingResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := vendorservice.UpdateOffboardingInput{DataReturnStatus: string(req.Body.DataReturnStatus), DestructionCertificateFileID: req.Body.DestructionCertificateFileId}
	if req.Body.AccessRevoked != nil {
		in.AccessRevoked = *req.Body.AccessRevoked
	}
	o, err := h.svc.UpdateOffboarding(ctx, req.Id, in, rv)
	if err != nil {
		return nil, problem(err)
	}
	return VendorUpdateOffboarding200JSONResponse{Body: toVendorOffboardingWire(o), Headers: VendorUpdateOffboarding200ResponseHeaders{ETag: etag(o.RowVersion)}}, nil
}

func (h *Strict) VendorTransitionVendor(ctx context.Context, req VendorTransitionVendorRequestObject) (VendorTransitionVendorResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	v, err := h.svc.TransitionVendor(ctx, req.Id, string(req.Body.To), rv)
	if err != nil {
		return nil, problem(err)
	}
	return VendorTransitionVendor200JSONResponse{Body: toVendorWire(v), Headers: VendorTransitionVendor200ResponseHeaders{ETag: etag(v.RowVersion)}}, nil
}

func toVendorOffboardingWire(o vendorservice.Offboarding) VendorOffboarding {
	w := VendorOffboarding{Id: o.ID, VendorId: o.VendorID, DataReturnStatus: VendorOffboardingDataReturnStatus(o.DataReturnStatus),
		DestructionCertificateFileId: o.DestructionCertificateFileID, RowVersion: int(o.RowVersion), CreatedAt: o.CreatedAt.UTC()}
	if o.AccessRevokedAt != nil {
		t := o.AccessRevokedAt.UTC()
		w.AccessRevokedAt = &t
	}
	if o.CompletedAt != nil {
		t := o.CompletedAt.UTC()
		w.CompletedAt = &t
	}
	return w
}
