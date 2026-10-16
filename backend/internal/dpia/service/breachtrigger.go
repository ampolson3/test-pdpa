package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// MarkActivityAssessmentForReview is DPIA-13's own half of the breach -> dpia link: when breach's own
// LinkActivity (BRE, via the breach.activity_linked outbox event) says an incident affected a RoPA
// processing activity, the activity's latest DPIA round — if it was already approved — is marked
// needs_review (ST-05#2's own "ครบรอบ / กิจกรรมเปลี่ยน" edge, already implemented by DPIA-10's own
// Transition/assessTransitions allow-list; needs_review is not in approverOnly, so this worker-context call
// needs no assessment.dpia.approve grant). A round that was never approved (screening/not_required/
// in_progress/etc.) or an activity with no DPIA at all has nothing to re-open — a harmless no-op, the same
// idempotent-external-trigger shape risktrigger.go's own TriggerFromRiskScore already established.
func (s *Service) MarkActivityAssessmentForReview(ctx context.Context, activityID uuid.UUID, reason string) error {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetLatestAssessmentForActivity(ctx, pgUUID(&activityID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.Status != "approved" {
		return nil
	}
	_, err = s.Transition(ctx, row.ID, row.RowVersion, "needs_review", reason)
	return err
}
