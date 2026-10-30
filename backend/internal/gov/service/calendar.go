// DPO-08 (compliance calendar): a forward-looking read of every training assignment's own due date,
// joined with its course's title — not a reminder job, just a window read for the calendar view.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	govstore "pdpa-platform/internal/gov/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// UpcomingTrainingDue is one training assignment's own due date, for DPO-08's own compliance calendar.
type UpcomingTrainingDue struct {
	AssignmentID uuid.UUID
	Title        string
	DueAt        time.Time
}

// ListUpcomingTrainingDueDates returns every assignment whose own due_at falls within [from, to].
func (s *Service) ListUpcomingTrainingDueDates(ctx context.Context, from, to time.Time) ([]UpcomingTrainingDue, error) {
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListUpcomingTrainingDueDates(ctx, govstore.ListUpcomingTrainingDueDatesParams{
		DueAt: pgDateOpt(&from), DueAt_2: pgDateOpt(&to),
	})
	if err != nil {
		return nil, err
	}
	out := make([]UpcomingTrainingDue, 0, len(rows))
	for _, r := range rows {
		out = append(out, UpcomingTrainingDue{AssignmentID: r.ID, Title: r.Title, DueAt: r.DueAt.Time})
	}
	return out, nil
}

// OrgUnitTrainingCompletion is DPO-10's own training factor: how many of an org unit's own training
// assignments have at least one passed attempt.
func (s *Service) OrgUnitTrainingCompletion(ctx context.Context, orgUnitID uuid.UUID) (total, completed int, err error) {
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).CountOrgUnitTrainingCompletion(ctx, pgUUIDPtr(&orgUnitID))
	if err != nil {
		return 0, 0, err
	}
	return int(row.Total), int(row.Completed), nil
}
