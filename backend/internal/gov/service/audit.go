// DPX-03 (organization-wide audit & maturity, ม.37(1)): gov.audits/gov.audit_findings were already
// fully specified in the baseline migrations — an org-wide (or per-subsidiary) PDPA-readiness audit, with
// a score per dimension and a remediation plan. assess.templates.assessment_type's own CHECK constraint
// has listed 'maturity' since the very first baseline migration (00011_assess.sql), unused until now.
// Deliberately one-shot, the same "answer in one step" pattern DPO-09/DPX-01's own checklists already
// use: RunAudit answers a published maturity questionnaire against a legal entity directly (no draft/
// in_progress multi-step flow — the literal acceptance criterion needs a result, not a workflow), through
// dpia's own generic, non-activity screening (ScreenSubject, built for DPO-02, reused here for its second
// caller).
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dpiaservice "pdpa-platform/internal/dpia/service"
	govstore "pdpa-platform/internal/gov/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

var findingSeverities = []string{"low", "medium", "high", "critical"}
var findingStatuses = []string{"open", "in_progress", "done", "accepted"}

// ErrBadMaturityTemplate is returned when the given template code names no published "maturity"
// assess.templates row.
var ErrBadMaturityTemplate = errors.New("gov: not a published maturity assessment template")

// Audit is one run of the organization-wide (or per-subsidiary) PDPA-readiness questionnaire.
type Audit struct {
	ID            uuid.UUID
	AuditType     string
	LegalEntityID *uuid.UUID
	AssessmentID  uuid.UUID
	Period        string
	Status        string
	OverallScore  *float64
	CompletedAt   *time.Time
	RowVersion    int32
	CreatedAt     time.Time
	UpdatedAt     time.Time

	// SectionScores/Band are read live from the dpia assessment this audit wraps, not persisted a second
	// time — Get/List both resolve them on every read so they can never go stale against the underlying
	// assess.assessments row.
	SectionScores []dpiaservice.SectionScore
	Band          string
	Findings      []AuditFinding
}

// RunAuditInput is what a caller provides to run a maturity audit.
type RunAuditInput struct {
	LegalEntityID uuid.UUID
	Period        string
	TemplateCode  string
	Answers       forms.Answers
}

