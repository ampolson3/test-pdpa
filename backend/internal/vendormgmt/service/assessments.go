package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dpiaservice "pdpa-platform/internal/dpia/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/forms"
	riskservice "pdpa-platform/internal/risk/service"
	vendorstore "pdpa-platform/internal/vendormgmt/store"
)

// VendorAssessment is one cycle of VEN-04's assessment templates answered against a vendor (VEN-07): the
// weighted score the form's own options produce, and the residual risk level that score classifies to on
// this tenant's own RRA-02 matrix. Decision/DecidedBy/DecidedAt stay unset here — VEN-08 (not built yet)
// owns writing them.
type VendorAssessment struct {
	ID            uuid.UUID
	VendorID      uuid.UUID
	AssessmentID  uuid.UUID
	CycleNo       int
	SubmittedAt   *time.Time
	Score         float64
	ResidualLevel string
	Decision      string
	DecidedBy     *uuid.UUID
	DecidedAt     *time.Time
	RowVersion    int32
	CreatedAt     time.Time
}

// RecordAssessment is VEN-07's own acceptance criterion: answer one of VEN-04's published "vendor"
// templates (by its own seeded code, e.g. "vendor_pdpa") for a vendor, in one atomic call — the same
// one-shot pattern VEN-02's own RecordIntake already uses for the intake questionnaire. dpia owns the
// assess schema (rule 9), so the actual answer-scoring write goes through its own generic
// RecordSubjectAssessment; this service only resolves the template, turns the raw score into a risk level
// on the tenant's own matrix, and records the vendor-specific cycle row.
func (s *Service) RecordAssessment(ctx context.Context, vendorID uuid.UUID, templateCode string, answers forms.Answers) (VendorAssessment, error) {
	if _, err := s.GetVendor(ctx, vendorID); err != nil {
		return VendorAssessment{}, err
	}
	if s.Dpia == nil || s.Risk == nil {
		return VendorAssessment{}, errors.New("vendor: assessment scoring not configured")
	}
	templates, err := s.Dpia.ListTemplates(ctx, "vendor")
	if err != nil {
		return VendorAssessment{}, err
	}
	var tpl *dpiaservice.Template
	for i := range templates {
		if templates[i].Code == templateCode && templates[i].Status == "published" {
			tpl = &templates[i]
			break
		}
	}
	if tpl == nil {
		return VendorAssessment{}, fmt.Errorf("%w: template_code", ErrInvalid)
	}

	result, err := s.Dpia.RecordSubjectAssessment(ctx, tpl.ID, "vendor", vendorID, tpl.Name, answers)
	if err != nil {
		return VendorAssessment{}, err
	}

	matrix, err := s.Risk.GetMatrix(ctx, nil)
	if err != nil {
		return VendorAssessment{}, fmt.Errorf("%w: no default risk matrix configured for this tenant (RRA-02)", ErrInvalid)
	}
	ratio := 0.0
	if result.MaxScore > 0 {
		ratio = 1 - result.Score/result.MaxScore
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
	}
	level, err := riskservice.ClassifyScore(matrix, ratio)
	if err != nil {
		return VendorAssessment{}, err
	}
	// risk/service's own levels (RRA-02's validLevels: low/medium/high/very_high, shared by every risk
	// score across the platform) don't line up one-to-one with vendor_assessments.residual_level's own
	// CHECK constraint (low/medium/high/critical, the baseline migration's own vocabulary) — translate
	// the one level name that differs rather than widen either CHECK to match the other's wording.
	if level == "very_high" {
		level = "critical"
	}

	q := vendorstore.New(pdb.MustTxFromContext(ctx))
	cycle, err := q.NextVendorAssessmentCycle(ctx, vendorID)
	if err != nil {
		return VendorAssessment{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return VendorAssessment{}, err
	}
	var row vendorstore.InsertVendorAssessmentRow
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		var err error
		row, err = vendorstore.New(pdb.MustTxFromContext(ctx)).InsertVendorAssessment(ctx, vendorstore.InsertVendorAssessmentParams{
			ID: id, VendorID: vendorID, AssessmentID: result.ID, CycleNo: int16(cycle),
			Score: numeric(result.Score), ResidualLevel: &level,
		})
		return err
	})
	if err != nil {
		return VendorAssessment{}, fmt.Errorf("%w: assessment cycle already recorded", ErrInvalid)
	}
	if err := s.audit(ctx, "vendor.vendor_assessment.record", vendorID, nil,
		map[string]any{"vendor_assessment_id": row.ID, "score": result.Score, "residual_level": level}); err != nil {
		return VendorAssessment{}, err
	}
	// ST-06#1's own "prospect -> onboarding" edge: "ส่งแบบประเมิน". A no-row update (vendor already past
	// "prospect") is deliberately not an error — a reassessment of an already-onboarded/approved/
	// conditional vendor must still be allowed to record a new cycle.
	if onboarded, err := vendorstore.New(pdb.MustTxFromContext(ctx)).MarkVendorOnboarding(ctx, vendorID); err == nil {
		if err := s.audit(ctx, "vendor.vendor.transition", vendorID,
			map[string]any{"status": "prospect"}, map[string]any{"status": "onboarding"}); err != nil {
			return VendorAssessment{}, err
		}
		_ = onboarded
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return VendorAssessment{}, err
	}
	return toVendorAssessment(row), nil
}

