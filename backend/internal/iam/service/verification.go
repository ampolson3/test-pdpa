package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	iamstore "pdpa-platform/internal/iam/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
)

// IAM-05 (docs/architecture/security.md, decisions.md D-01): an OTP-verified identity check for a data
// subject who has no iam.users account — the preference center, a DSAR request, double opt-in, or a
// guardian claim. Only otp_sms/otp_email are built here; magic_link/idp/thaid are listed in
// iam.subject_verifications' own CHECK constraint for later, per the module doc's own "รองรับภายหลัง".
var (
	ErrVerificationNotFound = errors.New("iam: verification not found")
	ErrVerificationInvalid  = errors.New("iam: invalid verification input")
	ErrVerificationExpired  = errors.New("iam: verification expired")
	ErrVerificationDecided  = errors.New("iam: verification already decided")
	ErrTooManyAttempts      = errors.New("iam: too many attempts")
	ErrCodeMismatch         = errors.New("iam: code does not match")
)

// OTPExpiry and MaxOTPAttempts are decisions.md D-01's fixed security values — not configuration, since
// changing them is a policy decision, not a tunable default (CLAUDE.md's own distinction).
const (
	OTPExpiry     = 5 * time.Minute
	MaxOTPAttempts = 5
)

var verificationPurposes = []string{"preference_center", "consent", "dsar", "double_opt_in", "guardian"}
var otpMethods = []string{"otp_sms", "otp_email"}

// SubjectVerification is one row of iam.subject_verifications — never carries the plaintext identifier or
// OTP code (rule 3); those exist only transiently in StartVerification/VerifyOTP's own arguments.
type SubjectVerification struct {
	ID             uuid.UUID
	SubjectID      *uuid.UUID
	Purpose        string
	Method         string
	Attempts       int32
	Status         string // pending | verified | failed | expired
	AssuranceLevel int16
	ExpiresAt      time.Time
	VerifiedAt     *time.Time
	RowVersion     int32
}

type StartVerificationInput struct {
	SubjectID       *uuid.UUID
	Purpose         string
	Method          string // otp_sms | otp_email
	IdentifierKind  crypto.IdentifierKind
	IdentifierValue string // raw phone/e-mail; used only to compute the blind index and send the OTP, never stored
}

// StartVerification issues a fresh 6-digit OTP (5-minute expiry, D-01), hashes it (SHA-256, never the
// plaintext) into iam.subject_verifications, and sends it over SMS or e-mail via PLT-04 — urgent, so it
// ignores quiet hours (an OTP that arrives after its own 5-minute window is useless).
func (s *Service) StartVerification(ctx context.Context, in StartVerificationInput) (SubjectVerification, error) {
	if !slices.Contains(verificationPurposes, in.Purpose) {
		return SubjectVerification{}, fmt.Errorf("%w: purpose", ErrVerificationInvalid)
	}
	if !slices.Contains(otpMethods, in.Method) {
		return SubjectVerification{}, fmt.Errorf("%w: method", ErrVerificationInvalid)
	}
	if in.IdentifierValue == "" {
		return SubjectVerification{}, fmt.Errorf("%w: identifier", ErrVerificationInvalid)
	}
	if s.Keyring == nil || s.Notify == nil {
		return SubjectVerification{}, fmt.Errorf("iam: verification service not wired")
	}

	code, err := randomOTP()
	if err != nil {
		return SubjectVerification{}, err
	}
	hash := hashOTP(code)

	blindIndex, err := s.Keyring.BlindIndex(ctx, in.IdentifierKind, in.IdentifierValue)
	if err != nil {
		return SubjectVerification{}, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return SubjectVerification{}, err
	}
	now := s.verifyNow()
	q := iamstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertSubjectVerification(ctx, iamstore.InsertSubjectVerificationParams{
		ID: id, SubjectID: pgUUIDVerify(in.SubjectID), IdentifierBlindIndex: blindIndex, Purpose: in.Purpose,
		Method: in.Method, OtpHash: &hash, AssuranceLevel: 1,
		ExpiresAt: pgTimestamptz(now.Add(OTPExpiry)),
	})
	if err != nil {
		return SubjectVerification{}, err
	}
	out := SubjectVerification{ID: row.ID, SubjectID: uuidPtrVerify(row.SubjectID), Purpose: row.Purpose, Method: row.Method,
		Attempts: row.Attempts, Status: row.Status, AssuranceLevel: row.AssuranceLevel, ExpiresAt: row.ExpiresAt.Time,
		VerifiedAt: timePtrVerify(row.VerifiedAt), RowVersion: row.RowVersion}

	channel := "sms"
	if in.Method == "otp_email" {
		channel = "email"
	}
	if _, err := s.Notify.Send(ctx, NotifyRequest{
		TemplateCode: "iam.otp", Channel: channel, RecipientAddress: in.IdentifierValue,
		Vars: map[string]any{"code": code}, Urgent: true,
	}); err != nil {
		return SubjectVerification{}, err
	}
	return out, s.auditVerification(ctx, "iam.subject_verification.start", out, nil)
}

