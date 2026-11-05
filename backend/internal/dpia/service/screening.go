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

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

// ScreeningFormType is the PLT-06 form type the DPIA screening form uses — "assessment", already registered
// in internal/wiring.Forms (unused until this feature) with assessment.template.*/assessment.dpia.* codes.
const ScreeningFormType = "assessment"

// ScreeningTemplateCode is the seeded global template's code (migration 00047, docs/decisions.md Q-26).
// A tenant may publish its own assess.templates row with the same code to override it (GetScreeningTemplate
// prefers the tenant's own row over the global default).
const ScreeningTemplateCode = "dpia_screening"

// DefaultMinFactors is DPIA-02's default threshold when a tenant hasn't configured its own
// (docs/decisions.md Q-26: a tunable, not a legally-mandated number).
const DefaultMinFactors = 2

// ErrBadTemplate is returned when the screening template isn't a real, published "dpia" template.
var ErrBadTemplate = errors.New("dpia: no published screening template")

// ScreeningRule is a tenant's DPIA-02 thresholds: an activity needs the criteria's min_factors risk factors
// flagged (or, if set, a score at or above min_score) to screen as "required".
type ScreeningRule struct {
	ID         uuid.UUID
	MinFactors int
	MinScore   *float64
	CreatedAt  time.Time
}

// Assessment is one DPIA screening round for a RoPA processing activity.
type Assessment struct {
	ID              uuid.UUID
	TemplateID      uuid.UUID
	FormVersionID   uuid.UUID
	Title           string
	ActivityID      uuid.UUID
	RoundNo         int
	PreviousID      *uuid.UUID
	Status          string // screening | not_required | in_progress (ST-05; DPIA-01/02 only reach these three)
	ScreeningResult string // required | recommended | not_required
	ScreeningReason string
	Score           float64
	RiskLevel       string // set when this round was opened by RRA-03's risk-score trigger (high | very_high)
	OwnerUserID     *uuid.UUID
	Factors         []forms.Contribution
	RowVersion      int32
	CreatedAt       time.Time
}

// screeningVersion resolves the template's form to its current *published* version — screening must only
// ever run against a live, reviewed form.
func (s *Service) screeningVersion(ctx context.Context, formID uuid.UUID) (forms.Version, error) {
	f, err := s.Forms.GetForm(ctx, formID)
	if errors.Is(err, forms.ErrNotFound) || errors.Is(err, forms.ErrForbidden) {
		return forms.Version{}, ErrBadTemplate
	}
	if err != nil {
		return forms.Version{}, err
	}
	if f.Type != ScreeningFormType || f.CurrentVersionID == nil {
		return forms.Version{}, ErrBadTemplate
	}
	for _, v := range f.Versions {
		if v.ID == *f.CurrentVersionID {
			return v, nil
		}
	}
	return forms.Version{}, ErrBadTemplate
}

// latestFormVersion resolves a form to its current published version if it has one, else its own open draft
// (PLT-06 keeps at most one unpublished version per form) — used by CloneTemplate, which may clone a
// template that was never published.
func (s *Service) latestFormVersion(ctx context.Context, formID uuid.UUID) (forms.Version, error) {
	f, err := s.Forms.GetForm(ctx, formID)
	if errors.Is(err, forms.ErrNotFound) || errors.Is(err, forms.ErrForbidden) {
		return forms.Version{}, ErrBadTemplate
	}
	if err != nil {
		return forms.Version{}, err
	}
	if f.CurrentVersionID != nil {
		for _, v := range f.Versions {
			if v.ID == *f.CurrentVersionID {
				return v, nil
			}
		}
	}
	if len(f.Versions) == 0 {
		return forms.Version{}, ErrBadTemplate
	}
	return f.Versions[len(f.Versions)-1], nil
}

// Rules returns the tenant's active DPIA-02 thresholds, or the default when none has been saved yet (the
// same "defaults until first save" pattern ORG-20's org_settings uses).
func (s *Service) Rules(ctx context.Context) (ScreeningRule, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetActiveScreeningRule(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return ScreeningRule{MinFactors: DefaultMinFactors}, nil
	}
	if err != nil {
		return ScreeningRule{}, err
	}
	return toRule(row), nil
}

