package service

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dsarstore "pdpa-platform/internal/dsar/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/publickeys"
)

// ---- DSAR-19: self-service status tracking + submitting additional documents ----
//
// A loginless public flow gated by the exact OTP identity check CON-18/CON-19/CON-23 already established —
// its own purpose, "dsar_track" (migration 00098), so a plain intake/identity OTP (DSAR-01/06's own "dsar"
// purpose) can never double as proof of someone else's tracking session. Unlike CON-18's own preference
// centre, VerifiedIdentifier alone only proves the given contact was OTP-verified under this purpose, not
// that it's the same person who filed *this* request — so every call also re-matches the verified contact's
// own blind index against this exact request's own stored requester_blind_index before doing anything, not
// just the OTP's own purpose+identifier. The public key names an org.legal_entities row directly
// (publickeys.EntityDsarTrack), the same "no dedicated table" pattern CON-18's own preference centre already
// uses — resolving the tenant only, never scoping which requests can be tracked through it: any of a
// tenant's own published tracking links works for any of that tenant's requests, since a request is
// resolved by its own number and the requester's own stored contact, not by the key's entity id.

// TrackingRef is what starting a tracking OTP hands back to the public page.
type TrackingRef struct {
	VerificationID uuid.UUID
	Channel        string // sms | email
}

