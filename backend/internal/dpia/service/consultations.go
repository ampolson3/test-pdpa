package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// consultationStakeholderTypes mirrors assess.consultations.stakeholder_type's own CHECK constraint
// (migration 00011) exactly — the table was already fully specified, unused until this feature.
var consultationStakeholderTypes = []string{"data_subject", "processor", "expert", "internal", "regulator"}

// Consultation is DPIA-11's own record: who was consulted, when, the issue raised (summary) and the
// explanation/objection response — "บันทึกการปรึกษาเจ้าของข้อมูล ผู้ประมวลผล หรือผู้เชี่ยวชาญ และข้อโต้แย้ง
// พร้อมคำชี้แจง" read literally: summary is the issue/objection, response is the explanation given back.
type Consultation struct {
	ID              uuid.UUID
	AssessmentID    uuid.UUID
	Stakeholder     string
	StakeholderType string
	ConsultedAt     time.Time
	Summary         string
	Response        string
	RowVersion      int32
	CreatedAt       time.Time
}

func toConsultation(stakeholder string, stakeholderType string, consultedAt pgtype.Date, summary string, response *string, id, assessmentID uuid.UUID, rowVersion int32, createdAt pgtype.Timestamptz) Consultation {
	c := Consultation{ID: id, AssessmentID: assessmentID, Stakeholder: stakeholder, StakeholderType: stakeholderType,
		Summary: summary, RowVersion: rowVersion}
	if consultedAt.Valid {
		c.ConsultedAt = consultedAt.Time
	}
	if createdAt.Valid {
		c.CreatedAt = createdAt.Time
	}
	if response != nil {
		c.Response = *response
	}
	return c
}

// AddConsultation records one stakeholder consultation on a DPIA assessment (DPIA-11's own acceptance
// criterion's data half) — GetAssessment already proves the assessment is the caller's tenant (rule 1), the
// same reasoning DPIA-04/05's own child writes already use, so no second RLS check is needed here.
func (s *Service) AddConsultation(ctx context.Context, assessmentID uuid.UUID, stakeholder, stakeholderType string, consultedAt time.Time, summary, response string) (Consultation, error) {
	if _, err := s.GetAssessment(ctx, assessmentID); err != nil {
		return Consultation{}, err
	}
	stakeholder = strings.TrimSpace(stakeholder)
	summary = strings.TrimSpace(summary)
	var fields []FieldError
	if stakeholder == "" {
		fields = append(fields, FieldError{"stakeholder", "required"})
	}
	if !slices.Contains(consultationStakeholderTypes, stakeholderType) {
		fields = append(fields, FieldError{"stakeholder_type", "invalid"})
	}
	if summary == "" {
		fields = append(fields, FieldError{"summary", "required"})
	}
	if consultedAt.IsZero() {
		fields = append(fields, FieldError{"consulted_at", "required"})
	}
	if len(fields) > 0 {
		return Consultation{}, &ValidationError{Fields: fields}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Consultation{}, err
	}
	var respPtr *string
	if r := strings.TrimSpace(response); r != "" {
		respPtr = &r
	}
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertConsultation(ctx, dpiastore.InsertConsultationParams{
		ID: id, AssessmentID: assessmentID, Stakeholder: stakeholder, StakeholderType: stakeholderType,
		ConsultedAt: pgDate(&consultedAt), Summary: summary, Response: respPtr,
	})
	if err != nil {
		return Consultation{}, err
	}
	c := toConsultation(row.Stakeholder, row.StakeholderType, row.ConsultedAt, row.Summary, row.Response, row.ID, row.AssessmentID, row.RowVersion, row.CreatedAt)
	if err := s.audit(ctx, "dpia.consultation.add", AssessmentEntityType, assessmentID, nil,
		map[string]any{"consultation_id": c.ID, "stakeholder_type": c.StakeholderType}); err != nil {
		return Consultation{}, err
	}
	return c, nil
}

// ListConsultations is both the admin UI's own list and DPIA-15's report source (oldest first).
func (s *Service) ListConsultations(ctx context.Context, assessmentID uuid.UUID) ([]Consultation, error) {
	if _, err := s.GetAssessment(ctx, assessmentID); err != nil {
		return nil, err
	}
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListConsultations(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	out := make([]Consultation, 0, len(rows))
	for _, r := range rows {
		out = append(out, toConsultation(r.Stakeholder, r.StakeholderType, r.ConsultedAt, r.Summary, r.Response, r.ID, r.AssessmentID, r.RowVersion, r.CreatedAt))
	}
	return out, nil
}

// RemoveConsultation deletes one consultation — ROPA-03's own "no per-row update; editing a child is
// delete+recreate" convention, since a consultation has no other feature depending on an in-place edit.
func (s *Service) RemoveConsultation(ctx context.Context, assessmentID, id uuid.UUID) error {
	if _, err := s.GetAssessment(ctx, assessmentID); err != nil {
		return err
	}
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	n, err := q.DeleteConsultation(ctx, dpiastore.DeleteConsultationParams{ID: id, AssessmentID: assessmentID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "dpia.consultation.remove", AssessmentEntityType, assessmentID, nil, map[string]any{"consultation_id": id})
}

func pgDate(t *time.Time) pgtype.Date {
	if t == nil || t.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}
