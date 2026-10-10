// Package noticepublichttp holds the /public/v1 notice endpoints (PNG-06): a published notice's current
// version and its version history, with no login — the tenant comes from the public key
// (publickeys.Middleware), the same mechanism consent's own /public/v1 collection-point form already uses.
// Types in public.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package noticepublichttp

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	noticeservice "pdpa-platform/internal/notice/service"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/publickeys"
)

func dateOf(t time.Time) openapi_types.Date { return openapi_types.Date{Time: t} }

type Strict struct {
	svc *noticeservice.Service
}

func NewStrict(svc *noticeservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

// notice is the key of this request, which must stand for a notice (PNG-06).
func notice(ctx context.Context) (publickeys.Key, error) {
	k, ok := publickeys.FromContext(ctx)
	if !ok || k.EntityType != publickeys.EntityNotice {
		return publickeys.Key{}, httpx.NotFound()
	}
	return k, nil
}

func problem(err error) error {
	if errors.Is(err, noticeservice.ErrNotFound) {
		return httpx.NotFound()
	}
	return err
}

func (h *Strict) NoticeGetPublicNotice(ctx context.Context, _ NoticeGetPublicNoticeRequestObject) (NoticeGetPublicNoticeResponseObject, error) {
	k, err := notice(ctx)
	if err != nil {
		return nil, err
	}
	n, err := h.svc.PublicNotice(ctx, k.EntityID)
	if err != nil {
		return nil, problem(err)
	}
	out, err := toPublicNotice(ctx, h.svc, n, nil)
	if err != nil {
		return nil, problem(err)
	}
	return NoticeGetPublicNotice200JSONResponse(out), nil
}

func (h *Strict) NoticeListPublicVersions(ctx context.Context, _ NoticeListPublicVersionsRequestObject) (NoticeListPublicVersionsResponseObject, error) {
	k, err := notice(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.svc.PublicNotice(ctx, k.EntityID); err != nil {
		return nil, problem(err)
	}
	versions, err := h.svc.PublicVersions(ctx, k.EntityID)
	if err != nil {
		return nil, problem(err)
	}
	resp := NoticeListPublicVersions200JSONResponse{Data: make([]PublicNoticeVersion, 0, len(versions))}
	for _, v := range versions {
		resp.Data = append(resp.Data, PublicNoticeVersion{VersionNo: int(v.VersionNo), EffectiveFrom: dateOf(v.EffectiveFrom), PublishedAt: v.PublishedAt.UTC()})
	}
	return resp, nil
}

func (h *Strict) NoticeGetPublicVersion(ctx context.Context, req NoticeGetPublicVersionRequestObject) (NoticeGetPublicVersionResponseObject, error) {
	k, err := notice(ctx)
	if err != nil {
		return nil, err
	}
	n, err := h.svc.PublicNotice(ctx, k.EntityID)
	if err != nil {
		return nil, problem(err)
	}
	no := int32(req.VersionNo)
	out, err := toPublicNotice(ctx, h.svc, n, &no)
	if err != nil {
		return nil, problem(err)
	}
	return NoticeGetPublicVersion200JSONResponse(out), nil
}

// toPublicNotice resolves one version (nil: the current one) and renders every language it carries.
func toPublicNotice(ctx context.Context, svc *noticeservice.Service, n noticeservice.Notice, versionNo *int32) (PublicNotice, error) {
	versions, err := svc.PublicVersions(ctx, n.ID)
	if err != nil {
		return PublicNotice{}, err
	}
	if len(versions) == 0 {
		return PublicNotice{}, noticeservice.ErrNotFound
	}
	var v noticeservice.NoticeVersion
	found := false
	if versionNo == nil {
		v, found = versions[0], true // ListNoticeVersions orders version_no DESC — [0] is the current one
	} else {
		for _, x := range versions {
			if x.VersionNo == *versionNo {
				v, found = x, true
				break
			}
		}
	}
	if !found {
		return PublicNotice{}, noticeservice.ErrNotFound
	}
	out := PublicNotice{Title: n.Title, NoticeType: NoticeType(n.NoticeType), VersionNo: int(v.VersionNo),
		EffectiveFrom: dateOf(v.EffectiveFrom), Languages: make([]PublicNoticeLanguages, 0, len(v.Languages)), Content: map[string]string{}}
	for _, lang := range v.Languages {
		out.Languages = append(out.Languages, PublicNoticeLanguages(lang))
		html, err := svc.PublicVersionHTML(ctx, n, &v.VersionNo, lang)
		if err != nil {
			return PublicNotice{}, err
		}
		out.Content[lang] = string(html)
	}
	return out, nil
}
