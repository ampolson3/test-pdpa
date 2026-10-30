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

// HasTaskForRisk is DPIA-08's own dedup check: true once any dpo.tasks row already names this risk as its
// own source — regardless of status — so EnsureRemediationTask (risk/service) never opens a second task for
// a risk that already has one, whether from an earlier DPIA-07 control (owner+due) or an earlier DPIA-08
// submit-to-register call. Implements risk/service's own local DpoTasks interface structurally (rule 9).
func (s *Service) HasTaskForRisk(ctx context.Context, riskID uuid.UUID) (bool, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListTasksBySource(ctx, dpostore.ListTasksBySourceParams{SourceType: "risk", SourceID: pgUUID(&riskID)})
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// TaskStatusForRisk is RRA-14's own "แผนแก้ไข และความคืบหน้า" (remediation plan and progress): the current
// status of this risk's own remediation task, if one exists — HasTaskForRisk's dedup already guarantees at
// most one in practice, but the most recently created row is returned if that invariant were ever broken.
// Implements risk/service's own local DpoTasks interface structurally (rule 9).
func (s *Service) TaskStatusForRisk(ctx context.Context, riskID uuid.UUID) (string, bool, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListTasksBySource(ctx, dpostore.ListTasksBySourceParams{SourceType: "risk", SourceID: pgUUID(&riskID)})
	if err != nil {
		return "", false, err
	}
	if len(rows) == 0 {
		return "", false, nil
	}
	return rows[len(rows)-1].Status, true, nil
}

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
