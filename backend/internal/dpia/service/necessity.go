package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

// NecessityTemplateCode is the seeded global template's code (migration 00048, docs/decisions.md Q-27).
const NecessityTemplateCode = "dpia_necessity"

// NecessitySectionCode groups this checklist's own answers under assess.sections/assess.answers, separate
// from DPIA-01's screening answers (which keep section_id NULL) — so re-reading a necessity assessment never
// bleeds into Assessment.Factors, and vice versa.
const NecessitySectionCode = "necessity"

// NecessityAssessment is DPIA-05's acceptance criterion: answer the necessity/proportionality checklist for
// a DPIA assessment and get an immediate conclusion.
type NecessityAssessment struct {
	AssessmentID uuid.UUID
	Result       string // necessary | needs_review
	Missing      []string
	Answers      []forms.Contribution
	SubmittedAt  time.Time
}

func (s *Service) necessityTemplate(ctx context.Context) (dpiastore.AssessTemplate, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetScreeningTemplate(ctx, NecessityTemplateCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return dpiastore.AssessTemplate{}, ErrBadTemplate
	}
	return row, err
}

// necessityResult is the pure rule behind the acceptance criterion: "necessary" only when every question is
// answered "yes" (data minimization, purpose specificity, an appropriate lawful basis, a less invasive
// alternative considered — ม.22/24/26); any other answer is flagged and the checklist reads "needs_review".
func necessityResult(schema forms.Schema, answers forms.Answers) (result string, missing []string) {
	for _, sec := range schema.Sections {
		for _, q := range sec.Questions {
			if answers[q.Key] != "yes" {
				missing = append(missing, q.Key)
			}
		}
	}
	if len(missing) == 0 {
		return "necessary", nil
	}
	return "needs_review", missing
}

// AssessNecessity answers (or re-answers) DPIA-05's necessity checklist for an in-progress DPIA assessment.
// Re-answering replaces the prior submission (a redo, not a new round like DPIA-01's own screening) — this
// checklist has no threshold configuration to make a fresh round meaningful the way re-screening does.
func (s *Service) AssessNecessity(ctx context.Context, assessmentID uuid.UUID, answers forms.Answers) (NecessityAssessment, error) {
	if _, err := s.GetAssessment(ctx, assessmentID); err != nil {
		return NecessityAssessment{}, err
	}
	tmpl, err := s.necessityTemplate(ctx)
	if err != nil {
		return NecessityAssessment{}, err
	}
	v, err := s.screeningVersion(ctx, tmpl.FormID)
	if err != nil {
		return NecessityAssessment{}, err
	}
	res := forms.Evaluate(v.Schema, v.Scoring, answers, true, nil)
	if len(res.Errors) > 0 {
		ve := &ValidationError{}
		for _, e := range res.Errors {
			ve.Fields = append(ve.Fields, FieldError{"answers." + e.Question, e.Code})
		}
		return NecessityAssessment{}, ve
	}
	contributions := forms.Contributions(v.Schema, res.Answers)

	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	section, err := q.GetSectionByCode(ctx, dpiastore.GetSectionByCodeParams{AssessmentID: assessmentID, SectionCode: NecessitySectionCode})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		id, err := uuid.NewV7()
		if err != nil {
			return NecessityAssessment{}, err
		}
		section, err = q.InsertSection(ctx, dpiastore.InsertSectionParams{ID: id, AssessmentID: assessmentID, SectionCode: NecessitySectionCode})
		if err != nil {
			return NecessityAssessment{}, err
		}
	case err != nil:
		return NecessityAssessment{}, err
	default:
		if err := q.DeleteAnswersForSection(ctx, pgUUID(&section.ID)); err != nil {
			return NecessityAssessment{}, err
		}
		if section, err = q.ResubmitSection(ctx, section.ID); err != nil {
			return NecessityAssessment{}, err
		}
	}

	for _, c := range contributions {
		aid, err := uuid.NewV7()
		if err != nil {
			return NecessityAssessment{}, err
		}
		raw, _ := json.Marshal(c.Answer)
		if _, err := q.InsertSectionAnswer(ctx, dpiastore.InsertSectionAnswerParams{
			ID: aid, AssessmentID: assessmentID, SectionID: pgUUID(&section.ID), QuestionCode: c.Question, Answer: raw,
		}); err != nil {
			return NecessityAssessment{}, err
		}
	}

	result, missing := necessityResult(v.Schema, res.Answers)
	if err := s.audit(ctx, "dpia.assessment.necessity", AssessmentEntityType, assessmentID, nil,
		map[string]any{"result": result, "missing": missing}); err != nil {
		return NecessityAssessment{}, err
	}

	return NecessityAssessment{AssessmentID: assessmentID, Result: result, Missing: missing, Answers: contributions, SubmittedAt: section.SubmittedAt.Time}, nil
}

// GetNecessity reads back the DPIA assessment's necessity checklist, recomputing the result live from the
// stored answers (never a stale persisted verdict).
func (s *Service) GetNecessity(ctx context.Context, assessmentID uuid.UUID) (NecessityAssessment, error) {
	if _, err := s.GetAssessment(ctx, assessmentID); err != nil {
		return NecessityAssessment{}, err
	}
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	section, err := q.GetSectionByCode(ctx, dpiastore.GetSectionByCodeParams{AssessmentID: assessmentID, SectionCode: NecessitySectionCode})
	if errors.Is(err, pgx.ErrNoRows) {
		return NecessityAssessment{}, ErrNotFound
	}
	if err != nil {
		return NecessityAssessment{}, err
	}
	rows, err := q.ListAnswersForSection(ctx, pgUUID(&section.ID))
	if err != nil {
		return NecessityAssessment{}, err
	}
	tmpl, err := s.necessityTemplate(ctx)
	if err != nil {
		return NecessityAssessment{}, err
	}
	v, err := s.screeningVersion(ctx, tmpl.FormID)
	if err != nil {
		return NecessityAssessment{}, err
	}

	answers := forms.Answers{}
	contributions := make([]forms.Contribution, 0, len(rows))
	for _, r := range rows {
		var answer any
		_ = json.Unmarshal(r.Answer, &answer)
		if str, ok := answer.(string); ok {
			answers[r.QuestionCode] = str
		}
		contributions = append(contributions, forms.Contribution{Question: r.QuestionCode, Answer: answer})
	}
	result, missing := necessityResult(v.Schema, answers)

	submittedAt := time.Time{}
	if section.SubmittedAt.Valid {
		submittedAt = section.SubmittedAt.Time
	}
	return NecessityAssessment{AssessmentID: assessmentID, Result: result, Missing: missing, Answers: contributions, SubmittedAt: submittedAt}, nil
}
