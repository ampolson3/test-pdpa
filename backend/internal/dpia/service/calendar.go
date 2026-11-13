package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// UpcomingReview is one approved DPIA round's own next_review_at, for DPO-08's own compliance calendar.
type UpcomingReview struct {
	AssessmentID uuid.UUID
	ActivityID   *uuid.UUID
	Title        string
	DueAt        time.Time
}

// ListUpcomingReviews is DPO-08's own read: every approved DPIA round whose own next_review_at falls
// within [from, to] — the column was already in the baseline schema (assess.assessments), unused by any
// sweep yet.
func (s *Service) ListUpcomingReviews(ctx context.Context, from, to time.Time) ([]UpcomingReview, error) {
	rows, err := dpiastore.New(pdb.MustTxFromContext(ctx)).ListUpcomingDpiaReviews(ctx, dpiastore.ListUpcomingDpiaReviewsParams{
		NextReviewAt: pgDate(&from), NextReviewAt_2: pgDate(&to),
	})
	if err != nil {
		return nil, err
	}
	out := make([]UpcomingReview, 0, len(rows))
	for _, r := range rows {
		item := UpcomingReview{AssessmentID: r.ID, Title: r.Title, DueAt: r.NextReviewAt.Time}
		if r.ActivityID.Valid {
			id := uuid.UUID(r.ActivityID.Bytes)
			item.ActivityID = &id
		}
		out = append(out, item)
	}
	return out, nil
}
