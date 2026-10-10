package orghttp

import (
	"context"
	"errors"
	"net/http"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/httpx"
)

// ORG-07 master data.

func (h *Strict) OrgListMasterData(ctx context.Context, req OrgListMasterDataRequestObject) (OrgListMasterDataResponseObject, error) {
	list, err := h.svc.ListMaster(ctx, string(req.Kind))
	if err != nil {
		return nil, masterProblem(err)
	}
	resp := OrgListMasterData200JSONResponse{Data: make([]MasterDataItem, 0, len(list))}
	for _, it := range list {
		resp.Data = append(resp.Data, masterWire(it))
	}
	return resp, nil
}

func (h *Strict) OrgCreateMasterData(ctx context.Context, req OrgCreateMasterDataRequestObject) (OrgCreateMasterDataResponseObject, error) {
	it, err := h.svc.CreateMaster(ctx, string(req.Kind), masterInput(*req.Body))
	if err != nil {
		return nil, masterProblem(err)
	}
	return OrgCreateMasterData201JSONResponse{Body: masterWire(it), Headers: OrgCreateMasterData201ResponseHeaders{ETag: etag(it.RowVersion)}}, nil
}

func (h *Strict) OrgUpdateMasterData(ctx context.Context, req OrgUpdateMasterDataRequestObject) (OrgUpdateMasterDataResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	it, err := h.svc.UpdateMaster(ctx, string(req.Kind), req.Id, v, masterInput(*req.Body))
	if err != nil {
		return nil, masterProblem(err)
	}
	return OrgUpdateMasterData200JSONResponse{Body: masterWire(it), Headers: OrgUpdateMasterData200ResponseHeaders{ETag: etag(it.RowVersion)}}, nil
}

func (h *Strict) OrgDeleteMasterData(ctx context.Context, req OrgDeleteMasterDataRequestObject) (OrgDeleteMasterDataResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	if err := h.svc.DeleteMaster(ctx, string(req.Kind), req.Id, v); err != nil {
		return nil, masterProblem(err)
	}
	return OrgDeleteMasterData204Response{}, nil
}

func masterInput(b MasterDataInput) orgservice.MasterItem {
	it := orgservice.MasterItem{Code: b.Code, NameTh: b.NameTh, NameEn: str(b.NameEn), SensitiveType: str(b.SensitiveType), ParentID: b.ParentId, Category: str(b.Category)}
	it.IsSensitive = b.IsSensitive != nil && *b.IsSensitive
	it.IsVulnerable = b.IsVulnerable != nil && *b.IsVulnerable
	return it
}

func masterWire(it orgservice.MasterItem) MasterDataItem {
	w := MasterDataItem{Id: it.ID, Code: it.Code, NameTh: it.NameTh, NameEn: ptr(it.NameEn), Global: it.Global, ParentId: it.ParentID,
		SensitiveType: ptr(it.SensitiveType), Category: ptr(it.Category), SectionRef: ptr(it.SectionRef), Region: ptr(it.Region)}
	if it.ID != nil {
		v := int(it.RowVersion)
		w.RowVersion = &v
	}
	b := func(v bool) *bool { return &v }
	switch {
	case it.SectionRef != "":
		w.ForSensitive, w.RequiresConsent, w.RequiresLia = b(it.ForSensitive), b(it.RequiresConsent), b(it.RequiresLIA)
	case it.AdequacyStatus != "":
		s := MasterDataItemAdequacyStatus(it.AdequacyStatus)
		w.AdequacyStatus = &s
	default:
		w.IsSensitive, w.IsVulnerable = b(it.IsSensitive), b(it.IsVulnerable)
	}
	return w
}

func masterProblem(err error) error {
	switch {
	case errors.Is(err, orgservice.ErrReadOnly):
		return httpx.Problem{Status: http.StatusConflict, Code: "org.master_read_only", Title: "Read-only master data"}
	case errors.Is(err, orgservice.ErrInUse):
		return httpx.Problem{Status: http.StatusConflict, Code: "org.master_in_use", Title: "Entry in use"}
	}
	return structureProblem(err)
}
