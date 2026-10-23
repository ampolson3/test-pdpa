package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	ropastore "pdpa-platform/internal/ropa/store"
)

// ActivityRejection is one DSAR rejection logged against a processing activity (ROPA-10, ม.39(7)) — recorded
// automatically from the `dsar.rejected` event DSAR-11 publishes (internal/wiring's outbox subscriber),
// never written directly by any HTTP endpoint.
type ActivityRejection struct {
	ID            uuid.UUID
	ActivityID    uuid.UUID
	DsarRequestID uuid.UUID
	ReasonCode    string
	RejectedAt    time.Time
}

// RecordRejection logs a DSAR rejection against activityID. Idempotent by (activity_id, dsar_request_id) —
// the unique index a redelivery of the same `dsar.rejected` event hits, so a retried subscriber call is a
// harmless no-op rather than a duplicate row (events are delivered at-least-once, PLT-11).
func (s *Service) RecordRejection(ctx context.Context, activityID, dsarRequestID uuid.UUID, reasonCode string, rejectedAt time.Time) error {
	if _, err := s.GetActivity(ctx, activityID); err != nil {
		return fmt.Errorf("%w: activity_id", ErrInvalid)
	}
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		return fmt.Errorf("%w: reason_code", ErrInvalid)
	}
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).InsertActivityRejection(ctx, ropastore.InsertActivityRejectionParams{
		ActivityID: activityID, DsarRequestID: dsarRequestID, ReasonCode: reasonCode, RejectedAt: pgtype.Timestamptz{Time: rejectedAt, Valid: true},
	})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil // already recorded (idempotent redelivery)
	}
	return s.audit(ctx, "ropa.activity.rejection_logged", activityID, nil, map[string]any{"dsar_request_id": dsarRequestID, "reason_code": reasonCode})
}

func (s *Service) ListActivityRejections(ctx context.Context, activityID uuid.UUID) ([]ActivityRejection, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListActivityRejections(ctx, activityID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityRejection, 0, len(rows))
	for _, r := range rows {
		out = append(out, ActivityRejection{ID: r.ID, ActivityID: r.ActivityID, DsarRequestID: r.DsarRequestID, ReasonCode: r.ReasonCode, RejectedAt: r.RejectedAt.Time})
	}
	return out, nil
}
