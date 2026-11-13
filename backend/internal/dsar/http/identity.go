package dsarhttp

import (
	"context"

	dsarservice "pdpa-platform/internal/dsar/service"
)

func (h *Strict) DsarListVerifications(ctx context.Context, req DsarListVerificationsRequestObject) (DsarListVerificationsResponseObject, error) {
	list, err := h.svc.ListVerifications(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := DsarListVerifications200JSONResponse{Data: make([]DsarVerification, 0, len(list))}
	for _, v := range list {
		resp.Data = append(resp.Data, toVerificationWire(v))
	}
	return resp, nil
}

func (h *Strict) DsarStartOtpVerification(ctx context.Context, req DsarStartOtpVerificationRequestObject) (DsarStartOtpVerificationResponseObject, error) {
	v, err := h.svc.StartOTPVerification(ctx, req.Id, string(req.Body.Method))
	if err != nil {
		return nil, problem(err)
	}
	return DsarStartOtpVerification201JSONResponse(toVerificationWire(v)), nil
}

func (h *Strict) DsarConfirmOtpVerification(ctx context.Context, req DsarConfirmOtpVerificationRequestObject) (DsarConfirmOtpVerificationResponseObject, error) {
	v, err := h.svc.ConfirmOTPVerification(ctx, req.Id, req.VerificationId, req.Body.Code)
	if err != nil {
		return nil, problem(err)
	}
	return DsarConfirmOtpVerification200JSONResponse(toVerificationWire(v)), nil
}

func (h *Strict) DsarSubmitIdDocumentVerification(ctx context.Context, req DsarSubmitIdDocumentVerificationRequestObject) (DsarSubmitIdDocumentVerificationResponseObject, error) {
	b := *req.Body
	rects := make([]dsarservice.Rect, 0, len(b.Redactions))
	for _, r := range b.Redactions {
		rects = append(rects, dsarservice.Rect{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height})
	}
	v, err := h.svc.SubmitIDDocumentVerification(ctx, req.Id, b.RawFileId, rects)
	if err != nil {
		return nil, problem(err)
	}
	return DsarSubmitIdDocumentVerification201JSONResponse(toVerificationWire(v)), nil
}

func (h *Strict) DsarDecideVerification(ctx context.Context, req DsarDecideVerificationRequestObject) (DsarDecideVerificationResponseObject, error) {
	v, err := h.svc.ConfirmIDDocument(ctx, req.Id, req.VerificationId, req.Body.Pass)
	if err != nil {
		return nil, problem(err)
	}
	return DsarDecideVerification200JSONResponse(toVerificationWire(v)), nil
}

func toVerificationWire(v dsarservice.Verification) DsarVerification {
	w := DsarVerification{Id: v.ID, RequestId: v.RequestID, Method: DsarVerificationMethod(v.Method),
		Status: DsarVerificationStatus(v.Status), MaskedIdFileId: v.MaskedIDFileID, VerifiedBy: v.VerifiedBy,
		CreatedAt: v.CreatedAt.UTC()}
	if v.VerifiedAt != nil {
		t := v.VerifiedAt.UTC()
		w.VerifiedAt = &t
	}
	return w
}