// RemediationItemInput is one item of a conditional approval's remediation plan (VEN-08).
type RemediationItemInput struct {
	Title    string
	Severity string
	DueAt    *time.Time
}

// RemediationItem mirrors vendor.remediation_items.
type RemediationItem struct {
	ID                 uuid.UUID
	VendorID           uuid.UUID
	VendorAssessmentID *uuid.UUID
	Title              string
	Severity           string
	DueAt              *time.Time
	Status             string
	ClosedAt           *time.Time
	RowVersion         int32
	CreatedAt          time.Time
}

var remediationSeverities = map[string]bool{"low": true, "medium": true, "high": true}

// DecideAssessment is VEN-08's own acceptance criterion: a human decision (approved/conditional/rejected)
// on an answered cycle, advancing ST-06#1's "onboarding -> approved|conditional|rejected" edge in the same
// transaction; a conditional decision automatically opens the given remediation items.
func (s *Service) DecideAssessment(ctx context.Context, assessmentID uuid.UUID, decision string, items []RemediationItemInput) (VendorAssessment, []RemediationItem, error) {
	if decision != "approved" && decision != "conditional" && decision != "rejected" {
		return VendorAssessment{}, nil, fmt.Errorf("%w: decision", ErrInvalid)
	}
	if decision == "conditional" && len(items) == 0 {
		return VendorAssessment{}, nil, fmt.Errorf("%w: remediation_items required for a conditional decision", ErrInvalid)
	}
	for _, it := range items {
		if len(it.Title) == 0 || len([]rune(it.Title)) > 2000 || !remediationSeverities[it.Severity] {
			return VendorAssessment{}, nil, fmt.Errorf("%w: remediation_items", ErrInvalid)
		}
	}

	current, err := s.GetAssessment(ctx, assessmentID)
	if err != nil {
		return VendorAssessment{}, nil, err
	}
	if current.Decision != "" {
		return VendorAssessment{}, nil, ErrInvalidTransition
	}
	vendor, err := s.GetVendor(ctx, current.VendorID)
	if err != nil {
		return VendorAssessment{}, nil, err
	}
	if vendor.Status != "onboarding" {
		return VendorAssessment{}, nil, ErrInvalidTransition
	}

	q := vendorstore.New(pdb.MustTxFromContext(ctx))
	decided, err := q.DecideVendorAssessment(ctx, vendorstore.DecideVendorAssessmentParams{ID: assessmentID, Decision: &decision})
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorAssessment{}, nil, ErrInvalidTransition
	}
	if err != nil {
		return VendorAssessment{}, nil, err
	}
	if _, err := q.DecideVendorStatus(ctx, vendorstore.DecideVendorStatusParams{ID: vendor.ID, Status: decision}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return VendorAssessment{}, nil, ErrInvalidTransition
		}
		return VendorAssessment{}, nil, err
	}

	var created []RemediationItem
	for _, it := range items {
		id, err := uuid.NewV7()
		if err != nil {
			return VendorAssessment{}, nil, err
		}
		var due pgtype.Date
		if it.DueAt != nil {
			due = pgtype.Date{Time: *it.DueAt, Valid: true}
		}
		row, err := q.InsertRemediationItem(ctx, vendorstore.InsertRemediationItemParams{
			ID: id, VendorID: vendor.ID, VendorAssessmentID: pgUUID(&assessmentID), Title: it.Title,
			Severity: it.Severity, DueAt: due,
		})
		if err != nil {
			return VendorAssessment{}, nil, err
		}
		created = append(created, toRemediationItem(vendorstore.ListRemediationItemsForVendorRow(row)))
	}

	if err := s.audit(ctx, "vendor.vendor_assessment.decide", vendor.ID,
		map[string]any{"decision": nil}, map[string]any{"decision": decision, "remediation_items": len(created)}); err != nil {
		return VendorAssessment{}, nil, err
	}

	if decision == "approved" && s.Events != nil {
		expiresAt := ""
		if vendor.NextAssessmentAt != nil {
			expiresAt = vendor.NextAssessmentAt.Format("2006-01-02")
		}
		tier := ""
		if vendor.Tier != nil {
			tier = *vendor.Tier
		}
		if _, err := s.Events.Publish(ctx, events.Event{Type: "vendor.approved", AggregateType: VendorEntityType, AggregateID: vendor.ID,
			Data: map[string]any{"vendor_id": vendor.ID.String(), "tier": tier, "expires_at": expiresAt}}); err != nil {
			return VendorAssessment{}, nil, err
		}
	}

	return toVendorAssessment(vendorstore.InsertVendorAssessmentRow(decided)), created, nil
}

