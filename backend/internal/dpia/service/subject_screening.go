package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

// SubjectScreeningResult is the outcome of a generic, non-DPIA assess.templates screening round — built for
// DPO-02 (docs/decisions.md Q-37), reusable by any future module that needs the same "answer a published
// yes_no checklist against assess.templates/assess.assessments, get a caller-defined result" shape without
// going through DPIA-01/02's own activity-specific Screen (which stays untouched by this feature, rule 9:
// dpia owns the assess schema, but a caller-supplied Decide function keeps the module-specific judgement —
// e.g. DPO-02's own ม.41(2) OR-rule — out of dpia entirely).
type SubjectScreeningResult struct {
	AssessmentID    uuid.UUID
	RoundNo         int
	ScreeningResult string
	ScreeningReason string
	Factors         []forms.Contribution
	Score           float64
	MaxScore        float64
	Band            string
	// SectionScores is DPX-03's own addition (the first caller needing a per-dimension breakdown, not just
	// an overall score): one entry per schema section, scored by re-running the same engine (forms.Evaluate)
	// scoped to that section alone — the exact use the engine's own `sections []string` parameter already
	// anticipated.
	SectionScores []SectionScore
}

// SectionScore is one schema section's own score, e.g. one "dimension" of a maturity assessment.
type SectionScore struct {
	Key      string
	Title    forms.Text
	Score    float64
	MaxScore float64
}

// Decide computes a screening's result/reason from its answered factors — the module-specific rule a caller
// (e.g. dpo's own ม.41(2) OR-criteria) supplies; dpia itself holds no opinion on what the answers mean.
type Decide func(factorCount int, factors []forms.Contribution) (result, reason string)

// ScreenSubject answers a published assess.templates checklist (assessment_type/code) for any subject_type/
// subject_id, in the caller's own request transaction, and stores the result exactly the way DPIA-01's own
// Screen does (one assess.assessments row + one assess.answers row per factor, round-chained via previous_id)
// — minus any activity_id (this is for a non-activity subject) and minus DPIA-02's own threshold machinery
// (the caller's Decide function already encodes the judgement).
func (s *Service) ScreenSubject(ctx context.Context, assessmentType, templateCode, subjectType string, subjectID uuid.UUID, title string, answers forms.Answers, decide Decide) (SubjectScreeningResult, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	tmpl, err := q.GetTemplateByCodeAndType(ctx, dpiastore.GetTemplateByCodeAndTypeParams{AssessmentType: assessmentType, Code: templateCode})
	if errors.Is(err, pgx.ErrNoRows) {
		return SubjectScreeningResult{}, ErrBadTemplate
	}
	if err != nil {
		return SubjectScreeningResult{}, err
	}
	v, err := s.screeningVersion(ctx, tmpl.FormID)
	if err != nil {
		return SubjectScreeningResult{}, err
	}
	res := forms.Evaluate(v.Schema, v.Scoring, answers, true, nil)
	if len(res.Errors) > 0 {
		ve := &ValidationError{}
		for _, e := range res.Errors {
			ve.Fields = append(ve.Fields, FieldError{"answers." + e.Question, e.Code})
		}
		return SubjectScreeningResult{}, ve
	}
	factors := forms.Contributions(v.Schema, res.Answers)
	sectionScores := make([]SectionScore, 0, len(v.Schema.Sections))
	for _, sec := range v.Schema.Sections {
		secRes := forms.Evaluate(v.Schema, v.Scoring, answers, false, []string{sec.Key})
		sectionScores = append(sectionScores, SectionScore{Key: sec.Key, Title: sec.Title, Score: secRes.Score, MaxScore: secRes.MaxScore})
	}
	factorCount := 0
	for _, f := range factors {
		if f.Points > 0 {
			factorCount++
		}
	}
	result, reason := decide(factorCount, factors)

	round := 1
	var previousID *uuid.UUID
	prevRow, err := q.GetLatestAssessmentForSubject(ctx, dpiastore.GetLatestAssessmentForSubjectParams{
		AssessmentType: assessmentType, SubjectID: pgUUID(&subjectID),
	})
	if err == nil {
		round = int(prevRow.RoundNo) + 1
		previousID = &prevRow.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return SubjectScreeningResult{}, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return SubjectScreeningResult{}, err
	}
	row, err := q.InsertAssessment(ctx, dpiastore.InsertAssessmentParams{
		ID: id, AssessmentType: assessmentType, TemplateID: tmpl.ID, FormVersionID: v.ID, Title: title,
		SubjectType: subjectType, SubjectID: pgUUID(&subjectID), RoundNo: int16(round), PreviousID: pgUUID(previousID),
		Status: "closed", ScreeningResult: &result, ScreeningReason: &reason, Score: numeric(&res.Score),
	})
	if err != nil {
		return SubjectScreeningResult{}, err
	}
	for _, f := range factors {
		aid, err := uuid.NewV7()
		if err != nil {
			return SubjectScreeningResult{}, err
		}
		raw, _ := json.Marshal(f.Answer)
		if _, err := q.InsertAnswer(ctx, dpiastore.InsertAnswerParams{ID: aid, AssessmentID: id, QuestionCode: f.Question, Answer: raw}); err != nil {
			return SubjectScreeningResult{}, err
		}
	}
	return SubjectScreeningResult{AssessmentID: row.ID, RoundNo: round, ScreeningResult: result, ScreeningReason: reason, Factors: factors,
		Score: res.Score, MaxScore: res.MaxScore, Band: res.Band, SectionScores: sectionScores}, nil
}
