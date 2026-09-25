// Package db holds the pdpa_app connection pool and the single choke point for opening a
// tenant-scoped transaction. See CLAUDE.md rule 1 (tenant isolation) and
// docs/architecture/code-structure.md ("transaction model = unit of work").
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens a pgx pool. connString must authenticate as a role without BYPASSRLS
// (pdpa_app for the request pool, pdpa_platform for internal/platform/provider's pool).
func NewPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("db: parse pool config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: open pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return pool, nil
}
