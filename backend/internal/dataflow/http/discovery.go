// DFG-09: handlers for data-discovery scans (internal/dataflow/service/discovery.go).
package dataflowhttp

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	dataflowservice "pdpa-platform/internal/dataflow/service"
	"pdpa-platform/internal/pkg/httpx"
)

func discoveryProblem(err error) error {
	switch {
	case errors.Is(err, dataflowservice.ErrDiscoveryNotFound):
		return httpx.NotFound()
	case errors.Is(err, dataflowservice.ErrDiscoveryVersion):
		return httpx.VersionMismatch()
	case errors.Is(err, dataflowservice.ErrDiscoveryNotConfigured):
		return httpx.UnprocessableEntity("dataflow.discovery_not_configured", "Connectors are not configured")
	case errors.Is(err, dataflowservice.ErrInvalid):
		return httpx.UnprocessableEntity("dataflow.invalid_input", err.Error())
	}
	return err
}

func (h *Strict) DataflowRunDiscoveryScan(ctx context.Context, req DataflowRunDiscoveryScanRequestObject) (DataflowRunDiscoveryScanResponseObject, error) {
	sc, err := h.svc.RunScan(ctx, req.Body.ConnectorId, req.Body.AssetId)
	if err != nil {
		return nil, discoveryProblem(err)
	}
	return DataflowRunDiscoveryScan201JSONResponse(toScanWire(sc)), nil
}

func (h *Strict) DataflowListDiscoveryScans(ctx context.Context, req DataflowListDiscoveryScansRequestObject) (DataflowListDiscoveryScansResponseObject, error) {
	var cur *dataflowservice.DiscoveryScanCursor
	if req.Params.Cursor != nil {
		c, err := decodeScanCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		cur = &c
	}
	limit := 0
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	list, next, err := h.svc.ListDiscoveryScans(ctx, cur, limit)
	if err != nil {
		return nil, err
	}
	resp := DataflowListDiscoveryScans200JSONResponse{Data: make([]DataflowDiscoveryScan, 0, len(list))}
	for _, sc := range list {
		resp.Data = append(resp.Data, toScanWire(sc))
	}
	if next != nil {
		s := base64.RawURLEncoding.EncodeToString([]byte(next.At.Format(time.RFC3339Nano) + "|" + next.ID.String()))
		resp.NextCursor = &s
	}
	return resp, nil
}

func (h *Strict) DataflowListDiscoveryFindings(ctx context.Context, req DataflowListDiscoveryFindingsRequestObject) (DataflowListDiscoveryFindingsResponseObject, error) {
	list, err := h.svc.ListDiscoveryFindings(ctx, req.Id)
	if err != nil {
		return nil, discoveryProblem(err)
	}
	resp := DataflowListDiscoveryFindings200JSONResponse{Data: make([]DataflowDiscoveryFinding, 0, len(list))}
	for _, f := range list {
		resp.Data = append(resp.Data, toFindingWire(f))
	}
	return resp, nil
}

func (h *Strict) DataflowReviewDiscoveryFinding(ctx context.Context, req DataflowReviewDiscoveryFindingRequestObject) (DataflowReviewDiscoveryFindingResponseObject, error) {
	v, err := dfParseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	f, err := h.svc.ReviewDiscoveryFinding(ctx, req.Id, string(req.Body.Decision), v)
	if err != nil {
		return nil, discoveryProblem(err)
	}
	return DataflowReviewDiscoveryFinding200JSONResponse{Body: toFindingWire(f), Headers: DataflowReviewDiscoveryFinding200ResponseHeaders{ETag: dfEtag(f.RowVersion)}}, nil
}

func decodeScanCursor(s string) (dataflowservice.DiscoveryScanCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return dataflowservice.DiscoveryScanCursor{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return dataflowservice.DiscoveryScanCursor{}, errors.New("cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return dataflowservice.DiscoveryScanCursor{}, err
	}
	u, err := uuid.Parse(id)
	return dataflowservice.DiscoveryScanCursor{At: t, ID: u}, err
}

func toScanWire(s dataflowservice.DiscoveryScan) DataflowDiscoveryScan {
	return DataflowDiscoveryScan{
		Id: s.ID, ConnectorId: s.ConnectorID, AssetId: s.AssetID, Status: DataflowDiscoveryScanStatus(s.Status),
		StartedAt: s.StartedAt, FinishedAt: s.FinishedAt, ObjectsScanned: int(s.ObjectsScanned),
		FindingsCount: int(s.FindingsCount), RowVersion: int(s.RowVersion), CreatedAt: s.CreatedAt,
	}
}

func toFindingWire(f dataflowservice.DiscoveryFinding) DataflowDiscoveryFinding {
	return DataflowDiscoveryFinding{
		Id: f.ID, ScanId: f.ScanID, ObjectPath: f.ObjectPath, ClassifierId: f.ClassifierID,
		ClassifierCode: f.ClassifierCode, ClassifierName: f.ClassifierName, MatchRatio: float32(f.MatchRatio),
		SampleCount: int(f.SampleCount), SuggestedCategoryId: f.SuggestedCategoryID,
		Status: DataflowDiscoveryFindingStatus(f.Status), RowVersion: int(f.RowVersion), CreatedAt: f.CreatedAt,
	}
}
