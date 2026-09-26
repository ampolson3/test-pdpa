package events_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/events/internal/catalogen"
	"pdpa-platform/internal/platform/jobs"
)

// catalog.gen.go must be exactly what docs/architecture/events.yaml generates (run `go generate`).
func TestCatalog_MatchesEventsYAML(t *testing.T) {
	want, err := catalogen.Render("../../../../docs/architecture/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("catalog.gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("catalog.gen.go is stale — run `go generate ./internal/platform/events`")
	}
	if len(events.Catalog) != 41 {
		t.Errorf("catalog has %d events, want 41", len(events.Catalog))
	}
	if !jobs.GlobalKinds[events.SweepArgs{}.Kind()] {
		t.Error("outbox.sweep must be registered in jobs.GlobalKinds")
	}
}

func TestPublish_RejectsEventsOutsideTheCatalog(t *testing.T) {
	p := &events.Publisher{}
	cases := map[string]events.Event{
		"unknown type":  {Type: "consent.teleported", AggregateType: "x", AggregateID: uuid.New(), Data: map[string]any{}},
		"missing field": {Type: "dsar.completed", AggregateType: "dsar_request", AggregateID: uuid.New(), Data: map[string]any{"request_no": "D-1"}},
		"extra field":   {Type: "user.provisioned", AggregateType: "user", AggregateID: uuid.New(), Data: map[string]any{"user_id": "u", "role_code": "DPO", "scope": "tenant", "email": "x@example.com"}},
		"no aggregate":  userProvisioned(uuid.Nil),
	}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			// Validation happens before anything touches the database, so no transaction is needed.
			_, err := p.Publish(context.Background(), e)
			if !errors.Is(err, events.ErrUnknownEvent) && !errors.Is(err, events.ErrInvalidData) {
				t.Errorf("got %v, want a catalog error", err)
			}
		})
	}
}

func userProvisioned(aggregate uuid.UUID) events.Event {
	return events.Event{
		Type: "user.provisioned", AggregateType: "user", AggregateID: aggregate,
		Data: map[string]any{"user_id": aggregate.String(), "role_code": "DPO", "scope": "tenant"},
	}
}

type fixture struct {
	pool      *pgxpool.Pool
	a, b      dbtest.Tenant
	publisher *events.Publisher
	river     *river.Client[pgx.Tx]
	now       time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	app := dbtest.Pool(t)
	platform := dbtest.PlatformPool(t)
	a := dbtest.SeedTenant(t, ctx, app, platform, "events-a")
	b := dbtest.SeedTenant(t, ctx, app, platform, "events-b")

	// Runs before SeedTenant's own cleanup (LIFO): remove what references the tenants first.
	t.Cleanup(func() {
		for _, tenant := range []dbtest.Tenant{a, b} {
			_ = pdb.WithTenantTx(context.Background(), app, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				for _, stmt := range []string{
					`DELETE FROM platform.webhook_deliveries`, `DELETE FROM platform.webhook_subscriptions`,
					`DELETE FROM iam.api_clients`, `DELETE FROM platform.outbox_events`,
				} {
					if _, err := tx.Exec(ctx, stmt); err != nil {
						return err
					}
				}
				return nil
			})
			_, _ = app.Exec(context.Background(), `DELETE FROM river_job WHERE kind = 'outbox.dispatch' AND args->>'tenant_id' = $1`, tenant.ID.String())
		}
	})

	client, err := jobs.NewInsertClient(app)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{pool: app, a: a, b: b, river: client, now: time.Date(2026, 9, 25, 3, 15, 0, 0, time.UTC)}
	f.publisher = &events.Publisher{River: client, Now: func() time.Time { return f.now }}
	return f
}

func (f *fixture) inTenant(t *testing.T, tenant dbtest.Tenant, fn func(ctx context.Context) error) error {
	t.Helper()
	return pdb.WithTenantTx(context.Background(), f.pool, tenant.ID.String(), "", fn)
}