// SaveRules replaces the tenant's active threshold with a new one (DPIA-02's acceptance criterion: the next
// screening round uses it). Rules are append-only history, not edited in place — DeactivateScreeningRules
// then a fresh insert, so a past round's own criteria stays on record.
func (s *Service) SaveRules(ctx context.Context, minFactors int, minScore *float64) (ScreeningRule, error) {
	// 6 is the number of risk factors the seeded screening form asks (migration 00047) — a tenant override
	// with more questions could raise this, but nothing in this pass adds that, so the bound stays literal.
	if minFactors < 1 || minFactors > 6 {
		return ScreeningRule{}, fmt.Errorf("%w: min_factors", ErrInvalid)
	}
	if minScore != nil && (*minScore < 0 || *minScore > 6) {
		return ScreeningRule{}, fmt.Errorf("%w: min_score", ErrInvalid)
	}
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	if err := q.DeactivateScreeningRules(ctx); err != nil {
		return ScreeningRule{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ScreeningRule{}, err
	}
	row, err := q.InsertScreeningRule(ctx, dpiastore.InsertScreeningRuleParams{
		ID: id, Criteria: []byte("{}"), MinFactors: int16(minFactors), MinScore: numeric(minScore),
	})
	if err != nil {
		return ScreeningRule{}, err
	}
	if err := s.audit(ctx, "dpia.screening_rule.save", ScreeningRuleEntityType, id, nil,
		map[string]any{"min_factors": minFactors, "min_score": minScore}); err != nil {
		return ScreeningRule{}, err
	}
	return toRule(row), nil
}

// Screen is DPIA-01's acceptance criterion: answer the screening form for a RoPA activity, and the system
// computes required / recommended / not_required against the tenant's own DPIA-02 thresholds — never left to
// a human's own reading of the factors.
func (s *Service) Screen(ctx context.Context, activityID uuid.UUID, answers forms.Answers) (Assessment, error) {
	activity, err := s.Ropa.GetActivity(ctx, activityID)
	if err != nil {
		return Assessment{}, fmt.Errorf("%w: activity_id", ErrInvalid)
	}
	tmpl, err := s.screeningTemplate(ctx)
	if err != nil {
		return Assessment{}, err
	}
	v, err := s.screeningVersion(ctx, tmpl.FormID)
	if err != nil {
		return Assessment{}, err
	}
	res := forms.Evaluate(v.Schema, v.Scoring, answers, true, nil)
	if len(res.Errors) > 0 {
		ve := &ValidationError{}
		for _, e := range res.Errors {
			ve.Fields = append(ve.Fields, FieldError{"answers." + e.Question, e.Code})
		}
		return Assessment{}, ve
	}
	factors := forms.Contributions(v.Schema, res.Answers)
	factorCount := 0
	for _, f := range factors {
		if f.Points > 0 {
			factorCount++
		}
	}
	rule, err := s.Rules(ctx)
	if err != nil {
		return Assessment{}, err
	}
	required := factorCount >= rule.MinFactors || (rule.MinScore != nil && res.Score >= *rule.MinScore)
	var result, status string
	switch {
	case required:
		result, status = "required", "in_progress"
	case factorCount > 0:
		result, status = "recommended", "in_progress"
	default:
		result, status = "not_required", "not_required"
	}
	reason := screeningReason(result, factorCount, rule, factors)

	round := 1
	var previousID *uuid.UUID
	prevRow, err := dpiastore.New(pdb.MustTxFromContext(ctx)).GetLatestAssessmentForActivity(ctx, pgUUID(&activityID))
	if err == nil {
		round = int(prevRow.RoundNo) + 1
		previousID = &prevRow.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Assessment{}, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Assessment{}, err
	}
	title := fmt.Sprintf("คัดกรอง DPIA: %s (%s)", activity.Name, activity.Code)
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertAssessment(ctx, dpiastore.InsertAssessmentParams{
		ID: id, AssessmentType: "dpia", TemplateID: tmpl.ID, FormVersionID: v.ID, Title: title, SubjectType: "activity",
		SubjectID: pgUUID(&activityID), ActivityID: pgUUID(&activityID), RoundNo: int16(round), PreviousID: pgUUID(previousID),
		Status: status, ScreeningResult: &result, ScreeningReason: &reason, Score: numeric(&res.Score),
	})
	if err != nil {
		return Assessment{}, err
	}
	for _, f := range factors {
		aid, err := uuid.NewV7()
		if err != nil {
			return Assessment{}, err
		}
		raw, _ := json.Marshal(f.Answer)
		if _, err := q.InsertAnswer(ctx, dpiastore.InsertAnswerParams{ID: aid, AssessmentID: id, QuestionCode: f.Question, Answer: raw}); err != nil {
			return Assessment{}, err
		}
	}
	if err := s.audit(ctx, "dpia.assessment.screen", AssessmentEntityType, id, nil,
		map[string]any{"activity_id": activityID, "round_no": round, "result": result, "status": status, "factors": factorCount}); err != nil {
		return Assessment{}, err
	}
	return toAssessment(row, factors), nil
}

// screeningTemplate resolves the live "dpia" screening template: the tenant's own override (assess.templates,
// same code, published) if one exists, else the global default seeded by migration 00047.
func (s *Service) screeningTemplate(ctx context.Context) (dpiastore.AssessTemplate, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetScreeningTemplate(ctx, ScreeningTemplateCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return dpiastore.AssessTemplate{}, ErrBadTemplate
	}
	return row, err
}

// screeningReason is a plain, operational description of why the result came out the way it did — not legal
// text shown to a data subject or the PDPC (rule 8 doesn't apply), so it's composed directly, not templated.
func screeningReason(result string, factorCount int, rule ScreeningRule, factors []forms.Contribution) string {
	var flagged []string
	for _, f := range factors {
		if f.Points > 0 {
			label := f.Label["th"]
			if label == "" {
				label = f.Question
			}
			flagged = append(flagged, label)
		}
	}
	switch result {
	case "not_required":
		return fmt.Sprintf("ไม่พบปัจจัยเสี่ยงสูงตามเกณฑ์ที่ตั้ง (ขั้นต่ำ %d ข้อ)", rule.MinFactors)
	case "recommended":
		return fmt.Sprintf("พบปัจจัยเสี่ยงสูง %d ข้อ (ยังไม่ถึงเกณฑ์บังคับ %d ข้อ): %v", factorCount, rule.MinFactors, flagged)
	default:
		return fmt.Sprintf("พบปัจจัยเสี่ยงสูง %d ข้อ (เข้าเกณฑ์บังคับ %d ข้อ): %v", factorCount, rule.MinFactors, flagged)
	}
}

// GetAssessment returns one DPIA screening round with its factor answers.
func (s *Service) GetAssessment(ctx context.Context, id uuid.UUID) (Assessment, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetAssessment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assessment{}, ErrNotFound
	}
	if err != nil {
		return Assessment{}, err
	}
	factors, err := s.factorsOf(ctx, id)
	if err != nil {
		return Assessment{}, err
	}
	return toAssessment(row, factors), nil
}

