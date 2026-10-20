package riskhttp

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	riskservice "pdpa-platform/internal/risk/service"

	"pdpa-platform/internal/pkg/httpx"
)

// RiskListRisks is RRA-09's own acceptance criterion: every risk.risks row, regardless of source_type,
// in one register list.
func (h *Strict) RiskListRisks(ctx context.Context, req RiskListRisksRequestObject) (RiskListRisksResponseObject, error) {
	f := riskservice.RiskFilter{}
	if req.Params.Status != nil {
		f.Status = string(*req.Params.Status)
	}
	if req.Params.SourceType != nil {
		f.SourceType = string(*req.Params.SourceType)
	}
	if req.Params.OwnerUserId != nil {
		f.OwnerUserID = req.Params.OwnerUserId
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		at, id, err := decodeRiskCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &riskservice.RiskCursor{CreatedAt: at, ID: id}
	}
	list, next, err := h.svc.ListRisks(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := RiskListRisks200JSONResponse{Data: make([]RiskRegisterEntry, 0, len(list))}
	for _, r := range list {
		resp.Data = append(resp.Data, toRiskRegisterWire(r))
	}
	if next != nil {
		c := encodeRiskCursor(next.CreatedAt, next.ID)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) RiskCreateRisk(ctx context.Context, req RiskCreateRiskRequestObject) (RiskCreateRiskResponseObject, error) {
	r, err := h.svc.IdentifyRisk(ctx, fromRiskRegisterInput(*req.Body))
	if err != nil {
		return nil, problem(err)
	}
	return RiskCreateRisk201JSONResponse{Body: toRiskRegisterWire(r), Headers: RiskCreateRisk201ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func (h *Strict) RiskGetRisk(ctx context.Context, req RiskGetRiskRequestObject) (RiskGetRiskResponseObject, error) {
	r, err := h.svc.GetRisk(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return RiskGetRisk200JSONResponse(toRiskRegisterWire(r)), nil
}

func (h *Strict) RiskUpdateRiskEntry(ctx context.Context, req RiskUpdateRiskEntryRequestObject) (RiskUpdateRiskEntryResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := fromRiskRegisterInput(*req.Body)
	r, err := h.svc.UpdateRisk(ctx, req.Id, in, rv)
	if err != nil {
		return nil, problem(err)
	}
	return RiskUpdateRiskEntry200JSONResponse{Body: toRiskRegisterWire(r), Headers: RiskUpdateRiskEntry200ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func fromRiskRegisterInput(b RiskRegisterInput) riskservice.Risk {
	r := riskservice.Risk{SourceType: string(b.SourceType), Title: b.Title, OwnerUserID: b.OwnerUserId,
		ActivityID: b.ActivityId, AssetID: b.AssetId, VendorID: b.VendorId, Likelihood: b.Likelihood, Impact: b.Impact}
	if b.Description != nil {
		r.Description = *b.Description
	}
	if b.Treatment != nil {
		r.Treatment = string(*b.Treatment)
	}
	if b.Status != nil {
		r.Status = string(*b.Status)
	}
	return r
}

func toRiskRegisterWire(r riskservice.Risk) RiskRegisterEntry {
	w := RiskRegisterEntry{Id: r.ID, SourceType: RiskRegisterEntrySourceType(r.SourceType), Title: r.Title,
		OwnerUserId: r.OwnerUserID, ActivityId: r.ActivityID, AssetId: r.AssetID, VendorId: r.VendorID,
		Likelihood: r.Likelihood, Impact: r.Impact, InherentScore: float32(r.InherentScore), Level: RiskRegisterEntryLevel(r.Level),
		Status: RiskRegisterEntryStatus(r.Status), RowVersion: int(r.RowVersion), CreatedAt: r.CreatedAt.UTC()}
	if r.Description != "" {
		w.Description = &r.Description
	}
	if r.Treatment != "" {
		t := RiskRegisterEntryTreatment(r.Treatment)
		w.Treatment = &t
	}
	if r.ResidualLikelihood != nil {
		w.ResidualLikelihood = r.ResidualLikelihood
	}
	if r.ResidualImpact != nil {
		w.ResidualImpact = r.ResidualImpact
	}
	if r.ResidualScore != nil {
		v := float32(*r.ResidualScore)
		w.ResidualScore = &v
	}
	if r.ResidualLevel != nil {
		l := RiskRegisterEntryResidualLevel(*r.ResidualLevel)
		w.ResidualLevel = &l
	}
	return w
}

// encodeRiskCursor/decodeRiskCursor are the register's own (created_at, id) keyset cursor codec.
func encodeRiskCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeRiskCursor(s string) (time.Time, uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, uuid.UUID{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return time.Time{}, uuid.UUID{}, errors.New("cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.UUID{}, err
	}
	u, err := uuid.Parse(id)
	return t, u, err
}
