package orghttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/httpx"
)

// ORG-01 legal entities and ORG-04 org units.

func (h *Strict) OrgListLegalEntities(ctx context.Context, _ OrgListLegalEntitiesRequestObject) (OrgListLegalEntitiesResponseObject, error) {
	list, err := h.svc.ListLegalEntities(ctx)
	if err != nil {
		return nil, err
	}
	resp := OrgListLegalEntities200JSONResponse{Data: make([]LegalEntity, 0, len(list))}
	for _, e := range list {
		resp.Data = append(resp.Data, legalEntityWire(e))
	}
	return resp, nil
}

func (h *Strict) OrgCreateLegalEntity(ctx context.Context, req OrgCreateLegalEntityRequestObject) (OrgCreateLegalEntityResponseObject, error) {
	e, err := h.svc.SaveLegalEntity(ctx, legalEntityInput(*req.Body), 0)
	if err != nil {
		return nil, structureProblem(err)
	}
	return OrgCreateLegalEntity201JSONResponse{Body: legalEntityWire(e), Headers: OrgCreateLegalEntity201ResponseHeaders{ETag: etag(e.RowVersion)}}, nil
}

func (h *Strict) OrgGetLegalEntity(ctx context.Context, req OrgGetLegalEntityRequestObject) (OrgGetLegalEntityResponseObject, error) {
	e, err := h.svc.GetLegalEntity(ctx, req.Id)
	if err != nil {
		return nil, structureProblem(err)
	}
	return OrgGetLegalEntity200JSONResponse{Body: legalEntityWire(e), Headers: OrgGetLegalEntity200ResponseHeaders{ETag: etag(e.RowVersion)}}, nil
}

func (h *Strict) OrgUpdateLegalEntity(ctx context.Context, req OrgUpdateLegalEntityRequestObject) (OrgUpdateLegalEntityResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := legalEntityInput(*req.Body)
	in.ID = req.Id
	e, err := h.svc.SaveLegalEntity(ctx, in, v)
	if err != nil {
		return nil, structureProblem(err)
	}
	return OrgUpdateLegalEntity200JSONResponse{Body: legalEntityWire(e), Headers: OrgUpdateLegalEntity200ResponseHeaders{ETag: etag(e.RowVersion)}}, nil
}

func (h *Strict) OrgListUnits(ctx context.Context, req OrgListUnitsRequestObject) (OrgListUnitsResponseObject, error) {
	closed := req.Params.IncludeClosed != nil && *req.Params.IncludeClosed
	list, err := h.svc.ListOrgUnits(ctx, req.Params.LegalEntityId, closed)
	if err != nil {
		return nil, err
	}
	resp := OrgListUnits200JSONResponse{Data: make([]OrgUnit, 0, len(list))}
	for _, u := range list {
		resp.Data = append(resp.Data, unitWire(u))
	}
	return resp, nil
}

func (h *Strict) OrgCreateUnit(ctx context.Context, req OrgCreateUnitRequestObject) (OrgCreateUnitResponseObject, error) {
	b := req.Body
	u, err := h.svc.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: b.LegalEntityId, ParentID: b.ParentId, Code: b.Code, NameTh: b.NameTh,
		NameEn: str(b.NameEn), UnitType: string(b.UnitType)})
	if err != nil {
		return nil, structureProblem(err)
	}
	return OrgCreateUnit201JSONResponse{Body: unitWire(u), Headers: OrgCreateUnit201ResponseHeaders{ETag: etag(u.RowVersion)}}, nil
}

func (h *Strict) OrgUpdateUnit(ctx context.Context, req OrgUpdateUnitRequestObject) (OrgUpdateUnitResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := req.Body
	u, err := h.svc.UpdateOrgUnit(ctx, req.Id, v, orgservice.OrgUnit{Code: b.Code, NameTh: b.NameTh, NameEn: str(b.NameEn), UnitType: string(b.UnitType)})
	if err != nil {
		return nil, structureProblem(err)
	}
	return OrgUpdateUnit200JSONResponse{Body: unitWire(u), Headers: OrgUpdateUnit200ResponseHeaders{ETag: etag(u.RowVersion)}}, nil
}

