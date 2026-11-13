package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	dpostore "pdpa-platform/internal/dpo/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// HasTaskForActivity is RTG-10's own dedup check — the same shape HasTaskForRisk (DPIA-08)/
// HasTaskForVendorAgreement (DPA-12) already established: true once any dpo.tasks row already names this
// activity as its source (source_type "ropa_gap" — the activity's own template went out of date, not a
// particular version), so the daily sweep never opens a second task for an activity already being chased.
func (s *Service) HasTaskForActivity(ctx context.Context, activityID uuid.UUID) (bool, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListTasksBySource(ctx, dpostore.ListTasksBySourceParams{SourceType: "ropa_gap", SourceID: pgUUID(&activityID)})
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// OpenTemplateReviewTask opens one dpo.tasks job ("RTG-<year>-NNNN", source_type "ropa_gap" — already in
// the table's own CHECK constraint, unused until now) for RTG-10's own acceptance criterion: an activity
// created from a template that has since been reissued at a newer version (a new PDPC announcement, say)
// gets a real tracked review task, not just a notification nobody owns.
func (s *Service) OpenTemplateReviewTask(ctx context.Context, activityID uuid.UUID, title, description string) (uuid.UUID, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	year := strconv.Itoa(time.Now().UTC().Year())
	if err := q.LockTaskNumbering(ctx, year); err != nil {
		return uuid.Nil, err
	}
	prefix := "RTG-" + year + "-"
	n, err := q.CountTasksInYear(ctx, prefix)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	g, _ := authz.FromContext(ctx)
	actor, _ := uuid.Parse(g.UserID)
	var actorPtr *uuid.UUID
	if actor != uuid.Nil {
		actorPtr = &actor
	}
	row, err := q.InsertTask(ctx, dpostore.InsertTaskParams{ID: id, TaskNo: fmt.Sprintf("%s%04d", prefix, n+1),
		Title: title, Description: opt(description), SourceType: "ropa_gap", SourceID: pgUUID(&activityID),
		Priority: "medium", Actor: pgUUID(actorPtr)})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}
