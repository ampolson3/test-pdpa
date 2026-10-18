package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	consentstore "pdpa-platform/internal/consent/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// DoubleOptInExpiry is CON-23's own confirmation window — a tunable default (CLAUDE.md's own "implement the
// listed default as config"), not a legally-mandated value, since the module doc names no specific window.
const DoubleOptInExpiry = 48 * time.Hour

// DoubleOptInRef is what Record returns for each purpose a double opt-in link was just sent for.
type DoubleOptInRef struct {
	ID          uuid.UUID
	PurposeCode string
	Channel     string // sms | email
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// requestDoubleOptInConfirmation is CON-23's write half: one consent.double_optin_requests row for the
// given PENDING transaction, with its own confirmation link sent to the subject's own identifier (the
// guardian flow, when it applies, always takes priority — see Decide — so this is only ever the subject's
// own channel, never a guardian's).
func (s *Service) requestDoubleOptInConfirmation(ctx context.Context, sub Submission, transactionID uuid.UUID, purposeCode string, cp CollectionPoint) (DoubleOptInRef, error) {
	if s.Notify == nil {
		return DoubleOptInRef{}, fmt.Errorf("consent: double opt-in notify not wired")
	}
	if len(sub.Identifiers) == 0 {
		return DoubleOptInRef{}, invalid("identifier")
	}
	first := sub.Identifiers[0]
	kind := crypto.IdentifierKind(first.Type)
	channel := notify.ChannelEmail
	if kind == crypto.KindPhone {
		channel = notify.ChannelSMS
	}

	token, err := randomToken()
	if err != nil {
		return DoubleOptInRef{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return DoubleOptInRef{}, err
	}
	now := s.now()
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertDoubleOptInRequest(ctx, consentstore.InsertDoubleOptInRequestParams{
		ID: id, TransactionID: transactionID, Channel: string(channel), TokenHash: hashToken(token),
		SentAt: pgtype.Timestamptz{Time: now, Valid: true}, ExpiresAt: pgtype.Timestamptz{Time: now.Add(DoubleOptInExpiry), Valid: true},
	})
	if err != nil {
		return DoubleOptInRef{}, err
	}

	base := strings.TrimRight(s.PortalBaseURL, "/")
	if base == "" {
		base = "http://localhost:3001"
	}
	link := fmt.Sprintf("%s/%s/consent-confirm/%s?token=%s", base, sub.Language, cp.PublicKey, token)
	if _, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "consent.double_opt_in", Channel: channel, Language: sub.Language,
		RecipientAddress: first.Value, Vars: map[string]any{"link": link}}); err != nil {
		return DoubleOptInRef{}, err
	}
	return DoubleOptInRef{ID: row.ID, PurposeCode: purposeCode, Channel: string(channel)}, nil
}

