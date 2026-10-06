// Package vendorhttp holds the vendor module's admin endpoints (VEN-01 so far). Types in vendor.gen.go
// are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package vendorhttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	vendorservice "pdpa-platform/internal/vendormgmt/service"

	"pdpa-platform/internal/pkg/httpx"
)

type Strict struct {
	svc *vendorservice.Service
}

func NewStrict(svc *vendorservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) VendorListVendors(ctx context.Context, req VendorListVendorsRequestObject) (VendorListVendorsResponseObject, error) {
	f := vendorservice.VendorFilter{}
	if req.Params.Status != nil {
		f.Status = string(*req.Params.Status)
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		c, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &c
	}
	list, next, err := h.svc.ListVendors(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := VendorListVendors200JSONResponse{Data: make([]Vendor, 0, len(list))}
	for _, v := range list {
		resp.Data = append(resp.Data, toVendorWire(v))
	}
	if next != nil {
		c := encodeCursor(*next)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) VendorCreateVendor(ctx context.Context, req VendorCreateVendorRequestObject) (VendorCreateVendorResponseObject, error) {
	v, err := h.svc.SaveVendor(ctx, toVendorInput(*req.Body), 0)
	if err != nil {
		return nil, problem(err)
	}
	return VendorCreateVendor201JSONResponse{Body: toVendorWire(v), Headers: VendorCreateVendor201ResponseHeaders{ETag: etag(v.RowVersion)}}, nil
}

func (h *Strict) VendorGetVendor(ctx context.Context, req VendorGetVendorRequestObject) (VendorGetVendorResponseObject, error) {
	v, err := h.svc.GetVendor(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return VendorGetVendor200JSONResponse{Body: toVendorWire(v), Headers: VendorGetVendor200ResponseHeaders{ETag: etag(v.RowVersion)}}, nil
}

func (h *Strict) VendorUpdateVendor(ctx context.Context, req VendorUpdateVendorRequestObject) (VendorUpdateVendorResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := toVendorInput(*req.Body)
	in.ID = req.Id
	v, err := h.svc.SaveVendor(ctx, in, rv)
	if err != nil {
		return nil, problem(err)
	}
	return VendorUpdateVendor200JSONResponse{Body: toVendorWire(v), Headers: VendorUpdateVendor200ResponseHeaders{ETag: etag(v.RowVersion)}}, nil
}

func toVendorInput(b VendorInput) vendorservice.Vendor {
	v := vendorservice.Vendor{PartyID: b.PartyId, ServiceDescription: b.ServiceDescription, RelationshipOwnerID: b.RelationshipOwnerId,
		IsProcessor: true}
	if b.IsProcessor != nil {
		v.IsProcessor = *b.IsProcessor
	}
	if b.DataAccess != nil {
		m, _ := json.Marshal(b.DataAccess)
		v.DataAccess = m
	}
	if b.ProcessingCountries != nil {
		v.ProcessingCountries = *b.ProcessingCountries
	}
	return v
}

func toVendorWire(v vendorservice.Vendor) Vendor {
	w := Vendor{Id: v.ID, PartyId: v.PartyID, ServiceDescription: v.ServiceDescription, RelationshipOwnerId: v.RelationshipOwnerID,
		IsProcessor: v.IsProcessor, ProcessingCountries: v.ProcessingCountries, Status: VendorStatus(v.Status),
		RowVersion: int(v.RowVersion), UpdatedAt: v.UpdatedAt.UTC()}
	if v.Tier != nil {
		t := VendorTier(*v.Tier)
		w.Tier = &t
	}
	if len(v.DataAccess) > 0 {
		_ = json.Unmarshal(v.DataAccess, &w.DataAccess)
	}
	if v.NextAssessmentAt != nil {
		w.NextAssessmentAt = &openapi_types.Date{Time: *v.NextAssessmentAt}
	}
	if v.ApprovedAt != nil {
		t := v.ApprovedAt.UTC()
		w.ApprovedAt = &t
	}
	if v.OffboardedAt != nil {
		t := v.OffboardedAt.UTC()
		w.OffboardedAt = &t
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

func encodeCursor(c vendorservice.VendorCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.CreatedAt.Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

func decodeCursor(s string) (vendorservice.VendorCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return vendorservice.VendorCursor{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return vendorservice.VendorCursor{}, errors.New("cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return vendorservice.VendorCursor{}, err
	}
	u, err := uuid.Parse(id)
	return vendorservice.VendorCursor{CreatedAt: t, ID: u}, err
}

func problem(err error) error {
	switch {
	case errors.Is(err, vendorservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, vendorservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, vendorservice.ErrInvalid):
		return httpx.UnprocessableEntity("vendor.invalid_input", err.Error())
	}
	return err
}
