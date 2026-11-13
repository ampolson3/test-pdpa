// DPIA-13 (re-assessment triggers): linking an incident to the RoPA processing activity it affected is the
// breach-side half of this feature — breach.incident_assets was already fully specified in the baseline
// migrations (00013_breach.sql) but completely unused until now. Linking publishes breach.activity_linked
// (PLT-11 outbox); internal/dpia's own subscriber (internal/wiring/events.go) marks that activity's latest
// approved DPIA round needs_review (ST-05#2's own already-declared edge) — breach never writes to the assess
// schema directly (rule 9).
package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	breachstore "pdpa-platform/internal/breach/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/events"
)

// IncidentActivity is one link row.
type IncidentActivity struct {
	ID         uuid.UUID
	IncidentID uuid.UUID
	ActivityID uuid.UUID
}

// LinkActivity records that incident affected the given RoPA processing activity. The activity must be
// visible under the caller's own RLS (rule 1) — checked via s.Ropa.GetActivity, never a raw FK write. A
// redelivery/double-click for an already-linked pair is a harmless no-op (migration 00074's own
// idempotency index + ON CONFLICT DO NOTHING).
func (s *Service) LinkActivity(ctx context.Context, incidentID, activityID uuid.UUID) (IncidentActivity, error) {
	if !has(ctx, PermUpdate) {
		return IncidentActivity{}, ErrForbidden
	}
	incident, err := s.Get(ctx, incidentID)
	if err != nil {
		return IncidentActivity{}, err
	}
	if s.Ropa == nil {
		return IncidentActivity{}, ErrInvalidRequest
	}
	if _, err := s.Ropa.GetActivity(ctx, activityID); err != nil {
		return IncidentActivity{}, ErrInvalidRequest
	}

	q := breachstore.New(pdb.MustTxFromContext(ctx))
	id := uuid.New()
	row, err := q.InsertIncidentActivity(ctx, breachstore.InsertIncidentActivityParams{
		ID: id, IncidentID: incidentID, ActivityID: pgUUID(&activityID),
	})
	// ON CONFLICT DO NOTHING returns no row on an already-linked pair — a harmless no-op, not an error.
	alreadyLinked := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !alreadyLinked {
		return IncidentActivity{}, err
	}
	linked := IncidentActivity{ID: id, IncidentID: incidentID, ActivityID: activityID}
	if !alreadyLinked {
		linked.ID = row.ID
	}
	if alreadyLinked {
		return linked, nil
	}

	if s.Events != nil {
		_, _ = s.Events.Publish(ctx, events.Event{
			Type: "breach.activity_linked", AggregateType: IncidentType, AggregateID: incidentID,
			Data: map[string]any{"incident_ref": incident.No, "activity_ref": activityID.String()},
		})
	}
	_ = s.audit(ctx, "breach.incident.link_activity", IncidentType, incidentID, nil, map[string]any{"activity_id": activityID})
	return linked, nil
}

// ListIncidentActivities lists every RoPA activity linked to this incident, oldest first.
func (s *Service) ListIncidentActivities(ctx context.Context, incidentID uuid.UUID) ([]IncidentActivity, error) {
	if !has(ctx, PermRead) {
		return nil, ErrForbidden
	}
	if _, err := s.Get(ctx, incidentID); err != nil {
		return nil, err
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListIncidentActivities(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	out := make([]IncidentActivity, 0, len(rows))
	for _, r := range rows {
		if !r.ActivityID.Valid {
			continue
		}
		out = append(out, IncidentActivity{ID: r.ID, IncidentID: r.IncidentID, ActivityID: uuid.UUID(r.ActivityID.Bytes)})
	}
	return out, nil
}