func (h *Strict) OrgMoveUnit(ctx context.Context, req OrgMoveUnitRequestObject) (OrgMoveUnitResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	u, err := h.svc.MoveOrgUnit(ctx, req.Id, v, req.Body.ParentId)
	if err != nil {
		return nil, structureProblem(err)
	}
	return OrgMoveUnit200JSONResponse{Body: unitWire(u), Headers: OrgMoveUnit200ResponseHeaders{ETag: etag(u.RowVersion)}}, nil
}

func (h *Strict) OrgCloseUnit(ctx context.Context, req OrgCloseUnitRequestObject) (OrgCloseUnitResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	u, err := h.svc.CloseOrgUnit(ctx, req.Id, v)
	if err != nil {
		return nil, structureProblem(err)
	}
	return OrgCloseUnit200JSONResponse{Body: unitWire(u), Headers: OrgCloseUnit200ResponseHeaders{ETag: etag(u.RowVersion)}}, nil
}

func legalEntityInput(b LegalEntityInput) orgservice.LegalEntity {
	e := orgservice.LegalEntity{ParentID: b.ParentId, NameTh: b.NameTh, NameEn: str(b.NameEn), RegistrationNo: str(b.RegistrationNo), TaxID: str(b.TaxId),
		ContactEmail: str(b.ContactEmail), ContactPhone: str(b.ContactPhone), LogoFileID: b.LogoFileId}
	if b.Address != nil {
		raw, _ := json.Marshal(b.Address)
		_ = json.Unmarshal(raw, &e.Address)
	}
	if b.IsController != nil {
		e.IsController = *b.IsController
	} else {
		e.IsController = true
	}
	if b.IsProcessor != nil {
		e.IsProcessor = *b.IsProcessor
	}
	if b.Status != nil {
		e.Status = string(*b.Status)
	}
	return e
}

func legalEntityWire(e orgservice.LegalEntity) LegalEntity {
	var a Address
	raw, _ := json.Marshal(e.Address)
	_ = json.Unmarshal(raw, &a)
	return LegalEntity{Id: e.ID, ParentId: e.ParentID, NameTh: e.NameTh, NameEn: ptr(e.NameEn), RegistrationNo: ptr(e.RegistrationNo), TaxId: ptr(e.TaxID),
		Address: a, ContactEmail: ptr(e.ContactEmail), ContactPhone: ptr(e.ContactPhone), LogoFileId: e.LogoFileID, IsController: e.IsController,
		IsProcessor: e.IsProcessor, Status: LegalEntityStatus(e.Status), RowVersion: int(e.RowVersion), UpdatedAt: e.UpdatedAt.UTC()}
}

func unitWire(u orgservice.OrgUnit) OrgUnit {
	w := OrgUnit{Id: u.ID, LegalEntityId: u.LegalEntityID, ParentId: u.ParentID, Depth: int(u.Depth), Code: u.Code, NameTh: u.NameTh, NameEn: ptr(u.NameEn),
		UnitType: OrgUnitType(u.UnitType), Status: OrgUnitStatus(u.Status), RowVersion: int(u.RowVersion), UpdatedAt: u.UpdatedAt.UTC()}
	if u.ClosedAt != nil {
		t := u.ClosedAt.UTC()
		w.ClosedAt = &t
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

func structureProblem(err error) error {
	switch {
	case errors.Is(err, orgservice.ErrCycle):
		return httpx.Problem{Status: http.StatusConflict, Code: "org.cycle", Title: "Cycle in the organization tree"}
	case errors.Is(err, orgservice.ErrHasActive):
		return httpx.Problem{Status: http.StatusConflict, Code: "org.has_active_units", Title: "Active units below"}
	case errors.Is(err, orgservice.ErrLogoNotUse):
		return httpx.UnprocessableEntity("org.logo_not_usable", err.Error())
	case errors.Is(err, orgservice.ErrInvalid):
		return httpx.UnprocessableEntity("org.invalid", err.Error())
	}
	return problem(err)
}