// PublishTrackingPortal issues the legal entity's status-tracking link on first use (coalesce-on-first-publish,
// the same pattern DSAR-01/PNG-06/BRE-01/CON-01/CON-18 already use).
func (s *Service) PublishTrackingPortal(ctx context.Context, legalEntityID uuid.UUID) (string, error) {
	if _, err := s.Org.GetLegalEntity(ctx, legalEntityID); err != nil {
		return "", fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	if key, ok, err := publickeys.Get(ctx, publickeys.EntityDsarTrack, legalEntityID); err != nil {
		return "", err
	} else if ok {
		return key, nil
	}
	key, err := publickeys.Issue(ctx, publickeys.EntityDsarTrack, legalEntityID, nil)
	if err != nil {
		return "", err
	}
	return key, s.audit(ctx, "dsar.tracking_portal.publish", legalEntityID, nil, map[string]any{"key_issued": true})
}

// RetireTrackingPortal stops the legal entity's current link from resolving; publishing again issues a new one.
func (s *Service) RetireTrackingPortal(ctx context.Context, legalEntityID uuid.UUID) error {
	key, ok, err := publickeys.Get(ctx, publickeys.EntityDsarTrack, legalEntityID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	if err := publickeys.Revoke(ctx, key); err != nil {
		return err
	}
	return s.audit(ctx, "dsar.tracking_portal.retire", legalEntityID, nil, nil)
}

// GetTrackingPortalLink reports whether the legal entity already has an active tracking link, and its key.
func (s *Service) GetTrackingPortalLink(ctx context.Context, legalEntityID uuid.UUID) (string, bool, error) {
	if _, err := s.Org.GetLegalEntity(ctx, legalEntityID); err != nil {
		return "", false, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	return publickeys.Get(ctx, publickeys.EntityDsarTrack, legalEntityID)
}

// resolveTrackedRequest finds the request by its own number and the given contact's own blind index — an
// unknown number or a contact that doesn't match it read identically (ErrNotFound), no information leak
// either way.
func (s *Service) resolveTrackedRequest(ctx context.Context, requestNo string, kind crypto.IdentifierKind, contact string) (Request, error) {
	bi, err := s.Keyring.BlindIndex(ctx, kind, contact)
	if err != nil {
		return Request{}, err
	}
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).GetRequestByNoAndBlindIndex(ctx, dsarstore.GetRequestByNoAndBlindIndexParams{
		RequestNo: requestNo, RequesterBlindIndex: bi,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	}
	if err != nil {
		return Request{}, err
	}
	return toRequest(dsarstore.GetRequestRow(row)), nil
}

func trackingKind(method string) (crypto.IdentifierKind, error) {
	switch method {
	case "otp_sms":
		return crypto.KindPhone, nil
	case "otp_email":
		return crypto.KindEmail, nil
	default:
		return "", fmt.Errorf("%w: method", ErrInvalid)
	}
}

// StartTrackingVerification resolves the request by its own number and the requester's own stored contact,
// then sends a fresh OTP to that contact.
func (s *Service) StartTrackingVerification(ctx context.Context, requestNo, method, contact string) (TrackingRef, error) {
	kind, err := trackingKind(method)
	if err != nil {
		return TrackingRef{}, err
	}
	if s.Verification == nil {
		return TrackingRef{}, fmt.Errorf("dsar: identity verification not wired")
	}
	normalized, err := crypto.Normalize(kind, contact)
	if err != nil {
		return TrackingRef{}, fmt.Errorf("%w: contact", ErrInvalid)
	}
	if _, err := s.resolveTrackedRequest(ctx, requestNo, kind, normalized); err != nil {
		return TrackingRef{}, err
	}
	sv, err := s.Verification.StartVerification(ctx, iamservice.StartVerificationInput{
		Purpose: "dsar_track", Method: method, IdentifierKind: kind, IdentifierValue: normalized,
	})
	if err != nil {
		return TrackingRef{}, mapVerificationErr(err)
	}
	channel := "sms"
	if method == "otp_email" {
		channel = "email"
	}
	return TrackingRef{VerificationID: sv.ID, Channel: channel}, nil
}

// verifyTrackingIdentity is the tracking portal's own identity gate: the given OTP (right after
// StartTrackingVerification, or empty on a later call in the same session — VerifyOTP refuses to re-check an
// already-decided row) must be verified under purpose "dsar_track" against exactly this contact, and that
// contact's own blind index must be the exact one stored on the resolved request — proving not just "some
// contact was OTP-verified" but "the requester of this specific request was."
func (s *Service) verifyTrackingIdentity(ctx context.Context, requestNo, method, contact string, verificationID uuid.UUID, code string) (Request, error) {
	kind, err := trackingKind(method)
	if err != nil {
		return Request{}, err
	}
	if s.Verification == nil {
		return Request{}, fmt.Errorf("dsar: identity verification not wired")
	}
	normalized, err := crypto.Normalize(kind, contact)
	if err != nil {
		return Request{}, fmt.Errorf("%w: contact", ErrInvalid)
	}
	req, err := s.resolveTrackedRequest(ctx, requestNo, kind, normalized)
	if err != nil {
		return Request{}, err
	}
	if code != "" {
		if _, err := s.Verification.VerifyOTP(ctx, verificationID, code); err != nil && !errors.Is(err, iamservice.ErrVerificationDecided) {
			return Request{}, mapVerificationErr(err)
		}
	}
	ok, err := s.Verification.VerifiedIdentifier(ctx, verificationID, "dsar_track", kind, normalized)
	if err != nil {
		return Request{}, err
	}
	if !ok {
		return Request{}, fmt.Errorf("%w: verification", ErrInvalid)
	}
	return req, nil
}

// GetTrackedStatus is the acceptance criterion's own first half: the data subject sees the request's
// current status, due date and — once decided — its outcome.
func (s *Service) GetTrackedStatus(ctx context.Context, requestNo, method, contact string, verificationID uuid.UUID, code string) (Request, error) {
	return s.verifyTrackingIdentity(ctx, requestNo, method, contact, verificationID, code)
}

// SubmitTrackingDocument is the acceptance criterion's own second half: the data subject submits an
// additional document themselves, through the same OTP gate — the file lands on the request's own
// attachments (PLT-07/PLT-09) exactly like a staff-recorded one, already queued for the ordinary virus scan.
func (s *Service) SubmitTrackingDocument(ctx context.Context, requestNo, method, contact string, verificationID uuid.UUID, code, fileName string, r io.Reader) (uuid.UUID, error) {
	if s.Files == nil {
		return uuid.Nil, fmt.Errorf("dsar: file storage not wired")
	}
	req, err := s.verifyTrackingIdentity(ctx, requestNo, method, contact, verificationID, code)
	if err != nil {
		return uuid.Nil, err
	}
	f, err := s.Files.Upload(ctx, fileName, r)
	if err != nil {
		return uuid.Nil, err
	}
	if err := s.Files.Attach(ctx, f.ID, "dsar_request", req.ID); err != nil {
		return uuid.Nil, err
	}
	return f.ID, s.audit(ctx, "dsar.request.document_submitted_by_subject", req.ID, nil, map[string]any{"file_id": f.ID})
}