func (f *fixture) publish(t *testing.T, tenant dbtest.Tenant, e events.Event) events.Event {
	t.Helper()
	var out events.Event
	if err := f.inTenant(t, tenant, func(ctx context.Context) error {
		var err error
		out, err = f.publisher.Publish(ctx, e)
		return err
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return out
}

// dispatch runs outbox.dispatch for tenant the way the worker does: inside that tenant's transaction.
func (f *fixture) dispatch(t *testing.T, tenant dbtest.Tenant, d *events.Dispatcher) {
	t.Helper()
	if err := f.inTenant(t, tenant, func(ctx context.Context) error {
		return d.Work(ctx, &river.Job[events.DispatchArgs]{})
	}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
}

type outboxRow struct {
	Published bool
	Attempts  int
}

func (f *fixture) outbox(t *testing.T, tenant dbtest.Tenant, id uuid.UUID) (outboxRow, bool) {
	t.Helper()
	var r outboxRow
	found := true
	_ = f.inTenant(t, tenant, func(ctx context.Context) error {
		err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT published_at IS NOT NULL, attempts FROM platform.outbox_events WHERE id = $1`, id).Scan(&r.Published, &r.Attempts)
		if errors.Is(err, pgx.ErrNoRows) {
			found = false
			return nil
		}
		return err
	})
	return r, found
}

func (f *fixture) countDispatchJobs(t *testing.T, tenant dbtest.Tenant) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'outbox.dispatch' AND args->>'tenant_id' = $1`, tenant.ID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// recorder is a subscriber that records every delivery and can be told to fail for some events.
type recorder struct {
	mu     sync.Mutex
	seen   []uuid.UUID
	failOn map[uuid.UUID]bool
}

func (r *recorder) handle(_ context.Context, e events.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, e.ID)
	if r.failOn[e.ID] {
		return errors.New("subscriber unavailable")
	}
	return nil
}

func (r *recorder) count(id uuid.UUID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, s := range r.seen {
		if s == id {
			n++
		}
	}
	return n
}

// Publish writes the outbox row and the dispatch job in the caller's transaction: both exist only
// if it commits, and a burst of events enqueues a single dispatch.
func TestPublish_FollowsTheCallersTransaction(t *testing.T) {
	f := setup(t)

	committed := f.publish(t, f.a, userProvisioned(uuid.New()))
	if committed.TenantID != f.a.ID || !committed.OccurredAt.Equal(f.now) {
		t.Errorf("stored event: tenant %s occurred %s, want %s / %s", committed.TenantID, committed.OccurredAt, f.a.ID, f.now)
	}
	f.publish(t, f.a, userProvisioned(uuid.New()))

	var rolledBack events.Event
	errBusiness := errors.New("business rule failed")
	err := f.inTenant(t, f.a, func(ctx context.Context) error {
		var err error
		if rolledBack, err = f.publisher.Publish(ctx, userProvisioned(uuid.New())); err != nil {
			return err
		}
		return errBusiness
	})
	if !errors.Is(err, errBusiness) {
		t.Fatalf("rollback path: %v", err)
	}

	if _, found := f.outbox(t, f.a, committed.ID); !found {
		t.Error("committed event missing from the outbox")
	}
	if _, found := f.outbox(t, f.a, rolledBack.ID); found {
		t.Error("rolled-back event is in the outbox")
	}
	if n := f.countDispatchJobs(t, f.a); n != 1 {
		t.Errorf("dispatch jobs for tenant A = %d, want 1 (unique per tenant)", n)
	}
}

// Dispatch hands the event to subscribers, creates a pending delivery for each active subscription
// of that type only, and marks it published.
func TestDispatch_DeliversToSubscribersAndWebhookSubscriptions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	var matching, paused, other uuid.UUID
	if err := f.inTenant(t, f.a, func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		var client uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO iam.api_clients (tenant_id, name, keycloak_client_id, scopes)
			VALUES ($1, 'crm', 'crm', '{}') RETURNING id`, f.a.ID).Scan(&client); err != nil {
			return err
		}
		insert := `INSERT INTO platform.webhook_subscriptions (tenant_id, api_client_id, url, event_types, secret_ref, status)
			VALUES ($1, $2, 'https://crm.example/hook', $3, 'kv/webhooks/test', $4) RETURNING id`
		if err := tx.QueryRow(ctx, insert, f.a.ID, client, []string{"user.provisioned", "access.granted"}, "active").Scan(&matching); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, insert, f.a.ID, client, []string{"user.provisioned"}, "paused").Scan(&paused); err != nil {
			return err
		}
		return tx.QueryRow(ctx, insert, f.a.ID, client, []string{"dsar.completed"}, "active").Scan(&other)
	}); err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	reg := events.NewRegistry()
	reg.Subscribe("user.provisioned", rec.handle)
	e := f.publish(t, f.a, userProvisioned(uuid.New()))
	f.dispatch(t, f.a, &events.Dispatcher{Subscribers: reg})

	if got := rec.count(e.ID); got != 1 {
		t.Errorf("subscriber called %d times, want 1", got)
	}
	if r, _ := f.outbox(t, f.a, e.ID); !r.Published || r.Attempts != 1 {
		t.Errorf("outbox row = %+v, want published after 1 attempt", r)
	}
	deliveries := map[uuid.UUID]string{}
	_ = f.inTenant(t, f.a, func(ctx context.Context) error {
		rows, err := pdb.MustTxFromContext(ctx).Query(ctx,
			`SELECT subscription_id, status FROM platform.webhook_deliveries WHERE event_id = $1`, e.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id uuid.UUID
			var status string
			if err := rows.Scan(&id, &status); err != nil {
				return err
			}
			deliveries[id] = status
		}
		return rows.Err()
	})
	if len(deliveries) != 1 || deliveries[matching] != "pending" {
		t.Errorf("deliveries = %v, want only subscription %s pending (paused %s and other-type %s excluded)", deliveries, matching, paused, other)
	}
	_ = ctx
}

// Acceptance criterion: "event ไม่สูญหายเมื่อระบบล่มกลางคัน (at-least-once + idempotent consumer)".
// A dispatch whose transaction never commits (the worker died) leaves the event unpublished; the
// next dispatch delivers it again, and a consumer that dedups by event id applies it once.
func TestDispatch_CrashMidwayLosesNothing(t *testing.T) {
	f := setup(t)
	rec := &recorder{}
	applied := map[uuid.UUID]int{}
	reg := events.NewRegistry()
	reg.Subscribe("user.provisioned", rec.handle)
	reg.Subscribe("user.provisioned", func(_ context.Context, e events.Event) error { // idempotent consumer
		if _, done := applied[e.ID]; !done {
			applied[e.ID] = 0
		}
		applied[e.ID]++
		return nil
	})
	e := f.publish(t, f.a, userProvisioned(uuid.New()))

	errCrash := errors.New("worker killed before commit")
	err := f.inTenant(t, f.a, func(ctx context.Context) error {
		if err := (&events.Dispatcher{Subscribers: reg}).Work(ctx, &river.Job[events.DispatchArgs]{}); err != nil {
			return err
		}
		return errCrash
	})
	if !errors.Is(err, errCrash) {
		t.Fatalf("crash run: %v", err)
	}
	if r, _ := f.outbox(t, f.a, e.ID); r.Published {
		t.Fatal("event marked published although the dispatch transaction never committed")
	}

	f.dispatch(t, f.a, &events.Dispatcher{Subscribers: reg})
	if r, _ := f.outbox(t, f.a, e.ID); !r.Published {
		t.Fatal("event still unpublished after the retry dispatch")
	}
	if got := rec.count(e.ID); got != 2 {
		t.Errorf("event delivered %d times, want 2 (at-least-once: before and after the crash)", got)
	}
	if len(applied) != 1 {
		t.Errorf("idempotent consumer recorded %d distinct events, want 1", len(applied))
	}
}

// A failing subscriber holds back its own aggregate (keeping order) but not other aggregates; the
// failed event is retried later and its aggregate's events then go out in order.
func TestDispatch_FailingEventBlocksOnlyItsAggregate(t *testing.T) {
	f := setup(t)
	aggA, aggB := uuid.New(), uuid.New()
	a1 := f.publish(t, f.a, userProvisioned(aggA))
	f.now = f.now.Add(time.Second)
	a2 := f.publish(t, f.a, userProvisioned(aggA))
	f.now = f.now.Add(time.Second)
	b1 := f.publish(t, f.a, userProvisioned(aggB))

	rec := &recorder{failOn: map[uuid.UUID]bool{a1.ID: true}}
	reg := events.NewRegistry()
	reg.Subscribe("user.provisioned", rec.handle)
	f.dispatch(t, f.a, &events.Dispatcher{Subscribers: reg, BatchSize: 2})

	if r, _ := f.outbox(t, f.a, a1.ID); r.Published || r.Attempts != 1 {
		t.Errorf("a1 = %+v, want unpublished with 1 failed attempt", r)
	}
	if r, _ := f.outbox(t, f.a, a2.ID); r.Published || rec.count(a2.ID) != 0 {
		t.Errorf("a2 = %+v delivered %d times, want held back behind a1", r, rec.count(a2.ID))
	}
	if r, _ := f.outbox(t, f.a, b1.ID); !r.Published {
		t.Error("b1 (another aggregate) was blocked by a1's failure")
	}

	rec.mu.Lock()
	rec.failOn = nil
	rec.seen = nil
	rec.mu.Unlock()
	f.dispatch(t, f.a, &events.Dispatcher{Subscribers: reg})
	for _, e := range []events.Event{a1, a2} {
		if r, _ := f.outbox(t, f.a, e.ID); !r.Published {
			t.Errorf("event %s still unpublished after recovery", e.ID)
		}
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.seen) != 2 || rec.seen[0] != a1.ID || rec.seen[1] != a2.ID {
		t.Errorf("recovery delivered %v, want a1 then a2", rec.seen)
	}
}

// Two-tenant isolation of the outbox (CLAUDE.md rule 1): tenant B's dispatcher cannot see, and so
// cannot publish, tenant A's events.
func TestDispatch_OnlySeesItsOwnTenantsOutbox(t *testing.T) {
	f := setup(t)
	rec := &recorder{}
	reg := events.NewRegistry()
	reg.Subscribe("user.provisioned", rec.handle)
	e := f.publish(t, f.a, userProvisioned(uuid.New()))

	f.dispatch(t, f.b, &events.Dispatcher{Subscribers: reg})
	if rec.count(e.ID) != 0 {
		t.Error("tenant B's dispatcher delivered tenant A's event")
	}
	if r, _ := f.outbox(t, f.a, e.ID); r.Published {
		t.Error("tenant B's dispatcher published tenant A's event")
	}
	if _, found := f.outbox(t, f.b, e.ID); found {
		t.Error("tenant A's outbox row is visible under tenant B")
	}
}

// The sweeper enqueues a dispatch per live tenant — how events stranded by a lost or skipped
// dispatch job get delivered.
func TestSweeper_EnqueuesDispatchPerTenant(t *testing.T) {
	f := setup(t)
	// The sweep enqueues for every live tenant in the database, not just this test's two.
	start := time.Now().Add(-time.Second)
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM river_job WHERE kind = 'outbox.dispatch' AND created_at >= $1`, start)
	})
	if err := (&events.Sweeper{Pool: f.pool, River: f.river}).Work(context.Background(), &river.Job[events.SweepArgs]{}); err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []dbtest.Tenant{f.a, f.b} {
		if n := f.countDispatchJobs(t, tenant); n != 1 {
			t.Errorf("tenant %s: %d dispatch jobs after sweep, want 1", tenant.ID, n)
		}
	}
}

func TestRegistry_RejectsUnknownEventType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Subscribe to an unknown event type did not panic")
		}
	}()
	events.NewRegistry().Subscribe("nope.happened", func(context.Context, events.Event) error { return nil })
}
