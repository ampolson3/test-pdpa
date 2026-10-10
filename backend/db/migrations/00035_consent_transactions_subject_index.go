package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

// 00035 (CON-15): a data subject's consent history, newest first, on the monthly-partitioned
// consent.consent_transactions — CREATE INDEX … ON ONLY the parent, CONCURRENTLY per partition, then ATTACH
// (CLAUDE.md rule 13, same steps as 00023). Later partitions inherit the index from the parent.
func init() {
	goose.AddNamedMigrationNoTxContext("00035_consent_transactions_subject_index.go", upConsentSubjectIndex, downConsentSubjectIndex)
}

const consentSubjectIndex = "ix_consent_transactions_subject_time"

func upConsentSubjectIndex(ctx context.Context, db *sql.DB) error {
	return partitionedIndex(ctx, db, "consent", "consent_transactions", consentSubjectIndex, "subject_time_idx", "(tenant_id, subject_id, occurred_at DESC)")
}

func downConsentSubjectIndex(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS consent.`+consentSubjectIndex)
	return err
}

// partitionedIndex builds parentIndex on schema.table and every existing partition without locking writes.
func partitionedIndex(ctx context.Context, db *sql.DB, schema, table, parentIndex, suffix, columns string) error {
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON ONLY %s.%s %s`, parentIndex, schema, table, columns)); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
		WHERE i.inhparent = ($1 || '.' || $2)::regclass ORDER BY c.relname`, schema, table)
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
		idx := p + "_" + suffix
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`
			DO $$ BEGIN
				IF EXISTS (SELECT 1 FROM pg_index x JOIN pg_class c ON c.oid = x.indexrelid
				           JOIN pg_namespace n ON n.oid = c.relnamespace
				           WHERE n.nspname = %s AND c.relname = %s AND NOT x.indisvalid) THEN
					EXECUTE 'DROP INDEX %s.%s';
				END IF;
			END $$`, quoteLiteral(schema), quoteLiteral(idx), schema, idx)); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON %s.%s %s`, idx, schema, p, columns)); err != nil {
			return fmt.Errorf("index %s: %w", p, err)
		}
		var attached bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_inherits WHERE inhrelid = ($1 || '.' || $2)::regclass
			               AND inhparent = ($1 || '.' || $3)::regclass)`, schema, idx, parentIndex).Scan(&attached); err != nil {
			return err
		}
		if !attached {
			if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER INDEX %s.%s ATTACH PARTITION %s.%s`, schema, parentIndex, schema, idx)); err != nil {
				return fmt.Errorf("attach %s: %w", idx, err)
			}
		}
	}
	return nil
}
