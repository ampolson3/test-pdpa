// Package dsarpublichttp holds the /public/v1 DSAR intake-form endpoints (DSAR-01): a data subject
// submitting a request directly, with no login — the tenant comes from the public key
// (publickeys.Middleware), the same mechanism consent's own /public/v1 collection-point form and notice's
// own /public/v1 notice pages already use. Types in public.gen.go are generated from
// api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package dsarpublichttp

import (
	"context"
	"errors"

	dsarservice "pdpa-platform/internal/dsar/service"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/publickeys"
)

type Strict struct {
	svc *dsarservice.Service
}

func NewStrict(svc *dsarservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

// intakeForm is the key of this request, which must stand for a dsar intake form.
func intakeForm(ctx context.Context) (publickeys.Key, error) {
	k, ok := publickeys.FromContext(ctx)
	if !ok || k.EntityType != publickeys.EntityDsarForm {
		return publickeys.Key{}, httpx.NotFound()
	}
	return k, nil
}

func problem(err error) error {
	switch {
	case errors.Is(err, dsarservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, dsarservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, dsarservice.ErrInvalidTransition), errors.Is(err, dsarservice.ErrVerificationNotPending):
		return httpx.InvalidTransition("dsar")
	case errors.Is(err, dsarservice.ErrInvalid):
		return httpx.UnprocessableEntity("dsar.invalid_input", err.Error())
	}
	return err
}

func (h *Strict) DsarGetPublicIntakeForm(ctx context.Context, _ DsarGetPublicIntakeFormRequestObject) (DsarGetPublicIntakeFormResponseObject, error) {
	k, err := intakeForm(ctx)
	if err != nil {
		return nil, err
	}
	f, types, err := h.svc.PublicIntakeForm(ctx, k.EntityID)
	if err != nil {
		return nil, problem(err)
	}
	out := DsarPublicIntakeForm{Id: f.ID, Name: f.Name, RequestTypes: make([]DsarRequestType, 0, len(types))}
	for _, t := range types {
		w := DsarRequestType{Id: t.ID, Code: DsarRequestTypeCode(t.Code), NameTh: t.NameTh, SlaDays: int(t.SLADays)}
		if t.NameEn != "" {
			w.NameEn = &t.NameEn
		}
		if t.LegalRef != "" {
			w.LegalRef = &t.LegalRef
		}
		out.RequestTypes = append(out.RequestTypes, w)
	}
	return DsarGetPublicIntakeForm200JSONResponse(out), nil
}

func (h *Strict) DsarSubmitPublicRequest(ctx context.Context, req DsarSubmitPublicRequestRequestObject) (DsarSubmitPublicRequestResponseObject, error) {
	k, err := intakeForm(ctx)
	if err != nil {
		return nil, err
	}
	b := *req.Body
	in := dsarservice.PublicSubmission{RequestTypeCode: string(b.RequestTypeCode), RequesterName: b.RequesterName, DataSource: b.DataSource}
	if b.RequesterEmail != nil {
		in.RequesterEmail = string(*b.RequesterEmail)
	}
	if b.RequesterPhone != nil {
		in.RequesterPhone = *b.RequesterPhone
	}
	r, v, err := h.svc.CreatePublicSubmission(ctx, k.EntityID, in)
	if err != nil {
		return nil, problem(err)
	}
	out := DsarPublicSubmissionResult{RequestNo: r.RequestNo, Status: DsarRequestStatus(r.Status)}
	if v != nil {
		w := toVerificationWire(*v)
		out.Verification = &w
	}
	return DsarSubmitPublicRequest201JSONResponse(out), nil
}

func (h *Strict) DsarConfirmPublicVerification(ctx context.Context, req DsarConfirmPublicVerificationRequestObject) (DsarConfirmPublicVerificationResponseObject, error) {
	if _, err := intakeForm(ctx); err != nil {
		return nil, err
	}
	b := *req.Body
	v, err := h.svc.ConfirmOTPVerification(ctx, req.RequestId, b.VerificationId, b.Code)
	if err != nil {
		return nil, problem(err)
	}
	return DsarConfirmPublicVerification200JSONResponse(toVerificationWire(v)), nil
}

func toVerificationWire(v dsarservice.Verification) DsarVerification {
	w := DsarVerification{Id: v.ID, RequestId: v.RequestID, Method: DsarVerificationMethod(v.Method),
		Status: DsarVerificationStatus(v.Status), MaskedIdFileId: v.MaskedIDFileID, VerifiedBy: v.VerifiedBy, CreatedAt: v.CreatedAt.UTC()}
	if v.VerifiedAt != nil {
		t := v.VerifiedAt.UTC()
		w.VerifiedAt = &t
	}
	return w
}
