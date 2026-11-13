package dpohttp

import (
	"context"

	dposervice "pdpa-platform/internal/dpo/service"
	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) DpoListAdvisories(ctx context.Context, req DpoListAdvisoriesRequestObject) (DpoListAdvisoriesResponseObject, error) {
	var status string
	if req.Params.Status != nil {
		status = string(*req.Params.Status)
	}
	list, err := h.svc.ListAdvisories(ctx, req.Params.OrgUnitId, status)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpoListAdvisories200JSONResponse{Data: make([]DpoAdvisory, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toAdvisoryWire(a))
	}
	return resp, nil
}

func (h *Strict) DpoSubmitAdvisory(ctx context.Context, req DpoSubmitAdvisoryRequestObject) (DpoSubmitAdvisoryResponseObject, error) {
	b := *req.Body
	a, err := h.svc.SubmitAdvisory(ctx, b.OrgUnitId, b.Subject, b.Question)
	if err != nil {
		return nil, problem(err)
	}
	return DpoSubmitAdvisory201JSONResponse(toAdvisoryWire(a)), nil
}

func (h *Strict) DpoGetAdvisory(ctx context.Context, req DpoGetAdvisoryRequestObject) (DpoGetAdvisoryResponseObject, error) {
	a, err := h.svc.GetAdvisory(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DpoGetAdvisory200JSONResponse(toAdvisoryWire(a)), nil
}

func (h *Strict) DpoAnswerAdvisory(ctx context.Context, req DpoAnswerAdvisoryRequestObject) (DpoAnswerAdvisoryResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	a, err := h.svc.AnswerAdvisory(ctx, req.Id, req.Body.Answer, v)
	if err != nil {
		return nil, problem(err)
	}
	return DpoAnswerAdvisory200JSONResponse{Body: toAdvisoryWire(a), Headers: DpoAnswerAdvisory200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) DpoSearchKbArticles(ctx context.Context, req DpoSearchKbArticlesRequestObject) (DpoSearchKbArticlesResponseObject, error) {
	var category, q string
	if req.Params.Category != nil {
		category = *req.Params.Category
	}
	if req.Params.Q != nil {
		q = *req.Params.Q
	}
	f := dposervice.KbFilter{Category: category, Q: q}
	if req.Params.Language != nil {
		f.Language = string(*req.Params.Language)
	}
	if req.Params.Source != nil {
		f.Source = string(*req.Params.Source)
	}
	if req.Params.Tag != nil {
		f.Tag = *req.Params.Tag
	}
	if req.Params.IncludeAll != nil {
		f.IncludeAll = *req.Params.IncludeAll
	}
	list, err := h.svc.ListKbArticles(ctx, f)
	if err != nil {
		return nil, problem(err)
	}
	resp := DpoSearchKbArticles200JSONResponse{Data: make([]DpoKbArticle, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toKbArticleWire(a))
	}
	return resp, nil
}

func toAdvisoryWire(a dposervice.Advisory) DpoAdvisory {
	w := DpoAdvisory{Id: a.ID, AdvisoryNo: a.AdvisoryNo, OrgUnitId: a.OrgUnitID, RequesterUserId: a.RequesterUserID,
		RequesterName: ptr(a.RequesterName), Subject: a.Subject, Question: a.Question, Status: DpoAdvisoryStatus(a.Status),
		Answer: a.Answer, AnsweredBy: a.AnsweredBy, KbArticleId: a.KbArticleID, RowVersion: int(a.RowVersion), CreatedAt: a.CreatedAt}
	if a.AnsweredAt != nil {
		w.AnsweredAt = a.AnsweredAt
	}
	return w
}

func toKbArticleWire(a dposervice.KbArticle) DpoKbArticle {
	body := map[string]interface{}(a.Body)
	w := DpoKbArticle{Id: a.ID, Category: a.Category, Title: a.Title, Body: &body, Language: a.Language, Tags: a.Tags,
		Source: DpoKbArticleSource(a.Source), RowVersion: int(a.RowVersion)}
	st, vn, g := DpoKbArticleStatus(a.Status), int(a.VersionNo), a.IsGlobal
	w.Status, w.VersionNo, w.IsGlobal = &st, &vn, &g
	if a.PublishedAt != nil {
		w.PublishedAt = a.PublishedAt
	}
	return w
}
