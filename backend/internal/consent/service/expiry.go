package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	consentstore "pdpa-platform/internal/consent/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/jobs"
)

// NearExpiryDefaultDays is CON-20's own "near expiry" window — a plain config default per CLAUDE.md's own
// "implement the listed default as config" (not a docs/decisions.md question: it only shapes which rows
// the admin UI's own list surfaces, never a contract or legal behaviour).
const NearExpiryDefaultDays = 30

// NearExpiry is one consent purpose a subject's consent is about to run out on — the module doc's own
// "รายการที่ใกล้หมดอายุ" (the near-expiry list).
type NearExpiry struct {
	SubjectID   uuid.UUID
	PurposeCode string
	PurposeName Text
	ExpiresAt   time.Time
}

// ListNearExpiry is CON-20's own frontend note: every ACTIVE consent expiring within withinDays (or the
// default window when 0/negative is given).
func (s *Service) ListNearExpiry(ctx context.Context, withinDays int) ([]NearExpiry, error) {
	if withinDays <= 0 {
		withinDays = NearExpiryDefaultDays
	}
	rows, err := consentstore.New(pdb.MustTxFromContext(ctx)).ListNearExpiry(ctx, int32(withinDays))
	if err != nil {
		return nil, err
	}
	out := make([]NearExpiry, 0, len(rows))
	for _, r := range rows {
		out = append(out, NearExpiry{SubjectID: r.SubjectID, PurposeCode: r.PurposeCode,
			PurposeName: Text{Th: r.NameTh, En: deref(r.NameEn)}, ExpiresAt: r.ExpiresAt.Time})
	}
	return out, nil
}

