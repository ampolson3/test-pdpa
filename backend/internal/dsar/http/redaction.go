package dsarhttp

import (
	"context"

	dsarservice "pdpa-platform/internal/dsar/service"
)

func (h *Strict) DsarListRedactions(ctx context.Context, req DsarListRedactionsRequestObject) (DsarListRedactionsResponseObject, error) {
	list, err := h.svc.ListRedactions(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DsarListRedactions200JSONResponse{Data: make([]DsarRedaction, 0, len(list))}
	for _, r := range list {
		resp.Data = append(resp.Data, toRedactionWire(r))
	}
	return resp, nil
}

func (h *Strict) DsarRedactFile(ctx context.Context, req DsarRedactFileRequestObject) (DsarRedactFileResponseObject, error) {
	b := *req.Body
	regions := make([]dsarservice.Rect, 0, len(b.Regions))
	for _, r := range b.Regions {
		regions = append(regions, dsarservice.Rect{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height})
	}
	out, err := h.svc.RedactFile(ctx, req.Id, b.RawFileId, regions)
	if err != nil {
		return nil, problem(err)
	}
	return DsarRedactFile201JSONResponse(toRedactionWire(out)), nil
}

func toRedactionWire(r dsarservice.Redaction) DsarRedaction {
	regions := make([]DsarRedactionRegion, 0, len(r.Regions))
	for _, rect := range r.Regions {
		regions = append(regions, DsarRedactionRegion{X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height})
	}
	return DsarRedaction{Id: r.ID, RequestId: r.RequestID, SourceFileId: r.SourceFileID, OutputFileId: r.OutputFileID,
		Regions: regions, Status: DsarRedactionStatus(r.Status), ReviewedBy: r.ReviewedBy, CreatedAt: r.CreatedAt.UTC()}
}
