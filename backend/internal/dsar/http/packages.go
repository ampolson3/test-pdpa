package dsarhttp

import (
	"context"

	dsarservice "pdpa-platform/internal/dsar/service"
)

func (h *Strict) DsarListPackages(ctx context.Context, req DsarListPackagesRequestObject) (DsarListPackagesResponseObject, error) {
	packages, err := h.svc.ListPackages(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DsarListPackages200JSONResponse{Data: make([]DsarPackage, 0, len(packages))}
	for _, p := range packages {
		resp.Data = append(resp.Data, toPackageWire(p))
	}
	return resp, nil
}

func (h *Strict) DsarCreatePackage(ctx context.Context, req DsarCreatePackageRequestObject) (DsarCreatePackageResponseObject, error) {
	b := *req.Body
	in := dsarservice.CreatePackageInput{FileID: b.FileId, Format: string(b.Format)}
	if b.PasswordProtected != nil {
		in.PasswordProtected = *b.PasswordProtected
	}
	if b.ExpiresAt != nil {
		in.ExpiresAt = b.ExpiresAt
	}
	p, code, err := h.svc.CreatePackage(ctx, req.Id, in)
	if err != nil {
		return nil, problem(err)
	}
	out := DsarCreatePackageResult{Package: toPackageWire(p), PublicKey: p.PublicKey}
	if code != "" && p.PasswordProtected {
		out.AccessCode = &code
	}
	return DsarCreatePackage201JSONResponse(out), nil
}

func toPackageWire(p dsarservice.Package) DsarPackage {
	w := DsarPackage{Id: p.ID, RequestId: p.RequestID, FileId: p.FileID, Format: DsarPackageFormat(p.Format),
		PasswordProtected: p.PasswordProtected, ExpiresAt: p.ExpiresAt, DownloadCount: int(p.DownloadCount),
		RowVersion: int(p.RowVersion), CreatedAt: p.CreatedAt}
	if p.LastDownloadedAt != nil {
		t := *p.LastDownloadedAt
		w.LastDownloadedAt = &t
	}
	if p.PurgedAt != nil {
		t := *p.PurgedAt
		w.PurgedAt = &t
	}
	return w
}
