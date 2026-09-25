// Command migrate applies the database schema for one environment: goose migrations, then River's
// own queue tables, then deploy/db/10-grants.sql — exactly the sequence in
// docs/architecture/deployment.md "Database migration". It connects as pdpa_migrator with
// options=-c role=pdpa_owner, so every object it creates is owned by pdpa_owner, never by the
// login role itself (CLAUDE.md rule 13). deploy/db/00-bootstrap.sql is a separate, one-time,
// superuser step per environment and is intentionally not run here.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"pdpa-platform/db/migrations"
)

func main() {
	down := flag.Bool("down", false, "roll back the most recent migration instead of applying pending ones")
	grantsPath := flag.String("grants", "deploy/db/10-grants.sql", "path to the grants SQL file, applied after migrations")
	flag.Parse()

	if err := run(*down, *grantsPath); err != nil {
		slog.Error("migrate: fatal", "error", err)
		os.Exit(1)
	}
}

func run(down bool, grantsPath string) error {
	ctx := context.Background()
	dsn := migratorDSN()

	if err := runGoose(dsn, down); err != nil {
		return fmt.Errorf("goose: %w", err)
	}
	slog.Info("migrate: goose done")

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("river: connect: %w", err)
	}
	defer pool.Close()

	if err := runRiver(ctx, pool, down); err != nil {
		return fmt.Errorf("river: %w", err)
	}
	slog.Info("migrate: river done")

	if !down {
		if err := applySQLFile(ctx, dsn, grantsPath); err != nil {
			return fmt.Errorf("grants: %w", err)
		}
		slog.Info("migrate: grants applied", "file", grantsPath)
	}

	return nil
}

// migratorDSN appends the parameterised SET ROLE (docs/architecture/deployment.md: "เชื่อมต่อเป็น
// pdpa_migrator ด้วย options=-c role=pdpa_owner") to DATABASE_URL / MIGRATOR_DATABASE_URL.
func migratorDSN() string {
	dsn := os.Getenv("MIGRATOR_DATABASE_URL")
	if dsn == "" {
		dsn = envOr("DATABASE_URL", "postgres://pdpa_migrator:pdpa_migrator@localhost:5432/pdpa?sslmode=disable")
	}
	sep := "?"
	if len(dsn) > 0 && contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "options=-c%20role%3Dpdpa_owner"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func runGoose(dsn string, down bool) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	if down {
		return goose.Down(db, ".")
	}
	return goose.Up(db, ".")
}

func runRiver(ctx context.Context, pool *pgxpool.Pool, down bool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}

	direction := rivermigrate.DirectionUp
	opts := &rivermigrate.MigrateOpts{}
	if down {
		direction = rivermigrate.DirectionDown
		opts.TargetVersion = -1
	}

	_, err = migrator.Migrate(ctx, direction, opts)
	return err
}

// applySQLFile runs the whole file as one script over the simple query protocol, which (unlike the
// extended protocol pgx uses by default) allows multiple ;-separated statements in one Exec — grants
// files have no bind parameters, so there is no injection concern in using it here.
func applySQLFile(ctx context.Context, dsn, path string) error {
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return err
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, string(sqlBytes))
	return err
}
