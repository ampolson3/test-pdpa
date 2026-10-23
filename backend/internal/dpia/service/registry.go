package service

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// RegistryEntry is one row of DPIA-12's register: a RoPA activity's current DPIA round, with enough of the
// activity/org context to file and filter the report without a second round trip per row.
type RegistryEntry struct {
	AssessmentID    uuid.UUID
	ActivityID      uuid.UUID
	ActivityCode    string
	ActivityName    string
	LegalEntityID   uuid.UUID
	LegalEntityName string
	OrgUnitID       uuid.UUID
	OrgUnitName     string
	RoundNo         int
	Status          string
	ScreeningResult string
	Score           float64
	CreatedAt       time.Time
}

// RegistryFilter narrows the report to one legal entity, one department (optionally including its
// subtree is not offered here — DPIA-12's acceptance criterion is "status matches reality", not a scope
// query like ROPA's own UnitWithin), or one status.
type RegistryFilter struct {
	LegalEntityID *uuid.UUID
	OrgUnitID     *uuid.UUID
	Status        string
}

// Registry is DPIA-12's acceptance criterion: a single list of every RoPA activity's current DPIA round and
// status, always read live off assess.assessments/ropa.assets — never a cached or separately-maintained
// copy, so the register can never drift from each DPIA's real current state. No pagination (ROPA-04's own
// precedent for a report meant to be viewed/filed as one list, not paged); filters are applied in Go after
// enrichment since the result set is a tenant's own activity count, not expected to be large.
func (s *Service) Registry(ctx context.Context, f RegistryFilter) ([]RegistryEntry, error) {
	rows, err := dpiastore.New(pdb.MustTxFromContext(ctx)).ListAllAssessments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RegistryEntry, 0, len(rows))
	for _, r := range rows {
		activityID := uuidVal(r.ActivityID)
		activity, err := s.Ropa.GetActivity(ctx, activityID)
		if err != nil {
			// The activity behind a past screening round was deleted or is no longer visible under this
			// caller's RLS (shouldn't happen under normal use, since ropa.assets rows aren't deleted) —
			// skip it rather than fail the whole registry over one stale row.
			continue
		}
		if f.LegalEntityID != nil && activity.LegalEntityID != *f.LegalEntityID {
			continue
		}
		if f.OrgUnitID != nil && activity.OrgUnitID != *f.OrgUnitID {
			continue
		}
		if f.Status != "" && r.Status != f.Status {
			continue
		}
		unit, err := s.Org.GetOrgUnit(ctx, activity.OrgUnitID)
		if err != nil {
			return nil, err
		}
		entity, err := s.Org.GetLegalEntity(ctx, activity.LegalEntityID)
		if err != nil {
			return nil, err
		}
		var score float64
		if r.Score.Valid {
			fv, _ := r.Score.Float64Value()
			score = fv.Float64
		}
		out = append(out, RegistryEntry{
			AssessmentID: r.ID, ActivityID: activityID, ActivityCode: activity.Code, ActivityName: activity.Name,
			LegalEntityID: activity.LegalEntityID, LegalEntityName: entity.NameTh, OrgUnitID: activity.OrgUnitID,
			OrgUnitName: unit.NameTh, RoundNo: int(r.RoundNo), Status: r.Status, ScreeningResult: derefStr(r.ScreeningResult),
			Score: score, CreatedAt: r.CreatedAt.Time,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
