package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	riskstore "pdpa-platform/internal/risk/store"
)

// sourceTypes is exactly risk.risks.source_type's own CHECK constraint.
var sourceTypes = map[string]bool{"dpia": true, "activity": true, "vendor": true, "breach": true, "audit": true, "manual": true}

// Risk is one risk.risks row — the shared risk register the module doc's own backend note anticipates every
// of DPIA/vendor/breach eventually feeding (DPIA-06 is its first real writer). Deliberately scoped to what
// DPIA-06's own acceptance criterion needs: identify + score against the tenant matrix. AssetID/VendorID and
// the whole residual-risk/treatment-plan half of the table are DPIA-07's own job layered on top later — the
// same "leave the FK/column for the feature that actually needs it" deferral this codebase uses throughout.
type Risk struct {
	ID            uuid.UUID
	SourceType    string // dpia | activity | vendor | breach | audit | manual
	SourceID      *uuid.UUID
	Title         string
	Description   string
	OwnerUserID   *uuid.UUID
	ActivityID    *uuid.UUID
	Likelihood    int
	Impact        int
	InherentScore float64
	Level         string
	// Residual* are DPIA-07's own fields — nil until at least one control is linked (AddRiskControl),
	// recomputed automatically whenever the linked control set changes (never set directly by a caller).
	ResidualLikelihood *int
	ResidualImpact     *int
	ResidualScore      *float64
	ResidualLevel      *string
	Treatment          string // mitigate | accept | transfer | avoid
	Status             string // open | in_treatment | accepted | closed
	RowVersion         int32
	CreatedAt          time.Time
}

var validTreatments = map[string]bool{"": true, "mitigate": true, "accept": true, "transfer": true, "avoid": true}

// IdentifyRisk creates a new risk.risks row, scoring it against the tenant's own default matrix (RRA-02) —
// DPIA-06's own acceptance criterion ("คะแนนความเสี่ยงคำนวณตาม matrix ของ tenant"), live and uncached, exactly
// like RRA-01's own Score. OwnerUserID/ActivityID are checked visible under the caller's own RLS first
// (rule 1) before anything is written.
func (s *Service) IdentifyRisk(ctx context.Context, r Risk) (Risk, error) {
	r.Title = strings.TrimSpace(r.Title)
	if r.Title == "" {
		return Risk{}, fmt.Errorf("%w: title", ErrInvalid)
	}
	if !sourceTypes[r.SourceType] {
		return Risk{}, fmt.Errorf("%w: source_type", ErrInvalid)
	}
	matrix, err := s.GetMatrix(ctx, nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Risk{}, fmt.Errorf("%w: no default risk matrix configured for this tenant (RRA-02)", ErrInvalid)
		}
		return Risk{}, err
	}
	score, level, err := Classify(matrix, r.Likelihood, r.Impact)
	if err != nil {
		return Risk{}, err
	}
	if r.OwnerUserID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*r.OwnerUserID})
		if err != nil {
			return Risk{}, err
		}
		if _, ok := names[*r.OwnerUserID]; !ok {
			return Risk{}, fmt.Errorf("%w: owner_user_id", ErrInvalid)
		}
	}
	if r.ActivityID != nil {
		ok, err := s.Ropa.ActivityVisible(ctx, *r.ActivityID)
		if err != nil {
			return Risk{}, err
		}
		if !ok {
			return Risk{}, fmt.Errorf("%w: activity_id", ErrInvalid)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Risk{}, err
	}
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).InsertRisk(ctx, riskstore.InsertRiskParams{
		ID: id, SourceType: r.SourceType, SourceID: pgUUID(r.SourceID), Title: r.Title, Description: opt(r.Description),
		OwnerUserID: pgUUID(r.OwnerUserID), ActivityID: pgUUID(r.ActivityID),
		Likelihood: int16(r.Likelihood), Impact: int16(r.Impact), InherentScore: numeric(score), Level: level,
	})
	if err != nil {
		return Risk{}, err
	}
	out := toRisk(row)
	if err := s.audit(ctx, "risk.risk.identify", "risk", out.ID, nil,
		map[string]any{"title": out.Title, "level": out.Level, "score": out.InherentScore}); err != nil {
		return Risk{}, err
	}
	return out, nil
}

