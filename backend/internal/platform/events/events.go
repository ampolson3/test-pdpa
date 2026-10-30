// Package events is the platform's domain-event layer (PLT-11, ADR outbox, decisions D-21):
//
//   - Publisher.Publish writes an event to platform.outbox_events inside the caller's own
//     transaction and enqueues outbox.dispatch for the tenant in that same transaction, so an event
//     exists if and only if the business change that caused it commits (CLAUDE.md rules 5 and 6).
//   - Dispatcher (job outbox.dispatch, one tenant transaction per run) hands each unpublished event
//     to the in-process subscribers registered for its type and creates one pending
//     platform.webhook_deliveries row per active subscription (sent by webhook.deliver, PLT-15),
//     then marks it published — all in the job's transaction.
//   - Sweeper (job outbox.sweep, every minute) enqueues outbox.dispatch for every live tenant, so
//     events whose dispatch was skipped, failed or lost are picked up again.
//
// Delivery is at-least-once: a crash before the dispatcher's transaction commits leaves the event
// unpublished and it is dispatched again. Subscribers therefore must be idempotent by Event.ID; one
// that only writes inside the dispatch transaction it is given gets that for free, since its writes
// commit or roll back together with the published mark.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	eventsstore "pdpa-platform/internal/platform/events/store"
	"pdpa-platform/internal/platform/jobs"
)

// Event is a domain event. Type must be in Catalog and Data must carry exactly the catalog's fields
// for that type — identifiers and codes only, never personal data beyond what the catalog lists
// (docs/architecture/integration.md: payload uses subject_ref, not raw identifiers).
type Event struct {
	ID            uuid.UUID // set by Publish (UUIDv7)
	TenantID      uuid.UUID // set by Publish from the transaction's tenant
	Type          string
	AggregateType string // envelope "subject" type, e.g. "dsar_request"
	AggregateID   uuid.UUID
	Data          map[string]any
	Version       int       // payload version; 0 means 1
	OccurredAt    time.Time // set by Publish
}

// payload is what platform.outbox_events.payload holds; id, type, tenant and time have columns.
type payload struct {
	Version int            `json:"version"`
	Data    map[string]any `json:"data"`
}

// ErrUnknownEvent / ErrInvalidData reject events that don't match docs/architecture/events.yaml.
var (
	ErrUnknownEvent = errors.New("events: event type is not in the catalog")
	ErrInvalidData  = errors.New("events: data does not match the catalog fields")
)

// Publisher writes events to the outbox. River is the insert-only client (jobs.NewInsertClient) in
// cmd/api or the worker's own client inside a job; Now is injectable for tests.
type Publisher struct {
	River *river.Client[pgx.Tx]
	Now   func() time.Time
}

// Publish records e in the outbox within the transaction in ctx and enqueues outbox.dispatch for
// the tenant. It returns the event as stored (ID, TenantID, OccurredAt filled in).
func (p *Publisher) Publish(ctx context.Context, e Event) (Event, error) {
	if err := validate(e); err != nil {
		return Event{}, err
	}
	if e.Version == 0 {
		e.Version = 1
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	e.OccurredAt = now().UTC()
	id, err := uuid.NewV7()
	if err != nil {
		return Event{}, err
	}
	e.ID = id

	body, err := json.Marshal(payload{Version: e.Version, Data: e.Data})
	if err != nil {
		return Event{}, fmt.Errorf("events: encode %s: %w", e.Type, err)
	}

	q := eventsstore.New(pdb.MustTxFromContext(ctx))
	e.TenantID, err = q.InsertOutboxEvent(ctx, eventsstore.InsertOutboxEventParams{
		ID:            e.ID,
		AggregateType: e.AggregateType,
		AggregateID:   e.AggregateID,
		EventType:     e.Type,
		Payload:       body,
		OccurredAt:    pgtype.Timestamptz{Time: e.OccurredAt, Valid: true},
	})
	if err != nil {
		return Event{}, fmt.Errorf("events: write outbox %s: %w", e.Type, err)
	}

	if _, err := jobs.Enqueue(ctx, p.River, DispatchArgs{TenantArgs: jobs.TenantArgs{TenantID: e.TenantID.String()}}, nil); err != nil {
		return Event{}, err
	}
	return e, nil
}

func validate(e Event) error {
	spec, ok := Catalog[e.Type]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownEvent, e.Type)
	}
	if e.AggregateType == "" || e.AggregateID == uuid.Nil {
		return fmt.Errorf("%w: %s needs an aggregate type and id", ErrInvalidData, e.Type)
	}
	want := map[string]bool{}
	for _, f := range spec.Data {
		want[f] = true
	}
	var missing, extra []string
	for f := range want {
		if _, ok := e.Data[f]; !ok {
			missing = append(missing, f)
		}
	}
	for f := range e.Data {
		if !want[f] {
			extra = append(extra, f)
		}
	}
	if len(missing) > 0 || len(extra) > 0 {
		sort.Strings(missing)
		sort.Strings(extra)
		return fmt.Errorf("%w: %s missing [%s] unexpected [%s]", ErrInvalidData, e.Type,
			strings.Join(missing, ", "), strings.Join(extra, ", "))
	}
	return nil
}

// Handler is an in-process subscriber. It runs inside the dispatch transaction (in its own
// savepoint), reads that transaction with db.MustTxFromContext, and must be idempotent by e.ID.
// Per CLAUDE.md rule 9 it writes only its own module's schema.
type Handler func(ctx context.Context, e Event) error

// Registry holds the in-process subscribers, keyed by event type. Modules register theirs at
// start-up in cmd/worker, before the worker client starts.
type Registry struct {
	handlers map[string][]Handler
}

func NewRegistry() *Registry { return &Registry{handlers: map[string][]Handler{}} }

// Subscribe registers h for eventType, which must be in the catalog.
func (r *Registry) Subscribe(eventType string, h Handler) {
	if _, ok := Catalog[eventType]; !ok {
		panic(fmt.Sprintf("events: subscribe to unknown event type %q", eventType))
	}
	r.handlers[eventType] = append(r.handlers[eventType], h)
}
