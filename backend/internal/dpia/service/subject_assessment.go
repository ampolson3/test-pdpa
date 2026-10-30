package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	dpiastore "pdpa-platform/internal/dpia/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

// SubjectAssessmentResult is the minimal result RecordSubjectAssessment's own caller needs — unlike the
// DPIA-01 screening's own Assessment (which is tied to subject_type "activity" by field name, ActivityID),
// this stays subject-type-agnostic.
type SubjectAssessmentResult struct {
	ID        uuid.UUID
	Score     float64
	MaxScore  float64
	CreatedAt time.Time
}

// RecordSubjectAssessment is a generic counterpart to Screen for any assess.assessments subject besides
// "activity" — today, vendor module's VEN-07. It answers one published template's form in one atomic call
// (the same one-shot "Record" pattern BRE-05/DPO-09 already use elsewhere) and persists the result as a
// closed assess.assessments row with no review cycle of its own, since this generic entry point has none —
// a module that needs one (DPO opinion, approval, etc.) builds it on top, the same way DPIA-10 already
// layers ST-05#2 onto DPIA-01's own screening rows.
func (s *Service) RecordSubjectAssessment(ctx context.Context, templateID uuid.UUID, subjectType string, subjectID uuid.UUID, title string, answers forms.Answers) (SubjectAssessmentResult, error) {
	tpl, err := s.GetTemplateByID(ctx, templateID)
	if err != nil {
		return SubjectAssessmentResult{}, err
	}
	if tpl.Status != "published" {
		return SubjectAssessmentResult{}, ErrBadTemplate
	}
	v, err := s.screeningVersion(ctx, tpl.FormID)
	if err != nil {
		return SubjectAssessmentResult{}, err
	}
	g, _ := authz.FromContext(ctx)
	var me *uuid.UUID
	if u, err := uuid.Parse(g.UserID); err == nil {
		me = &u
	}
	_, res, err := s.Forms.Record(ctx, v.ID, answers, "user", me, "", nil)
	var ae *forms.AnswerErrors
	if errors.As(err, &ae) {
		ve := &ValidationError{}
		for _, f := range ae.Fields {
			ve.Fields = append(ve.Fields, FieldError{"answers." + f.Question, f.Code})
		}
		return SubjectAssessmentResult{}, ve
	}
	if err != nil {
		return SubjectAssessmentResult{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return SubjectAssessmentResult{}, err
	}
	score := res.Score
	row, err := dpiastore.New(pdb.MustTxFromContext(ctx)).InsertAssessment(ctx, dpiastore.InsertAssessmentParams{
		ID: id, AssessmentType: tpl.AssessmentType, TemplateID: templateID, FormVersionID: v.ID, Title: title,
		SubjectType: subjectType, SubjectID: pgUUID(&subjectID), ActivityID: pgUUID(nil), RoundNo: 1,
		PreviousID: pgUUID(nil), Status: "closed", Score: numeric(&score),
	})
	if err != nil {
		return SubjectAssessmentResult{}, err
	}
	return SubjectAssessmentResult{ID: row.ID, Score: res.Score, MaxScore: res.MaxScore, CreatedAt: row.CreatedAt.Time}, nil
}
