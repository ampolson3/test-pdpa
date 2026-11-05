package dsarpublichttp

import (
	"context"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/publickeys"
)

// deliveryPackage is the key of this request, which must stand for a DSAR-14 secure delivery package.
func deliveryPackage(ctx context.Context) (publickeys.Key, error) {
	k, ok := publickeys.FromContext(ctx)
	if !ok || k.EntityType != publickeys.EntityDsarPackage {
		return publickeys.Key{}, httpx.NotFound()
	}
	return k, nil
}

func (h *Strict) DsarGetPublicPackage(ctx context.Context, _ DsarGetPublicPackageRequestObject) (DsarGetPublicPackageResponseObject, error) {
	k, err := deliveryPackage(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.svc.PublicPackage(ctx, k.EntityID)
	if err != nil {
		return nil, problem(err)
	}
	return DsarGetPublicPackage200JSONResponse{Format: DsarPackageFormat(p.Format), PasswordProtected: p.PasswordProtected, ExpiresAt: p.ExpiresAt}, nil
}

func (h *Strict) DsarDownloadPublicPackage(ctx context.Context, req DsarDownloadPublicPackageRequestObject) (DsarDownloadPublicPackageResponseObject, error) {
	k, err := deliveryPackage(ctx)
	if err != nil {
		return nil, err
	}
	code := ""
	if req.Body != nil && req.Body.AccessCode != nil {
		code = *req.Body.AccessCode
	}
	url, err := h.svc.PublicDownload(ctx, k.EntityID, code)
	if err != nil {
		return nil, problem(err)
	}
	return DsarDownloadPublicPackage200JSONResponse{DownloadUrl: url}, nil
}
