package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	riskstore "pdpa-platform/internal/risk/store"
)

var (
	ErrNotFound        = errors.New("risk: not found")
	ErrInvalid         = errors.New("risk: invalid")
	ErrVersionMismatch = errors.New("risk: version mismatch")
)

// validLevels is exactly risk.activity_scores.level's own CHECK constraint — a matrix's thresholds may only
// ever classify a score into one of these, so RRA-01's own activity_scores write never finds a level the
// column itself would refuse.
var validLevels = map[string]bool{"low": true, "medium": true, "high": true, "very_high": true}

// Threshold is the score at and above which a risk is classified at Level — RRA-02's own "เกณฑ์สูง/กลาง/ต่ำ".
type Threshold struct {
	Level    string  `json:"level"`
	MinScore float64 `json:"min_score"`
}

// RiskMatrix is one tenant-configured likelihood x impact matrix (RRA-02) — the shared "risk engine" the
// module doc's own backend note says DPIA/vendor/breach will all read from once they exist.
type RiskMatrix struct {
	ID               uuid.UUID
	Name             string
	LikelihoodLevels []string
	ImpactLevels     []string
	Thresholds       []Threshold
	IsDefault        bool
	RowVersion       int32
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (m *RiskMatrix) normalize() error {
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" {
		return fmt.Errorf("%w: name", ErrInvalid)
	}
	if len(m.LikelihoodLevels) < 2 || len(m.LikelihoodLevels) > 10 {
		return fmt.Errorf("%w: likelihood_levels", ErrInvalid)
	}
	if len(m.ImpactLevels) < 2 || len(m.ImpactLevels) > 10 {
		return fmt.Errorf("%w: impact_levels", ErrInvalid)
	}
	if len(m.Thresholds) == 0 {
		return fmt.Errorf("%w: thresholds", ErrInvalid)
	}
	sorted := make([]Threshold, len(m.Thresholds))
	copy(sorted, m.Thresholds)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MinScore < sorted[j].MinScore })
	for i, th := range sorted {
		if !validLevels[th.Level] {
			return fmt.Errorf("%w: thresholds[].level", ErrInvalid)
		}
		if th.MinScore < 0 {
			return fmt.Errorf("%w: thresholds[].min_score", ErrInvalid)
		}
		if i > 0 && sorted[i].MinScore == sorted[i-1].MinScore {
			return fmt.Errorf("%w: thresholds[].min_score duplicate", ErrInvalid)
		}
	}
	maxScore := float64(len(m.LikelihoodLevels) * len(m.ImpactLevels))
	if sorted[0].MinScore > 1 {
		return fmt.Errorf("%w: thresholds must cover every score from 1 to %v", ErrInvalid, maxScore)
	}
	m.Thresholds = sorted
	return nil
}

// Classify returns the score (likelihood x impact) and the matrix's own level for it — a pure function so
// changing the matrix changes every future classification at once, with nothing cached anywhere (RRA-02's
// acceptance criterion: "เปลี่ยน matrix แล้วคะแนนทุกโมดูลคำนวณตามค่าใหม่").
func Classify(m RiskMatrix, likelihood, impact int) (score float64, level string, err error) {
	if likelihood < 1 || likelihood > len(m.LikelihoodLevels) {
		return 0, "", fmt.Errorf("%w: likelihood", ErrInvalid)
	}
	if impact < 1 || impact > len(m.ImpactLevels) {
		return 0, "", fmt.Errorf("%w: impact", ErrInvalid)
	}
	score = float64(likelihood * impact)
	for i := len(m.Thresholds) - 1; i >= 0; i-- {
		if score >= m.Thresholds[i].MinScore {
			return score, m.Thresholds[i].Level, nil
		}
	}
	return score, "", fmt.Errorf("%w: no threshold covers score %v", ErrInvalid, score)
}

// ClassifyScore is Classify's one-dimensional counterpart for VEN-07's own "สรุประดับความเสี่ยง": a
// vendor assessment produces a single weighted-answer score, not a likelihood x impact pair to multiply,
// but it still needs to land on this same tenant's own thresholds so a vendor's risk reads on the same
// scale as everything else risk/service classifies. ratio is 0 (no risk signal, every answer scored the
// best it could) to 1 (the worst possible answers) — the caller computes it as 1 - score/maxScore, scaled
// here onto the same 1..(likelihood x impact) point range Classify's own grid uses, then classified
// against the identical Thresholds ladder. No SA spec defined this mapping — flagged for review in VEN.md.
func ClassifyScore(m RiskMatrix, ratio float64) (level string, err error) {
	if ratio < 0 || ratio > 1 {
		return "", fmt.Errorf("%w: ratio", ErrInvalid)
	}
	if len(m.Thresholds) == 0 {
		return "", fmt.Errorf("%w: matrix has no thresholds", ErrInvalid)
	}
	maxScore := float64(len(m.LikelihoodLevels) * len(m.ImpactLevels))
	score := ratio * maxScore
	// Classify's own grid never scores below 1 (likelihood and impact are both >= 1), so a matrix's
	// lowest threshold only ever needs to cover from 1 upward (normalize()'s own rule) — but ratio 0
	// (every answer scored the best it could) lands exactly on 0, below that floor. Rather than treat a
	// perfect score as an error, it floors at the matrix's own most lenient threshold.
	lowest := m.Thresholds[0]
	for _, th := range m.Thresholds {
		if th.MinScore < lowest.MinScore {
			lowest = th
		}
	}
	for i := len(m.Thresholds) - 1; i >= 0; i-- {
		if score >= m.Thresholds[i].MinScore {
			return m.Thresholds[i].Level, nil
		}
	}
	return lowest.Level, nil
}