// CheckDueExpirations is CON-20's own acceptance criterion: every ACTIVE consent past its own expiry moves
// to EXPIRED (ST-01's already-declared ACTIVE -> EXPIRED edge), with its own transaction appended to the
// subject's hash chain (rule 4) and a consent.expired event published, so a downstream subscriber sees
// expiry exactly like any other decision. Called once per tenant by the River sweep (expiry_check.go).
func (s *Service) CheckDueExpirations(ctx context.Context) (int, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	due, err := q.ListDueExpirations(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range due {
		expired, err := s.expireStatus(ctx, d)
		if err != nil {
			return n, err
		}
		if expired {
			n++
		}
	}
	return n, nil
}

// expireStatus writes one EXPIRED transaction for a single due row, re-checking under lock that it is
// still ACTIVE and still due (another write may have renewed or withdrawn it since ListDueExpirations ran).
// It mirrors writeDecisions' own receipt-chain mechanics directly rather than routing through
// checkDecisions/Decide, which model a data subject's own decision — expiry is the system's, not theirs.
func (s *Service) expireStatus(ctx context.Context, d consentstore.ListDueExpirationsRow) (bool, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	if err := q.LockSubjectChain(ctx, d.SubjectID); err != nil {
		return false, err
	}
	cur, err := q.LockStatus(ctx, consentstore.LockStatusParams{SubjectID: d.SubjectID, PurposeID: d.PurposeID})
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	now := s.now()
	if cur.Status != StatusActive || !cur.ExpiresAt.Valid || cur.ExpiresAt.Time.After(now) {
		return false, nil // renewed, withdrawn or already handled since the sweep listed it
	}

	prev := ""
	occurred := now.Truncate(time.Microsecond)
	last, err := q.LastReceipt(ctx, d.SubjectID)
	switch {
	case err == pgx.ErrNoRows:
	case err != nil:
		return false, err
	default:
		prev = last.Hash
		if !occurred.After(last.OccurredAt.Time) {
			occurred = last.OccurredAt.Time.Add(time.Microsecond)
		}
	}

	rid, err := uuid.NewV7()
	if err != nil {
		return false, err
	}
	rec := receiptRecord{ReceiptNo: newReceiptNo(occurred), SubjectID: d.SubjectID, CollectionPointID: d.CollectionPointID,
		Channel: d.Channel, Language: "th", OccurredAt: occurred, PrevHash: prev}
	tid, err := uuid.NewV7()
	if err != nil {
		return false, err
	}
	rec.Transactions = []txRecord{{ID: tid, PurposeID: d.PurposeID, PurposeVersionID: d.PurposeVersionID, Type: TxExpired, Source: "system"}}
	rec.Hash = rec.hash()

	if err := q.InsertReceipt(ctx, consentstore.InsertReceiptParams{ID: rid, ReceiptNo: rec.ReceiptNo, SubjectID: d.SubjectID,
		CollectionPointID: d.CollectionPointID, Channel: d.Channel, Language: optText("th"),
		OccurredAt: pgtype.Timestamptz{Time: occurred, Valid: true}, PrevHash: prev, Hash: rec.Hash}); err != nil {
		return false, err
	}
	if err := q.InsertTransaction(ctx, consentstore.InsertTransactionParams{ID: tid, OccurredAt: pgtype.Timestamptz{Time: occurred, Valid: true},
		ReceiptID: rid, SubjectID: d.SubjectID, PurposeID: d.PurposeID, PurposeVersionID: d.PurposeVersionID,
		TransactionType: TxExpired, Source: "system"}); err != nil {
		return false, err
	}
	if err := q.UpsertStatus(ctx, consentstore.UpsertStatusParams{SubjectID: d.SubjectID, PurposeID: d.PurposeID, Status: StatusExpired,
		PurposeVersionID: d.PurposeVersionID, LastTransactionID: tid}); err != nil {
		return false, err
	}
	if err := q.TouchSubject(ctx, d.SubjectID); err != nil {
		return false, err
	}
	if s.Events != nil {
		if _, err := s.Events.Publish(ctx, events.Event{Type: "consent.expired", AggregateType: SubjectType, AggregateID: d.SubjectID, OccurredAt: occurred,
			Data: map[string]any{"subject_ref": d.SubjectID.String(), "purpose_code": d.PurposeCode, "purpose_version": d.VersionNo,
				"channel": d.Channel, "occurred_at": occurred.Format(time.RFC3339Nano)}}); err != nil {
			return false, err
		}
	}
	return true, nil
}

// ExpirySweepArgs is consent.expiry_sweep: platform-wide (jobs.GlobalKinds), no tenant transaction — it
// only enqueues one consent.expiry_check per tenant, which does the real (RLS-scoped) read, the same
// "sweep lists tenants, per-tenant job reads its own rows" pattern CON-04/PNG-15/ROPA-15/CON-23 already
// established.
type ExpirySweepArgs struct{}

func (ExpirySweepArgs) Kind() string { return "consent.expiry_sweep" }

// ExpirySweeper enqueues consent.expiry_check for every trial/active tenant.
type ExpirySweeper struct {
	river.WorkerDefaults[ExpirySweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx]
}

func (w *ExpirySweeper) Work(ctx context.Context, _ *river.Job[ExpirySweepArgs]) error {
	tenants, err := consentstore.New(w.Pool).ListActiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("consent.expiry_sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: ExpiryCheckArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := w.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("consent.expiry_sweep: enqueue: %w", err)
	}
	return nil
}

// ExpirySweepPeriodicJob runs consent.expiry_sweep once a day — expiry is date-granular (lifespan_days),
// unlike CON-23's own hourly double opt-in sweep, which races a much shorter 48h window.
func ExpirySweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return ExpirySweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "consent-expiry-sweep", RunOnStart: false},
	)
}

// ExpiryCheckArgs is consent.expiry_check: one tenant's own turn to look at its due expirations.
type ExpiryCheckArgs struct {
	jobs.TenantArgs
}

func (ExpiryCheckArgs) Kind() string { return "consent.expiry_check" }

// ExpiryChecker works consent.expiry_check inside the tenant's own transaction.
type ExpiryChecker struct {
	river.WorkerDefaults[ExpiryCheckArgs]
	Service *Service
}

func (w *ExpiryChecker) Work(ctx context.Context, _ *river.Job[ExpiryCheckArgs]) error {
	_, err := w.Service.CheckDueExpirations(ctx)
	return err
}
