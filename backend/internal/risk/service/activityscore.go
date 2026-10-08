package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	riskstore "pdpa-platform/internal/risk/store"
)

// ActivitySignals is exactly what RRA-01's scoring needs from a RoPA activity — a plain local type, not
// ropa's own, since internal/ropa/service already imports internal/risk/service (ROPA-09's own
// security-controls catalog); importing it back here would cycle. internal/wiring builds the adapter that
// fills this from the real ropa + org services, where both concrete packages are already importable.
type ActivitySignals struct {
	HasSensitiveData       bool
	HasVulnerableSubject   bool
	HighVolume             bool // any data point at 10,000+ records
	RecipientCount         int
	HasCrossBorderTransfer bool
	ControlCount           int
}

// Ropa is what risk reads from the ropa module (rule 9) to score one activity.
type Ropa interface {
	// ActivityVisible reports whether id is a real activity visible under the caller's RLS.
	ActivityVisible(ctx context.Context, id uuid.UUID) (bool, error)
	Signals(ctx context.Context, id uuid.UUID) (ActivitySignals, error)
}

// Factor is one contribution to the score, named so the UI can explain why an activity scored the way it
// did — RRA-01's own acceptance criterion ("อธิบายปัจจัยที่ทำให้สูงได้").
type Factor struct {
	Code          string `json:"code"`
	ContributesTo string `json:"contributes_to"` // likelihood | impact
}

// ActivityScore is one computation (risk.activity_scores): always a fresh row, never updated in place —
// a plain history trail of how the activity scored over time as its RoPA data changed.
type ActivityScore struct {
	ID         uuid.UUID
	ActivityID uuid.UUID
	MatrixID   uuid.UUID
	Likelihood int
	Impact     int
	Score      float64
	Level      string
	Factors    []Factor
	ComputedAt time.Time
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Score computes RRA-01's own acceptance criterion: a fresh likelihood/impact pair from the activity's
// current RoPA data, classified against the tenant's default matrix (RRA-02), explained by the factors
// that moved it — then persisted as a new risk.activity_scores row. Always live: nothing here is cached,
// so a RoPA edit changes the very next call's result.
func (s *Service) Score(ctx context.Context, activityID uuid.UUID) (ActivityScore, error) {
	ok, err := s.Ropa.ActivityVisible(ctx, activityID)
	if err != nil {
		return ActivityScore{}, err
	}
	if !ok {
		return ActivityScore{}, ErrNotFound
	}
	matrix, err := s.GetMatrix(ctx, nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ActivityScore{}, fmt.Errorf("%w: no default risk matrix configured for this tenant (RRA-02)", ErrInvalid)
		}
		return ActivityScore{}, err
	}
	sig, err := s.Ropa.Signals(ctx, activityID)
	if err != nil {
		return ActivityScore{}, err
	}

	likelihood, impact := 1, 1
	var factors []Factor
	if sig.HasSensitiveData {
		impact++
		factors = append(factors, Factor{Code: "sensitive_data", ContributesTo: "impact"})
	}
	if sig.HasVulnerableSubject {
		impact++
		factors = append(factors, Factor{Code: "vulnerable_subjects", ContributesTo: "impact"})
	}
	if sig.HighVolume {
		impact++
		factors = append(factors, Factor{Code: "high_volume", ContributesTo: "impact"})
	}
	if sig.HasCrossBorderTransfer {
		impact++
		factors = append(factors, Factor{Code: "cross_border_transfer", ContributesTo: "impact"})
	}
	if sig.RecipientCount > 0 {
		likelihood++
		factors = append(factors, Factor{Code: "external_recipients", ContributesTo: "likelihood"})
	}
	if sig.HasCrossBorderTransfer {
		likelihood++
		factors = append(factors, Factor{Code: "cross_border_transfer", ContributesTo: "likelihood"})
	}
	if sig.ControlCount == 0 {
		likelihood++
		factors = append(factors, Factor{Code: "no_controls", ContributesTo: "likelihood"})
	}
	likelihood = clamp(likelihood, 1, len(matrix.LikelihoodLevels))
	impact = clamp(impact, 1, len(matrix.ImpactLevels))

	score, level, err := Classify(matrix, likelihood, impact)
	if err != nil {
		return ActivityScore{}, err
	}

	breakdown, err := json.Marshal(factors)
	if err != nil {
		return ActivityScore{}, err
	}
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).InsertActivityScore(ctx, riskstore.InsertActivityScoreParams{
		ActivityID: activityID, MatrixID: matrix.ID, Likelihood: int16(likelihood), Impact: int16(impact),
		InherentScore: numeric(score), Level: level, FactorBreakdown: breakdown,
	})
	if err != nil {
		return ActivityScore{}, err
	}
	return toActivityScore(row, factors), nil
}

// LatestScore returns the most recent computation without recomputing — ErrNotFound if the activity has
// never been scored yet.
func (s *Service) LatestScore(ctx context.Context, activityID uuid.UUID) (ActivityScore, error) {
	ok, err := s.Ropa.ActivityVisible(ctx, activityID)
	if err != nil {
		return ActivityScore{}, err
	}
	if !ok {
		return ActivityScore{}, ErrNotFound
	}
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).GetLatestActivityScore(ctx, activityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivityScore{}, ErrNotFound
	}
	if err != nil {
		return ActivityScore{}, err
	}
	var factors []Factor
	_ = json.Unmarshal(row.FactorBreakdown, &factors)
	return toActivityScore(row, factors), nil
}

func toActivityScore(r riskstore.RiskActivityScore, factors []Factor) ActivityScore {
	f, _ := r.InherentScore.Float64Value()
	a := ActivityScore{ID: r.ID, ActivityID: r.ActivityID, MatrixID: r.MatrixID, Likelihood: int(r.Likelihood),
		Impact: int(r.Impact), Score: f.Float64, Level: r.Level, Factors: factors}
	if r.ComputedAt.Valid {
		a.ComputedAt = r.ComputedAt.Time
	}
	return a
}

func numeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(f, 'f', 2, 64))
	return n
}
