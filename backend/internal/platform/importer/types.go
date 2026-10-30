// Package importer is the asynchronous bulk-import framework (PLT-14): Excel (.xlsx) or CSV, column
// mapping, row-by-row validation as a dry run with a downloadable error report, then an all-or-nothing
// import of the rows that passed. Modules register import types; the framework owns the job, the file
// handling and the state machine (PLT-14 in docs/states/state-machines.yaml).
package importer

import (
	"context"
	"strings"
)

// Column is one field an import type reads.
type Column struct {
	Key      string            // e.g. "holiday_date"
	Label    map[string]string // by language, shown in the mapping UI and matched against headers
	Required bool
	// Aliases are other header texts that map to this column automatically (case-insensitive).
	Aliases []string
}

// Row is one data row, keyed by Column.Key, values trimmed.
type Row map[string]string

// FieldError is one problem with one row, for the error report. Message is in the import type's
// words; it must not quote the cell's value (rows may carry personal data, and the report is a file).
type FieldError struct {
	Column  string
	Message string
}

// Type is a kind of import a module offers.
type Type struct {
	// Permission the caller needs to start, map, confirm and read imports of this type.
	Permission string
	Columns    []Column
	// Validate checks one row (line = 1-based line in the file, header = 1). It runs in the job's tenant
	// transaction and may read the database (references, duplicates). No writes.
	Validate func(ctx context.Context, line int, row Row) []FieldError
	// Apply writes one row that passed Validate, in the import's single transaction.
	Apply func(ctx context.Context, line int, row Row) error
}

// Registry holds the import types of the process.
type Registry map[string]Type

// suggestMapping maps each column to the first header that equals its key, a label or an alias.
func (t Type) suggestMapping(headers []string) map[string]string {
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	out := map[string]string{}
	for _, c := range t.Columns {
		names := []string{c.Key}
		for _, l := range c.Label {
			names = append(names, l)
		}
		names = append(names, c.Aliases...)
		for _, h := range headers {
			for _, n := range names {
				if norm(h) == norm(n) {
					out[c.Key] = h
					break
				}
			}
			if out[c.Key] != "" {
				break
			}
		}
	}
	return out
}
