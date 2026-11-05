// Package service writes and verifies platform.audit_log (PLT-12, CLAUDE.md rule 4): an
// append-only, per-tenant hash chain. The Tx middleware (#13 in docs/architecture/code-structure.md)
// appends one request-level entry per request in the request's own transaction, before commit;
// services append domain entries (with before/after) the same way through Write.
package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	store "pdpa-platform/internal/platform/audit/store"
)

// Entry is one row to append to platform.audit_log. ActorType/ActorID identify who did it;
// Before/After are optional domain snapshots for a state change (nil for a plain read) — pass them
// through Changes to keep only what changed. They must never contain personal data in the clear:
// mask first (CLAUDE.md rule 3).
type Entry struct {
	TenantID   uuid.UUID
	ActorType  string // user | api_client | guest | data_subject | system
	ActorID    *uuid.UUID
	Action     string // e.g. "GET /admin/v1/me", "dsar.request.approve"
	EntityType string
	EntityID   *uuid.UUID
	Before     any
	After      any
	IP         *netip.Addr
	UserAgent  string
}

type Service struct{}

func New() *Service { return &Service{} }

// Write appends one hash-chained row to platform.audit_log inside the transaction in ctx (opened by
// db.WithTenantTx). It takes the tenant's chain lock for the rest of that transaction, so call it as
// late as possible — the Tx middleware calls it right before COMMIT.
func (s *Service) Write(ctx context.Context, e Entry) error {
	q := store.New(pdb.MustTxFromContext(ctx))

	before, err := canonicalJSON(e.Before)
	if err != nil {
		return fmt.Errorf("audit: before: %w", err)
	}
	after, err := canonicalJSON(e.After)
	if err != nil {
		return fmt.Errorf("audit: after: %w", err)
	}

	if err := q.LockAuditChain(ctx, e.TenantID); err != nil {
		return fmt.Errorf("audit: lock chain: %w", err)
	}
	link, err := q.NextAuditChainLink(ctx, e.TenantID)
	if err != nil {
		return fmt.Errorf("audit: read chain head: %w", err)
	}

	r := record{
		TenantID: e.TenantID, OccurredAt: link.OccurredAt.Time, ActorType: e.ActorType, ActorID: e.ActorID,
		Action: e.Action, EntityType: e.EntityType, EntityID: e.EntityID, Before: before, After: after,
		IP: e.IP, UserAgent: e.UserAgent, PrevHash: link.PrevHash,
	}

	params := store.InsertAuditLogParams{
		TenantID:   e.TenantID,
		OccurredAt: pgtype.Timestamptz{Time: r.OccurredAt, Valid: true},
		ActorType:  e.ActorType,
		Action:     e.Action,
		Before:     before,
		After:      after,
		Ip:         e.IP,
		PrevHash:   link.PrevHash,
		Hash:       r.hash(),
	}
	if e.ActorID != nil {
		params.ActorID = pgtype.UUID{Bytes: *e.ActorID, Valid: true}
	}
	if e.EntityType != "" {
		params.EntityType = &e.EntityType
	}
	if e.EntityID != nil {
		params.EntityID = pgtype.UUID{Bytes: *e.EntityID, Valid: true}
	}
	if e.UserAgent != "" {
		params.UserAgent = &e.UserAgent
	}

	if _, err := q.InsertAuditLog(ctx, params); err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

// record is every column of an audit_log row that the hash covers — all of them except id (assigned
// by the database) and hash itself.
type record struct {
	TenantID   uuid.UUID
	OccurredAt time.Time
	ActorType  string
	ActorID    *uuid.UUID
	Action     string
	EntityType string
	EntityID   *uuid.UUID
	Before     []byte // canonical JSON or nil
	After      []byte
	IP         *netip.Addr
	UserAgent  string
	PrevHash   string
}

// hash = sha256 over every field, each length-prefixed so no two different rows can serialize to
// the same bytes. Anyone holding the rows can replay a tenant's chain (Verify) and find a row that
// was altered, inserted or deleted out of band.
func (r record) hash() string {
	h := sha256.New()
	field := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	optUUID := func(id *uuid.UUID) []byte {
		if id == nil {
			return nil
		}
		return []byte(id.String())
	}
	ip := []byte(nil)
	if r.IP != nil {
		ip = []byte(r.IP.String())
	}
	field([]byte("audit/v2"))
	field([]byte(r.PrevHash))
	field([]byte(r.TenantID.String()))
	field([]byte(r.OccurredAt.UTC().Format("2006-01-02T15:04:05.000000Z")))
	field([]byte(r.ActorType))
	field(optUUID(r.ActorID))
	field([]byte(r.Action))
	field([]byte(r.EntityType))
	field(optUUID(r.EntityID))
	field(r.Before)
	field(r.After)
	field(ip)
	field([]byte(r.UserAgent))
	return hex.EncodeToString(h.Sum(nil))
}

// canonicalJSON renders v the same way whether it comes from Go values at write time or back out of
// jsonb at verify time (which reorders keys and drops whitespace): decode with UseNumber, re-encode
// (encoding/json sorts map keys). nil stays nil.
func canonicalJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	raw, ok := v.([]byte)
	if !ok {
		var err error
		if raw, err = json.Marshal(v); err != nil {
			return nil, err
		}
	}
	if raw == nil || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	return json.Marshal(generic)
}

// Changes reduces two snapshots of the same entity (structs or maps that marshal to JSON objects) to
// the top-level fields that differ — the "before/after (JSON diff)" PLT-12 asks for. A field present
// on one side only appears on that side. Either argument may be nil (create / delete).
func Changes(before, after any) (map[string]any, map[string]any, error) {
	b, err := toObject(before)
	if err != nil {
		return nil, nil, fmt.Errorf("audit: before: %w", err)
	}
	a, err := toObject(after)
	if err != nil {
		return nil, nil, fmt.Errorf("audit: after: %w", err)
	}
	outB, outA := map[string]any{}, map[string]any{}
	for k, bv := range b {
		if av, ok := a[k]; !ok || !reflect.DeepEqual(av, bv) {
			outB[k] = bv
		}
	}
	for k, av := range a {
		if bv, ok := b[k]; !ok || !reflect.DeepEqual(av, bv) {
			outA[k] = av
		}
	}
	if before == nil {
		outB = nil
	}
	if after == nil {
		outA = nil
	}
	return outB, outA, nil
}

func toObject(v any) (map[string]any, error) {
	out := map[string]any{}
	if v == nil {
		return out, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("snapshot must be a JSON object: %w", err)
	}
	return out, nil
}
