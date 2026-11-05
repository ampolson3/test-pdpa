// DFG-05 (แก้ไขแผนผังแบบลากวาง / drag & drop editor): dataflow.layouts was already fully specified in the
// baseline migration (00009), unused until now — one jsonb overlay per (tenant, view_type, scope_id,
// user_id), a personal arrangement layered on top of DFG-01/03/04/06's own live-computed Graph, never
// persisted as part of it. The acceptance criterion ("ตำแหน่งที่จัดเองยังอยู่หลังข้อมูลอัปเดต") holds by
// construction: every position/note is keyed by a DataflowNode's own stable id string
// ("activity:<uuid>", "org_unit:<uuid>", ...), which a RoPA data refresh never changes for an activity
// that still exists — so GetLayout's own overlay keeps applying to the exact same nodes after the graph
// is recomputed, with nothing to go stale. A node the overlay still names but the current graph no longer
// has (the activity was deleted/ended) is simply never drawn — a harmless, self-healing orphan, not an
// error, and not worth a cleanup job: the next SaveLayout call naturally replaces the whole overlay anyway.
package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dataflowstore "pdpa-platform/internal/dataflow/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// ErrVersionMismatch is returned when the caller's If-Match no longer matches the saved overlay's own
// row_version — the same sentinel/HTTP-412 shape every other ETag-guarded write in this codebase uses.
var ErrVersionMismatch = errors.New("dataflow: layout version mismatch")

// NodePosition is one dragged node's own x/y, keyed by its DataflowNode.id in Layout.Positions.
type NodePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Layout is DFG-05's own saved overlay for one (view, scope) diagram — never merged into Graph itself,
// applied by the caller (the frontend) on top of the live graph it already fetched.
type Layout struct {
	Positions  map[string]NodePosition
	Notes      map[string]string
	RowVersion int32
	UpdatedAt  pgtype.Timestamptz
}

type layoutJSON struct {
	Positions map[string]NodePosition `json:"positions,omitempty"`
	Notes     map[string]string       `json:"notes,omitempty"`
}

// GetLayout reads the caller's own saved overlay for this (legal_entity, view) diagram, or the empty
// defaults (row_version 0) if nothing has been saved yet — the same "defaults until first save"
// convention ORG-20's own org_settings already established, so the very first PUT can carry If-Match: "0".
func (s *Service) GetLayout(ctx context.Context, legalEntityID *uuid.UUID, view View) (Layout, error) {
	if view == "" {
		view = ViewDepartment
	}
	q := dataflowstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetLayout(ctx, dataflowstore.GetLayoutParams{ViewType: string(view), ScopeID: dfPgUUID(legalEntityID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Layout{Positions: map[string]NodePosition{}, Notes: map[string]string{}}, nil
	}
	if err != nil {
		return Layout{}, err
	}
	return toLayout(row.Layout, row.RowVersion, row.UpdatedAt)
}

// SaveLayout is a full replace of the overlay (never a field-by-field merge) — the caller sends every
// position/note it wants kept, including ones it didn't itself just change. expectedRowVersion is the
// caller's own If-Match ("0" for a diagram never saved before).
func (s *Service) SaveLayout(ctx context.Context, legalEntityID *uuid.UUID, view View, positions map[string]NodePosition, notes map[string]string, expectedRowVersion int32) (Layout, error) {
	if view == "" {
		view = ViewDepartment
	}
	if positions == nil {
		positions = map[string]NodePosition{}
	}
	if notes == nil {
		notes = map[string]string{}
	}
	raw, err := json.Marshal(layoutJSON{Positions: positions, Notes: notes})
	if err != nil {
		return Layout{}, err
	}
	q := dataflowstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.UpsertLayout(ctx, dataflowstore.UpsertLayoutParams{
		ViewType: string(view), ScopeID: dfPgUUID(legalEntityID), Layout: raw, ExpectedRowVersion: expectedRowVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Layout{}, ErrVersionMismatch
	}
	if err != nil {
		return Layout{}, err
	}
	return toLayout(row.Layout, row.RowVersion, row.UpdatedAt)
}

// ResetLayout is "คืนค่าเดิม" — discard the caller's own saved overlay, reverting the diagram to its
// automatic ELK layout with no annotations. A harmless no-op if nothing was saved.
func (s *Service) ResetLayout(ctx context.Context, legalEntityID *uuid.UUID, view View) error {
	if view == "" {
		view = ViewDepartment
	}
	q := dataflowstore.New(pdb.MustTxFromContext(ctx))
	return q.DeleteLayout(ctx, dataflowstore.DeleteLayoutParams{ViewType: string(view), ScopeID: dfPgUUID(legalEntityID)})
}

func toLayout(raw []byte, rowVersion int32, updatedAt pgtype.Timestamptz) (Layout, error) {
	var lj layoutJSON
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &lj); err != nil {
			return Layout{}, err
		}
	}
	if lj.Positions == nil {
		lj.Positions = map[string]NodePosition{}
	}
	if lj.Notes == nil {
		lj.Notes = map[string]string{}
	}
	return Layout{Positions: lj.Positions, Notes: lj.Notes, RowVersion: rowVersion, UpdatedAt: updatedAt}, nil
}
