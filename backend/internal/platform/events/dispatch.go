package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	eventsstore "pdpa-platform/internal/platform/events/store"
	"pdpa-platform/internal/platform/jobs"
)

// DispatchArgs is outbox.dispatch for one tenant. It is unique per tenant while available,
// scheduled, pending or running, so a burst of events enqueues one dispatch, not one per event; an
// event committed while a dispatch is already running is picked up by the next sweep at the latest.
type DispatchArgs struct {
	jobs.TenantArgs
}

func (DispatchArgs) Kind() string { return "outbox.dispatch" }

func (DispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// AlertAfterAttempts is when a repeatedly failing event is logged as an alert rather than a warning.
const AlertAfterAttempts = 5

// Dispatcher works outbox.dispatch inside the tenant transaction TenantTxMiddleware opened.
type Dispatcher struct {
	river.WorkerDefaults[DispatchArgs]

	Subscribers *Registry
	Logger      *slog.Logger
	BatchSize   int // default 100
}

func (d *Dispatcher) Work(ctx context.Context, _ *river.Job[DispatchArgs]) error {
	q := eventsstore.New(pdb.MustTxFromContext(ctx))
	batch := d.BatchSize
	if batch <= 0 {
		batch = 100
	}

	// Events left unpublished in this run: failed ones, and later events of the same aggregate
	// (held back so an aggregate's events are never delivered out of order).
	skip := []uuid.UUID{}
	blocked := map[string]bool{}
	for {
		rows, err := q.LockUnpublishedEvents(ctx, eventsstore.LockUnpublishedEventsParams{SkipIds: skip, BatchSize: int32(batch)})
		if err != nil {
			return fmt.Errorf("outbox.dispatch: lock events: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			aggregate := row.AggregateType + "/" + row.AggregateID.String()
			if blocked[aggregate] {
				skip = append(skip, row.ID)
				continue
			}
			if err := d.dispatchOne(ctx, row); err != nil {
				skip = append(skip, row.ID)
				blocked[aggregate] = true
				attempts, markErr := q.MarkEventFailed(ctx, row.ID)
				if markErr != nil {
					return fmt.Errorf("outbox.dispatch: mark failed: %w", markErr)
				}
				d.logFailure(ctx, row, attempts, err)
			}
		}
	}
}

// dispatchOne delivers one event in its own savepoint: subscribers, webhook deliveries and the
// published mark commit together, or none of them do.
func (d *Dispatcher) dispatchOne(ctx context.Context, row eventsstore.LockUnpublishedEventsRow) error {
	var p payload
	if err := json.Unmarshal(row.Payload, &p); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	e := Event{
		ID: row.ID, TenantID: row.TenantID, Type: row.EventType,
		AggregateType: row.AggregateType, AggregateID: row.AggregateID,
		Data: p.Data, Version: p.Version, OccurredAt: row.OccurredAt.Time.UTC(),
	}

	return pdb.Savepoint(ctx, func(ctx context.Context) error {
		if d.Subscribers != nil {
			for _, h := range d.Subscribers.handlers[e.Type] {
				if err := h(ctx, e); err != nil {
					return fmt.Errorf("subscriber: %w", err)
				}
			}
		}
		q := eventsstore.New(pdb.MustTxFromContext(ctx))
		if _, err := q.CreateWebhookDeliveries(ctx, eventsstore.CreateWebhookDeliveriesParams{EventID: e.ID, EventType: e.Type}); err != nil {
			return fmt.Errorf("webhook deliveries: %w", err)
		}
		return q.MarkEventPublished(ctx, e.ID)
	})
}

// logFailure never logs the payload or the subscriber's error text beyond its message — both could
// carry data about a person (CLAUDE.md rule 3); the event id is enough to find it.
func (d *Dispatcher) logFailure(ctx context.Context, row eventsstore.LockUnpublishedEventsRow, attempts int32, err error) {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	attrs := []any{
		slog.String("event_id", row.ID.String()), slog.String("event_type", row.EventType),
		slog.String("tenant_id", row.TenantID.String()), slog.Int("attempts", int(attempts)),
		slog.String("error", err.Error()),
	}
	if attempts >= AlertAfterAttempts {
		logger.ErrorContext(ctx, "outbox event keeps failing; its aggregate is blocked", append(attrs, slog.String("alert", "outbox_event_stuck"))...)
		return
	}
	logger.WarnContext(ctx, "outbox event dispatch failed; retried on the next sweep", attrs...)
}

// SweepArgs is outbox.sweep: platform-wide (jobs.GlobalKinds), no tenant transaction.
type SweepArgs struct{}

func (SweepArgs) Kind() string { return "outbox.sweep" }

// Sweeper enqueues outbox.dispatch for every trial/active tenant (decisions D-21: the worker loops
// over platform.tenants instead of reading every tenant's outbox through a BYPASSRLS role).
// Uniqueness makes this a no-op for tenants whose dispatch is already queued or running.
type Sweeper struct {
	river.WorkerDefaults[SweepArgs]

	Pool *pgxpool.Pool
	// River enqueues the dispatch jobs; nil means the client working this job.
	River *river.Client[pgx.Tx]
}

func (s *Sweeper) Work(ctx context.Context, _ *river.Job[SweepArgs]) error {
	tenants, err := eventsstore.New(s.Pool).ListDispatchableTenants(ctx)
	if err != nil {
		return fmt.Errorf("outbox.sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: DispatchArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := s.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("outbox.sweep: enqueue: %w", err)
	}
	return nil
}

// SweepPeriodicJob runs outbox.sweep every minute on the leader (docs/architecture/integration.md).
func SweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) {
			return SweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: time.Minute}}
		},
		&river.PeriodicJobOpts{ID: "outbox-sweep", RunOnStart: true},
	)
}
