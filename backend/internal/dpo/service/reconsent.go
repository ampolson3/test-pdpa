package service

import (
	"fmt"
	"strconv"
	"time"

	"context"

	"github.com/google/uuid"

	dpostore "pdpa-platform/internal/dpo/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// HasTaskForConsentPurpose is ROPA-19's own dedup check, the same shape HasTaskForRisk/HasTaskForVendorAgreement
// already use: true once any dpo.tasks row already names this consent purpose as its own source — whether
// opened by an earlier PNG-07 publish or an earlier ROPA-19 activity-change sweep — so a repeat RoPA
// approval never opens a second task for a purpose that's already flagged.
func (s *Service) HasTaskForConsentPurpose(ctx context.Context, consentPurposeID uuid.UUID) (bool, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListTasksBySource(ctx, dpostore.ListTasksBySourceParams{SourceType: "consent", SourceID: pgUUID(&consentPurposeID)})
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// OpenConsentTask opens one dpo.tasks job ("CON-<year>-NNNN", source_type "consent") for PNG-07: a notice
// publish that changes a purpose never auto-generates the new consent text itself (rule 8 — legal wording
// stays a human's job), it just makes sure someone is asked to go author/publish it in CON (CON-12).
// notice calls this through its own Dpo interface (rule 9 — dpo.tasks is this module's own schema).
func (s *Service) OpenConsentTask(ctx context.Context, consentPurposeID uuid.UUID, title, description string) (Task, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	year := strconv.Itoa(time.Now().UTC().Year())
	if err := q.LockTaskNumbering(ctx, year); err != nil {
		return Task{}, err
	}
	prefix := "CON-" + year + "-"
	n, err := q.CountTasksInYear(ctx, prefix)
	if err != nil {
		return Task{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Task{}, err
	}
	g, _ := authz.FromContext(ctx)
	actor, _ := uuid.Parse(g.UserID)
	var actorPtr *uuid.UUID
	if actor != uuid.Nil {
		actorPtr = &actor
	}
	row, err := q.InsertTask(ctx, dpostore.InsertTaskParams{ID: id, TaskNo: fmt.Sprintf("%s%04d", prefix, n+1),
		Title: title, Description: opt(description), SourceType: "consent",
		SourceID: pgUUID(&consentPurposeID), Priority: "high", Actor: pgUUID(actorPtr)})
	if err != nil {
		return Task{}, err
	}
	return toTask(row), nil
}

// OpenConsentReviewTask is ROPA-19's own entry point into OpenConsentTask, returning just the new task's
// id — ropa's own Dpo interface declares no dependency on this package's Task type to avoid an import
// cycle (dpo already imports dsar, which imports ropa).
func (s *Service) OpenConsentReviewTask(ctx context.Context, consentPurposeID uuid.UUID, title, description string) (uuid.UUID, error) {
	t, err := s.OpenConsentTask(ctx, consentPurposeID, title, description)
	if err != nil {
		return uuid.Nil, err
	}
	return t.ID, nil
}
