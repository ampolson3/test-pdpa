package ropahttp

import (
	"context"

	ropaservice "pdpa-platform/internal/ropa/service"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) RopaListDataInventory(ctx context.Context, req RopaListDataInventoryRequestObject) (RopaListDataInventoryResponseObject, error) {
	f := ropaservice.InventoryFilter{}
	if req.Params.AssetId != nil {
		f.AssetID = req.Params.AssetId
	}
	if req.Params.OrgUnitId != nil {
		f.OrgUnitID = req.Params.OrgUnitId
	}
	if req.Params.DataCategoryId != nil {
		f.DataCategoryID = req.Params.DataCategoryId
	}
	if req.Params.SensitiveOnly != nil {
		f.SensitiveOnly = *req.Params.SensitiveOnly
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		at, id, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &ropaservice.InventoryCursor{CreatedAt: at, ID: id}
	}
	list, next, err := h.svc.ListDataInventory(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := RopaListDataInventory200JSONResponse{Data: make([]DataInventoryItem, 0, len(list))}
	for _, it := range list {
		resp.Data = append(resp.Data, toInventoryWire(it))
	}
	if next != nil {
		c := encodeCursor(next.CreatedAt, next.ID)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) RopaCreateDataInventoryItem(ctx context.Context, req RopaCreateDataInventoryItemRequestObject) (RopaCreateDataInventoryItemResponseObject, error) {
	it, err := h.svc.SaveDataInventoryItem(ctx, toInventoryInput(*req.Body), 0)
	if err != nil {
		return nil, problem(err)
	}
	return RopaCreateDataInventoryItem201JSONResponse{Body: toInventoryWire(it), Headers: RopaCreateDataInventoryItem201ResponseHeaders{ETag: etag(it.RowVersion)}}, nil
}

func (h *Strict) RopaGetDataInventoryItem(ctx context.Context, req RopaGetDataInventoryItemRequestObject) (RopaGetDataInventoryItemResponseObject, error) {
	it, err := h.svc.GetDataInventoryItem(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return RopaGetDataInventoryItem200JSONResponse{Body: toInventoryWire(it), Headers: RopaGetDataInventoryItem200ResponseHeaders{ETag: etag(it.RowVersion)}}, nil
}

func (h *Strict) RopaUpdateDataInventoryItem(ctx context.Context, req RopaUpdateDataInventoryItemRequestObject) (RopaUpdateDataInventoryItemResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := toInventoryInput(*req.Body)
	in.ID = req.Id
	it, err := h.svc.SaveDataInventoryItem(ctx, in, v)
	if err != nil {
		return nil, problem(err)
	}
	return RopaUpdateDataInventoryItem200JSONResponse{Body: toInventoryWire(it), Headers: RopaUpdateDataInventoryItem200ResponseHeaders{ETag: etag(it.RowVersion)}}, nil
}

func toInventoryInput(b DataInventoryItemInput) ropaservice.DataInventoryItem {
	it := ropaservice.DataInventoryItem{AssetID: b.AssetId, DataCategoryID: b.DataCategoryId, OrgUnitID: b.OrgUnitId,
		OwnerUserID: b.OwnerUserId, LocationDetail: str(b.LocationDetail)}
	if b.Source != nil {
		it.Source = string(*b.Source)
	}
	return it
}

func toInventoryWire(it ropaservice.DataInventoryItem) DataInventoryItem {
	w := DataInventoryItem{Id: it.ID, AssetId: it.AssetID, DataCategoryId: it.DataCategoryID, OrgUnitId: it.OrgUnitID,
		OwnerUserId: it.OwnerUserID, LocationDetail: ptr(it.LocationDetail), IsSensitive: it.IsSensitive,
		SensitiveType: ptr(it.SensitiveType), CategoryNameTh: it.CategoryNameTh, CategoryNameEn: ptr(it.CategoryNameEn),
		RowVersion: int(it.RowVersion), UpdatedAt: it.UpdatedAt.UTC()}
	if it.Source != "" {
		s := DataInventorySource(it.Source)
		w.Source = &s
	}
	return w
}
