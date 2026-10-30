package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dpiastore "pdpa-platform/internal/dpia/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

// TemplateEntityType is the audit entity type for assess.templates rows.
const TemplateEntityType = "dpia_template"

// assessmentTypes mirrors assess.templates.assessment_type's own CHECK constraint (migration 00011) — kept
// here only so a bad value is refused with a 422 before it ever reaches the database, not as a second
// source of truth for what the column allows.
var assessmentTypes = map[string]bool{
	"dpia": true, "pia": true, "lia": true, "tia": true, "ai": true, "security": true,
	"maturity": true, "dpo_check": true, "sme_check": true, "vendor": true, "inbound_dpa": true, "independence": true,
}

// Template is one assess.templates row: a DPIA/PIA/LIA/... template, wrapping a PLT-06 "assessment" form
// (its questions, options and scoring) with the assess-specific bits (type, code, legal references, status).
type Template struct {
	ID             uuid.UUID
	AssessmentType string
	Code           string
	Name           string
	FormID         uuid.UUID
	VersionNo      int
	LegalRefs      []string
	Status         string // draft | published | retired
	RowVersion     int32
	CreatedAt      time.Time
}

// ListTemplates lists every template visible to the tenant (global defaults + the tenant's own), optionally
// filtered to one assessment_type — the template library's own listing.
func (s *Service) ListTemplates(ctx context.Context, assessmentType string) ([]Template, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	var at *string
	if assessmentType != "" {
		at = &assessmentType
	}
	rows, err := q.ListTemplates(ctx, at)
	if err != nil {
		return nil, err
	}
	out := make([]Template, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTemplate(r))
	}
	return out, nil
}

// GetTemplateByID returns one template.
func (s *Service) GetTemplateByID(ctx context.Context, id uuid.UUID) (Template, error) {
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetTemplateByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	return toTemplate(r), nil
}

// CreateTemplate authors a brand-new template: a fresh PLT-06 "assessment" form (its own draft version, the
// caller's schema/scoring) plus the assess.templates wrapper that makes it show up in the library and be
// resolvable by assessment_type + code (the way DPIA-01's screening template already is).
func (s *Service) CreateTemplate(ctx context.Context, assessmentType, code, name string, legalRefs []string, d forms.Draft) (Template, error) {
	if !assessmentTypes[assessmentType] {
		return Template{}, fmt.Errorf("%w: assessment_type", ErrInvalid)
	}
	form, err := s.Forms.CreateForm(ctx, code, name, ScreeningFormType, d)
	if err != nil {
		return Template{}, err
	}
	return s.insertTemplate(ctx, assessmentType, code, name, form.ID, 1, legalRefs, "draft")
}

// CloneTemplate is DPIA-03's acceptance criterion: a new, fully independent form + template carrying the
// source's current content (published version if it has one, else its open draft) — editing the clone
// afterwards (through PLT-06's own form builder) never touches the source, because they are different
// platform.form_definitions rows from the start, not a shared one.
func (s *Service) CloneTemplate(ctx context.Context, sourceID uuid.UUID, code, name string) (Template, error) {
	src, err := s.GetTemplateByID(ctx, sourceID)
	if err != nil {
		return Template{}, err
	}
	v, err := s.latestFormVersion(ctx, src.FormID)
	if err != nil {
		return Template{}, err
	}
	form, err := s.Forms.CreateForm(ctx, code, name, ScreeningFormType, forms.Draft{Schema: v.Schema, Scoring: v.Scoring, Languages: v.Languages})
	if err != nil {
		return Template{}, err
	}
	return s.insertTemplate(ctx, src.AssessmentType, code, name, form.ID, 1, src.LegalRefs, "draft")
}

func (s *Service) insertTemplate(ctx context.Context, assessmentType, code, name string, formID uuid.UUID, versionNo int, legalRefs []string, status string) (Template, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Template{}, err
	}
	if legalRefs == nil {
		legalRefs = []string{}
	}
	var row dpiastore.AssessTemplate
	if err := pdb.Savepoint(ctx, func(ctx context.Context) error {
		var err error
		row, err = dpiastore.New(pdb.MustTxFromContext(ctx)).InsertTemplateRow(ctx, dpiastore.InsertTemplateRowParams{
			ID: id, AssessmentType: assessmentType, Code: code, Name: name, FormID: formID,
			VersionNo: int32(versionNo), LegalRefs: legalRefs, Status: status,
		})
		return err
	}); err != nil {
		if isUnique(err) {
			return Template{}, fmt.Errorf("%w: code exists for this assessment_type", ErrInvalid)
		}
		return Template{}, err
	}
	if err := s.audit(ctx, "dpia.template.create", TemplateEntityType, id, nil, map[string]any{"assessment_type": assessmentType, "code": code}); err != nil {
		return Template{}, err
	}
	return toTemplate(row), nil
}

// PublishTemplate publishes the template's current draft (through PLT-06's own Publish, which freezes the
// version and makes it the form's current one) and flips the template's own status live in the same step.
func (s *Service) PublishTemplate(ctx context.Context, id uuid.UUID, draftVersion int32) (Template, error) {
	tmpl, err := s.GetTemplateByID(ctx, id)
	if err != nil {
		return Template{}, err
	}
	form, err := s.Forms.Publish(ctx, tmpl.FormID, draftVersion)
	if err != nil {
		return Template{}, err
	}
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.UpdateTemplateStatus(ctx, dpiastore.UpdateTemplateStatusParams{ID: id, Status: "published", VersionNo: form.LatestVersion})
	if err != nil {
		return Template{}, err
	}
	if err := s.audit(ctx, "dpia.template.publish", TemplateEntityType, id, map[string]any{"status": tmpl.Status}, map[string]any{"status": "published"}); err != nil {
		return Template{}, err
	}
	return toTemplate(row), nil
}

// RetireTemplate is a permanent status change (like ORG-06's party merge), not a plain field edit — gated by
// assessment.template.delete at the handler, not .update. rowVersion is the template's own ETag (If-Match) —
// unlike PublishTemplate, retiring has no underlying form ETag to lean on for optimistic concurrency.
func (s *Service) RetireTemplate(ctx context.Context, id uuid.UUID, rowVersion int32) (Template, error) {
	tmpl, err := s.GetTemplateByID(ctx, id)
	if err != nil {
		return Template{}, err
	}
	q := dpiastore.New(pdb.MustTxFromContext(ctx))
	row, err := q.RetireTemplateRow(ctx, dpiastore.RetireTemplateRowParams{ID: id, RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrVersionMismatch
	}
	if err != nil {
		return Template{}, err
	}
	if err := s.audit(ctx, "dpia.template.retire", TemplateEntityType, id, map[string]any{"status": tmpl.Status}, map[string]any{"status": "retired"}); err != nil {
		return Template{}, err
	}
	return toTemplate(row), nil
}

func toTemplate(r dpiastore.AssessTemplate) Template {
	return Template{
		ID: r.ID, AssessmentType: r.AssessmentType, Code: r.Code, Name: r.Name, FormID: r.FormID,
		VersionNo: int(r.VersionNo), LegalRefs: r.LegalRefs, Status: r.Status, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time,
	}
}