// GetRisk returns one risk by id, RLS-scoped.
func (s *Service) GetRisk(ctx context.Context, id uuid.UUID) (Risk, error) {
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).GetRisk(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Risk{}, ErrNotFound
	}
	if err != nil {
		return Risk{}, err
	}
	return toRisk(row), nil
}

// ListRisksByIDs returns the given risks (RLS already filters out any id from another tenant), oldest first
// — used by dpia/service to resolve the risk.risks rows an assessment links to.
func (s *Service) ListRisksByIDs(ctx context.Context, ids []uuid.UUID) ([]Risk, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := riskstore.New(pdb.MustTxFromContext(ctx)).ListRisksByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Risk, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRisk(r))
	}
	return out, nil
}

// UpdateRisk edits an existing risk's identification/score (and, once a module calls it with one, its
// treatment/status) by its current row_version. Likelihood/impact changes are re-classified against the
// tenant's current default matrix, never left stale.
func (s *Service) UpdateRisk(ctx context.Context, id uuid.UUID, r Risk, version int32) (Risk, error) {
	r.Title = strings.TrimSpace(r.Title)
	if r.Title == "" {
		return Risk{}, fmt.Errorf("%w: title", ErrInvalid)
	}
	if !validTreatments[r.Treatment] {
		return Risk{}, fmt.Errorf("%w: treatment", ErrInvalid)
	}
	if r.Status == "" {
		r.Status = "open"
	}
	matrix, err := s.GetMatrix(ctx, nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Risk{}, fmt.Errorf("%w: no default risk matrix configured for this tenant (RRA-02)", ErrInvalid)
		}
		return Risk{}, err
	}
	score, level, err := Classify(matrix, r.Likelihood, r.Impact)
	if err != nil {
		return Risk{}, err
	}
	if r.OwnerUserID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*r.OwnerUserID})
		if err != nil {
			return Risk{}, err
		}
		if _, ok := names[*r.OwnerUserID]; !ok {
			return Risk{}, fmt.Errorf("%w: owner_user_id", ErrInvalid)
		}
	}
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).UpdateRisk(ctx, riskstore.UpdateRiskParams{
		ID: id, Title: r.Title, Description: opt(r.Description), OwnerUserID: pgUUID(r.OwnerUserID),
		Likelihood: int16(r.Likelihood), Impact: int16(r.Impact), InherentScore: numeric(score), Level: level,
		Treatment: opt(r.Treatment), Status: r.Status, RowVersion: version,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Risk{}, ErrVersionMismatch
	}
	if err != nil {
		return Risk{}, err
	}
	out := toRisk(row)
	if err := s.audit(ctx, "risk.risk.update", "risk", out.ID, nil,
		map[string]any{"title": out.Title, "level": out.Level, "score": out.InherentScore, "status": out.Status}); err != nil {
		return Risk{}, err
	}
	return out, nil
}

func toRisk(r riskstore.RiskRisk) Risk {
	f, _ := r.InherentScore.Float64Value()
	out := Risk{
		ID: r.ID, SourceType: r.SourceType, SourceID: uuidPtr(r.SourceID), Title: r.Title, Description: derefStr(r.Description),
		OwnerUserID: uuidPtr(r.OwnerUserID), ActivityID: uuidPtr(r.ActivityID), Likelihood: int(r.Likelihood), Impact: int(r.Impact),
		InherentScore: f.Float64, Level: r.Level, ResidualLevel: r.ResidualLevel,
		Treatment: derefStr(r.Treatment), Status: r.Status, RowVersion: r.RowVersion,
	}
	if r.ResidualLikelihood != nil {
		v := int(*r.ResidualLikelihood)
		out.ResidualLikelihood = &v
	}
	if r.ResidualImpact != nil {
		v := int(*r.ResidualImpact)
		out.ResidualImpact = &v
	}
	if r.ResidualScore.Valid {
		rf, _ := r.ResidualScore.Float64Value()
		v := rf.Float64
		out.ResidualScore = &v
	}
	if r.CreatedAt.Valid {
		out.CreatedAt = r.CreatedAt.Time
	}
	return out
}

func pgUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	u := uuid.UUID(v.Bytes)
	return &u
}

func opt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
