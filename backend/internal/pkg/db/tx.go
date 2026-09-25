package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txCtxKey struct{}

// TxFromContext returns the transaction opened by WithTenantTx for the current request or job.
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txCtxKey{}).(pgx.Tx)
	return tx, ok
}

// MustTxFromContext panics if called outside a transaction opened by WithTenantTx. Stores and
// services must read the transaction this way and must never call pool.Begin themselves
// (CLAUDE.md rule 1) — a panic here means that invariant was broken upstream.
func MustTxFromContext(ctx context.Context) pgx.Tx {
	tx, ok := TxFromContext(ctx)
	if !ok {
		panic("db: no transaction in context — call sites must run inside WithTenantTx")
	}
	return tx
}

// WithTenantTx is the only place a transaction may be opened against pool: the Tx middleware calls
// it once per HTTP request and the worker job wrapper calls it once per job. It begins a
// transaction, scopes row-level security to tenantID/userID for the lifetime of the transaction via
// the parameterised form of SET LOCAL (set_config(..., true)), then runs fn with the transaction
// attached to the context. A non-nil error from fn rolls the transaction back; nil commits it.
// userID may be "" for system-initiated work with no acting user — the RLS policies read it via
// NULLIF(current_setting('app.user_id', true), ”)::uuid.
func WithTenantTx(ctx context.Context, pool *pgxpool.Pool, tenantID, userID string, fn func(ctx context.Context) error) (err error) {
	if tenantID == "" {
		return errors.New("db: WithTenantTx requires a tenant id")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
			return
		}
		err = tx.Commit(ctx)
	}()

	if _, err = tx.Exec(ctx,
		`SELECT set_config('app.tenant_id', $1, true), set_config('app.user_id', $2, true)`,
		tenantID, userID,
	); err != nil {
		return fmt.Errorf("db: set tenant context: %w", err)
	}

	return fn(context.WithValue(ctx, txCtxKey{}, tx))
}
