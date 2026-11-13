package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	consentstore "pdpa-platform/internal/consent/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
)

// guardianOK is CON-11's own precondition (ม.20): a guardian must be named with a relationship and at least
// one identifier IAM-05 can send an OTP to (SMS or e-mail only — magic_link/idp/thaid aren't built).
func guardianOK(g *Guardian) bool {
	if g == nil || g.Relationship == "" || len(g.Identifiers) == 0 {
		return false
	}
	kind := crypto.IdentifierKind(g.Identifiers[0].Type)
	return kind == crypto.KindEmail || kind == crypto.KindPhone
}

// requestGuardianApproval is the write half of CON-11: resolve (or create) the guardian's own data-subject
// record, log one consent.guardian_approvals row, and start their IAM-05 OTP — all in Record's own
// transaction, so a guardian request is never left dangling if anything after it fails.
func (s *Service) requestGuardianApproval(ctx context.Context, minorSubjectID uuid.UUID, sub Submission, receiptID uuid.UUID) (GuardianApprovalRef, error) {
	if s.Verification == nil {
		return GuardianApprovalRef{}, fmt.Errorf("consent: guardian verification not wired")
	}
	guardianSubjectID, _, err := s.resolveSubject(ctx, Submission{Identifiers: sub.Guardian.Identifiers})
	if err != nil {
		return GuardianApprovalRef{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return GuardianApprovalRef{}, err
	}
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	if _, err := q.InsertGuardianApproval(ctx, consentstore.InsertGuardianApprovalParams{
		ID: id, MinorSubjectID: minorSubjectID, GuardianSubjectID: guardianSubjectID, ReceiptID: pgtype.UUID{Bytes: receiptID, Valid: true},
		Relationship: sub.Guardian.Relationship, RequestedAt: pgtype.Timestamptz{Time: s.now(), Valid: true},
	}); err != nil {
		return GuardianApprovalRef{}, err
	}

	first := sub.Guardian.Identifiers[0]
	kind := crypto.IdentifierKind(first.Type)
	method, channel := "otp_email", "email"
	if kind == crypto.KindPhone {
		method, channel = "otp_sms", "sms"
	}
	v, err := s.Verification.StartVerification(ctx, iamservice.StartVerificationInput{
		SubjectID: &guardianSubjectID, Purpose: "guardian", Method: method, IdentifierKind: kind, IdentifierValue: first.Value,
	})
	if err != nil {
		return GuardianApprovalRef{}, err
	}
	return GuardianApprovalRef{ID: id, VerificationID: v.ID, Channel: channel}, nil
}

// ConfirmGuardianApproval is CON-11's acceptance criterion itself: a guardian-gated consent has effect only
// once this succeeds. verificationID/code are the IAM-05 OTP the guardian was sent; on success every purpose
// still PENDING for the minor (not just the one that first triggered the request — a single receipt can
// guardian-gate several purposes at once) becomes ACTIVE in one step.
func (s *Service) ConfirmGuardianApproval(ctx context.Context, approvalID, verificationID uuid.UUID, code string) ([]string, error) {
	if s.Verification == nil {
		return nil, fmt.Errorf("consent: guardian verification not wired")
	}
	if _, err := s.Verification.VerifyOTP(ctx, verificationID, code); err != nil {
		return nil, mapVerificationErr(err)
	}

	q := consentstore.New(pdb.MustTxFromContext(ctx))
	appr, err := q.GetGuardianApproval(ctx, approvalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if appr.Status != "requested" {
		return nil, ErrInvalidTransition
	}
	if _, err := q.ApproveGuardianApproval(ctx, approvalID); err != nil {
		return nil, err
	}

	rows, err := q.ListPendingStatusForSubject(ctx, appr.MinorSubjectID)
	if err != nil {
		return nil, err
	}
	occurred := s.now()
	var confirmed []string
	for _, r := range rows {
		purpose, err := s.GetPurpose(ctx, r.PurposeID)
		if err != nil {
			return nil, err
		}
		version, err := q.GetPurposeVersion(ctx, r.PurposeVersionID)
		if err != nil {
			return nil, err
		}
		txType, status := ConfirmPending()
		var expires *time.Time
		if purpose.Live.LifespanDays != nil {
			e := occurred.AddDate(0, 0, *purpose.Live.LifespanDays)
			expires = &e
		}
		tid, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}
		if err := q.InsertTransaction(ctx, consentstore.InsertTransactionParams{ID: tid, OccurredAt: pgtype.Timestamptz{Time: occurred, Valid: true},
			ReceiptID: uuid.UUID(appr.ReceiptID.Bytes), SubjectID: appr.MinorSubjectID, PurposeID: r.PurposeID, PurposeVersionID: r.PurposeVersionID,
			TransactionType: txType, Preferences: r.Preferences, ExpiresAt: tsPtr(expires), Source: "system"}); err != nil {
			return nil, err
		}
		if err := q.UpsertStatus(ctx, consentstore.UpsertStatusParams{SubjectID: appr.MinorSubjectID, PurposeID: r.PurposeID, Status: status,
			PurposeVersionID: r.PurposeVersionID, LastTransactionID: tid, Preferences: r.Preferences, ExpiresAt: tsPtr(expires)}); err != nil {
			return nil, err
		}
		if ev := eventFor(txType); ev != "" && s.Events != nil {
			if _, err := s.Events.Publish(ctx, events.Event{Type: ev, AggregateType: SubjectType, AggregateID: appr.MinorSubjectID, OccurredAt: occurred,
				Data: map[string]any{"subject_ref": appr.MinorSubjectID.String(), "purpose_code": purpose.Code, "purpose_version": version.VersionNo,
					"channel": appr.Channel, "occurred_at": occurred.Format(time.RFC3339Nano)}}); err != nil {
				return nil, err
			}
		}
		confirmed = append(confirmed, purpose.Code)
	}
	return confirmed, s.audit(ctx, "consent.guardian_approval.approve", SubjectType, appr.MinorSubjectID,
		map[string]any{"status": "requested"}, map[string]any{"status": "approved", "confirmed_purposes": confirmed})
}

// mapVerificationErr keeps IAM-05's own sentinels out of consent's error surface — the HTTP layer only knows
// consent's own errors (rule 9).
func mapVerificationErr(err error) error {
	switch {
	case errors.Is(err, iamservice.ErrVerificationNotFound):
		return ErrNotFound
	case errors.Is(err, iamservice.ErrVerificationDecided), errors.Is(err, iamservice.ErrVerificationExpired), errors.Is(err, iamservice.ErrTooManyAttempts):
		return ErrInvalidTransition
	case errors.Is(err, iamservice.ErrCodeMismatch):
		return fmt.Errorf("%w: code", ErrInvalidRequest)
	default:
		return err
	}
}
