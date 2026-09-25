// Package migrations embeds the goose SQL migration files so cmd/migrate can run them from a
// single compiled binary with no filesystem dependency at deploy time.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
