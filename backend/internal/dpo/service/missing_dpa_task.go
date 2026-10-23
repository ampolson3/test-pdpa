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

// HasTaskForVendorAgreement is DPA-12's own dedup check — the same shape HasTaskForRisk (DPIA-08) already
// established: true once any dpo.tasks row already names this vendor as its source (source_type
// "agreement" — the vendor is missing one, not a particular agreement row), regardless of status, so the
// daily sweep never opens a second task for a vendor already being chased.
func (s *Service) HasTaskForVendorAgreement(ctx context.Context, vendorID uuid.UUID) (bool, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListTasksBySource(ctx, dpostore.ListTasksBySourceParams{SourceType: "agreement", SourceID: pgUUID(&vendorID)})
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// OpenMissingDPATask opens one dpo.tasks job ("DPA-<year>-NNNN", source_type "agreement" — already in the
// table's own CHECK constraint, unused until now) for DPA-12's own acceptance criterion: a processor vendor
// found with no agreement "ที่มีผลบังคับ" gets a real tracked task, not just a notification nobody owns.
func (s *Service) OpenMissingDPATask(ctx context.Context, vendorID uuid.UUID, title, description string) (uuid.UUID, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	year := strconv.Itoa(time.Now().UTC().Year())
	if err := q.LockTaskNumbering(ctx, year); err != nil {
		return uuid.Nil, err
	}
	prefix := "DPA-" + year + "-"
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
		Title: title, Description: opt(description), SourceType: "agreement", SourceID: pgUUID(&vendorID),
		Priority: "high", Actor: pgUUID(actorPtr)})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}
