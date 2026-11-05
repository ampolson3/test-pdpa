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

// OpenAuditFindingTask opens one dpo.tasks job ("AUD-<year>-NNNN", source_type "audit" — already in the
// table's own CHECK constraint, unused until now) for DPX-04's own "มอบหมายงานตามข้อตรวจพบ": the exact
// shape OpenGapRemediationTask (RRA-07) already established, with the caller's own assignee/due date/
// priority instead of a fixed one.
func (s *Service) OpenAuditFindingTask(ctx context.Context, findingID uuid.UUID, title, description string,
	assigneeUserID *uuid.UUID, dueAt *time.Time, priority string) (uuid.UUID, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	year := strconv.Itoa(time.Now().UTC().Year())
	if err := q.LockTaskNumbering(ctx, year); err != nil {
		return uuid.Nil, err
	}
	prefix := "AUD-" + year + "-"
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
		Title: title, Description: opt(description), SourceType: "audit", SourceID: pgUUID(&findingID),
		Priority: priority, AssigneeUserID: pgUUID(assigneeUserID), DueAt: pgDate(dueAt), Actor: pgUUID(actorPtr)})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}