// ListRemediationItems lists a vendor's remediation plan, newest first.
func (s *Service) ListRemediationItems(ctx context.Context, vendorID uuid.UUID) ([]RemediationItem, error) {
	if _, err := s.GetVendor(ctx, vendorID); err != nil {
		return nil, err
	}
	rows, err := vendorstore.New(pdb.MustTxFromContext(ctx)).ListRemediationItemsForVendor(ctx, vendorID)
	if err != nil {
		return nil, err
	}
	out := make([]RemediationItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRemediationItem(r))
	}
	return out, nil
}

func toRemediationItem(r vendorstore.ListRemediationItemsForVendorRow) RemediationItem {
	it := RemediationItem{ID: r.ID, VendorID: r.VendorID, VendorAssessmentID: uuidPtr(r.VendorAssessmentID),
		Title: r.Title, Severity: r.Severity, Status: r.Status, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.DueAt.Valid {
		t := r.DueAt.Time
		it.DueAt = &t
	}
	if r.ClosedAt.Valid {
		t := r.ClosedAt.Time
		it.ClosedAt = &t
	}
	return it
}

// ListAssessments lists a vendor's own assessment cycles, newest first.
func (s *Service) ListAssessments(ctx context.Context, vendorID uuid.UUID) ([]VendorAssessment, error) {
	if _, err := s.GetVendor(ctx, vendorID); err != nil {
		return nil, err
	}
	rows, err := vendorstore.New(pdb.MustTxFromContext(ctx)).ListVendorAssessments(ctx, vendorID)
	if err != nil {
		return nil, err
	}
	out := make([]VendorAssessment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toVendorAssessment(vendorstore.InsertVendorAssessmentRow(r)))
	}
	return out, nil
}

// GetAssessment returns one vendor assessment cycle by id, RLS-scoped to the caller's tenant.
func (s *Service) GetAssessment(ctx context.Context, id uuid.UUID) (VendorAssessment, error) {
	row, err := vendorstore.New(pdb.MustTxFromContext(ctx)).GetVendorAssessment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorAssessment{}, ErrNotFound
	}
	if err != nil {
		return VendorAssessment{}, err
	}
	return toVendorAssessment(vendorstore.InsertVendorAssessmentRow(row)), nil
}

func toVendorAssessment(r vendorstore.InsertVendorAssessmentRow) VendorAssessment {
	score, _ := r.Score.Float64Value()
	va := VendorAssessment{ID: r.ID, VendorID: r.VendorID, AssessmentID: r.AssessmentID, CycleNo: int(r.CycleNo),
		Score: score.Float64, ResidualLevel: deref(r.ResidualLevel), Decision: deref(r.Decision),
		DecidedBy: uuidPtr(r.DecidedBy), RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.SubmittedAt.Valid {
		t := r.SubmittedAt.Time
		va.SubmittedAt = &t
	}
	if r.DecidedAt.Valid {
		t := r.DecidedAt.Time
		va.DecidedAt = &t
	}
	return va
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
