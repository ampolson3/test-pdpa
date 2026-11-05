// Command devseed creates the demo organization and admin user that the admin app's dev login signs in as
// (AUTH_DEV_LOGIN, apps/admin/src/lib/dev-auth.ts) — for running the platform locally without Keycloak. It is
// idempotent and uses fixed ids, so .env.example can name them. Never run it against a real deployment.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	pdb "pdpa-platform/internal/pkg/db"
)

var (
	// DemoTenantID and DemoUserID match AUTH_DEV_TENANT_ID / AUTH_DEV_USER_ID in .env.example.
	DemoTenantID = uuid.MustParse("0199a000-0000-7000-8000-00000000d001")
	DemoUserID   = uuid.MustParse("0199a000-0000-7000-8000-00000000d002")
	demoRoles    = []string{"ORGADMIN", "DPO"}
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("devseed", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	platform, err := pdb.NewPool(ctx, envOr("PLATFORM_DATABASE_URL", "postgres://pdpa_platform:pdpa_platform@localhost:5432/pdpa?sslmode=disable"))
	if err != nil {
		return err
	}
	defer platform.Close()
	app, err := pdb.NewPool(ctx, envOr("DATABASE_URL", "postgres://pdpa_app:pdpa_app@localhost:5432/pdpa?sslmode=disable"))
	if err != nil {
		return err
	}
	defer app.Close()

	// platform.tenants is written only by the provider role (10-grants.sql).
	if _, err := platform.Exec(ctx, `INSERT INTO platform.tenants (id, code, name, plan_code) VALUES ($1, 'demo', 'องค์กรตัวอย่าง (dev)', 'free')
		ON CONFLICT (id) DO NOTHING`, DemoTenantID); err != nil {
		return fmt.Errorf("tenant: %w", err)
	}
	err = pdb.WithTenantTx(ctx, app, DemoTenantID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if _, err := tx.Exec(ctx, `INSERT INTO iam.users (id, tenant_id, email, display_name, status) VALUES ($1, $2, 'admin@demo.example', 'ผู้ดูแลระบบตัวอย่าง', 'active')
			ON CONFLICT (id) DO NOTHING`, DemoUserID, DemoTenantID); err != nil {
			return fmt.Errorf("user: %w", err)
		}
		for _, code := range demoRoles {
			var roleID uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT id FROM iam.roles WHERE code = $1 AND tenant_id IS NULL`, code).Scan(&roleID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("role %s not found — run make migrate first", code)
				}
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
				SELECT $1, $2, $3, 'tenant'
				WHERE NOT EXISTS (SELECT 1 FROM iam.role_assignments WHERE user_id = $2 AND role_id = $3)`, DemoTenantID, DemoUserID, roleID); err != nil {
				return fmt.Errorf("role %s: %w", code, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("demo tenant %s, admin user %s (roles %v) ready\n", DemoTenantID, DemoUserID, demoRoles)
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
