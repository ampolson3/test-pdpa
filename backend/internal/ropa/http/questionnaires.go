package ropahttp

import (
	"context"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/forms"
	ropaservice "pdpa-platform/internal/ropa/service"
)

func (h *Strict) RopaListQuestionnaires(ctx context.Context, req RopaListQuestionnairesRequestObject) (RopaListQuestionnairesResponseObject, error) {
	var status string
	if req.Params.Status != nil {
		status = string(*req.Params.Status)
	}
	list, err := h.svc.ListQuestionnaires(ctx, req.Params.OrgUnitId, status)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListQuestionnaires200JSONResponse{Data: make([]RopaQuestionnaire, 0, len(list))}
	for _, q := range list {
		resp.Data = append(resp.Data, toQuestionnaireWire(q))
	}
	return resp, nil
}

func (h *Strict) RopaSendQuestionnaire(ctx context.Context, req RopaSendQuestionnaireRequestObject) (RopaSendQuestionnaireResponseObject, error) {
	b := *req.Body
	q, err := h.svc.SendQuestionnaire(ctx, b.OrgUnitId, b.RespondentUserId, b.DueAt)
	if err != nil {
		return nil, problem(err)
	}
	return RopaSendQuestionnaire201JSONResponse(toQuestionnaireWire(q)), nil
}

func (h *Strict) RopaGetQuestionnaire(ctx context.Context, req RopaGetQuestionnaireRequestObject) (RopaGetQuestionnaireResponseObject, error) {
	q, err := h.svc.GetQuestionnaire(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return RopaGetQuestionnaire200JSONResponse(toQuestionnaireWire(q)), nil
}

func (h *Strict) RopaAnswerQuestionnaire(ctx context.Context, req RopaAnswerQuestionnaireRequestObject) (RopaAnswerQuestionnaireResponseObject, error) {
	b := *req.Body
	answers := forms.Answers(b.Answers)
	q, err := h.svc.AnswerQuestionnaire(ctx, req.Id, b.FormVersionId, answers)
	if err != nil {
		return nil, problem(err)
	}
	return RopaAnswerQuestionnaire200JSONResponse(toQuestionnaireWire(q)), nil
}

func (h *Strict) RopaApproveQuestionnaire(ctx context.Context, req RopaApproveQuestionnaireRequestObject) (RopaApproveQuestionnaireResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := *req.Body
	q, _, err := h.svc.ApproveQuestionnaire(ctx, req.Id, b.LegalEntityId, b.Code, b.OwnerUserId, v)
	if err != nil {
		return nil, problem(err)
	}
	return RopaApproveQuestionnaire200JSONResponse{Body: toQuestionnaireWire(q), Headers: RopaApproveQuestionnaire200ResponseHeaders{ETag: etag(q.RowVersion)}}, nil
}

func (h *Strict) RopaRejectQuestionnaire(ctx context.Context, req RopaRejectQuestionnaireRequestObject) (RopaRejectQuestionnaireResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	q, err := h.svc.RejectQuestionnaire(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return RopaRejectQuestionnaire200JSONResponse{Body: toQuestionnaireWire(q), Headers: RopaRejectQuestionnaire200ResponseHeaders{ETag: etag(q.RowVersion)}}, nil
}

func toQuestionnaireWire(q ropaservice.Questionnaire) RopaQuestionnaire {
	w := RopaQuestionnaire{Id: q.ID, OrgUnitId: q.OrgUnitID, RespondentUserId: q.RespondentUserID,
		RespondentName: ptr(q.RespondentName), Status: RopaQuestionnaireStatus(q.Status), ActivityId: q.ActivityID,
		FormSubmissionId: q.FormSubmissionID, RowVersion: int(q.RowVersion), CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt}
	if q.DueAt != nil {
		w.DueAt = q.DueAt
	}
	return w
}
