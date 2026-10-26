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

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
	riskstore "pdpa-platform/internal/risk/store"
)

// ChecklistFormType is the PLT-06 form type RRA-05's per-activity checklist uses (migration 00085
// widened platform.form_definitions.form_type to accept it). The checklist's own questions are never
// hard-coded here (rule 8) — the DPO authors and publishes an ordinary form of this type.
const ChecklistFormType = "checklist"

// ActivityChecklist is one completed run of the checklist against a RoPA processing activity.
type ActivityChecklist struct {
	ID               uuid.UUID
	ActivityID       uuid.UUID
	FormSubmissionID uuid.UUID
	Score            float64
	Result           string
	Factors          []forms.Contribution
	CompletedBy      *uuid.UUID
	CompletedAt      time.Time
	RowVersion       int32
}

// ErrBadChecklistForm is returned when the given form isn't a published checklist-type form.
var ErrBadChecklistForm = errors.New("risk: not a published checklist form")

// FieldError and ValidationError surface the form engine's own per-question validation errors
// (forms.AnswerErrors) without risk/service needing to import forms' error type at every call site.
type FieldError struct{ Field, Code string }
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return fmt.Sprintf("risk: invalid answers: %v", e.Fields) }
func (e *ValidationError) Unwrap() error { return ErrInvalid }

func (s *Service) checklistVersion(ctx context.Context, formID uuid.UUID) (forms.Version, error) {
	f, err := s.Forms.GetForm(ctx, formID)
	if errors.Is(err, forms.ErrNotFound) || errors.Is(err, forms.ErrForbidden) {
		return forms.Version{}, ErrBadChecklistForm
	}
	if err != nil {
		return forms.Version{}, err
	}
	if f.Type != ChecklistFormType || f.CurrentVersionID == nil {
		return forms.Version{}, ErrBadChecklistForm
	}
	for _, v := range f.Versions {
		if v.ID == *f.CurrentVersionID {
			return v, nil
		}
	}
	return forms.Version{}, ErrBadChecklistForm
}

// AnswerChecklist records one run of a published checklist against a RoPA processing activity: the
// answers are validated and scored by the form engine (PLT-06) and stored against the activity itself
// via forms.Record's own entityType/entityID — "ropa_activity" is already a registered PLT-07
// collaboration record type (ROPA-12), so any question the checklist leaves open for a consultant or
// the DPO is a comment on that same activity, not a second channel. This is the acceptance
// criterion's own "คำถามและคำตอบถูกเก็บในกิจกรรม" literally: the submission's answers live on the
// activity through entityID, and any follow-up question/reply lives on the same activity's own
// comment thread.
func (s *Service) AnswerChecklist(ctx context.Context, activityID, formID uuid.UUID, answers forms.Answers) (ActivityChecklist, error) {
	ok, err := s.Ropa.ActivityVisible(ctx, activityID)
	if err != nil {
		return ActivityChecklist{}, err
	}
	if !ok {
		return ActivityChecklist{}, fmt.Errorf("%w: activity_id", ErrInvalid)
	}
	v, err := s.checklistVersion(ctx, formID)
	if err != nil {
		return ActivityChecklist{}, err
	}
	g, _ := authz.FromContext(ctx)
	var me *uuid.UUID
	if u, err := uuid.Parse(g.UserID); err == nil {
		me = &u
	}
	subID, res, err := s.Forms.Record(ctx, v.ID, answers, "user", me, "ropa_activity", &activityID)
	var ae *forms.AnswerErrors
	if errors.As(err, &ae) {
		ve := &ValidationError{}
		for _, f := range ae.Fields {
			ve.Fields = append(ve.Fields, FieldError{Field: "answers." + f.Question, Code: f.Code})
		}
		return ActivityChecklist{}, ve
	}
	if err != nil {
		return ActivityChecklist{}, err
	}
	factors := forms.Contributions(v.Schema, res.Answers)
	raw, _ := json.Marshal(factors)
	id, err := uuid.NewV7()
	if err != nil {
		return ActivityChecklist{}, err
	}
	now := time.Now().UTC()
	var score pgtype.Numeric
	if err := score.Scan(strconv.FormatFloat(res.Score, 'f', 2, 64)); err != nil {
		return ActivityChecklist{}, err
	}
	q := riskstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertActivityChecklist(ctx, riskstore.InsertActivityChecklistParams{ID: id, ActivityID: activityID,
		FormSubmissionID: subID, Score: score, Result: opt(res.Band), Factors: raw, CompletedBy: pgUUID(me),
		CompletedAt: pgtype.Timestamptz{Time: now, Valid: true}})
	if err != nil {
		return ActivityChecklist{}, err
	}
	if err := s.audit(ctx, "risk.activity_checklist.record", "activity_checklist", row.ID, nil,
		map[string]any{"activity_id": activityID, "score": res.Score, "result": res.Band}); err != nil {
		return ActivityChecklist{}, err
	}
	return toActivityChecklist(riskstore.InsertActivityChecklistRow(row)), nil
}

// ListActivityChecklists returns every past checklist run against an activity, newest first.
func (s *Service) ListActivityChecklists(ctx context.Context, activityID uuid.UUID) ([]ActivityChecklist, error) {
	ok, err := s.Ropa.ActivityVisible(ctx, activityID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: activity_id", ErrInvalid)
	}
	rows, err := riskstore.New(pdb.MustTxFromContext(ctx)).ListActivityChecklists(ctx, activityID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityChecklist, 0, len(rows))
	for _, r := range rows {
		out = append(out, toActivityChecklist(riskstore.InsertActivityChecklistRow(r)))
	}
	return out, nil
}

// GetActivityChecklist returns one checklist run.
func (s *Service) GetActivityChecklist(ctx context.Context, id uuid.UUID) (ActivityChecklist, error) {
	row, err := riskstore.New(pdb.MustTxFromContext(ctx)).GetActivityChecklist(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivityChecklist{}, ErrNotFound
	}
	if err != nil {
		return ActivityChecklist{}, err
	}
	return toActivityChecklist(riskstore.InsertActivityChecklistRow(row)), nil
}

func toActivityChecklist(r riskstore.InsertActivityChecklistRow) ActivityChecklist {
	score, _ := r.Score.Float64Value()
	var factors []forms.Contribution
	_ = json.Unmarshal(r.Factors, &factors)
	return ActivityChecklist{ID: r.ID, ActivityID: r.ActivityID, FormSubmissionID: r.FormSubmissionID, Score: score.Float64,
		Result: derefStr(r.Result), Factors: factors, CompletedBy: uuidPtr(r.CompletedBy), CompletedAt: r.CompletedAt.Time,
		RowVersion: r.RowVersion}
}