func (s *Service) audit(ctx context.Context, action, entityType string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		var t string
		if terr := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); terr != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("risk: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: entityType, EntityID: &id, Before: before, After: after}
	if u, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &u
	}
	return s.Audit.Write(ctx, e)
}

// ListMatrices lists the tenant's own matrices, defaults first.
func (s *Service) ListMatrices(ctx context.Context) ([]RiskMatrix, error) {
	rows, err := riskstore.New(pdb.MustTxFromContext(ctx)).ListRiskMatrices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RiskMatrix, 0, len(rows))
	for _, r := range rows {
		m, err := toMatrix(r)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// GetMatrix resolves a specific matrix, or the tenant's own default when id is nil — the same
// "BusinessCalendar(ctx, id|nil)" pattern ORG-20's own default calendar already established, for RRA-01/
// DPIA-06 to call once they exist.
func (s *Service) GetMatrix(ctx context.Context, id *uuid.UUID) (RiskMatrix, error) {
	q := riskstore.New(pdb.MustTxFromContext(ctx))
	var row riskstore.RiskRiskMatrix
	var err error
	if id != nil {
		row, err = q.GetRiskMatrix(ctx, *id)
	} else {
		row, err = q.GetDefaultRiskMatrix(ctx)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return RiskMatrix{}, ErrNotFound
	}
	if err != nil {
		return RiskMatrix{}, err
	}
	return toMatrix(row)
}

// SaveMatrix creates (version 0) or updates (its current row_version) a matrix. Setting is_default clears
// every other matrix's default flag in the same transaction first, so the partial unique index
// (migration 00055) is never violated by the swap itself.
func (s *Service) SaveMatrix(ctx context.Context, m RiskMatrix, version int32) (RiskMatrix, error) {
	if err := m.normalize(); err != nil {
		return RiskMatrix{}, err
	}
	likelihood, err := marshalJSON(m.LikelihoodLevels)
	if err != nil {
		return RiskMatrix{}, err
	}
	impact, err := marshalJSON(m.ImpactLevels)
	if err != nil {
		return RiskMatrix{}, err
	}
	thresholds, err := marshalJSON(m.Thresholds)
	if err != nil {
		return RiskMatrix{}, err
	}

	var row riskstore.RiskRiskMatrix
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		q := riskstore.New(pdb.MustTxFromContext(ctx))
		if m.IsDefault {
			if err := q.ClearOtherDefaultRiskMatrices(ctx, m.ID); err != nil {
				return err
			}
		}
		var innerErr error
		if m.ID == uuid.Nil {
			row, innerErr = q.InsertRiskMatrix(ctx, riskstore.InsertRiskMatrixParams{
				Name: m.Name, LikelihoodLevels: likelihood, ImpactLevels: impact, Thresholds: thresholds, IsDefault: m.IsDefault,
			})
			return innerErr
		}
		row, innerErr = q.UpdateRiskMatrix(ctx, riskstore.UpdateRiskMatrixParams{
			ID: m.ID, RowVersion: version, Name: m.Name, LikelihoodLevels: likelihood, ImpactLevels: impact,
			Thresholds: thresholds, IsDefault: m.IsDefault,
		})
		if errors.Is(innerErr, pgx.ErrNoRows) {
			return ErrVersionMismatch
		}
		return innerErr
	})
	if err != nil {
		return RiskMatrix{}, err
	}
	out, err := toMatrix(row)
	if err != nil {
		return RiskMatrix{}, err
	}
	action := "risk.matrix.update"
	if m.ID == uuid.Nil {
		action = "risk.matrix.create"
	}
	if err := s.audit(ctx, action, "risk_matrix", out.ID, nil, map[string]any{"name": out.Name, "is_default": out.IsDefault}); err != nil {
		return RiskMatrix{}, err
	}
	return out, nil
}

// DeleteMatrix removes a matrix by its current row_version. Deleting the tenant's own default simply
// leaves none — GetMatrix(ctx, nil) then reports ErrNotFound until a new one is created or marked default,
// the same "no built-in default" reasoning this feature already uses (no sensible NxN shape to guess).
func (s *Service) DeleteMatrix(ctx context.Context, id uuid.UUID, version int32) error {
	n, err := riskstore.New(pdb.MustTxFromContext(ctx)).DeleteRiskMatrix(ctx, riskstore.DeleteRiskMatrixParams{ID: id, RowVersion: version})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrVersionMismatch
	}
	return s.audit(ctx, "risk.matrix.delete", "risk_matrix", id, nil, nil)
}

func toMatrix(r riskstore.RiskRiskMatrix) (RiskMatrix, error) {
	m := RiskMatrix{ID: r.ID, Name: r.Name, IsDefault: r.IsDefault, RowVersion: r.RowVersion}
	if err := unmarshalJSON(r.LikelihoodLevels, &m.LikelihoodLevels); err != nil {
		return RiskMatrix{}, err
	}
	if err := unmarshalJSON(r.ImpactLevels, &m.ImpactLevels); err != nil {
		return RiskMatrix{}, err
	}
	if err := unmarshalJSON(r.Thresholds, &m.Thresholds); err != nil {
		return RiskMatrix{}, err
	}
	if r.CreatedAt.Valid {
		m.CreatedAt = r.CreatedAt.Time
	}
	if r.UpdatedAt.Valid {
		m.UpdatedAt = r.UpdatedAt.Time
	}
	return m, nil
}

func marshalJSON(v any) ([]byte, error) { return json.Marshal(v) }

func unmarshalJSON(b []byte, v any) error {
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}
