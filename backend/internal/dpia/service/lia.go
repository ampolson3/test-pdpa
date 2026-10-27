package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

// LiaTemplateCode is the seeded global LIA template's code (migration 00086, docs/decisions.md Q-41).
const LiaTemplateCode = "dpia_lia"

// liaTransitions is this feature's own, deliberately small machine — unlike DPIA-10's full ST-05#2 review
// graph, a legitimate-interest round only ever goes from answered to decided: in_progress -> approved |
// rejected. Re-answering (a fresh AssessLIA call) always opens a new round rather than reopening a decided
// one, so there is no edge back out of either terminal state.
var liaTransitions = map[[2]string]bool{
	{"in_progress", "approved"}: true,
	{"in_progress", "rejected"}: true,
}

func (s *Service) liaTemplate(ctx context.Context) (dpiastore.AssessTemplate, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetTemplateByCodeAndType(ctx, dpiastore.GetTemplateByCodeAndTypeParams{AssessmentType: "lia", Code: LiaTemplateCode})
	if errors.Is(err, pgx.ErrNoRows) {
		return dpiastore.AssessTemplate{}, ErrBadTemplate
	}
	return row, err
}

// AssessLIA is DPIA-16's own answer step: the DPO (or OWNER) works through the published 3-part LIA test
// (purpose / necessity / balancing, ม.24(5)) against a RoPA activity and opens a new round pending DPO
// approval — the acceptance criterion itself ("ต้องมี LIA ที่อนุมัติ") names an *approved* LIA, so this
// never writes "approved" itself; DecideLIA does. Re-answering always opens a fresh round (round-chained via
// previous_id, the same shape DPIA-01's own Screen uses), never edits a past one in place.
func (s *Service) AssessLIA(ctx context.Context, activityID uuid.UUID, answers forms.Answers) (Assessment, error) {
	activity, err := s.Ropa.GetActivity(ctx, activityID)
	if err != nil {
		return Assessment{}, fmt.Errorf("%w: activity_id", ErrInvalid)
	}
	tmpl, err := s.liaTemplate(ctx)
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

	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	round := 1
	var previousID *uuid.UUID
	prevRow, err := q.GetLatestLiaForActivity(ctx, pgUUID(&activityID))
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
	title := fmt.Sprintf("LIA: %s (%s)", activity.Name, activity.Code)
	row, err := q.InsertAssessment(ctx, dpiastore.InsertAssessmentParams{
		ID: id, AssessmentType: "lia", TemplateID: tmpl.ID, FormVersionID: v.ID, Title: title, SubjectType: "activity",
		SubjectID: pgUUID(&activityID), ActivityID: pgUUID(&activityID), RoundNo: int16(round), PreviousID: pgUUID(previousID),
		Status: "in_progress", Score: numeric(&res.Score),
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
	if err := s.audit(ctx, "dpia.lia.record", AssessmentEntityType, id, nil,
		map[string]any{"activity_id": activityID, "round_no": round}); err != nil {
		return Assessment{}, err
	}
	return toAssessment(row, factors), nil
}

// DecideLIA is the DPO's own sign-off on an answered LIA round — approved or rejected, needing
// assessment.dpia.approve beyond the endpoint's own assessment.dpia.update (the same approver-only gate
// DPIA-10's Transition already uses for its own decisions). A reason is required rejecting, the same
// discipline DPIA-10's own rejected/needs_review edges already apply.
func (s *Service) DecideLIA(ctx context.Context, id uuid.UUID, version int32, to, reason string) (Assessment, error) {
	if !has(ctx, PermApprove) {
		return Assessment{}, ErrForbidden
	}
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetLiaAssessment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assessment{}, ErrNotFound
	}
	if err != nil {
		return Assessment{}, err
	}
	if row.RowVersion != version {
		return Assessment{}, ErrVersionMismatch
	}
	if !liaTransitions[[2]string{row.Status, to}] {
		return Assessment{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, row.Status, to)
	}
	reason = strings.TrimSpace(reason)
	if to == "rejected" && reason == "" {
		return Assessment{}, fmt.Errorf("%w: reason", ErrInvalid)
	}
	var approvedAt pgtype.Timestamptz
	if to == "approved" {
		approvedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}
	updated, err := q.SetLiaAssessmentStatus(ctx, dpiastore.SetLiaAssessmentStatusParams{ID: id, Status: to, ApprovedAt: approvedAt, RowVersion: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return Assessment{}, ErrVersionMismatch
	}
	if err != nil {
		return Assessment{}, err
	}
	if err := s.audit(ctx, "dpia.lia.transition", AssessmentEntityType, id,
		map[string]any{"status": row.Status}, map[string]any{"status": to, "reason": reason}); err != nil {
		return Assessment{}, err
	}
	factors, err := s.factorsOf(ctx, id)
	if err != nil {
		return Assessment{}, err
	}
	return toAssessment(updated, factors), nil
}

// GetLIA returns one legitimate-interest assessment round with its factor answers.
func (s *Service) GetLIA(ctx context.Context, id uuid.UUID) (Assessment, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetLiaAssessment(ctx, id)
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

// ListLIAForActivity returns every LIA round recorded against an activity, newest first.
func (s *Service) ListLIAForActivity(ctx context.Context, activityID uuid.UUID) ([]Assessment, error) {
	if _, err := s.Ropa.GetActivity(ctx, activityID); err != nil {
		return nil, fmt.Errorf("%w: activity_id", ErrInvalid)
	}
	rows, err := dpiastore.New(pdb.MustTxFromContext(ctx)).ListLiaForActivity(ctx, pgUUID(&activityID))
	if err != nil {
		return nil, err
	}
	out := make([]Assessment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAssessment(r, nil))
	}
	return out, nil
}

// HasApprovedLIA backs ROPA-03's own completeness check (DPIA-16's acceptance criterion taken literally:
// "กิจกรรมที่ใช้ฐาน ม.24(5) ต้องมี LIA ที่อนุมัติ"): whether the activity's own latest LIA round is approved.
// No round at all, or a round still in_progress/rejected, both read false — only the most recent round
// counts, the same "latest, not ever" reading RRA-08's own acceptance-review check uses.
func (s *Service) HasApprovedLIA(ctx context.Context, activityID uuid.UUID) (bool, error) {
	row, err := dpiastore.New(pdb.MustTxFromContext(ctx)).GetLatestLiaForActivity(ctx, pgUUID(&activityID))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.Status == "approved", nil
}
