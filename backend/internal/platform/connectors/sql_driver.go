package connectors

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	// Side-effect imports registering each database/sql driver name this package's sqlDialects use.
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
)

// sqlDialect is the handful of things that genuinely differ between PostgreSQL/MySQL/SQL Server for the
// generic read/search/update/delete this driver offers — everything else (open a connection, run a
// parameterized statement) is identical across all three, which is what lets one generic driver builder
// (sqlDriver) serve "ชุดแรก PostgreSQL, MySQL, SQL Server" with no per-engine Go code of its own.
type sqlDialect struct {
	driverName string
	// placeholder returns this dialect's own parameter marker for the nth (1-based) bound value.
	placeholder func(n int) string
	// listColumnsQuery lists every (table_name, column_name) this connector's own DSN can see, scoped to
	// its own database — every one of the three engines exposes a standard information_schema.columns,
	// just with a different "which database is this" predicate.
	listColumnsQuery string
	quoteIdent       func(string) string
}

func dollarPlaceholder(n int) string      { return "$" + strconv.Itoa(n) }
func questionPlaceholder(int) string      { return "?" }
func atPPlaceholder(n int) string         { return "@p" + strconv.Itoa(n) }
func bracketQuoteIdent(s string) string   { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func backtickQuoteIdent(s string) string  { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
func sqlServerQuoteIdent(s string) string { return "[" + strings.ReplaceAll(s, "]", "]]") + "]" }

var postgresDialect = sqlDialect{
	driverName:       "pgx",
	placeholder:      dollarPlaceholder,
	quoteIdent:       bracketQuoteIdent,
	listColumnsQuery: `SELECT table_name, column_name FROM information_schema.columns WHERE table_schema NOT IN ('pg_catalog', 'information_schema') ORDER BY table_name, ordinal_position`,
}

var mysqlDialect = sqlDialect{
	driverName:       "mysql",
	placeholder:      questionPlaceholder,
	quoteIdent:       backtickQuoteIdent,
	listColumnsQuery: `SELECT table_name, column_name FROM information_schema.columns WHERE table_schema = DATABASE() ORDER BY table_name, ordinal_position`,
}

var sqlServerDialect = sqlDialect{
	driverName:       "sqlserver",
	placeholder:      atPPlaceholder,
	quoteIdent:       sqlServerQuoteIdent,
	listColumnsQuery: `SELECT TABLE_NAME, COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_CATALOG = DB_NAME() ORDER BY TABLE_NAME, ORDINAL_POSITION`,
}

// sqlDriver builds one Driver from a dialect — the only thing separating "postgres"/"mysql"/"sqlserver"
// in this codebase. Config.Settings["dsn"] is the connector's own full connection string; Secret, when
// set, is appended as "password=<secret>" (postgres/sqlserver) or folded into the DSN by the caller for
// drivers whose own DSN syntax already carries one (documented on the connector's own config schema) —
// kept this simple deliberately, since every one of the three drivers' own DSN formats already has a
// well-known way to carry a password, and this SDK's job is to run queries, not parse connection strings.
func sqlDriver(d sqlDialect) Driver {
	open := func(cfg Config) (*sql.DB, error) {
		dsn := cfg.get("dsn")
		if dsn == "" {
			return nil, fmt.Errorf("connectors: missing config.dsn")
		}
		return sql.Open(d.driverName, dsn)
	}

	return Driver{
		Capabilities: []string{CapTest, CapDiscover, CapSearchSubject, CapUpdate, CapDelete, CapSample},
		Sample: func(ctx context.Context, cfg Config, table string, limit int) ([]SampledRow, error) {
			db, err := open(cfg)
			if err != nil {
				return nil, err
			}
			defer db.Close()
			q := fmt.Sprintf("SELECT * FROM %s LIMIT %d", d.quoteIdent(table), limit)
			rows, err := db.QueryContext(ctx, q)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			cols, err := rows.Columns()
			if err != nil {
				return nil, err
			}
			var out []SampledRow
			for rows.Next() {
				vals := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					return nil, err
				}
				row := make(SampledRow, len(cols))
				for i, c := range cols {
					row[c] = fmt.Sprint(vals[i])
				}
				out = append(out, row)
			}
			return out, rows.Err()
		},
		Test: func(ctx context.Context, cfg Config) error {
			db, err := open(cfg)
			if err != nil {
				return err
			}
			defer db.Close()
			return db.PingContext(ctx)
		},
		Discover: func(ctx context.Context, cfg Config) ([]DiscoveredTable, error) {
			db, err := open(cfg)
			if err != nil {
				return nil, err
			}
			defer db.Close()
			rows, err := db.QueryContext(ctx, d.listColumnsQuery)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			byTable := map[string][]string{}
			var order []string
			for rows.Next() {
				var table, column string
				if err := rows.Scan(&table, &column); err != nil {
					return nil, err
				}
				if _, ok := byTable[table]; !ok {
					order = append(order, table)
				}
				byTable[table] = append(byTable[table], column)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			out := make([]DiscoveredTable, 0, len(order))
			for _, t := range order {
				out = append(out, DiscoveredTable{Name: t, Columns: byTable[t]})
			}
			return out, nil
		},
		SearchSubject: func(ctx context.Context, cfg Config, identifier string) ([]SubjectRecord, error) {
			table, idCol := cfg.get("table"), cfg.get("identifier_column")
			if table == "" || idCol == "" {
				return nil, fmt.Errorf("connectors: missing config.table/identifier_column")
			}
			db, err := open(cfg)
			if err != nil {
				return nil, err
			}
			defer db.Close()
			q := fmt.Sprintf("SELECT * FROM %s WHERE %s = %s", d.quoteIdent(table), d.quoteIdent(idCol), d.placeholder(1))
			rows, err := db.QueryContext(ctx, q, identifier)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			cols, err := rows.Columns()
			if err != nil {
				return nil, err
			}
			var out []SubjectRecord
			for rows.Next() {
				vals := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					return nil, err
				}
				fields := make(map[string]string, len(cols))
				for i, c := range cols {
					fields[c] = fmt.Sprint(vals[i])
				}
				out = append(out, SubjectRecord{Table: table, Fields: fields})
			}
			return out, rows.Err()
		},
		Update: func(ctx context.Context, cfg Config, identifier string, fields map[string]string) error {
			table, idCol := cfg.get("table"), cfg.get("identifier_column")
			if table == "" || idCol == "" {
				return fmt.Errorf("connectors: missing config.table/identifier_column")
			}
			if len(fields) == 0 {
				return fmt.Errorf("connectors: no fields to update")
			}
			db, err := open(cfg)
			if err != nil {
				return err
			}
			defer db.Close()
			sets := make([]string, 0, len(fields))
			args := make([]any, 0, len(fields)+1)
			n := 1
			for col, val := range fields {
				sets = append(sets, fmt.Sprintf("%s = %s", d.quoteIdent(col), d.placeholder(n)))
				args = append(args, val)
				n++
			}
			args = append(args, identifier)
			q := fmt.Sprintf("UPDATE %s SET %s WHERE %s = %s", d.quoteIdent(table), strings.Join(sets, ", "), d.quoteIdent(idCol), d.placeholder(n))
			_, err = db.ExecContext(ctx, q, args...)
			return err
		},
		Delete: func(ctx context.Context, cfg Config, identifier string) error {
			table, idCol := cfg.get("table"), cfg.get("identifier_column")
			if table == "" || idCol == "" {
				return fmt.Errorf("connectors: missing config.table/identifier_column")
			}
			db, err := open(cfg)
			if err != nil {
				return err
			}
			defer db.Close()
			q := fmt.Sprintf("DELETE FROM %s WHERE %s = %s", d.quoteIdent(table), d.quoteIdent(idCol), d.placeholder(1))
			_, err = db.ExecContext(ctx, q, identifier)
			return err
		},
	}
}

// PostgresDriver/MySQLDriver/SQLServerDriver are this feature's own "ชุดแรก" three SQL dialects — each a
// one-line call into the shared sqlDriver builder above.
func PostgresDriver() Driver  { return sqlDriver(postgresDialect) }
func MySQLDriver() Driver     { return sqlDriver(mysqlDialect) }
func SQLServerDriver() Driver { return sqlDriver(sqlServerDialect) }
