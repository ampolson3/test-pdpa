// Package dbtest gives every module's repository tests a consistent way to run against a real
// Postgres and prove tenant isolation — CLAUDE.md rule 1: "Every new repository gets a two-tenant
// isolation test." docs/architecture/code-structure.md's testing table calls for testcontainers-go
// here; that isn't wired up yet (needs Docker, not available while this was written — see
// CLAUDE.md's Scaffold status), so this package instead points at a real Postgres via
// TEST_DATABASE_URL / TEST_PLATFORM_DATABASE_URL and skips cleanly when neither is set. Replacing
// the connection source with testcontainers later shouldn't need to change any test that uses this
// package's API.
package dbtest

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	pdb "pdpa-platform/internal/pkg/db"
)

const (
	defaultAppURL      = "postgres://pdpa_app:pdpa_app@localhost:5433/pdpa?sslmode=disable"
	defaultPlatformURL = "postgres://pdpa_platform:pdpa_platform@localhost:5433/pdpa?sslmode=disable"
)

// Pool opens a pool as pdpa_app (the role backend/cmd/api and backend/cmd/worker connect as — no
// BYPASSRLS), or skips the test if TEST_DATABASE_URL is unset and the built-in local default
// (matching README.md's "Local Postgres โดยไม่ใช้ Docker") isn't reachable.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return connect(t, "TEST_DATABASE_URL", defaultAppURL)
}

// PlatformPool opens a pool as pdpa_platform — the only role allowed to write platform.tenants
// (deploy/db/10-grants.sql), needed here purely to seed a tenant for a test to run against.
// Production code must never use this pool outside internal/platform/provider (CLAUDE.md rule 1).
func PlatformPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return connect(t, "TEST_PLATFORM_DATABASE_URL", defaultPlatformURL)
}

func connect(t *testing.T, envVar, fallback string) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv(envVar)
	if dsn == "" {
		dsn = fallback
	}

	ctx := context.Background()
	pool, err := pdb.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("dbtest: no reachable Postgres (set %s to run this test): %v", envVar, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Tenant is a tenant + one user seeded inside it, ready for a test to open a WithTenantTx against.
type Tenant struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

// SeedTenant creates a tenant (via platformPool, the only role allowed to) and one iam.user inside
// it (via appPool, under RLS for that tenant), and registers cleanup to remove both — in that
// order, since iam.users.tenant_id references platform.tenants. codeSuffix must be unique per test
// (it becomes part of the tenant's code, which has a uniqueness constraint).
func SeedTenant(t *testing.T, ctx context.Context, appPool, platformPool *pgxpool.Pool, codeSuffix string) Tenant {
	t.Helper()

	tenantID := uuid.New()
	code := fmt.Sprintf("dbtest-%s-%s", codeSuffix, tenantID.String()[:8])

	if _, err := platformPool.Exec(ctx,
		`INSERT INTO platform.tenants (id, code, name, plan_code) VALUES ($1, $2, $3, 'free')`,
		tenantID, code, "dbtest "+codeSuffix,
	); err != nil {
		t.Fatalf("dbtest: seed tenant: %v", err)
	}
	t.Cleanup(func() {
		if _, err := platformPool.Exec(context.Background(),
			`DELETE FROM platform.tenants WHERE id = $1`, tenantID,
		); err != nil {
			t.Logf("dbtest: cleanup tenant %s: %v (fine if a later test still references it)", tenantID, err)
		}
	})

	var userID uuid.UUID
	err := pdb.WithTenantTx(ctx, appPool, tenantID.String(), "", func(ctx context.Context) error {
		tx, _ := pdb.TxFromContext(ctx)
		return tx.QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name) VALUES ($1, $2, $3) RETURNING id`,
			tenantID, fmt.Sprintf("%s@dbtest.example", codeSuffix), "dbtest "+codeSuffix,
		).Scan(&userID)
	})
	if err != nil {
		t.Fatalf("dbtest: seed user: %v", err)
	}
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), appPool, tenantID.String(), "", func(ctx context.Context) error {
			tx, _ := pdb.TxFromContext(ctx)
			_, err := tx.Exec(ctx, `DELETE FROM iam.users WHERE id = $1`, userID)
			return err
		})
	})

	return Tenant{ID: tenantID, UserID: userID}
}
