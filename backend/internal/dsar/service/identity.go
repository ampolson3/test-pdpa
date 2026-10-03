package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dsarstore "pdpa-platform/internal/dsar/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/files"
)

// DSAR-06 (ม.39(2)'s ยืนยันตัวตน, BP-06): confirms the requester actually controls the contact they gave at
// intake (OTP, reusing IAM-05's own otp_sms/otp_email verification with purpose "dsar") or, when staff need
// to check a photographed ID card, redacts the card's number before the image is ever stored at all — the
// acceptance criterion's "เลขบัตรในไฟล์ที่เก็บถูกปกปิดเสมอ" is literal: only the already-redacted output is
// ever attached to the request (dsar.verifications.masked_id_file_id); the caller's raw upload is never
// attached to anything and simply expires via PLT-09's own 24h orphan cleanup. No OCR library is available in
// this environment to *find* the number automatically, so the caller (staff reviewing the photo) marks the
// rectangle(s) to redact — a deliberately scoped default, not a guess at unavailable OCR tooling.
//
// ตรวจกับข้อมูลในระบบ ("check against system data"): for OTP, this is implicit — the code is sent to the
// contact already on file for this request, so a correct code proves the requester controls that exact
// channel. For id_document, it is a human judgement call (ConfirmIDDocument), not automated: no trusted
// national-id database exists to check against from here.
var (
	ErrVerificationNotPending = errors.New("dsar: verification is not pending")
	ErrRedactionOutOfBounds   = errors.New("dsar: redaction rectangle is outside the image")
)

