// DPO-08 (compliance calendar): a forward-looking read of every vendor's own reassessment date, distinct
// from the VEN-10 sweep's own "already due" filter.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	vendorstore "pdpa-platform/internal/vendormgmt/store"
)

// UpcomingReassessment is one vendor's own next_assessment_at, for DPO-08's own compliance calendar.
type UpcomingReassessment struct {
	VendorID uuid.UUID
	Title    string
	DueAt    time.Time
}

// ListUpcomingReassessments returns every approved/conditional vendor whose own next_assessment_at falls
// within [from, to].
func (s *Service) ListUpcomingReassessments(ctx context.Context, from, to time.Time) ([]UpcomingReassessment, error) {
	rows, err := vendorstore.New(pdb.MustTxFromContext(ctx)).ListVendorsWithReassessmentWindow(ctx, vendorstore.ListVendorsWithReassessmentWindowParams{
		NextAssessmentAt: pgtype.Date{Time: from, Valid: true}, NextAssessmentAt_2: pgtype.Date{Time: to, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := make([]UpcomingReassessment, 0, len(rows))
	for _, r := range rows {
		out = append(out, UpcomingReassessment{VendorID: r.ID, Title: r.ServiceDescription, DueAt: r.NextAssessmentAt.Time})
	}
	return out, nil
}
