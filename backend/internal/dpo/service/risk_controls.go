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

// OpenRiskControlTask opens one dpo.tasks job ("SEC-<year>-NNNN", source_type "risk", the same bucket
// DPO-09's own openRemediationTask already uses for security findings) for DPIA-07's own "ผู้รับผิดชอบและ
// วันเสร็จ → task": a mitigation measure given both an owner and a due date gets a real tracked task, not
// just a note on the risk. Implements risk/service's own local DpoTasks interface structurally (rule 9 —
// risk/service cannot import this package back: dpo already imports dsar, which imports ropa, which
// imports risk/service, so the reverse import would cycle).
func (s *Service) OpenRiskControlTask(ctx context.Context, riskID uuid.UUID, title, description string, assigneeUserID *uuid.UUID, dueAt *time.Time) (uuid.UUID, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	year := strconv.Itoa(time.Now().UTC().Year())
	if err := q.LockTaskNumbering(ctx, year); err != nil {
		return uuid.Nil, err
	}
	prefix := "SEC-" + year + "-"
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
		Title: title, Description: opt(description), SourceType: "risk", SourceID: pgUUID(&riskID), Priority: "high",
		AssigneeUserID: pgUUID(assigneeUserID), DueAt: pgDate(dueAt), Actor: pgUUID(actorPtr)})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}
