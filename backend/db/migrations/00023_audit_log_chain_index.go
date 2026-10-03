package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

// 00023 (PLT-12): index for reading a tenant's audit chain in order — the chain head on every audited
// request (audit service NextAuditChainLink) and the verifier's pages. platform.audit_log is
// partitioned by month and the set of partitions differs per environment, so this is a Go migration
// following CLAUDE.md rule 13: CREATE INDEX … ON ONLY the parent, CONCURRENTLY on each partition,
// then ATTACH PARTITION. Partitions created later by platform.ensure_monthly_partitions() get the
// index automatically from the parent. Not in a transaction (CONCURRENTLY can't run in one).
func init() {
	goose.AddNamedMigrationNoTxContext("00023_audit_log_chain_index.go", upAuditLogChainIndex, downAuditLogChainIndex)
}

const auditChainIndex = "ix_platform_audit_log_chain"

func upAuditLogChainIndex(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS `+auditChainIndex+
		` ON ONLY platform.audit_log (tenant_id, occurred_at, id)`); err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
		WHERE i.inhparent = 'platform.audit_log'::regclass ORDER BY c.relname`)
	if err != nil {
		return err
	}
	var partitions []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		partitions = append(partitions, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range partitions {
		idx := p + "_chain_idx"
		// A failed earlier run can leave an INVALID index behind; drop it so it is rebuilt.
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`
			DO $$ BEGIN
				IF EXISTS (SELECT 1 FROM pg_index x JOIN pg_class c ON c.oid = x.indexrelid
				           JOIN pg_namespace n ON n.oid = c.relnamespace
				           WHERE n.nspname = 'platform' AND c.relname = %s AND NOT x.indisvalid) THEN
					EXECUTE 'DROP INDEX platform.%s';
				END IF;
			END $$`, quoteLiteral(idx), idx)); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf(
			`CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON platform.%s (tenant_id, occurred_at, id)`, idx, p)); err != nil {
			return fmt.Errorf("index %s: %w", p, err)
		}
		var attached bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_inherits WHERE inhrelid = ('platform.' || $1)::regclass
			               AND inhparent = ('platform.' || $2)::regclass)`, idx, auditChainIndex).Scan(&attached); err != nil {
			return err
		}
		if !attached {
			if _, err := db.ExecContext(ctx, fmt.Sprintf(
				`ALTER INDEX platform.%s ATTACH PARTITION platform.%s`, auditChainIndex, idx)); err != nil {
				return fmt.Errorf("attach %s: %w", idx, err)
			}
		}
	}
	return nil
}

func downAuditLogChainIndex(ctx context.Context, db *sql.DB) error {
	// Dropping the parent's partitioned index drops every attached partition index with it.
	_, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS platform.`+auditChainIndex)
	return err
}

func quoteLiteral(s string) string { return "'" + s + "'" }
