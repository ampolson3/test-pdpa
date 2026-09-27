// Package dsarhttp holds the dsar module's admin endpoints (DSAR-13, the automatic response-letter composer,
// plus the minimal request intake/transition slice it depends on). Types in dsar.gen.go are generated from
// api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package dsarhttp

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/crypto"
)

type Strict struct {
	svc *dsarservice.Service
}

func NewStrict(svc *dsarservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) DsarListRequestTypes(ctx context.Context, req DsarListRequestTypesRequestObject) (DsarListRequestTypesResponseObject, error) {
	types, err := h.svc.ListRequestTypes(ctx)
	if err != nil {
		return nil, err
	}
	resp := DsarListRequestTypes200JSONResponse{Data: make([]DsarRequestType, 0, len(types))}
	for _, t := range types {
		resp.Data = append(resp.Data, toRequestTypeWire(t))
	}
	return resp, nil
}

func (h *Strict) DsarListRequests(ctx context.Context, req DsarListRequestsRequestObject) (DsarListRequestsResponseObject, error) {
	f := dsarservice.RequestFilter{}
	if req.Params.Status != nil {
		s := string(*req.Params.Status)
		f.Status = s
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		at, id, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &dsarservice.RequestCursor{ReceivedAt: at, ID: id}
	}
	list, next, err := h.svc.ListRequests(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := DsarListRequests200JSONResponse{Data: make([]DsarRequest, 0, len(list))}
	for _, r := range list {
		resp.Data = append(resp.Data, toRequestWire(r))
	}
	if next != nil {
		c := encodeCursor(next.ReceivedAt, next.ID)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) DsarCreateRequest(ctx context.Context, req DsarCreateRequestRequestObject) (DsarCreateRequestResponseObject, error) {
	b := *req.Body
	in := dsarservice.CreateRequestInput{
		RequestTypeID: b.RequestTypeId, LegalEntityID: b.LegalEntityId, Channel: string(b.Channel),
		RequesterName: b.RequesterName, RequesterContact: b.RequesterContact, ContactKind: crypto.IdentifierKind(b.ContactKind),
	}
	if b.OnBehalf != nil {
		in.OnBehalf = *b.OnBehalf
	}
	r, err := h.svc.CreateRequest(ctx, in)
	if err != nil {
		return nil, problem(err)
	}
	return DsarCreateRequest201JSONResponse{Body: toRequestWire(r), Headers: DsarCreateRequest201ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func (h *Strict) DsarGetRequest(ctx context.Context, req DsarGetRequestRequestObject) (DsarGetRequestResponseObject, error) {
	r, err := h.svc.GetRequest(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return DsarGetRequest200JSONResponse{Body: toRequestWire(r), Headers: DsarGetRequest200ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func (h *Strict) DsarTransitionRequest(ctx context.Context, req DsarTransitionRequestRequestObject) (DsarTransitionRequestResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := *req.Body
	in := dsarservice.TransitionInput{To: string(b.To)}
	if b.Outcome != nil {
		o := string(*b.Outcome)
		in.Outcome = &o
	}
	in.RejectionReasonCode = b.RejectionReasonCode
	r, docID, err := h.svc.Transition(ctx, req.Id, v, in)
	if err != nil {
		return nil, problem(err)
	}
	result := DsarTransitionResult{Request: toRequestWire(r), DocumentId: docID}
	return DsarTransitionRequest200JSONResponse{Body: result, Headers: DsarTransitionRequest200ResponseHeaders{ETag: etag(r.RowVersion)}}, nil
}

func toRequestTypeWire(t dsarservice.RequestType) DsarRequestType {
	w := DsarRequestType{Id: t.ID, Code: DsarRequestTypeCode(t.Code), NameTh: t.NameTh, SlaDays: int(t.SLADays)}
	if t.NameEn != "" {
		w.NameEn = &t.NameEn
	}
	if t.LegalRef != "" {
		w.LegalRef = &t.LegalRef
	}
	return w
}

func toRequestWire(r dsarservice.Request) DsarRequest {
	w := DsarRequest{Id: r.ID, RequestNo: r.RequestNo, RequestTypeId: r.RequestTypeID, LegalEntityId: r.LegalEntityID,
		Channel: DsarRequestChannel(r.Channel), OnBehalf: r.OnBehalf, Status: DsarRequestStatus(r.Status),
		ReceivedAt: r.ReceivedAt.UTC(), DueAt: r.DueAt.UTC(), RejectionReasonCode: r.RejectionReasonCode,
		RowVersion: int(r.RowVersion), UpdatedAt: r.UpdatedAt.UTC()}
	if r.Outcome != nil {
		o := DsarOutcome(*r.Outcome)
		w.Outcome = &o
	}
	if r.VerifiedAt != nil {
		t := r.VerifiedAt.UTC()
		w.VerifiedAt = &t
	}
	if r.ClosedAt != nil {
		t := r.ClosedAt.UTC()
		w.ClosedAt = &t
	}
	if r.AssigneeUserID != nil {
		w.AssigneeUserId = r.AssigneeUserID
	}
	return w
}

func etag(v int32) *string {
	s := `"` + strconv.Itoa(int(v)) + `"`
	return &s
}

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

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func problem(err error) error {
	switch {
	case errors.Is(err, dsarservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, dsarservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, dsarservice.ErrInvalidTransition):
		return httpx.InvalidTransition("dsar")
	case errors.Is(err, dsarservice.ErrInvalid):
		return httpx.UnprocessableEntity("dsar.invalid_input", err.Error())
	}
	return err
}
