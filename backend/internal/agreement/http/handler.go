// Package agreementhttp holds the agreement module's admin endpoints (DPA-02 so far). Types in
// agreement.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package agreementhttp

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	agreementservice "pdpa-platform/internal/agreement/service"

	"pdpa-platform/internal/pkg/httpx"
)

type Strict struct {
	svc *agreementservice.Service
}

func NewStrict(svc *agreementservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) AgreementListAgreements(ctx context.Context, req AgreementListAgreementsRequestObject) (AgreementListAgreementsResponseObject, error) {
	f := agreementservice.AgreementFilter{}
	if req.Params.AgreementType != nil {
		f.AgreementType = string(*req.Params.AgreementType)
	}
	if req.Params.VendorId != nil {
		f.VendorID = req.Params.VendorId
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
	list, next, err := h.svc.ListAgreements(ctx, f)
	if err != nil {
		return nil, problem(err)
	}
	resp := AgreementListAgreements200JSONResponse{Data: make([]Agreement, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toAgreementWire(a))
	}
	if next != nil {
		c := encodeCursor(*next)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) AgreementCreateAgreement(ctx context.Context, req AgreementCreateAgreementRequestObject) (AgreementCreateAgreementResponseObject, error) {
	in := toCreateInput(*req.Body)
	a, err := h.svc.CreateWizard(ctx, in)
	if err != nil {
		return nil, problem(err)
	}
	return AgreementCreateAgreement201JSONResponse(toAgreementWire(a)), nil
}

func (h *Strict) AgreementGetAgreement(ctx context.Context, req AgreementGetAgreementRequestObject) (AgreementGetAgreementResponseObject, error) {
	a, err := h.svc.GetAgreement(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return AgreementGetAgreement200JSONResponse(toAgreementWire(a)), nil
}

func (h *Strict) AgreementListClauses(ctx context.Context, req AgreementListClausesRequestObject) (AgreementListClausesResponseObject, error) {
	list, err := h.svc.ListClauses(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := AgreementListClauses200JSONResponse{Data: make([]AgreementClause, 0, len(list))}
	for _, c := range list {
		resp.Data = append(resp.Data, toClauseWire(c))
	}
	return resp, nil
}

func (h *Strict) AgreementAddClause(ctx context.Context, req AgreementAddClauseRequestObject) (AgreementAddClauseResponseObject, error) {
	in := agreementservice.AddClauseInput{ClauseID: req.Body.ClauseId}
	if req.Body.Position != nil {
		in.Position = int16(*req.Body.Position)
	}
	c, err := h.svc.AddClause(ctx, req.Id, in)
	if err != nil {
		return nil, problem(err)
	}
	return AgreementAddClause201JSONResponse(toClauseWire(c)), nil
}

func (h *Strict) AgreementRemoveClause(ctx context.Context, req AgreementRemoveClauseRequestObject) (AgreementRemoveClauseResponseObject, error) {
	if err := h.svc.RemoveClause(ctx, req.Id, req.ClauseRowId); err != nil {
		return nil, problem(err)
	}
	return AgreementRemoveClause204Response{}, nil
}

func (h *Strict) AgreementMissingClauses(ctx context.Context, req AgreementMissingClausesRequestObject) (AgreementMissingClausesResponseObject, error) {
	list, err := h.svc.MissingMandatoryClauses(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := AgreementMissingClauses200JSONResponse{Data: make([]AgreementMissingClause, 0, len(list))}
	for _, m := range list {
		resp.Data = append(resp.Data, AgreementMissingClause{ClauseCode: m.Code, LegalRef: m.LegalRef})
	}
	return resp, nil
}

func (h *Strict) AgreementVendorContractStatus(ctx context.Context, req AgreementVendorContractStatusRequestObject) (AgreementVendorContractStatusResponseObject, error) {
	st, err := h.svc.VendorContractStatus(ctx, req.Params.VendorId)
	if err != nil {
		return nil, problem(err)
	}
	return AgreementVendorContractStatus200JSONResponse{VendorId: st.VendorID, IsProcessor: st.IsProcessor, HasDpa: st.HasDPA}, nil
}

func toPartyWire(p agreementservice.Party) AgreementParty {
	return AgreementParty{
		Id: p.ID, AgreementId: p.AgreementID, PartyId: p.PartyID, LegalEntityId: p.LegalEntityID,
		PartyRole: AgreementPartyPartyRole(p.PartyRole), SignatoryName: p.SignatoryName, SignatoryEmail: p.SignatoryEmail,
		RowVersion: int(p.RowVersion), CreatedAt: p.CreatedAt.UTC(),
	}
}

func (h *Strict) AgreementListParties(ctx context.Context, req AgreementListPartiesRequestObject) (AgreementListPartiesResponseObject, error) {
	list, err := h.svc.ListParties(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := AgreementListParties200JSONResponse{Data: make([]AgreementParty, 0, len(list))}
	for _, p := range list {
		resp.Data = append(resp.Data, toPartyWire(p))
	}
	return resp, nil
}

func (h *Strict) AgreementAddParty(ctx context.Context, req AgreementAddPartyRequestObject) (AgreementAddPartyResponseObject, error) {
	in := agreementservice.AddPartyInput{
		PartyID: req.Body.PartyId, LegalEntityID: req.Body.LegalEntityId, PartyRole: string(req.Body.PartyRole),
		SignatoryName: req.Body.SignatoryName,
	}
	if req.Body.SignatoryEmail != nil {
		e := string(*req.Body.SignatoryEmail)
		in.SignatoryEmail = &e
	}
	p, err := h.svc.AddParty(ctx, req.Id, in)
	if err != nil {
		return nil, problem(err)
	}
	return AgreementAddParty201JSONResponse(toPartyWire(p)), nil
}

func (h *Strict) AgreementRemoveParty(ctx context.Context, req AgreementRemovePartyRequestObject) (AgreementRemovePartyResponseObject, error) {
	if err := h.svc.RemoveParty(ctx, req.Id, req.PartyRowId); err != nil {
		return nil, problem(err)
	}
	return AgreementRemoveParty204Response{}, nil
}

func (h *Strict) AgreementTypeCheck(ctx context.Context, req AgreementTypeCheckRequestObject) (AgreementTypeCheckResponseObject, error) {
	rec, err := agreementservice.RecommendAgreementType(string(req.Params.CounterpartyRole))
	if err != nil {
		return nil, problem(err)
	}
	return AgreementTypeCheck200JSONResponse{
		AgreementType: AgreementTypeRecommendationAgreementType(rec.AgreementType),
		LegalRef:      rec.LegalRef,
		ReasonCode:    rec.ReasonCode,
	}, nil
}

func (h *Strict) AgreementSetSchedule(ctx context.Context, req AgreementSetScheduleRequestObject) (AgreementSetScheduleResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := agreementservice.ScheduleInput{RenewalNoticeDays: req.Body.RenewalNoticeDays}
	if req.Body.AutoRenew != nil {
		in.AutoRenew = *req.Body.AutoRenew
	}
	if req.Body.EffectiveFrom != nil {
		t := req.Body.EffectiveFrom.Time
		in.EffectiveFrom = &t
	}
	if req.Body.EffectiveTo != nil {
		t := req.Body.EffectiveTo.Time
		in.EffectiveTo = &t
	}
	a, err := h.svc.SetSchedule(ctx, req.Id, rv, in)
	if err != nil {
		return nil, problem(err)
	}
	return AgreementSetSchedule200JSONResponse{Body: toAgreementWire(a), Headers: AgreementSetSchedule200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
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

func toClauseWire(c agreementservice.Clause) AgreementClause {
	w := AgreementClause{
		Id: c.ID, AgreementId: c.AgreementID, ClauseId: c.ClauseID, ClauseCode: c.ClauseCode, ClauseTitle: c.ClauseTitle,
		LegalRef: c.LegalRef, IsMandatory: c.IsMandatory, Position: int(c.Position), RowVersion: int(c.RowVersion),
		CreatedAt: c.CreatedAt.UTC(),
	}
	if c.ClauseVersionNo != 0 {
		n := int(c.ClauseVersionNo)
		w.ClauseVersionNo = &n
	}
	return w
}

func toCreateInput(b AgreementCreateInput) agreementservice.CreateInput {
	in := agreementservice.CreateInput{
		AgreementType: string(b.AgreementType),
		OurRole:       string(b.OurRole),
		LegalEntityID: b.LegalEntityId,
		TemplateID:    b.TemplateId,
		Title:         b.Title,
	}
	if b.VendorId != nil {
		in.VendorID = *b.VendorId
	}
	if b.CounterpartyPartyId != nil {
		in.CounterpartyPartyID = *b.CounterpartyPartyId
	}
	if b.CounterpartyRole != nil {
		in.CounterpartyRole = string(*b.CounterpartyRole)
	}
	if b.ActivityIds != nil {
		in.ActivityIDs = *b.ActivityIds
	}
	if b.AutoRenew != nil {
		in.AutoRenew = *b.AutoRenew
	}
	if b.RenewalNoticeDays != nil {
		in.RenewalNoticeDays = *b.RenewalNoticeDays
	}
	if b.EffectiveFrom != nil {
		t := b.EffectiveFrom.Time
		in.EffectiveFrom = &t
	}
	return in
}

func toAgreementWire(a agreementservice.Agreement) Agreement {
	w := Agreement{
		Id: a.ID, AgreementType: AgreementType(a.AgreementType), AgreementNo: a.AgreementNo, Title: a.Title,
		OurRole: AgreementOurRole(a.OurRole), CounterpartyId: a.CounterpartyID, DocumentId: a.DocumentID,
		Status: AgreementStatus(a.Status), AutoRenew: a.AutoRenew, RenewalNoticeDays: a.RenewalNoticeDays,
		RowVersion: int(a.RowVersion), CreatedAt: a.CreatedAt.UTC(), ActivityIds: a.ActivityIDs,
	}
	if w.ActivityIds == nil {
		w.ActivityIds = []uuid.UUID{}
	}
	if a.VendorID != nil {
		w.VendorId = a.VendorID
	}
	if a.TemplateID != nil {
		w.TemplateId = a.TemplateID
	}
	if a.EffectiveFrom != nil {
		d := openapi_types.Date{Time: *a.EffectiveFrom}
		w.EffectiveFrom = &d
	}
	if a.EffectiveTo != nil {
		d := openapi_types.Date{Time: *a.EffectiveTo}
		w.EffectiveTo = &d
	}
	return w
}

func encodeCursor(c agreementservice.AgreementCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.CreatedAt.Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

func decodeCursor(s string) (agreementservice.AgreementCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return agreementservice.AgreementCursor{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return agreementservice.AgreementCursor{}, errors.New("cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return agreementservice.AgreementCursor{}, err
	}
	u, err := uuid.Parse(id)
	return agreementservice.AgreementCursor{CreatedAt: t, ID: u}, err
}

func problem(err error) error {
	switch {
	case errors.Is(err, agreementservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, agreementservice.ErrInvalid):
		return httpx.UnprocessableEntity("agreement.invalid_input", err.Error())
	case errors.Is(err, agreementservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	}
	return err
}
