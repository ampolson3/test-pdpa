package orghttp

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) OrgListExternalParties(ctx context.Context, req OrgListExternalPartiesRequestObject) (OrgListExternalPartiesResponseObject, error) {
	f := orgservice.PartyFilter{}
	if req.Params.PartyType != nil {
		f.PartyType = string(*req.Params.PartyType)
	}
	if req.Params.CountryCode != nil {
		f.CountryCode = *req.Params.CountryCode
	}
	if req.Params.Q != nil {
		f.Query = *req.Params.Q
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		c, err := decodePartyCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &c
	}
	list, next, err := h.svc.ListExternalParties(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := OrgListExternalParties200JSONResponse{Data: make([]ExternalParty, 0, len(list))}
	for _, p := range list {
		resp.Data = append(resp.Data, toPartyWire(p))
	}
	if next != nil {
		c := encodePartyCursor(*next)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) OrgCreateExternalParty(ctx context.Context, req OrgCreateExternalPartyRequestObject) (OrgCreateExternalPartyResponseObject, error) {
	p, err := h.svc.SaveExternalParty(ctx, toPartyInput(*req.Body), 0)
	if err != nil {
		return nil, partyProblem(err)
	}
	return OrgCreateExternalParty201JSONResponse{Body: toPartyWire(p), Headers: OrgCreateExternalParty201ResponseHeaders{ETag: etag(p.RowVersion)}}, nil
}

func (h *Strict) OrgGetExternalParty(ctx context.Context, req OrgGetExternalPartyRequestObject) (OrgGetExternalPartyResponseObject, error) {
	p, err := h.svc.GetExternalParty(ctx, req.Id)
	if err != nil {
		return nil, partyProblem(err)
	}
	return OrgGetExternalParty200JSONResponse{Body: toPartyWire(p), Headers: OrgGetExternalParty200ResponseHeaders{ETag: etag(p.RowVersion)}}, nil
}

func (h *Strict) OrgUpdateExternalParty(ctx context.Context, req OrgUpdateExternalPartyRequestObject) (OrgUpdateExternalPartyResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := toPartyInput(*req.Body)
	in.ID = req.Id
	p, err := h.svc.SaveExternalParty(ctx, in, v)
	if err != nil {
		return nil, partyProblem(err)
	}
	return OrgUpdateExternalParty200JSONResponse{Body: toPartyWire(p), Headers: OrgUpdateExternalParty200ResponseHeaders{ETag: etag(p.RowVersion)}}, nil
}

func (h *Strict) OrgListDuplicateExternalParties(ctx context.Context, _ OrgListDuplicateExternalPartiesRequestObject) (OrgListDuplicateExternalPartiesResponseObject, error) {
	groups, err := h.svc.DuplicateExternalParties(ctx)
	if err != nil {
		return nil, err
	}
	resp := OrgListDuplicateExternalParties200JSONResponse{Data: make([]ExternalPartyDuplicateGroup, 0, len(groups))}
	for key, parties := range groups {
		g := ExternalPartyDuplicateGroup{DedupeKey: key, Parties: make([]ExternalPartyDuplicate, 0, len(parties))}
		for _, p := range parties {
			g.Parties = append(g.Parties, ExternalPartyDuplicate{Id: p.ID, PartyType: ExternalPartyType(p.PartyType), NameTh: p.NameTh, NameEn: ptr(p.NameEn), CountryCode: p.CountryCode})
		}
		resp.Data = append(resp.Data, g)
	}
	return resp, nil
}

func (h *Strict) OrgMergeExternalParty(ctx context.Context, req OrgMergeExternalPartyRequestObject) (OrgMergeExternalPartyResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	if err := h.svc.MergeExternalParty(ctx, req.Id, req.Body.TargetId, v); err != nil {
		return nil, partyProblem(err)
	}
	p, err := h.svc.GetExternalParty(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return OrgMergeExternalParty200JSONResponse(toPartyWire(p)), nil
}

func toPartyInput(b ExternalPartyInput) orgservice.ExternalParty {
	p := orgservice.ExternalParty{PartyType: string(b.PartyType), NameTh: b.NameTh, NameEn: str(b.NameEn), RegistrationNo: str(b.RegistrationNo),
		CountryCode: b.CountryCode, Website: str(b.Website)}
	if b.Status != nil {
		p.Status = string(*b.Status)
	}
	if b.Contact != nil {
		p.Contact = orgservice.Contact{Name: str(b.Contact.Name), Phone: str(b.Contact.Phone)}
		if b.Contact.Email != nil {
			p.Contact.Email = string(*b.Contact.Email)
		}
	}
	return p
}

func toPartyWire(p orgservice.ExternalParty) ExternalParty {
	w := ExternalParty{Id: p.ID, PartyType: ExternalPartyType(p.PartyType), NameTh: p.NameTh, NameEn: ptr(p.NameEn),
		RegistrationNo: ptr(p.RegistrationNo), CountryCode: p.CountryCode, Website: ptr(p.Website), Status: ExternalPartyStatus(p.Status),
		RowVersion: int(p.RowVersion), UpdatedAt: p.UpdatedAt.UTC(), MergedIntoId: p.MergedIntoID}
	if p.Contact != (orgservice.Contact{}) {
		c := ExternalPartyContact{Name: ptr(p.Contact.Name), Phone: ptr(p.Contact.Phone)}
		if p.Contact.Email != "" {
			e := openapi_types.Email(p.Contact.Email)
			c.Email = &e
		}
		w.Contact = &c
	}
	return w
}

func encodePartyCursor(c orgservice.PartyCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.CreatedAt.Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

func decodePartyCursor(s string) (orgservice.PartyCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return orgservice.PartyCursor{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return orgservice.PartyCursor{}, errors.New("cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return orgservice.PartyCursor{}, err
	}
	u, err := uuid.Parse(id)
	return orgservice.PartyCursor{CreatedAt: t, ID: u}, err
}

func partyProblem(err error) error {
	switch {
	case errors.Is(err, orgservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, orgservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, orgservice.ErrPartyMerged):
		return httpx.Problem{Status: http.StatusConflict, Code: "org.party_merged", Title: "This party was already merged"}
	case errors.Is(err, orgservice.ErrInvalid):
		return httpx.UnprocessableEntity("org.invalid_party", err.Error())
	}
	return err
}
