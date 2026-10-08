// Package agreementhttp holds the agreement module's admin endpoints (DPA-02 so far). Types in
// agreement.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package agreementhttp

import (
	"context"
	"encoding/base64"
	"errors"
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

func toCreateInput(b AgreementCreateInput) agreementservice.CreateInput {
	in := agreementservice.CreateInput{
		AgreementType: string(b.AgreementType),
		OurRole:       string(b.OurRole),
		VendorID:      b.VendorId,
		LegalEntityID: b.LegalEntityId,
		TemplateID:    b.TemplateId,
		Title:         b.Title,
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
	}
	return err
}
