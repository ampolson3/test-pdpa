package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	store "pdpa-platform/internal/platform/audit/store"
)

// VerifyResult is the outcome of replaying one tenant's chain.
type VerifyResult struct {
	Checked  int
	OK       bool
	BrokenAt int64  // id of the first row that fails, 0 when OK
	Reason   string // "hash_mismatch" (row altered) or "prev_hash_mismatch" (row removed, inserted or reordered)
}

// Verify replays the chain of the transaction's tenant from its first row, oldest first: every row's
// prev_hash must equal the previous row's hash (the first row's must be empty, or the newest retention
// anchor's hash after a purge) and its hash must
// equal the hash recomputed from its columns. It reads in pages, so it works on chains of any size.
func (s *Service) Verify(ctx context.Context, tenantID uuid.UUID) (VerifyResult, error) {
	q := store.New(pdb.MustTxFromContext(ctx))
	const pageSize = 1000

	res := VerifyResult{OK: true}
	// After a retention purge (decisions.md D-22) the oldest surviving row chains to the last purged one,
	// whose hash the purge recorded; with no purge the chain starts from nothing.
	prev, err := q.LatestAuditAnchor(ctx, tenantID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return VerifyResult{}, fmt.Errorf("audit: verify anchor: %w", err)
	}
	after := pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), InfinityModifier: pgtype.NegativeInfinity, Valid: true}
	afterID := int64(math.MinInt64)
	for {
		rows, err := q.ListAuditChainPage(ctx, store.ListAuditChainPageParams{
			TenantID: tenantID, AfterOccurredAt: after, AfterID: afterID, PageSize: pageSize,
		})
		if err != nil {
			return VerifyResult{}, fmt.Errorf("audit: verify: %w", err)
		}
		for _, row := range rows {
			res.Checked++
			if row.PrevHash != prev {
				return VerifyResult{Checked: res.Checked, BrokenAt: row.ID, Reason: "prev_hash_mismatch"}, nil
			}
			r := record{
				TenantID: tenantID, OccurredAt: row.OccurredAt.Time, ActorType: row.ActorType,
				ActorID: optUUID(row.ActorID), Action: row.Action, EntityID: optUUID(row.EntityID),
				IP: row.Ip, PrevHash: row.PrevHash,
			}
			if row.EntityType != nil {
				r.EntityType = *row.EntityType
			}
			if row.UserAgent != nil {
				r.UserAgent = *row.UserAgent
			}
			if r.Before, err = canonicalJSON(row.Before); err != nil {
				return VerifyResult{}, fmt.Errorf("audit: verify row %d before: %w", row.ID, err)
			}
			if r.After, err = canonicalJSON(row.After); err != nil {
				return VerifyResult{}, fmt.Errorf("audit: verify row %d after: %w", row.ID, err)
			}
			if r.hash() != row.Hash {
				return VerifyResult{Checked: res.Checked, BrokenAt: row.ID, Reason: "hash_mismatch"}, nil
			}
			prev = row.Hash
			after, afterID = row.OccurredAt, row.ID
		}
		if len(rows) < pageSize {
			return res, nil
		}
	}
}

func optUUID(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}