// Files is what dsar needs from file storage (PLT-09): DSAR-06's caller-owned raw upload + saving the
// redacted output as a new, already-attached file, and DSAR-08's own Attach for a subtask's evidence (the
// caller's own clean, still-unattached upload).
type Files interface {
	Get(ctx context.Context, id uuid.UUID) (files.File, error)
	Open(ctx context.Context, id uuid.UUID) (io.ReadCloser, files.File, error)
	SaveGenerated(ctx context.Context, fileName string, r io.Reader, entityType string, entityID uuid.UUID) (files.File, error)
	Attach(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error
}

// Verification is one identity-check attempt against a DSAR request (dsar.verifications).
type Verification struct {
	ID                    uuid.UUID
	RequestID             uuid.UUID
	Method                string // otp_sms | otp_email | id_document
	SubjectVerificationID *uuid.UUID
	MaskedIDFileID        *uuid.UUID
	Status                string // pending | passed | failed
	VerifiedBy            *uuid.UUID
	VerifiedAt            *time.Time
	CreatedAt             time.Time
}

// Rect is one pixel rectangle, in the uploaded image's own coordinate space, to black out.
type Rect struct{ X, Y, Width, Height int }

func (s *Service) decryptRequesterContact(ctx context.Context, requestID uuid.UUID) (string, error) {
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).GetRequestRequesterContact(ctx, requestID)
	if err != nil {
		return "", err
	}
	plain, err := s.Keyring.Decrypt(ctx, cryptoClass, requesterContactContext, row)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// StartOTPVerification sends a fresh OTP to the request's own stored contact (decrypted via PLT-13, never
// logged — rule 3) and opens a pending verification attempt; the first call on a "received" request moves it
// to "verifying" (ST-02).
func (s *Service) StartOTPVerification(ctx context.Context, requestID uuid.UUID, method string) (Verification, error) {
	if method != "otp_sms" && method != "otp_email" {
		return Verification{}, fmt.Errorf("%w: method", ErrInvalid)
	}
	if s.Verification == nil {
		return Verification{}, fmt.Errorf("dsar: identity verification not wired")
	}
	req, err := s.GetRequest(ctx, requestID)
	if err != nil {
		return Verification{}, err
	}
	contact, err := s.decryptRequesterContact(ctx, requestID)
	if err != nil {
		return Verification{}, err
	}
	kind := crypto.KindPhone
	if method == "otp_email" {
		kind = crypto.KindEmail
	}
	sv, err := s.Verification.StartVerification(ctx, iamservice.StartVerificationInput{
		Purpose: "dsar", Method: method, IdentifierKind: kind, IdentifierValue: contact,
	})
	if err != nil {
		return Verification{}, mapVerificationErr(err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Verification{}, err
	}
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertVerification(ctx, dsarstore.InsertVerificationParams{
		ID: id, RequestID: requestID, Method: method, SubjectVerificationID: pgUUID(&sv.ID),
	})
	if err != nil {
		return Verification{}, err
	}
	out := toVerification(dsarstore.GetVerificationRow(row))
	if req.Status == "received" {
		if _, _, err := s.Transition(ctx, requestID, req.RowVersion, TransitionInput{To: "verifying"}); err != nil {
			return Verification{}, err
		}
	}
	return out, s.audit(ctx, "dsar.verification.start", requestID, nil, map[string]any{"method": method})
}

// ConfirmOTPVerification checks a submitted OTP code. Passing stamps the request's verified_at and moves it
// verifying → in_review (the acceptance criterion's "บันทึกวันยืนยันตัวตน"); a wrong code lets the caller
// retry (IAM-05's own 5-attempt cap, D-01) without this verification row changing status. Only once IAM-05
// itself gives up (too many attempts or expired) does this verification get marked failed.
func (s *Service) ConfirmOTPVerification(ctx context.Context, requestID, verificationID uuid.UUID, code string) (Verification, error) {
	v, err := s.getVerificationFor(ctx, requestID, verificationID)
	if err != nil {
		return Verification{}, err
	}
	if v.Status != "pending" || v.SubjectVerificationID == nil {
		return Verification{}, ErrVerificationNotPending
	}
	_, verr := s.Verification.VerifyOTP(ctx, *v.SubjectVerificationID, code)
	if verr == nil {
		return s.decideVerification(ctx, requestID, verificationID, "passed")
	}
	if errors.Is(verr, iamservice.ErrTooManyAttempts) || errors.Is(verr, iamservice.ErrVerificationExpired) {
		return s.decideVerification(ctx, requestID, verificationID, "failed")
	}
	return Verification{}, mapVerificationErr(verr)
}

// SubmitIDDocumentVerification redacts the given rectangles out of the caller's own clean, unattached upload
// and saves only that redacted copy — the raw original is never attached to the request or anything else, so
// it is never the "stored file" the acceptance criterion means (PLT-09 expires any unattached upload after
// 24h). Opens a pending verification for staff to decide (ConfirmIDDocument).
func (s *Service) SubmitIDDocumentVerification(ctx context.Context, requestID, rawFileID uuid.UUID, redactions []Rect) (Verification, error) {
	if len(redactions) == 0 {
		return Verification{}, fmt.Errorf("%w: redactions", ErrInvalid)
	}
	req, err := s.GetRequest(ctx, requestID)
	if err != nil {
		return Verification{}, err
	}
	f, err := s.Files.Get(ctx, rawFileID)
	if err != nil || f.EntityType != "" {
		return Verification{}, fmt.Errorf("%w: raw_file_id", ErrInvalid)
	}
	r, _, err := s.Files.Open(ctx, rawFileID)
	if err != nil {
		return Verification{}, fmt.Errorf("%w: raw_file_id", ErrInvalid)
	}
	defer r.Close()
	redacted, format, err := redactImage(r, redactions)
	if err != nil {
		return Verification{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	// The masked copy is saved (and attached to the request) before any dsar.verifications row exists for
	// it — the raw upload itself is never attached to anything, so if anything below fails, it simply
	// expires unattached via PLT-09's own 24h orphan cleanup rather than lingering as a reachable file.
	masked, err := s.Files.SaveGenerated(ctx, "id-card-masked."+format, bytes.NewReader(redacted), "dsar_request", requestID)
	if err != nil {
		return Verification{}, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Verification{}, err
	}
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertVerification(ctx, dsarstore.InsertVerificationParams{
		ID: id, RequestID: requestID, Method: "id_document", MaskedIDFileID: pgUUID(&masked.ID)})
	if err != nil {
		return Verification{}, err
	}
	out := toVerification(dsarstore.GetVerificationRow(row))
	if req.Status == "received" {
		if _, _, err := s.Transition(ctx, requestID, req.RowVersion, TransitionInput{To: "verifying"}); err != nil {
			return Verification{}, err
		}
	}
	return out, s.audit(ctx, "dsar.verification.submit_id_document", requestID, nil, map[string]any{"verification_id": id})
}

// ConfirmIDDocument is staff's own judgement call after checking the redacted card against system records —
// there is no trusted national-id database to automate this against from here.
func (s *Service) ConfirmIDDocument(ctx context.Context, requestID, verificationID uuid.UUID, pass bool) (Verification, error) {
	v, err := s.getVerificationFor(ctx, requestID, verificationID)
	if err != nil {
		return Verification{}, err
	}
	if v.Status != "pending" || v.Method != "id_document" {
		return Verification{}, ErrVerificationNotPending
	}
	status := "failed"
	if pass {
		status = "passed"
	}
	return s.decideVerification(ctx, requestID, verificationID, status)
}

func (s *Service) decideVerification(ctx context.Context, requestID, verificationID uuid.UUID, status string) (Verification, error) {
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).DecideVerification(ctx, dsarstore.DecideVerificationParams{ID: verificationID, Status: status})
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, ErrVerificationNotPending
	}
	if err != nil {
		return Verification{}, err
	}
	out := toVerification(dsarstore.GetVerificationRow(row))
	if status == "passed" {
		if err := dsarstore.New(pdb.MustTxFromContext(ctx)).SetRequestVerified(ctx, dsarstore.SetRequestVerifiedParams{
			ID: requestID, VerifiedAt: pgtype.Timestamptz{Time: s.now(), Valid: true}}); err != nil {
			return Verification{}, err
		}
		req, err := s.GetRequest(ctx, requestID)
		if err != nil {
			return Verification{}, err
		}
		if req.Status == "verifying" {
			if _, _, err := s.Transition(ctx, requestID, req.RowVersion, TransitionInput{To: "in_review"}); err != nil {
				return Verification{}, err
			}
		}
	}
	return out, s.audit(ctx, "dsar.verification.decide", requestID, nil, map[string]any{"verification_id": out.ID, "status": status})
}

// ListVerifications returns every attempt against a request, oldest first.
func (s *Service) ListVerifications(ctx context.Context, requestID uuid.UUID) ([]Verification, error) {
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return nil, err
	}
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListVerificationsForRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	out := make([]Verification, 0, len(rows))
	for _, r := range rows {
		out = append(out, toVerification(dsarstore.GetVerificationRow(r)))
	}
	return out, nil
}

func (s *Service) getVerificationFor(ctx context.Context, requestID, verificationID uuid.UUID) (Verification, error) {
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).GetVerification(ctx, verificationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, ErrNotFound
	}
	if err != nil {
		return Verification{}, err
	}
	if row.RequestID != requestID {
		return Verification{}, ErrNotFound
	}
	return toVerification(row), nil
}

