package vendorhttp

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

func (h *Strict) VendorListCertificates(ctx context.Context, req VendorListCertificatesRequestObject) (VendorListCertificatesResponseObject, error) {
	list, err := h.svc.ListCertificates(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := VendorListCertificates200JSONResponse{Data: make([]VendorCertificate, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toCertificateWire(c))
	}
	return resp, nil
}

func (h *Strict) VendorAddCertificate(ctx context.Context, req VendorAddCertificateRequestObject) (VendorAddCertificateResponseObject, error) {
	b := req.Body
	in := vendorservice.CertificateInput{VendorID: req.Id, CertType: b.CertType, Issuer: b.Issuer, FileID: b.FileId}
	if b.ValidFrom != nil {
		t := b.ValidFrom.Time
		in.ValidFrom = &t
	}
	if b.ValidTo != nil {
		t := b.ValidTo.Time
		in.ValidTo = &t
	}
	c, err := h.svc.AddCertificate(ctx, in)
	if err != nil {
		return nil, problem(err)
	}
	return VendorAddCertificate201JSONResponse(toCertificateWire(c)), nil
}

func (h *Strict) VendorDeleteCertificate(ctx context.Context, req VendorDeleteCertificateRequestObject) (VendorDeleteCertificateResponseObject, error) {
	if err := h.svc.DeleteCertificate(ctx, req.Id, req.CertificateId); err != nil {
		return nil, problem(err)
	}
	return VendorDeleteCertificate204Response{}, nil
}

func toCertificateWire(c vendorservice.Certificate) VendorCertificate {
	w := VendorCertificate{Id: c.ID, VendorId: c.VendorID, CertType: c.CertType, Issuer: c.Issuer, FileId: c.FileID,
		RowVersion: int(c.RowVersion), CreatedAt: c.CreatedAt.UTC(), UpdatedAt: c.UpdatedAt.UTC()}
	if c.ValidFrom != nil {
		d := openapi_types.Date{Time: *c.ValidFrom}
		w.ValidFrom = &d
	}
	if c.ValidTo != nil {
		d := openapi_types.Date{Time: *c.ValidTo}
		w.ValidTo = &d
	}
	if c.VerifiedBy != nil {
		w.VerifiedBy = c.VerifiedBy
	}
	return w
}