// VerifyOTP checks a submitted code against a pending verification. Every call — right or wrong — increments
// attempts and is audited (the acceptance criterion's "บันทึกผลการยืนยันทุกครั้ง"); the plaintext code
// itself is never written to the audit log (rule 3), only the outcome.
func (s *Service) VerifyOTP(ctx context.Context, id uuid.UUID, code string) (SubjectVerification, error) {
	q := iamstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetSubjectVerification(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubjectVerification{}, ErrVerificationNotFound
	}
	if err != nil {
		return SubjectVerification{}, err
	}
	before := SubjectVerification{ID: row.ID, SubjectID: uuidPtrVerify(row.SubjectID), Purpose: row.Purpose, Method: row.Method,
		Attempts: row.Attempts, Status: row.Status, AssuranceLevel: row.AssuranceLevel, ExpiresAt: row.ExpiresAt.Time,
		VerifiedAt: timePtrVerify(row.VerifiedAt), RowVersion: row.RowVersion}

	if before.Status != "pending" {
		return SubjectVerification{}, ErrVerificationDecided
	}
	now := s.verifyNow()
	if now.After(before.ExpiresAt) {
		out, err := s.decide(ctx, id, before.RowVersion, "expired")
		if err != nil {
			return SubjectVerification{}, err
		}
		return out, s.andThen(ctx, "iam.subject_verification.attempt", before, out, ErrVerificationExpired)
	}
	if before.Attempts >= MaxOTPAttempts {
		out, err := s.decide(ctx, id, before.RowVersion, "failed")
		if err != nil {
			return SubjectVerification{}, err
		}
		return out, s.andThen(ctx, "iam.subject_verification.attempt", before, out, ErrTooManyAttempts)
	}

	match := row.OtpHash != nil && subtle.ConstantTimeCompare([]byte(*row.OtpHash), []byte(hashOTP(code))) == 1
	nextStatus := "pending"
	if match {
		nextStatus = "verified"
	} else if before.Attempts+1 >= MaxOTPAttempts {
		nextStatus = "failed"
	}
	out, err := s.decide(ctx, id, before.RowVersion, nextStatus)
	if err != nil {
		return SubjectVerification{}, err
	}
	if !match {
		return out, s.andThen(ctx, "iam.subject_verification.attempt", before, out, ErrCodeMismatch)
	}
	return out, s.auditVerification(ctx, "iam.subject_verification.attempt", out, &before)
}

func (s *Service) decide(ctx context.Context, id uuid.UUID, rowVersion int32, status string) (SubjectVerification, error) {
	q := iamstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.UpdateSubjectVerificationStatus(ctx, iamstore.UpdateSubjectVerificationStatusParams{ID: id, Status: status, RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return SubjectVerification{}, ErrVerificationDecided
	}
	if err != nil {
		return SubjectVerification{}, err
	}
	return SubjectVerification{ID: row.ID, SubjectID: uuidPtrVerify(row.SubjectID), Purpose: row.Purpose, Method: row.Method,
		Attempts: row.Attempts, Status: row.Status, AssuranceLevel: row.AssuranceLevel, ExpiresAt: row.ExpiresAt.Time,
		VerifiedAt: timePtrVerify(row.VerifiedAt), RowVersion: row.RowVersion}, nil
}

// andThen audits the attempt and then returns the sentinel error, so a failed/expired/mismatched attempt is
// always logged before the caller sees the error.
func (s *Service) andThen(ctx context.Context, action string, before, after SubjectVerification, sentinel error) error {
	if err := s.auditVerification(ctx, action, after, &before); err != nil {
		return err
	}
	return sentinel
}

func (s *Service) auditVerification(ctx context.Context, action string, out SubjectVerification, before *SubjectVerification) error {
	if s.Audit == nil {
		return nil
	}
	tenant, err := verificationTenantID(ctx)
	if err != nil {
		return err
	}
	var beforeAny any
	if before != nil {
		beforeAny = verificationAuditFields(*before)
	}
	return s.Audit.Write(ctx, AuditEntry{TenantID: tenant, ActorType: "system", EntityType: "subject_verification",
		EntityID: &out.ID, Action: action, Before: beforeAny, After: verificationAuditFields(out)})
}

func verificationAuditFields(v SubjectVerification) map[string]any {
	return map[string]any{"purpose": v.Purpose, "method": v.Method, "status": v.Status, "attempts": v.Attempts}
}

func randomOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func hashOTP(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func (s *Service) verifyNow() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func pgUUIDVerify(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func uuidPtrVerify(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	u := uuid.UUID(v.Bytes)
	return &u
}

func timePtrVerify(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func verificationTenantID(ctx context.Context) (uuid.UUID, error) {
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(t)
}
