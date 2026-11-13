// DPO-08 (compliance calendar): a forward-looking read of every notice's own next_review_at, distinct from
// PNG-15's own sweep "already due" filter. Returns dpo/service's own CalendarItem directly — notice already
// imports dpo/service (PNG-07's OpenConsentTask), so dpo's own local Notice interface is satisfied here
// with no reverse import or adapter needed.
package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dposervice "pdpa-platform/internal/dpo/service"
	noticestore "pdpa-platform/internal/notice/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// ListUpcomingReviews returns every published notice whose own next_review_at falls within [from, to].
func (s *Service) ListUpcomingReviews(ctx context.Context, from, to time.Time) ([]dposervice.CalendarItem, error) {
	rows, err := noticestore.New(pdb.MustTxFromContext(ctx)).ListNoticesWithReviewWindow(ctx, noticestore.ListNoticesWithReviewWindowParams{
		NextReviewAt: pgtype.Date{Time: from, Valid: true}, NextReviewAt_2: pgtype.Date{Time: to, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := make([]dposervice.CalendarItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, dposervice.CalendarItem{Source: "notice_review", Title: r.Title, DueAt: r.NextReviewAt.Time, RefID: r.ID})
	}
	return out, nil
}
