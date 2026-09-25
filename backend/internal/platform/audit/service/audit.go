// Package service computes the platform.audit_log hash chain (CLAUDE.md rule 4: audit_log is
// append-only and tamper-evident) and writes the one request-level entry the Tx middleware (#13 in
// docs/architecture/code-structure.md) makes for every request, in the same transaction as the
// handler, before commit.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	store "pdpa-platform/internal/platform/audit/store"
)

// Entry is one row to append to platform.audit_log. ActorType/ActorID identify who did it;
// Before/After are optional domain snapshots for a state change (nil for a plain read like
// GET /admin/v1/me). Before/After must never contain personal data in the clear — mask first
// (CLAUDE.md rule 3).
type Entry struct {
	TenantID   uuid.UUID
	ActorType  string // user | api_client | guest | data_subject | system
	ActorID    *uuid.UUID
	Action     string // e.g. "iam.me.read", "dsar.request.approve"
	EntityType string
	EntityID   *uuid.UUID
	Before     any
	After      any
	IP         *netip.Addr
	UserAgent  string
}

type Service struct{}

func New() *Service { return &Service{} }

// Write appends one hash-chained row to platform.audit_log. Must run inside the same transaction
// that will commit the request/job, opened by db.WithTenantTx, so the read of the previous hash and
// the insert of the next one are atomic per tenant.
func (s *Service) Write(ctx context.Context, e Entry) error {
	q := store.New(pdb.MustTxFromContext(ctx))

	prevHash, err := q.GetLatestAuditHash(ctx, e.TenantID)
	if err != nil {
		return fmt.Errorf("audit: read latest hash: %w", err)
	}

	before, err := marshalOrNil(e.Before)
	if err != nil {
		return fmt.Errorf("audit: marshal before: %w", err)
	}
	after, err := marshalOrNil(e.After)
	if err != nil {
		return fmt.Errorf("audit: marshal after: %w", err)
	}

	hash := chainHash(prevHash, e.TenantID, e.ActorType, e.Action, e.EntityType, before, after)

	params := store.InsertAuditLogParams{
		TenantID:  e.TenantID,
		ActorType: e.ActorType,
		Action:    e.Action,
		Before:    before,
		After:     after,
		Column11:  prevHash,
		Hash:      hash,
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
	if e.IP != nil {
		params.Ip = e.IP
	}
	if e.UserAgent != "" {
		params.UserAgent = &e.UserAgent
	}

	if _, err := q.InsertAuditLog(ctx, params); err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

func marshalOrNil(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// chainHash links each row to the one before it for this tenant: hash = sha256(prev_hash || the
// row's own immutable fields). Anyone can replay the chain from 00001 to detect a row that was
// altered or deleted out of band.
func chainHash(prevHash string, tenantID uuid.UUID, actorType, action, entityType string, before, after []byte) string {
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write([]byte(tenantID.String()))
	h.Write([]byte(actorType))
	h.Write([]byte(action))
	h.Write([]byte(entityType))
	h.Write(before)
	h.Write(after)
	return hex.EncodeToString(h.Sum(nil))
}
