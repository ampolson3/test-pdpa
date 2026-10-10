package dpiahttp

import (
	"context"
	"strconv"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	dpiaservice "pdpa-platform/internal/dpia/service"
	riskservice "pdpa-platform/internal/risk/service"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DpiaRiskCatalog(ctx context.Context, req DpiaRiskCatalogRequestObject) (DpiaRiskCatalogResponseObject, error) {
	resp := DpiaRiskCatalog200JSONResponse{}
	for _, c := range dpiaservice.RiskCatalog() {
		resp.Data = append(resp.Data, DpiaRiskCatalogItem{Code: c.Code, TitleTh: c.TitleTh, TitleEn: c.TitleEn, DescriptionTh: c.DescriptionTh})
	}
	return resp, nil
}

func (h *Strict) DpiaListAssessmentRisks(ctx context.Context, req DpiaListAssessmentRisksRequestObject) (DpiaListAssessmentRisksResponseObject, error) {
	list, err := h.svc.ListAssessmentRisks(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpiaListAssessmentRisks200JSONResponse{Data: make([]DpiaRisk, 0, len(list))}
	for _, r := range list {
		resp.Data = append(resp.Data, toRiskWire(r))
	}
	return resp, nil
}

func (h *Strict) DpiaIdentifyRisk(ctx context.Context, req DpiaIdentifyRiskRequestObject) (DpiaIdentifyRiskResponseObject, error) {
	r, err := h.svc.IdentifyRisk(ctx, req.Id, fromRiskInput(*req.Body))
	if err != nil {
		return nil, problem(err)
	}
	return DpiaIdentifyRisk201JSONResponse(toRiskWire(r)), nil
}

func (h *Strict) DpiaUpdateRisk(ctx context.Context, req DpiaUpdateRiskRequestObject) (DpiaUpdateRiskResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.PreconditionRequired("If-Match")
	}
	r, err := h.svc.UpdateRisk(ctx, req.Id, req.RiskId, fromRiskInput(*req.Body), rv)
	if err != nil {
		return nil, problem(err)
	}
	etag := `"` + strconv.Itoa(int(r.RowVersion)) + `"`
	return DpiaUpdateRisk200JSONResponse{Body: toRiskWire(r), Headers: DpiaUpdateRisk200ResponseHeaders{ETag: &etag}}, nil
}

func (h *Strict) DpiaRemoveAssessmentRisk(ctx context.Context, req DpiaRemoveAssessmentRiskRequestObject) (DpiaRemoveAssessmentRiskResponseObject, error) {
	if err := h.svc.RemoveAssessmentRisk(ctx, req.Id, req.RiskId); err != nil {
		return nil, problem(err)
	}
	return DpiaRemoveAssessmentRisk204Response{}, nil
}

func (h *Strict) DpiaListRiskControls(ctx context.Context, req DpiaListRiskControlsRequestObject) (DpiaListRiskControlsResponseObject, error) {
	list, err := h.svc.ListRiskControls(ctx, req.Id, req.RiskId)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpiaListRiskControls200JSONResponse{Data: make([]DpiaRiskControl, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toRiskControlWire(c))
	}
	return resp, nil
}

func (h *Strict) DpiaAddRiskControl(ctx context.Context, req DpiaAddRiskControlRequestObject) (DpiaAddRiskControlResponseObject, error) {
	var dueAt *time.Time
	if req.Body.DueAt != nil {
		t := req.Body.DueAt.Time
		dueAt = &t
	}
	c, err := h.svc.AddRiskControl(ctx, req.Id, req.RiskId, req.Body.ControlId, req.Body.OwnerUserId, dueAt)
	if err != nil {
		return nil, problem(err)
	}
	return DpiaAddRiskControl201JSONResponse(toRiskControlWire(c)), nil
}

func (h *Strict) DpiaUpdateRiskControlStatus(ctx context.Context, req DpiaUpdateRiskControlStatusRequestObject) (DpiaUpdateRiskControlStatusResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.PreconditionRequired("If-Match")
	}
	c, err := h.svc.UpdateRiskControlStatus(ctx, req.Id, req.RiskId, req.ControlId, string(req.Body.Status), rv)
	if err != nil {
		return nil, problem(err)
	}
	etag := `"` + strconv.Itoa(int(c.RowVersion)) + `"`
	return DpiaUpdateRiskControlStatus200JSONResponse{Body: toRiskControlWire(c), Headers: DpiaUpdateRiskControlStatus200ResponseHeaders{ETag: &etag}}, nil
}

func (h *Strict) DpiaRemoveRiskControl(ctx context.Context, req DpiaRemoveRiskControlRequestObject) (DpiaRemoveRiskControlResponseObject, error) {
	if err := h.svc.RemoveRiskControl(ctx, req.Id, req.RiskId, req.ControlId); err != nil {
		return nil, problem(err)
	}
	return DpiaRemoveRiskControl204Response{}, nil
}

func toRiskControlWire(c riskservice.RiskControl) DpiaRiskControl {
	w := DpiaRiskControl{RiskId: c.RiskID, ControlId: c.ControlID, ControlCode: c.ControlCode, ControlName: c.ControlName,
		ControlCategory: c.ControlCategory, Status: DpiaRiskControlStatus(c.Status), OwnerUserId: c.OwnerUserID,
		TaskId: c.TaskID, RowVersion: int(c.RowVersion), CreatedAt: c.CreatedAt.UTC()}
	if c.DueAt != nil {
		w.DueAt = &openapi_types.Date{Time: *c.DueAt}
	}
	return w
}

func fromRiskInput(in DpiaRiskInput) riskservice.Risk {
	r := riskservice.Risk{Title: in.Title, Likelihood: in.Likelihood, Impact: in.Impact, OwnerUserID: in.OwnerUserId}
	if in.Description != nil {
		r.Description = *in.Description
	}
	if in.Treatment != nil {
		r.Treatment = string(*in.Treatment)
	}
	if in.Status != nil {
		r.Status = string(*in.Status)
	}
	return r
}

func toRiskWire(r riskservice.Risk) DpiaRisk {
	w := DpiaRisk{Id: r.ID, Title: r.Title, Likelihood: r.Likelihood, Impact: r.Impact, InherentScore: float32(r.InherentScore),
		Level: DpiaRiskLevel(r.Level), Status: DpiaRiskStatus(r.Status), OwnerUserId: r.OwnerUserID, RowVersion: int(r.RowVersion),
		CreatedAt: r.CreatedAt.UTC(), ResidualLikelihood: r.ResidualLikelihood, ResidualImpact: r.ResidualImpact}
	if r.Description != "" {
		w.Description = &r.Description
	}
	if r.Treatment != "" {
		t := DpiaRiskTreatment(r.Treatment)
		w.Treatment = &t
	}
	if r.ResidualScore != nil {
		v := float32(*r.ResidualScore)
		w.ResidualScore = &v
	}
	if r.ResidualLevel != nil {
		l := DpiaRiskResidualLevel(*r.ResidualLevel)
		w.ResidualLevel = &l
	}
	return w
}
