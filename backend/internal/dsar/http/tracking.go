package dsarhttp

import "context"

// DSAR-19's admin side: publish/retire/read the legal entity's self-service status-tracking link — the
// same coalesce-on-first-publish shape every other public-facing surface's own admin endpoints use
// (notice.gen.go's PublishNotice, consent.gen.go's ConsentPublishPreferenceCenter, ...).

func (h *Strict) DsarGetTrackingPortal(ctx context.Context, req DsarGetTrackingPortalRequestObject) (DsarGetTrackingPortalResponseObject, error) {
	key, ok, err := h.svc.GetTrackingPortalLink(ctx, req.LegalEntityId)
	if err != nil {
		return nil, problem(err)
	}
	out := DsarTrackingPortalLink{}
	if ok {
		out.PublicKey = &key
	}
	return DsarGetTrackingPortal200JSONResponse(out), nil
}

func (h *Strict) DsarPublishTrackingPortal(ctx context.Context, req DsarPublishTrackingPortalRequestObject) (DsarPublishTrackingPortalResponseObject, error) {
	key, err := h.svc.PublishTrackingPortal(ctx, req.LegalEntityId)
	if err != nil {
		return nil, problem(err)
	}
	return DsarPublishTrackingPortal200JSONResponse{PublicKey: &key}, nil
}

func (h *Strict) DsarRetireTrackingPortal(ctx context.Context, req DsarRetireTrackingPortalRequestObject) (DsarRetireTrackingPortalResponseObject, error) {
	if err := h.svc.RetireTrackingPortal(ctx, req.LegalEntityId); err != nil {
		return nil, problem(err)
	}
	return DsarRetireTrackingPortal204Response{}, nil
}