// mapVerificationErr translates IAM-05's own sentinel errors into dsar's (rule 9 — dsar never leaks iam's
// error types past its own service boundary), the same move CON-11's own mapVerificationErr already made.
func mapVerificationErr(err error) error {
	switch {
	case errors.Is(err, iamservice.ErrVerificationNotFound):
		return ErrNotFound
	case errors.Is(err, iamservice.ErrVerificationInvalid):
		return fmt.Errorf("%w: method", ErrInvalid)
	case errors.Is(err, iamservice.ErrVerificationExpired), errors.Is(err, iamservice.ErrVerificationDecided),
		errors.Is(err, iamservice.ErrTooManyAttempts), errors.Is(err, iamservice.ErrCodeMismatch):
		return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	return err
}

// redactImage decodes a JPEG/PNG, draws an opaque black box over each rectangle (clamped to the image's own
// bounds — any rectangle reaching outside it is refused rather than silently clipped, so a caller never gets
// a false sense of full coverage from a rectangle that was actually cut short) and re-encodes in the same
// format it was given.
func redactImage(r io.Reader, rects []Rect) ([]byte, string, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, "", fmt.Errorf("could not decode image: %w", err)
	}
	if format != "jpeg" && format != "png" {
		return nil, "", fmt.Errorf("unsupported image format %q (only jpeg/png)", format)
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	bounds := rgba.Bounds()
	for _, rect := range rects {
		box := image.Rect(rect.X, rect.Y, rect.X+rect.Width, rect.Y+rect.Height)
		if !box.In(bounds) {
			return nil, "", ErrRedactionOutOfBounds
		}
		draw.Draw(rgba, box, &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	}
	var buf bytes.Buffer
	switch format {
	case "jpeg":
		err = jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: 90})
	case "png":
		err = png.Encode(&buf, rgba)
	}
	if err != nil {
		return nil, "", err
	}
	return buf.Bytes(), format, nil
}

func toVerification(r dsarstore.GetVerificationRow) Verification {
	v := Verification{ID: r.ID, RequestID: r.RequestID, Method: r.Method, Status: r.Status, CreatedAt: r.CreatedAt.Time}
	if r.SubjectVerificationID.Valid {
		id := uuid.UUID(r.SubjectVerificationID.Bytes)
		v.SubjectVerificationID = &id
	}
	if r.MaskedIDFileID.Valid {
		id := uuid.UUID(r.MaskedIDFileID.Bytes)
		v.MaskedIDFileID = &id
	}
	if r.VerifiedBy.Valid {
		id := uuid.UUID(r.VerifiedBy.Bytes)
		v.VerifiedBy = &id
	}
	if r.VerifiedAt.Valid {
		t := r.VerifiedAt.Time
		v.VerifiedAt = &t
	}
	return v
}