// ConfirmDoubleOptIn is CON-23's acceptance criterion itself ("ความยินยอมมีผลหลังยืนยันเท่านั้น"): the
// decision this token was issued for has effect only now. Looked up by the token's own hash — the token is
// the only secret the link carries, so there is nothing else to resolve a tenant-scoped row by until the
// public key names the tenant.
func (s *Service) ConfirmDoubleOptIn(ctx context.Context, token string) (string, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	req, err := q.LockDoubleOptInRequestByTokenHash(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if req.Status != "sent" {
		return "", ErrInvalidTransition
	}
	now := s.now()
	if now.After(req.ExpiresAt.Time) {
		if _, err := q.ExpireDoubleOptInRequest(ctx, req.ID); err != nil {
			return "", err
		}
		return "", ErrInvalidTransition
	}

	tx, err := q.GetTransaction(ctx, req.TransactionID)
	if err != nil {
		return "", err
	}
	if tx.TransactionType != TxPending {
		return "", ErrInvalidTransition // already confirmed/cancelled by another path
	}
	purpose, err := s.GetPurpose(ctx, tx.PurposeID)
	if err != nil {
		return "", err
	}

	if n, err := q.ConfirmDoubleOptInRequest(ctx, consentstore.ConfirmDoubleOptInRequestParams{ID: req.ID, ConfirmedAt: pgtype.Timestamptz{Time: now, Valid: true}}); err != nil {
		return "", err
	} else if n == 0 {
		return "", ErrInvalidTransition
	}

	txType, status := ConfirmPending()
	var expires *time.Time
	if purpose.Live.LifespanDays != nil {
		e := now.AddDate(0, 0, *purpose.Live.LifespanDays)
		expires = &e
	}
	tid, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	if err := q.InsertTransaction(ctx, consentstore.InsertTransactionParams{ID: tid, OccurredAt: pgtype.Timestamptz{Time: now, Valid: true},
		ReceiptID: tx.ReceiptID, SubjectID: tx.SubjectID, PurposeID: tx.PurposeID, PurposeVersionID: tx.PurposeVersionID,
		TransactionType: txType, Preferences: tx.Preferences, ExpiresAt: tsPtr(expires), Source: "system"}); err != nil {
		return "", err
	}
	if err := q.UpsertStatus(ctx, consentstore.UpsertStatusParams{SubjectID: tx.SubjectID, PurposeID: tx.PurposeID, Status: status,
		PurposeVersionID: tx.PurposeVersionID, LastTransactionID: tid, Preferences: tx.Preferences, ExpiresAt: tsPtr(expires)}); err != nil {
		return "", err
	}
	if s.Events != nil {
		if _, err := s.Events.Publish(ctx, events.Event{Type: "consent.granted", AggregateType: SubjectType, AggregateID: tx.SubjectID, OccurredAt: now,
			Data: map[string]any{"subject_ref": tx.SubjectID.String(), "purpose_code": purpose.Code, "occurred_at": now.Format(time.RFC3339Nano)}}); err != nil {
			return "", err
		}
	}
	if err := s.audit(ctx, "consent.double_opt_in.confirm", SubjectType, tx.SubjectID, map[string]any{"status": "sent"},
		map[string]any{"status": "confirmed", "purpose_code": purpose.Code}); err != nil {
		return "", err
	}
	return purpose.Code, nil
}

// SweepDueDoubleOptInRequests is CON-23's own expiry path (River, the usual "sweep lists tenants, per-tenant
// job reads its own rows" pattern this codebase uses throughout): a confirmation link nobody ever clicked
// never takes effect — its PENDING decision is cancelled, not left dangling forever.
func (s *Service) SweepDueDoubleOptInRequests(ctx context.Context) error {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListDueDoubleOptInRequests(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for _, r := range rows {
		if n, err := q.ExpireDoubleOptInRequest(ctx, r.ID); err != nil {
			return err
		} else if n == 0 {
			continue // already confirmed/expired by another path
		}
		tx, err := q.GetTransaction(ctx, r.TransactionID)
		if err != nil {
			return err
		}
		if tx.TransactionType != TxPending {
			continue
		}
		purpose, err := s.GetPurpose(ctx, tx.PurposeID)
		if err != nil {
			return err
		}
		txType, status := CancelPending()
		tid, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if err := q.InsertTransaction(ctx, consentstore.InsertTransactionParams{ID: tid, OccurredAt: pgtype.Timestamptz{Time: now, Valid: true},
			ReceiptID: tx.ReceiptID, SubjectID: tx.SubjectID, PurposeID: tx.PurposeID, PurposeVersionID: tx.PurposeVersionID,
			TransactionType: txType, Source: "system"}); err != nil {
			return err
		}
		if err := q.UpsertStatus(ctx, consentstore.UpsertStatusParams{SubjectID: tx.SubjectID, PurposeID: tx.PurposeID, Status: status,
			PurposeVersionID: tx.PurposeVersionID, LastTransactionID: tid}); err != nil {
			return err
		}
		if s.Events != nil {
			if _, err := s.Events.Publish(ctx, events.Event{Type: "consent.denied", AggregateType: SubjectType, AggregateID: tx.SubjectID, OccurredAt: now,
				Data: map[string]any{"subject_ref": tx.SubjectID.String(), "purpose_code": purpose.Code, "occurred_at": now.Format(time.RFC3339Nano)}}); err != nil {
				return err
			}
		}
		if err := s.audit(ctx, "consent.double_opt_in.expire", SubjectType, tx.SubjectID, map[string]any{"status": "sent"},
			map[string]any{"status": "expired", "purpose_code": purpose.Code}); err != nil {
			return err
		}
	}
	return nil
}

// DoubleOptInSweepArgs is consent.double_opt_in_sweep: platform-wide (jobs.GlobalKinds), no tenant
// transaction — it only enqueues one consent.double_opt_in_check per tenant, which does the real
// (RLS-scoped) read, the same "sweep lists tenants, per-tenant job reads its own rows" pattern
// CON-04/PNG-15/ROPA-15 already established.
type DoubleOptInSweepArgs struct{}

func (DoubleOptInSweepArgs) Kind() string { return "consent.double_opt_in_sweep" }

// DoubleOptInSweeper enqueues consent.double_opt_in_check for every trial/active tenant.
type DoubleOptInSweeper struct {
	river.WorkerDefaults[DoubleOptInSweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx]
}

func (w *DoubleOptInSweeper) Work(ctx context.Context, _ *river.Job[DoubleOptInSweepArgs]) error {
	tenants, err := consentstore.New(w.Pool).ListActiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("consent.double_opt_in_sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: DoubleOptInCheckArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := w.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("consent.double_opt_in_sweep: enqueue: %w", err)
	}
	return nil
}

// DoubleOptInSweepPeriodicJob runs consent.double_opt_in_sweep every hour — comfortably more often than
// DoubleOptInExpiry (48h), so an unconfirmed link is cancelled within an hour of its own window closing.
func DoubleOptInSweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return DoubleOptInSweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "consent-double-opt-in-sweep", RunOnStart: false},
	)
}

// DoubleOptInCheckArgs is consent.double_opt_in_check: one tenant's own turn to look at its due requests.
type DoubleOptInCheckArgs struct {
	jobs.TenantArgs
}

func (DoubleOptInCheckArgs) Kind() string { return "consent.double_opt_in_check" }

// DoubleOptInChecker works consent.double_opt_in_check inside the tenant's own transaction.
type DoubleOptInChecker struct {
	river.WorkerDefaults[DoubleOptInCheckArgs]
	Service *Service
}

func (w *DoubleOptInChecker) Work(ctx context.Context, _ *river.Job[DoubleOptInCheckArgs]) error {
	return w.Service.SweepDueDoubleOptInRequests(ctx)
}
