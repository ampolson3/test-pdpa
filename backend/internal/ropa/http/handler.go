// Package ropahttp holds the ropa module's admin endpoints (ROPA-02 asset register so far). Types in
// ropa.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package ropahttp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	ropaservice "pdpa-platform/internal/ropa/service"

	"pdpa-platform/internal/pkg/httpx"
)

type Strict struct {
	svc *ropaservice.Service
}

func NewStrict(svc *ropaservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) RopaListAssets(ctx context.Context, req RopaListAssetsRequestObject) (RopaListAssetsResponseObject, error) {
	f := ropaservice.AssetFilter{}
	if req.Params.AssetType != nil {
		f.AssetType = string(*req.Params.AssetType)
	}
	if req.Params.OrgUnitId != nil {
		f.OrgUnitID = req.Params.OrgUnitId
	}
	if req.Params.Status != nil {
		f.Status = string(*req.Params.Status)
	}
	if req.Params.Q != nil {
		f.Query = *req.Params.Q
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		c, err := decodeAssetCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &c
	}
	list, next, err := h.svc.ListAssets(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := RopaListAssets200JSONResponse{Data: make([]Asset, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toAssetWire(a))
	}
	if next != nil {
		c := encodeAssetCursor(*next)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) RopaCreateAsset(ctx context.Context, req RopaCreateAssetRequestObject) (RopaCreateAssetResponseObject, error) {
	a, err := h.svc.SaveAsset(ctx, toAssetInput(*req.Body), 0)
	if err != nil {
		return nil, problem(err)
	}
	return RopaCreateAsset201JSONResponse{Body: toAssetWire(a), Headers: RopaCreateAsset201ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaGetAsset(ctx context.Context, req RopaGetAssetRequestObject) (RopaGetAssetResponseObject, error) {
	a, err := h.svc.GetAsset(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return RopaGetAsset200JSONResponse{Body: toAssetWire(a), Headers: RopaGetAsset200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaUpdateAsset(ctx context.Context, req RopaUpdateAssetRequestObject) (RopaUpdateAssetResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := toAssetInput(*req.Body)
	in.ID = req.Id
	a, err := h.svc.SaveAsset(ctx, in, v)
	if err != nil {
		return nil, problem(err)
	}
	return RopaUpdateAsset200JSONResponse{Body: toAssetWire(a), Headers: RopaUpdateAsset200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func toAssetInput(b AssetInput) ropaservice.Asset {
	a := ropaservice.Asset{Name: b.Name, AssetType: string(b.AssetType), OrgUnitID: b.OrgUnitId, OwnerUserID: b.OwnerUserId,
		ProviderPartyID: b.ProviderPartyId, HostingCountryCode: str(b.HostingCountryCode)}
	if b.HostingType != nil {
		a.HostingType = string(*b.HostingType)
	}
	if b.Classification != nil {
		a.Classification = string(*b.Classification)
	}
	if b.Status != nil {
		a.Status = string(*b.Status)
	}
	return a
}

func toAssetWire(a ropaservice.Asset) Asset {
	w := Asset{Id: a.ID, Name: a.Name, AssetType: AssetType(a.AssetType), OrgUnitId: a.OrgUnitID, OwnerUserId: a.OwnerUserID,
		ProviderPartyId: a.ProviderPartyID, HostingCountryCode: ptr(a.HostingCountryCode), Status: AssetStatus(a.Status),
		RowVersion: int(a.RowVersion), UpdatedAt: a.UpdatedAt.UTC()}
	if a.HostingType != "" {
		t := AssetHostingType(a.HostingType)
		w.HostingType = &t
	}
	if a.Classification != "" {
		c := AssetClassification(a.Classification)
		w.Classification = &c
	}
	return w
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func etag(v int32) *string {
	s := fmt.Sprintf("%q", strconv.Itoa(int(v)))
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

// encodeCursor/decodeCursor are shared by every (created_at, id) keyset-paginated list in this module.
func encodeCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(s string) (time.Time, uuid.UUID, error) {
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

func encodeAssetCursor(c ropaservice.AssetCursor) string { return encodeCursor(c.CreatedAt, c.ID) }

func decodeAssetCursor(s string) (ropaservice.AssetCursor, error) {
	t, u, err := decodeCursor(s)
	return ropaservice.AssetCursor{CreatedAt: t, ID: u}, err
}

func problem(err error) error {
	switch {
	case errors.Is(err, ropaservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, ropaservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, ropaservice.ErrInvalid):
		return httpx.UnprocessableEntity("ropa.invalid_input", err.Error())
	}
	return err
}
