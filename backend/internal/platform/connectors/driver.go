// Package connectors is PLT-20's own Connector SDK: a Driver per connector_type (postgres/mysql/
// sqlserver/rest, this feature's own "ชุดแรก"), each implementing whichever of this module's five
// capabilities (test/discover/search_subject/update/delete) it supports. The acceptance criterion itself
// ("เพิ่ม connector ใหม่ได้โดยไม่แก้โมดูลหลัก") holds by construction: Service (service.go) never
// switches on connector_type at all — it only ever looks a Driver up in its own Registry, so a brand-new
// connector type is exactly one new Driver value plus one new map entry in internal/wiring.Connectors(),
// the same shape internal/platform/importer's own Type/Registry already established for PLT-14.
package connectors

import (
	"context"
	"errors"
)

// ErrUnsupported is returned by a capability a Driver doesn't implement (a nil func field) — DSAR-09/12's
// own "ใช้ร่วม" callers must expect this for, e.g., a REST connector asked to Discover.
var ErrUnsupported = errors.New("connectors: capability not supported by this driver")

// Config is what a Driver call needs to reach the customer's own system: the connector's own config
// jsonb, decoded, plus its decrypted secret (empty string if the connector has none).
type Config struct {
	Settings map[string]string
	Secret   string
}

func (c Config) get(key string) string { return c.Settings[key] }

// DiscoveredTable is one table/collection a Discover call found, with its own column/field names —
// deliberately this narrow (no types, no row counts): CON-04's own scanner and DSAR-09's own "ค้นหา"
// screen only ever need to know where personal data *might* live, not a full schema document.
type DiscoveredTable struct {
	Name    string
	Columns []string
}

// SubjectRecord is one row/object a SearchSubject call found, naming its own table/collection.
type SubjectRecord struct {
	Table  string
	Fields map[string]string
}

// Driver is the connector SDK's own interface — a plain struct of optional functions rather than a Go
// `interface`, so a driver can leave any capability nil (ErrUnsupported) without a stub method; Service
// checks Capabilities before ever calling one, so a nil func is read as "the connector_type's own
// Capabilities already say so," never as an unexpected nil-pointer call.
type Driver struct {
	// Capabilities this connector type is able to offer at all (a given *connector* may still narrow this
	// further via its own `connectors.capabilities` column, e.g. a read-only replica with no update/delete).
	Capabilities []string

	Test          func(ctx context.Context, cfg Config) error
	Discover      func(ctx context.Context, cfg Config) ([]DiscoveredTable, error)
	SearchSubject func(ctx context.Context, cfg Config, identifier string) ([]SubjectRecord, error)
	Update        func(ctx context.Context, cfg Config, identifier string, fields map[string]string) error
	Delete        func(ctx context.Context, cfg Config, identifier string) error
}

// Registry is every connector_type this process knows how to drive — built once in
// internal/wiring.Connectors() and shared by cmd/api and cmd/worker, the same convention
// importer.Registry already established.
type Registry map[string]Driver

const (
	CapTest          = "test"
	CapDiscover      = "discover"
	CapSearchSubject = "search_subject"
	CapUpdate        = "update"
	CapDelete        = "delete"
)
