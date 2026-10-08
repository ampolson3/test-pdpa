// Package riskhttp holds the risk module's admin endpoints (RRA-02's risk matrices so far). Types in
// risk.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package riskhttp

import (
	"context"
	"errors"
	"strconv"
	"strings"

	riskservice "pdpa-platform/internal/risk/service"

	"pdpa-platform/internal/pkg/httpx"
)

type Strict struct {
	svc *riskservice.Service
}

func NewStrict(svc *riskservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) RiskListMatrices(ctx context.Context, req RiskListMatricesRequestObject) (RiskListMatricesResponseObject, error) {
	list, err := h.svc.ListMatrices(ctx)
	if err != nil {
		return nil, err
	}
	resp := RiskListMatrices200JSONResponse{Data: make([]RiskMatrix, 0, len(list))}
	for _, m := range list {
		resp.Data = append(resp.Data, toMatrixWire(m))
	}
	return resp, nil
}

func (h *Strict) RiskCreateMatrix(ctx context.Context, req RiskCreateMatrixRequestObject) (RiskCreateMatrixResponseObject, error) {
	m, err := h.svc.SaveMatrix(ctx, toMatrixInput(*req.Body), 0)
	if err != nil {
		return nil, problem(err)
	}
	return RiskCreateMatrix201JSONResponse{Body: toMatrixWire(m), Headers: RiskCreateMatrix201ResponseHeaders{ETag: etag(m.RowVersion)}}, nil
}

func (h *Strict) RiskGetMatrix(ctx context.Context, req RiskGetMatrixRequestObject) (RiskGetMatrixResponseObject, error) {
	m, err := h.svc.GetMatrix(ctx, &req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return RiskGetMatrix200JSONResponse{Body: toMatrixWire(m), Headers: RiskGetMatrix200ResponseHeaders{ETag: etag(m.RowVersion)}}, nil
}

func (h *Strict) RiskUpdateMatrix(ctx context.Context, req RiskUpdateMatrixRequestObject) (RiskUpdateMatrixResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := toMatrixInput(*req.Body)
	in.ID = req.Id
	m, err := h.svc.SaveMatrix(ctx, in, rv)
	if err != nil {
		return nil, problem(err)
	}
	return RiskUpdateMatrix200JSONResponse{Body: toMatrixWire(m), Headers: RiskUpdateMatrix200ResponseHeaders{ETag: etag(m.RowVersion)}}, nil
}

func (h *Strict) RiskDeleteMatrix(ctx context.Context, req RiskDeleteMatrixRequestObject) (RiskDeleteMatrixResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	if err := h.svc.DeleteMatrix(ctx, req.Id, rv); err != nil {
		return nil, problem(err)
	}
	return RiskDeleteMatrix204Response{}, nil
}

func (h *Strict) RiskGetLatestActivityScore(ctx context.Context, req RiskGetLatestActivityScoreRequestObject) (RiskGetLatestActivityScoreResponseObject, error) {
	s, err := h.svc.LatestScore(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return RiskGetLatestActivityScore200JSONResponse(toScoreWire(s)), nil
}

func (h *Strict) RiskScoreActivity(ctx context.Context, req RiskScoreActivityRequestObject) (RiskScoreActivityResponseObject, error) {
	s, err := h.svc.Score(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return RiskScoreActivity201JSONResponse(toScoreWire(s)), nil
}

func toScoreWire(s riskservice.ActivityScore) ActivityRiskScore {
	w := ActivityRiskScore{Id: s.ID, ActivityId: s.ActivityID, MatrixId: s.MatrixID, Likelihood: s.Likelihood,
		Impact: s.Impact, Score: float32(s.Score), Level: RiskLevel(s.Level), ComputedAt: s.ComputedAt.UTC(),
		Factors: []RiskFactorContribution{}}
	for _, f := range s.Factors {
		w.Factors = append(w.Factors, RiskFactorContribution{Code: f.Code, ContributesTo: RiskFactorContributionContributesTo(f.ContributesTo)})
	}
	return w
}

func toMatrixInput(b RiskMatrixInput) riskservice.RiskMatrix {
	m := riskservice.RiskMatrix{Name: b.Name, LikelihoodLevels: b.LikelihoodLevels, ImpactLevels: b.ImpactLevels}
	for _, t := range b.Thresholds {
		m.Thresholds = append(m.Thresholds, riskservice.Threshold{Level: string(t.Level), MinScore: float64(t.MinScore)})
	}
	if b.IsDefault != nil {
		m.IsDefault = *b.IsDefault
	}
	return m
}

func toMatrixWire(m riskservice.RiskMatrix) RiskMatrix {
	w := RiskMatrix{Id: m.ID, Name: m.Name, LikelihoodLevels: m.LikelihoodLevels, ImpactLevels: m.ImpactLevels,
		IsDefault: m.IsDefault, RowVersion: int(m.RowVersion)}
	for _, t := range m.Thresholds {
		w.Thresholds = append(w.Thresholds, RiskThreshold{Level: RiskLevel(t.Level), MinScore: float32(t.MinScore)})
	}
	return w
}

func etag(v int32) *string {
	s := `"` + strconv.Itoa(int(v)) + `"`
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func problem(err error) error {
	switch {
	case errors.Is(err, riskservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, riskservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, riskservice.ErrInvalid):
		return httpx.UnprocessableEntity("risk.invalid_input", err.Error())
	}
	return err
}
