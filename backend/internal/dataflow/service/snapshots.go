package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	dataflowstore "pdpa-platform/internal/dataflow/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// ViewTypeOrgUnit is the only view DFG-01/03/04 build (org-unit swimlanes) — the one dataflow.snapshots'
// own view_type column (no CHECK constraint, unlike dataflow.layouts) carries until DFG-06's own
// multi-view feature adds others.
const ViewTypeOrgUnit = "org_unit"

var ErrSnapshotNotFound = errors.New("dataflow: snapshot not found")

// SnapshotSummary is one row of DFG-07's own version history — the graph itself is left out (ListSnapshots)
// and read on demand (GetSnapshot), same split as every other module's own list/detail pair.
type SnapshotSummary struct {
	ID          uuid.UUID
	VersionNo   int32
	PublishedBy *uuid.UUID
	PublishedAt time.Time
	CreatedAt   time.Time
}

// Snapshot is one published version, graph included.
type Snapshot struct {
	SnapshotSummary
	Graph Graph
}

// PublishSnapshot is DFG-07's own acceptance criterion's write half: freeze the current live Graph (the
// exact same DFG-01/03/04 computation every other read of this module already uses) as a new, immutable
// version — wrapped in pdb.Savepoint since two concurrent publishes of the same (view_type, scope_id)
// could still race the real unique constraint, the same pattern org.SaveLegalEntity/ropa's own
// SaveActivity already use for a write that can hit a real DB constraint.
func (s *Service) PublishSnapshot(ctx context.Context, legalEntityID *uuid.UUID) (Snapshot, error) {
	g, err := s.Graph(ctx, legalEntityID)
	if err != nil {
		return Snapshot{}, err
	}
	graphJSON, err := json.Marshal(g)
	if err != nil {
		return Snapshot{}, err
	}

	var row dataflowstore.InsertSnapshotRow
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		q := dataflowstore.New(pdb.MustTxFromContext(ctx))
		var err error
		row, err = q.InsertSnapshot(ctx, dataflowstore.InsertSnapshotParams{
			ViewType: ViewTypeOrgUnit, ScopeID: dfPgUUID(legalEntityID), Graph: graphJSON,
		})
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Snapshot{}, fmt.Errorf("%w: concurrent publish, retry", ErrInvalid)
	}
	if err != nil {
		return Snapshot{}, err
	}
	return toSnapshot(row.ID, row.VersionNo, row.PublishedBy, row.PublishedAt, row.CreatedAt, g), nil
}

// ListSnapshots is DFG-07's own acceptance criterion's read half, "ดูแผนผังที่ publish ย้อนหลังได้" —
// every published version of this scope's own diagram, newest first.
func (s *Service) ListSnapshots(ctx context.Context, legalEntityID *uuid.UUID) ([]SnapshotSummary, error) {
	q := dataflowstore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListSnapshots(ctx, dataflowstore.ListSnapshotsParams{ViewType: ViewTypeOrgUnit, ScopeID: dfPgUUID(legalEntityID)})
	if err != nil {
		return nil, err
	}
	out := make([]SnapshotSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, SnapshotSummary{ID: r.ID, VersionNo: r.VersionNo, PublishedBy: dfUUIDPtr(r.PublishedBy),
			PublishedAt: r.PublishedAt.Time, CreatedAt: r.CreatedAt.Time})
	}
	return out, nil
}

// GetSnapshot reads one past published version's full graph, by id — RLS already scopes it to this
// tenant, so an unknown or cross-tenant id reads as ErrSnapshotNotFound.
func (s *Service) GetSnapshot(ctx context.Context, id uuid.UUID) (Snapshot, error) {
	q := dataflowstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetSnapshot(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, ErrSnapshotNotFound
	}
	if err != nil {
		return Snapshot{}, err
	}
	var g Graph
	if err := json.Unmarshal(row.Graph, &g); err != nil {
		return Snapshot{}, err
	}
	return toSnapshot(row.ID, row.VersionNo, row.PublishedBy, row.PublishedAt, row.CreatedAt, g), nil
}

func toSnapshot(id uuid.UUID, versionNo int32, publishedBy pgtype.UUID, publishedAt, createdAt pgtype.Timestamptz, g Graph) Snapshot {
	return Snapshot{
		SnapshotSummary: SnapshotSummary{ID: id, VersionNo: versionNo, PublishedBy: dfUUIDPtr(publishedBy),
			PublishedAt: publishedAt.Time, CreatedAt: createdAt.Time},
		Graph: g,
	}
}

func dfPgUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func dfUUIDPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	u := uuid.UUID(v.Bytes)
	return &u
}
