package dsarhttp

import (
	"context"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DsarListLegalHolds(ctx context.Context, req DsarListLegalHoldsRequestObject) (DsarListLegalHoldsResponseObject, error) {
	holds, err := h.svc.ListLegalHolds(ctx)
	if err != nil {
		return nil, problem(err)
	}
	resp := DsarListLegalHolds200JSONResponse{Data: make([]DsarLegalHold, 0, len(holds))}
	for _, hld := range holds {
		resp.Data = append(resp.Data, toLegalHoldWire(hld))
	}
	return resp, nil
}

func (h *Strict) DsarCreateLegalHold(ctx context.Context, req DsarCreateLegalHoldRequestObject) (DsarCreateLegalHoldResponseObject, error) {
	b := *req.Body
	var holdRule map[string]any
	if b.HoldRule != nil {
		holdRule = *b.HoldRule
	}
	in := dsarservice.LegalHold{DataCategoryID: b.DataCategoryId, LawRef: b.LawRef, HoldRule: holdRule}
	if b.AssetId != nil {
		in.AssetID = *b.AssetId
	}
	hld, err := h.svc.CreateLegalHold(ctx, in)
	if err != nil {
		return nil, problem(err)
	}
	return DsarCreateLegalHold201JSONResponse(toLegalHoldWire(hld)), nil
}

func (h *Strict) DsarDeactivateLegalHold(ctx context.Context, req DsarDeactivateLegalHoldRequestObject) (DsarDeactivateLegalHoldResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	hld, err := h.svc.DeactivateLegalHold(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return DsarDeactivateLegalHold200JSONResponse(toLegalHoldWire(hld)), nil
}

func (h *Strict) DsarListExemptionChecks(ctx context.Context, req DsarListExemptionChecksRequestObject) (DsarListExemptionChecksResponseObject, error) {
	checks, err := h.svc.ListExemptionChecks(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DsarListExemptionChecks200JSONResponse{Data: make([]DsarExemptionCheck, 0, len(checks))}
	for _, c := range checks {
		resp.Data = append(resp.Data, toExemptionCheckWire(c))
	}
	return resp, nil
}

func (h *Strict) DsarRunExemptionCheck(ctx context.Context, req DsarRunExemptionCheckRequestObject) (DsarRunExemptionCheckResponseObject, error) {
	b := *req.Body
	c, err := h.svc.RunExemptionCheck(ctx, req.Id, b.AssetIds)
	if err != nil {
		return nil, problem(err)
	}
	return DsarRunExemptionCheck201JSONResponse(toExemptionCheckWire(c)), nil
}

func toLegalHoldWire(h dsarservice.LegalHold) DsarLegalHold {
	w := DsarLegalHold{Id: h.ID, LawRef: h.LawRef, IsActive: h.IsActive, RowVersion: int(h.RowVersion), CreatedAt: h.CreatedAt.UTC()}
	if h.DataCategoryID != nil {
		w.DataCategoryId = h.DataCategoryID
	}
	if h.AssetID != uuid.Nil {
		id := h.AssetID
		w.AssetId = &id
	}
	if h.HoldRule != nil {
		w.HoldRule = &h.HoldRule
	}
	return w
}

func toExemptionCheckWire(c dsarservice.ExemptionCheck) DsarExemptionCheck {
	return DsarExemptionCheck{Id: c.ID, RequestId: c.RequestID, LegalHoldId: c.LegalHoldID, Result: DsarExemptionResult(c.Result), Reason: c.Reason, CreatedAt: c.CreatedAt.UTC()}
}
