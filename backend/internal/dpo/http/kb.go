// DPO-12: knowledge-base management handlers (internal/dpo/service/kb.go).
package dpohttp

import (
	"context"

	dposervice "pdpa-platform/internal/dpo/service"
	"pdpa-platform/internal/pkg/httpx"
)

func kbInput(b *DpoKbArticleInput) dposervice.KbInput {
	in := dposervice.KbInput{Category: b.Category, Title: b.Title, Body: b.Body, Source: string(b.Source)}
	if b.Language != nil {
		in.Language = string(*b.Language)
	}
	if b.Tags != nil {
		in.Tags = *b.Tags
	}
	return in
}

func (h *Strict) DpoCreateKbArticle(ctx context.Context, req DpoCreateKbArticleRequestObject) (DpoCreateKbArticleResponseObject, error) {
	a, err := h.svc.CreateKbArticle(ctx, kbInput(req.Body))
	if err != nil {
		return nil, problem(err)
	}
	return DpoCreateKbArticle201JSONResponse{Body: toKbArticleWire(a), Headers: DpoCreateKbArticle201ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) DpoGetKbArticle(ctx context.Context, req DpoGetKbArticleRequestObject) (DpoGetKbArticleResponseObject, error) {
	a, err := h.svc.GetKbArticle(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DpoGetKbArticle200JSONResponse{Body: toKbArticleWire(a), Headers: DpoGetKbArticle200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) DpoUpdateKbArticle(ctx context.Context, req DpoUpdateKbArticleRequestObject) (DpoUpdateKbArticleResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	a, err := h.svc.UpdateKbArticle(ctx, req.Id, kbInput(req.Body), v)
	if err != nil {
		return nil, problem(err)
	}
	return DpoUpdateKbArticle200JSONResponse{Body: toKbArticleWire(a), Headers: DpoUpdateKbArticle200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) DpoSetKbArticleStatus(ctx context.Context, req DpoSetKbArticleStatusRequestObject) (DpoSetKbArticleStatusResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	a, err := h.svc.SetKbArticleStatus(ctx, req.Id, string(req.Body.Status), v)
	if err != nil {
		return nil, problem(err)
	}
	return DpoSetKbArticleStatus200JSONResponse{Body: toKbArticleWire(a), Headers: DpoSetKbArticleStatus200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}
