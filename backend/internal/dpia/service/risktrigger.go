package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// openStatuses are the ST-05#2 states that still need a decision — a second high score while one of these
// is already open must not spawn a duplicate round (RRA-03's own idempotency: Score is called on every
// page view and every "recompute").
var openStatuses = map[string]bool{"screening": true, "in_progress": true, "in_review": true, "needs_review": true}

// TriggerFromRiskScore is RRA-03: a RRA-01 risk score of "high" or "very_high" opens a DPIA directly at
// in_progress — screening_result is fixed to "required" (the risk score itself already establishes the
// need; there is no screening questionnaire to answer again) and the activity's own owner (if any) is
// assigned automatically, per the module doc's own "สร้าง DPIA (สถานะคัดกรอง) + มอบหมายเจ้าของกิจกรรม".
// Implements risk/service's own local DpiaTrigger interface (rule 9: risk never imports dpia back, since
// dpia already imports risk for ROPA-09's security-controls catalog).
func (s *Service) TriggerFromRiskScore(ctx context.Context, activityID uuid.UUID, score float64, level string) error {
	if level != "high" && level != "very_high" {
		return nil
	}
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	round := 1
	var previousID *uuid.UUID
	prevRow, err := q.GetLatestAssessmentForActivity(ctx, pgUUID(&activityID))
	switch {
	case err == nil:
		if openStatuses[prevRow.Status] {
			return nil
		}
		round = int(prevRow.RoundNo) + 1
		previousID = &prevRow.ID
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}

	activity, err := s.Ropa.GetActivity(ctx, activityID)
	if err != nil {
		return fmt.Errorf("%w: activity_id", ErrInvalid)
	}
	tmpl, err := s.screeningTemplate(ctx)
	if err != nil {
		return err
	}
	v, err := s.screeningVersion(ctx, tmpl.FormID)
	if err != nil {
		return err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	result := "required"
	reason := fmt.Sprintf("คะแนนความเสี่ยงระดับ %s จากการประเมินความเสี่ยงรายกิจกรรม (RRA-01)", riskLevelLabel(level))
	title := fmt.Sprintf("คัดกรอง DPIA: %s (%s)", activity.Name, activity.Code)
	row, err := q.InsertRiskTriggeredAssessment(ctx, dpiastore.InsertRiskTriggeredAssessmentParams{
		ID: id, AssessmentType: "dpia", TemplateID: tmpl.ID, FormVersionID: v.ID, Title: title, SubjectType: "activity",
		SubjectID: pgUUID(&activityID), ActivityID: pgUUID(&activityID), RoundNo: int16(round), PreviousID: pgUUID(previousID),
		Status: "in_progress", ScreeningResult: &result, ScreeningReason: &reason, Score: numeric(&score), RiskLevel: &level,
		OwnerUserID: pgUUID(activity.OwnerUserID),
	})
	if err != nil {
		return err
	}
	return s.audit(ctx, "dpia.assessment.risk_trigger", AssessmentEntityType, row.ID, nil,
		map[string]any{"activity_id": activityID, "round_no": round, "risk_level": level, "score": score})
}

func riskLevelLabel(level string) string {
	switch level {
	case "very_high":
		return "สูงมาก"
	default:
		return "สูง"
	}
}
