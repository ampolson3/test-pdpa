package dsarhttp

import (
	"context"

	dsarservice "pdpa-platform/internal/dsar/service"
)

func (h *Strict) DsarListSearchResults(ctx context.Context, req DsarListSearchResultsRequestObject) (DsarListSearchResultsResponseObject, error) {
	results, err := h.svc.ListSearchResults(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DsarListSearchResults200JSONResponse{Data: make([]DsarSearchResult, 0, len(results))}
	for _, r := range results {
		resp.Data = append(resp.Data, toSearchResultWire(r))
	}
	return resp, nil
}

func (h *Strict) DsarRunSearch(ctx context.Context, req DsarRunSearchRequestObject) (DsarRunSearchResponseObject, error) {
	results, err := h.svc.SearchAcrossSystems(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DsarRunSearch201JSONResponse{Data: make([]DsarSearchResult, 0, len(results))}
	for _, r := range results {
		resp.Data = append(resp.Data, toSearchResultWire(r))
	}
	return resp, nil
}

func toSearchResultWire(r dsarservice.SearchResult) DsarSearchResult {
	w := DsarSearchResult{
		Id: r.ID, ConnectorId: r.ConnectorID, ConnectorName: r.ConnectorName, ConnectorType: r.ConnectorType,
		Found: r.Found, SearchedAt: r.SearchedAt.UTC(),
	}
	if r.RecordCount != nil {
		n := int(*r.RecordCount)
		w.RecordCount = &n
	}
	if r.PurgedAt != nil {
		t := r.PurgedAt.UTC()
		w.PurgedAt = &t
	}
	return w
}