// AuditFinding is one gap the audit found — the acceptance criterion's own "แผนแก้ไข" (remediation plan).
type AuditFinding struct {
	ID             uuid.UUID
	AuditID        uuid.UUID
	Title          string
	Severity       string
	RequirementRef *string
	Recommendation *string
	OwnerUserID    *uuid.UUID
	DueAt          *time.Time
	Status         string
	TaskID         *uuid.UUID
	EvidenceFileID *uuid.UUID
	RowVersion     int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// AuditFindingInput is what a caller provides when recording or editing a finding by hand (RunAudit
// already opens one automatically per failed yes_no question — this is for anything the questionnaire
// itself didn't capture, or for editing an auto-opened one's recommendation/owner/due date).
type AuditFindingInput struct {
	Title          string
	Severity       string
	RequirementRef *string
	Recommendation *string
	OwnerUserID    *uuid.UUID
	DueAt          *time.Time
}

func validSeverity(s string) bool      { return contains(findingSeverities, s) }
func validFindingStatus(s string) bool { return contains(findingStatuses, s) }

// decideMaturityBand classifies the audit's own overall score against a fixed, tunable scale (CLAUDE.md's
// "implement the listed default as config", not a legally-mandated banding) — the band itself is also
// stored on the underlying assess.assessments row as its own screening_result/reason.
func decideMaturityBand(factorCount int, factors []forms.Contribution) (result, reason string) {
	total := 0.0
	for _, f := range factors {
		total += f.Points
	}
	band := "developing"
	switch {
	case total >= 8:
		band = "mature"
	case total >= 5:
		band = "established"
	}
	return band, fmt.Sprintf("คะแนนรวม %.2f จากคำตอบ %d รายการ", total, factorCount)
}

// RunAudit is the acceptance criterion itself: answers a published maturity questionnaire against a legal
// entity (checked visible under RLS, rule 1) in one step, records the per-dimension scores dpia's own
// ScreenSubject already computes, and opens one open finding per yes_no question answered "no" — the
// exact "failing items" pattern DPO-09's own security checklist already established, just for a module
// that doesn't own the assess schema itself (rule 9 — gov never writes to assess.assessments/assess.answers
// directly, only through dpia's exported service).
func (s *Service) RunAudit(ctx context.Context, in RunAuditInput) (Audit, error) {
	if !has(ctx, AuditPermCreate) {
		return Audit{}, ErrForbidden
	}
	if in.Period == "" {
		return Audit{}, fmt.Errorf("%w: period", ErrInvalid)
	}
	if s.Dpia == nil {
		return Audit{}, ErrBadMaturityTemplate
	}
	le, err := s.Org.GetLegalEntity(ctx, in.LegalEntityID)
	if err != nil {
		return Audit{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	title := fmt.Sprintf("%s — %s", le.NameTh, in.Period)
	screening, err := s.Dpia.ScreenSubject(ctx, MaturityAssessmentType, in.TemplateCode, "legal_entity", in.LegalEntityID, title, in.Answers, decideMaturityBand)
	if errors.Is(err, dpiaservice.ErrBadTemplate) {
		return Audit{}, ErrBadMaturityTemplate
	}
	var ve *dpiaservice.ValidationError
	if errors.As(err, &ve) {
		gve := &ValidationError{}
		for _, f := range ve.Fields {
			gve.Fields = append(gve.Fields, FieldError{Field: f.Field, Code: f.Code})
		}
		return Audit{}, gve
	}
	if err != nil {
		return Audit{}, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Audit{}, err
	}
	now := s.now()
	var score pgtype.Numeric
	if err := score.Scan(strconv.FormatFloat(screening.Score, 'f', 2, 64)); err != nil {
		return Audit{}, err
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).InsertAudit(ctx, govstore.InsertAuditParams{
		ID: id, AuditType: MaturityAssessmentType, LegalEntityID: pgUUIDPtr(&in.LegalEntityID), AssessmentID: screening.AssessmentID,
		Period: in.Period, Status: "completed", OverallScore: score, CompletedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return Audit{}, err
	}
	out := toAudit(auditRow(row))
	out.SectionScores = screening.SectionScores
	out.Band = screening.ScreeningResult

	var findings []AuditFinding
	for _, item := range failingYesNoAnswers(in.Answers, screening.Factors) {
		f, err := s.openAuditFinding(ctx, out.ID, item)
		if err != nil {
			return Audit{}, err
		}
		findings = append(findings, f)
	}
	out.Findings = findings

	if err := s.audit(ctx, "gov.audit.run", AuditType, out.ID, nil,
		map[string]any{"legal_entity_id": in.LegalEntityID, "period": in.Period, "score": screening.Score, "band": screening.ScreeningResult, "findings": len(findings)}); err != nil {
		return Audit{}, err
	}
	return out, nil
}

// failingYesNoAnswers walks the factors a screening round actually answered and returns every yes_no
// question answered "no" — the same shape DPO-09's own failingItems already established, generalized to
// not need the schema itself (gov never reads platform.form_definitions for this — screening.Factors
// already carries each question's own answer and label).
func failingYesNoAnswers(answers forms.Answers, factors []forms.Contribution) []forms.Contribution {
	var out []forms.Contribution
	for _, f := range factors {
		if a, ok := f.Answer.(string); ok && a == "no" {
			out = append(out, f)
		}
	}
	return out
}

func (s *Service) openAuditFinding(ctx context.Context, auditID uuid.UUID, item forms.Contribution) (AuditFinding, error) {
	title := item.Label["th"]
	if title == "" {
		title = item.Question
	}
	id, err := uuid.NewV7()
	if err != nil {
		return AuditFinding{}, err
	}
	ref := item.Question
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).InsertAuditFinding(ctx, govstore.InsertAuditFindingParams{
		ID: id, AuditID: auditID, Title: title, Severity: "medium", RequirementRef: &ref,
	})
	if err != nil {
		return AuditFinding{}, err
	}
	return toFinding(auditFindingRow(row)), nil
}

// GetAudit reads one audit with its own current per-dimension scores/band and findings.
func (s *Service) GetAudit(ctx context.Context, id uuid.UUID) (Audit, error) {
	if !has(ctx, AuditPermRead) {
		return Audit{}, ErrForbidden
	}
	r, err := govstore.New(pdb.MustTxFromContext(ctx)).GetAudit(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Audit{}, ErrNotFound
	}
	if err != nil {
		return Audit{}, err
	}
	out := toAudit(auditRow(r))
	findings, err := s.ListAuditFindings(ctx, id)
	if err != nil {
		return Audit{}, err
	}
	out.Findings = findings
	return out, nil
}

// ListAudits lists every audit, optionally narrowed by legal entity, newest first — without section scores
// (a per-row detail, drilled into via GetAudit, the same "list is for scanning" convention DPO-09's own
// ListAssessments already established).
func (s *Service) ListAudits(ctx context.Context, legalEntityID *uuid.UUID) ([]Audit, error) {
	if !has(ctx, AuditPermRead) {
		return nil, ErrForbidden
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListAudits(ctx, pgUUIDPtr(legalEntityID))
	if err != nil {
		return nil, err
	}
	out := make([]Audit, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAudit(auditRow(r)))
	}
	return out, nil
}

// DeleteAudit removes an audit record (not the underlying assess.assessments round, which is dpia's own
// append-style evidence — deleting only this module's own wrapper row).
func (s *Service) DeleteAudit(ctx context.Context, id uuid.UUID) error {
	if !has(ctx, AuditPermDelete) {
		return ErrForbidden
	}
	n, err := govstore.New(pdb.MustTxFromContext(ctx)).DeleteAudit(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "gov.audit.delete", AuditType, id, nil, nil)
}

// AddAuditFinding records a finding by hand — alongside whatever RunAudit already opened automatically.
func (s *Service) AddAuditFinding(ctx context.Context, auditID uuid.UUID, in AuditFindingInput) (AuditFinding, error) {
	if !has(ctx, AuditPermUpdate) {
		return AuditFinding{}, ErrForbidden
	}
	if err := s.validateFinding(in); err != nil {
		return AuditFinding{}, err
	}
	if _, err := s.GetAudit(ctx, auditID); err != nil {
		return AuditFinding{}, fmt.Errorf("%w: audit_id", ErrInvalid)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return AuditFinding{}, err
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).InsertAuditFinding(ctx, govstore.InsertAuditFindingParams{
		ID: id, AuditID: auditID, Title: in.Title, Severity: in.Severity, RequirementRef: in.RequirementRef,
		Recommendation: in.Recommendation, OwnerUserID: pgUUIDPtr(in.OwnerUserID), DueAt: pgDateOpt(in.DueAt),
	})
	if err != nil {
		return AuditFinding{}, err
	}
	out := toFinding(auditFindingRow(row))
	return out, s.audit(ctx, "gov.audit_finding.create", AuditFindingType, out.ID, nil, map[string]any{"audit_id": auditID, "title": out.Title})
}

func (s *Service) validateFinding(in AuditFindingInput) error {
	if in.Title == "" {
		return fmt.Errorf("%w: title", ErrInvalid)
	}
	if !validSeverity(in.Severity) {
		return fmt.Errorf("%w: severity", ErrInvalid)
	}
	return nil
}

// ListAuditFindings lists one audit's own findings, most severe first.
func (s *Service) ListAuditFindings(ctx context.Context, auditID uuid.UUID) ([]AuditFinding, error) {
	if !has(ctx, AuditPermRead) {
		return nil, ErrForbidden
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListAuditFindings(ctx, auditID)
	if err != nil {
		return nil, err
	}
	out := make([]AuditFinding, 0, len(rows))
	for _, r := range rows {
		out = append(out, toFinding(auditFindingRow(r)))
	}
	return out, nil
}

// UpdateAuditFinding edits a finding's own details.
func (s *Service) UpdateAuditFinding(ctx context.Context, id uuid.UUID, rowVersion int32, in AuditFindingInput) (AuditFinding, error) {
	if !has(ctx, AuditPermUpdate) {
		return AuditFinding{}, ErrForbidden
	}
	if err := s.validateFinding(in); err != nil {
		return AuditFinding{}, err
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).UpdateAuditFinding(ctx, govstore.UpdateAuditFindingParams{
		ID: id, RowVersion: rowVersion, Title: in.Title, Severity: in.Severity, RequirementRef: in.RequirementRef,
		Recommendation: in.Recommendation, OwnerUserID: pgUUIDPtr(in.OwnerUserID), DueAt: pgDateOpt(in.DueAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuditFinding{}, ErrVersionMismatch
		}
		return AuditFinding{}, err
	}
	out := toFinding(auditFindingRow(row))
	return out, s.audit(ctx, "gov.audit_finding.update", AuditFindingType, out.ID, nil, map[string]any{"title": out.Title})
}

// SetAuditFindingStatus moves a finding open/in_progress/done/accepted. DPX-04's own acceptance criterion
// ("ข้อตรวจพบปิดได้เมื่อแนบหลักฐาน" — a finding closes only once evidence is attached) gates both terminal
// states on evidence_file_id already being set — AttachFindingEvidence's own job, not this call's; "accepted"
// is additionally a permanent decision (accepting a gap as-is rather than fixing it), so it needs
// dpx.audit.approve beyond this endpoint's own dpx.audit.update, the same reasoning VEN-14/ORG-06/DPIA-03
// already use for reusing a stronger permission rather than minting a new code.
func (s *Service) SetAuditFindingStatus(ctx context.Context, id uuid.UUID, rowVersion int32, status string) (AuditFinding, error) {
	if !has(ctx, AuditPermUpdate) {
		return AuditFinding{}, ErrForbidden
	}
	if !validFindingStatus(status) {
		return AuditFinding{}, fmt.Errorf("%w: status", ErrInvalid)
	}
	if status == "accepted" && !has(ctx, AuditPermApprove) {
		return AuditFinding{}, ErrForbidden
	}
	if status == "done" || status == "accepted" {
		current, err := s.GetAuditFinding(ctx, id)
		if err != nil {
			return AuditFinding{}, err
		}
		if current.EvidenceFileID == nil {
			return AuditFinding{}, ErrEvidenceRequired
		}
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).SetAuditFindingStatus(ctx, govstore.SetAuditFindingStatusParams{
		ID: id, RowVersion: rowVersion, Status: status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuditFinding{}, ErrVersionMismatch
		}
		return AuditFinding{}, err
	}
	out := toFinding(auditFindingRow(row))
	return out, s.audit(ctx, "gov.audit_finding.status", AuditFindingType, out.ID, nil, map[string]any{"status": status})
}

// auditRow/auditFindingRow mirror each query's own shared column layout (sqlc generates a distinct named
// Row type per query despite the identical layout — the same workaround BRE-15/VEN-03/VEN-14/DPX-08/DPX-01
// already established), so every query's own Row type converts to them directly.
type auditRow struct {
	ID            uuid.UUID
	AuditType     string
	LegalEntityID pgtype.UUID
	AssessmentID  uuid.UUID
	Period        string
	Status        string
	OverallScore  pgtype.Numeric
	CompletedAt   pgtype.Timestamptz
	RowVersion    int32
	CreatedAt     pgtype.Timestamptz
	UpdatedAt     pgtype.Timestamptz
}

type auditFindingRow struct {
	ID             uuid.UUID
	AuditID        uuid.UUID
	Title          string
	Severity       string
	RequirementRef *string
	Recommendation *string
	OwnerUserID    pgtype.UUID
	DueAt          pgtype.Date
	Status         string
	TaskID         pgtype.UUID
	EvidenceFileID pgtype.UUID
	RowVersion     int32
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

func toAudit(r auditRow) Audit {
	out := Audit{ID: r.ID, AuditType: r.AuditType, AssessmentID: r.AssessmentID, Period: r.Period, Status: r.Status,
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.LegalEntityID.Valid {
		u := uuid.UUID(r.LegalEntityID.Bytes)
		out.LegalEntityID = &u
	}
	if r.OverallScore.Valid {
		f, _ := r.OverallScore.Float64Value()
		out.OverallScore = &f.Float64
	}
	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time
		out.CompletedAt = &t
	}
	return out
}

func toFinding(r auditFindingRow) AuditFinding {
	out := AuditFinding{ID: r.ID, AuditID: r.AuditID, Title: r.Title, Severity: r.Severity, RequirementRef: r.RequirementRef,
		Recommendation: r.Recommendation, Status: r.Status, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.OwnerUserID.Valid {
		u := uuid.UUID(r.OwnerUserID.Bytes)
		out.OwnerUserID = &u
	}
	if r.DueAt.Valid {
		t := r.DueAt.Time
		out.DueAt = &t
	}
	if r.TaskID.Valid {
		u := uuid.UUID(r.TaskID.Bytes)
		out.TaskID = &u
	}
	if r.EvidenceFileID.Valid {
		u := uuid.UUID(r.EvidenceFileID.Bytes)
		out.EvidenceFileID = &u
	}
	return out
}

func pgDateOpt(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}
