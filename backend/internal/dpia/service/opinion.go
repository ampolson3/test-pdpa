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

	dpiastore "pdpa-platform/internal/dpia/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// ErrForbidden is returned when the caller holds assessment.dpia.update (enough to submit/resume) but not
// the .approve a decision or close needs — DPIA-10's own actor line (DPO/EXEC) beyond what the generic
// endpoint permission alone would allow.
var ErrForbidden = errors.New("dpia: forbidden")

// PermApprove gates a decision and closing (DPIA-10) — only DPO holds it in the seeded RBAC; the module
// doc's own EXEC actor has no grant on assessment.dpia in the baseline seed (the same gap DPIA-02/DPIA-05
// already documented for their own OWNER/DPO actor lines).
const PermApprove = "assessment.dpia.approve"

// Opinion is one assess.dpo_opinions row (DPIA-10): the DPO's written opinion and recommendation on a round
// already screened "in".
type Opinion struct {
	ID             uuid.UUID
	AssessmentID   uuid.UUID
	DpoUserID      uuid.UUID
	Opinion        string
	Recommendation string
	CreatedAt      time.Time
}

var recommendations = map[string]bool{"proceed": true, "proceed_with_conditions": true, "do_not_proceed": true, "consult_pdpc": true}

// assessTransitions is ST-05#2 (docs/states/state-machines.yaml) beyond screening's own two initial edges
// (screening -> not_required | in_progress, already written by Screen): the review/approval graph DPIA-10
// adds. screening/not_required are deliberately absent here — a round never re-enters screening, and
// not_required only ever reaches closed.
var assessTransitions = map[[2]string]bool{
	{"in_progress", "in_review"}:    true, // ส่งตรวจ
	{"in_review", "in_progress"}:    true, // ขอข้อมูลเพิ่ม
	{"in_review", "approved"}:       true, // อนุมัติ
	{"in_review", "rejected"}:       true, // ไม่อนุมัติ
	{"approved", "needs_review"}:    true, // ครบรอบ / กิจกรรมเปลี่ยน
	{"needs_review", "in_progress"}: true, // เริ่มทบทวน
	{"approved", "closed"}:          true, // ปิด
	{"rejected", "closed"}:          true, // ปิด
	{"not_required", "closed"}:      true, // ปิด
}

// decisions requiring authz.PermApprove, not just the endpoint's own assessment.dpia.update: entering any
// of these is the module doc's "ผู้บริหารอนุมัติหรือยอมรับความเสี่ยง" step, never a plain edit.
var approverOnly = map[string]bool{"approved": true, "rejected": true, "closed": true}

func has(ctx context.Context, perm string) bool {
	g, _ := authz.FromContext(ctx)
	return g.Has(perm)
}

// Transition is DPIA-10's state machine entry point (ST-05#2): submit for review, request more info, decide
// (approved/rejected/needs_review) and close — the four edges a round takes beyond DPIA-01/02's own
// screening. reason is required entering rejected/needs_review (the module doc's own "บันทึกเหตุผล").
func (s *Service) Transition(ctx context.Context, id uuid.UUID, version int32, to, reason string) (Assessment, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetAssessment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assessment{}, ErrNotFound
	}
	if err != nil {
		return Assessment{}, err
	}
	if row.RowVersion != version {
		return Assessment{}, ErrVersionMismatch
	}
	if !assessTransitions[[2]string{row.Status, to}] {
		return Assessment{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, row.Status, to)
	}
	if approverOnly[to] && !has(ctx, PermApprove) {
		return Assessment{}, ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if (to == "rejected" || to == "needs_review") && reason == "" {
		return Assessment{}, fmt.Errorf("%w: reason", ErrInvalid)
	}
	// DPIA-10's acceptance criterion literally: closing a round that was actually assessed (approved or
	// rejected — not_required never had anything to opine on) needs at least one DPO opinion already on
	// record. "has an opinion" is the acceptance criterion's own stand-in for "DPO ให้ความเห็น... ครบ".
	if to == "closed" && row.Status != "not_required" {
		opinions, err := q.ListOpinionsForAssessment(ctx, id)
		if err != nil {
			return Assessment{}, err
		}
		if len(opinions) == 0 {
			return Assessment{}, fmt.Errorf("%w: opinion_required", ErrInvalid)
		}
	}
	var approvedAt pgtype.Timestamptz
	if to == "approved" {
		approvedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	} else if row.ApprovedAt.Valid {
		approvedAt = row.ApprovedAt
	}
	updated, err := q.SetAssessmentStatus(ctx, dpiastore.SetAssessmentStatusParams{ID: id, Status: to, ApprovedAt: approvedAt, RowVersion: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return Assessment{}, ErrVersionMismatch
	}
	if err != nil {
		return Assessment{}, err
	}
	if err := s.audit(ctx, "dpia.assessment.transition", AssessmentEntityType, id,
		map[string]any{"status": row.Status}, map[string]any{"status": to, "reason": reason}); err != nil {
		return Assessment{}, err
	}
	return toAssessment(updated, nil), nil
}

// ErrInvalidTransition is returned for an edge ST-05#2 doesn't declare.
var ErrInvalidTransition = errors.New("dpia: invalid transition")

// RecordOpinion is DPIA-10's other half: the DPO's written opinion and recommendation on a round under
// review — recorded while the round is in_progress or in_review (before any decision), never on a closed
// or not_required round.
func (s *Service) RecordOpinion(ctx context.Context, assessmentID uuid.UUID, opinion, recommendation string) (Opinion, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetAssessment(ctx, assessmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Opinion{}, ErrNotFound
	}
	if err != nil {
		return Opinion{}, err
	}
	if row.Status != "in_progress" && row.Status != "in_review" {
		return Opinion{}, fmt.Errorf("%w: status", ErrInvalidTransition)
	}
	opinion = strings.TrimSpace(opinion)
	if opinion == "" {
		return Opinion{}, fmt.Errorf("%w: opinion", ErrInvalid)
	}
	if !recommendations[recommendation] {
		return Opinion{}, fmt.Errorf("%w: recommendation", ErrInvalid)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Opinion{}, err
	}
	r, err := q.InsertDpoOpinion(ctx, dpiastore.InsertDpoOpinionParams{ID: id, AssessmentID: assessmentID, Opinion: opinion, Recommendation: recommendation})
	if err != nil {
		return Opinion{}, err
	}
	if err := s.audit(ctx, "dpia.opinion.record", AssessmentEntityType, assessmentID, nil,
		map[string]any{"recommendation": recommendation}); err != nil {
		return Opinion{}, err
	}
	return toOpinion(r), nil
}

// ListOpinions returns every DPO opinion recorded against an assessment round, oldest first.
func (s *Service) ListOpinions(ctx context.Context, assessmentID uuid.UUID) ([]Opinion, error) {
	if _, err := s.GetAssessment(ctx, assessmentID); err != nil {
		return nil, err
	}
	rows, err := dpiastore.New(pdb.MustTxFromContext(ctx)).ListOpinionsForAssessment(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	out := make([]Opinion, 0, len(rows))
	for _, r := range rows {
		out = append(out, toOpinion(r))
	}
	return out, nil
}

func toOpinion(r dpiastore.AssessDpoOpinion) Opinion {
	return Opinion{ID: r.ID, AssessmentID: r.AssessmentID, DpoUserID: r.DpoUserID, Opinion: r.Opinion,
		Recommendation: r.Recommendation, CreatedAt: r.CreatedAt.Time}
}