func (s *Service) factorsOf(ctx context.Context, assessmentID uuid.UUID) ([]forms.Contribution, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListAnswersForAssessment(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	out := make([]forms.Contribution, 0, len(rows))
	for _, r := range rows {
		var answer any
		_ = json.Unmarshal(r.Answer, &answer)
		out = append(out, forms.Contribution{Question: r.QuestionCode, Answer: answer, Points: 1})
	}
	return out, nil
}

type AssessmentCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

type AssessmentFilter struct {
	ActivityID *uuid.UUID
	After      *AssessmentCursor
	Limit      int
}

const assessmentPageSize = 50

// ListAssessments lists the tenant's DPIA screening rounds (optionally one activity's), newest first —
// without their factor answers (GetAssessment includes those; a list row is meant to be scanned).
func (s *Service) ListAssessments(ctx context.Context, f AssessmentFilter) ([]Assessment, *AssessmentCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > assessmentPageSize {
		limit = assessmentPageSize
	}
	p := dpiastore.ListAssessmentsParams{Lim: int32(limit + 1), ActivityID: pgUUID(f.ActivityID)}
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.CreatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := dpiastore.New(pdb.MustTxFromContext(ctx)).ListAssessments(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Assessment, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &AssessmentCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
		}
		out = append(out, toAssessment(r, nil))
	}
	return out, nil, nil
}

func toRule(r dpiastore.AssessScreeningRule) ScreeningRule {
	var minScore *float64
	if r.MinScore.Valid {
		f, _ := r.MinScore.Float64Value()
		v := f.Float64
		minScore = &v
	}
	return ScreeningRule{ID: r.ID, MinFactors: int(r.MinFactors), MinScore: minScore, CreatedAt: r.CreatedAt.Time}
}

func toAssessment(r dpiastore.AssessAssessment, factors []forms.Contribution) Assessment {
	var score float64
	if r.Score.Valid {
		f, _ := r.Score.Float64Value()
		score = f.Float64
	}
	return Assessment{
		ID: r.ID, TemplateID: r.TemplateID, FormVersionID: r.FormVersionID, Title: r.Title,
		ActivityID: uuidVal(r.ActivityID), RoundNo: int(r.RoundNo), PreviousID: uuidPtr(r.PreviousID),
		Status: r.Status, ScreeningResult: derefStr(r.ScreeningResult), ScreeningReason: derefStr(r.ScreeningReason),
		Score: score, RiskLevel: derefStr(r.RiskLevel), OwnerUserID: uuidPtr(r.OwnerUserID),
		Factors: factors, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time,
	}
}

func uuidVal(v pgtype.UUID) uuid.UUID {
	if !v.Valid {
		return uuid.Nil
	}
	return uuid.UUID(v.Bytes)
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func numeric(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(*f, 'f', 2, 64))
	return n
}
