package dpiahttp

import (
	"context"
	"strconv"

	dpiaservice "pdpa-platform/internal/dpia/service"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DpiaTransitionAssessment(ctx context.Context, req DpiaTransitionAssessmentRequestObject) (DpiaTransitionAssessmentResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.PreconditionRequired("If-Match")
	}
	var reason string
	if req.Body.Reason != nil {
		reason = *req.Body.Reason
	}
	a, err := h.svc.Transition(ctx, req.Id, rv, string(req.Body.To), reason)
	if err != nil {
		return nil, problem(err)
	}
	w := toAssessmentWire(a)
	etag := `"` + strconv.Itoa(int(a.RowVersion)) + `"`
	return DpiaTransitionAssessment200JSONResponse{Body: w, Headers: DpiaTransitionAssessment200ResponseHeaders{ETag: &etag}}, nil
}

func (h *Strict) DpiaListOpinions(ctx context.Context, req DpiaListOpinionsRequestObject) (DpiaListOpinionsResponseObject, error) {
	list, err := h.svc.ListOpinions(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpiaListOpinions200JSONResponse{Data: make([]DpiaOpinion, 0, len(list))}
	for _, o := range list {
		resp.Data = append(resp.Data, toOpinionWire(o))
	}
	return resp, nil
}

func (h *Strict) DpiaRecordOpinion(ctx context.Context, req DpiaRecordOpinionRequestObject) (DpiaRecordOpinionResponseObject, error) {
	o, err := h.svc.RecordOpinion(ctx, req.Id, req.Body.Opinion, string(req.Body.Recommendation))
	if err != nil {
		return nil, problem(err)
	}
	return DpiaRecordOpinion201JSONResponse(toOpinionWire(o)), nil
}

func toOpinionWire(o dpiaservice.Opinion) DpiaOpinion {
	return DpiaOpinion{Id: o.ID, AssessmentId: o.AssessmentID, DpoUserId: o.DpoUserID, Opinion: o.Opinion,
		Recommendation: DpiaOpinionRecommendation(o.Recommendation), CreatedAt: o.CreatedAt.UTC()}
}
