package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	govstore "pdpa-platform/internal/gov/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

// FrameworkFormType is the PLT-06 form type DPX-10's framework checklists use (migration 00096 widened
// platform.form_definitions.form_type to accept it). Each related law/notice is turned into an ordinary
// form the DPO authors — rule 8: the wording is never hard-coded here, only the seeded, DRAFT-pending-
// legal-review content migration 00096 writes.
const FrameworkFormType = "framework"

// DPX-10's own permission codes (dpx.framework.*, migration 00096). The catalog itself is global/seeded —
// create/update/publish on the underlying form are never exercised by a real tenant screen (the same
// reasoning ROPA-20's own sme_exemption note already gives) — execute is the action a real screen uses, to
// answer one framework's checklist.
const (
	FrameworkPermRead    = "dpx.framework.read"
	FrameworkPermCreate  = "dpx.framework.create"
	FrameworkPermUpdate  = "dpx.framework.update"
	FrameworkPermPublish = "dpx.framework.publish"
	FrameworkPermExecute = "dpx.framework.execute"
)

// FrameworkAssessmentType is the entity type for audit (DPX-10).
const FrameworkAssessmentType = "gov_framework_assessment"

// ErrBadFrameworkForm is returned when a framework has no published checklist to answer.
var ErrBadFrameworkForm = errors.New("gov: framework has no published checklist")

// Framework is one related law/notice in the global catalog (DPX-10) — gov.regulatory_frameworks, seeded by
// migration, never created through this module's own API.
type Framework struct {
	ID              uuid.UUID
	Code            string
	NameTh, NameEn  string
	LegalRefs       []string
	DescriptionTh   *string
	DescriptionEn   *string
	ChecklistFormID *uuid.UUID
	RowVersion      int32
}

func toFramework(r govstore.ListRegulatoryFrameworksRow) Framework {
	return Framework{ID: r.ID, Code: r.Code, NameTh: r.NameTh, NameEn: r.NameEn, LegalRefs: r.LegalRefs,
		DescriptionTh: r.DescriptionTh, DescriptionEn: r.DescriptionEn, ChecklistFormID: fromPgUUID(r.ChecklistFormID), RowVersion: r.RowVersion}
}

// ListFrameworks returns the whole catalog (no pagination — a handful of laws, the same "platform-wide
// list" reasoning DPO-05/RRA-09/RTG-01 already use for their own small reference lists).
func (s *Service) ListFrameworks(ctx context.Context) ([]Framework, error) {
	if !has(ctx, FrameworkPermRead) {
		return nil, ErrForbidden
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListRegulatoryFrameworks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Framework, 0, len(rows))
	for _, r := range rows {
		out = append(out, toFramework(govstore.ListRegulatoryFrameworksRow(r)))
	}
	return out, nil
}

// GetFrameworkByCode is the acceptance criterion's own first half ("เลือก framework"): resolves one
// framework by its stable code.
func (s *Service) GetFrameworkByCode(ctx context.Context, code string) (Framework, error) {
	if !has(ctx, FrameworkPermRead) {
		return Framework{}, ErrForbidden
	}
	r, err := govstore.New(pdb.MustTxFromContext(ctx)).GetRegulatoryFrameworkByCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Framework{}, ErrNotFound
		}
		return Framework{}, err
	}
	return toFramework(govstore.ListRegulatoryFrameworksRow(r)), nil
}

// frameworkChecklistVersion resolves a framework's own checklist to its current published version — the
// acceptance criterion's other half ("แล้วได้ checklist พร้อมใช้"): a framework with no form linked, or
// whose form isn't a published 'framework'-type checklist, has nothing ready to use.
func (s *Service) frameworkChecklistVersion(ctx context.Context, f Framework) (forms.Version, error) {
	if f.ChecklistFormID == nil {
		return forms.Version{}, ErrBadFrameworkForm
	}
	form, err := s.Forms.GetForm(ctx, *f.ChecklistFormID)
	if errors.Is(err, forms.ErrNotFound) || errors.Is(err, forms.ErrForbidden) {
		return forms.Version{}, ErrBadFrameworkForm
	}
	if err != nil {
		return forms.Version{}, err
	}
	if form.Type != FrameworkFormType || form.CurrentVersionID == nil {
		return forms.Version{}, ErrBadFrameworkForm
	}
	for _, v := range form.Versions {
		if v.ID == *form.CurrentVersionID {
			return v, nil
		}
	}
	return forms.Version{}, ErrBadFrameworkForm
}

// FrameworkAssessment is one run of a framework's own checklist (DPX-10), the same thin shape DPO-09's own
// Assessment already uses (score/band + a form_submission_id), scoped to a framework rather than a legal
// entity — this feature has no per-legal-entity dimension to the question (a related law either applies to
// the tenant's whole operation or it doesn't).
type FrameworkAssessment struct {
	ID               uuid.UUID
	FrameworkID      uuid.UUID
	FormSubmissionID uuid.UUID
	Score            float64
	Result           string
	Factors          []forms.Contribution
	AssessedBy       *uuid.UUID
	AssessedAt       time.Time
	RowVersion       int32
	CreatedAt        time.Time
}

func toFrameworkAssessment(id, frameworkID, formSubmissionID uuid.UUID, score pgtype.Numeric, result string, factors []byte,
	assessedBy pgtype.UUID, assessedAt pgtype.Timestamptz, rowVersion int32, createdAt pgtype.Timestamptz) (FrameworkAssessment, error) {
	var sc float64
	if score.Valid {
		f, err := score.Float64Value()
		if err != nil {
			return FrameworkAssessment{}, err
		}
		sc = f.Float64
	}
	var fc []forms.Contribution
	if len(factors) > 0 {
		if err := json.Unmarshal(factors, &fc); err != nil {
			return FrameworkAssessment{}, err
		}
	}
	return FrameworkAssessment{ID: id, FrameworkID: frameworkID, FormSubmissionID: formSubmissionID, Score: sc, Result: result,
		Factors: fc, AssessedBy: fromPgUUID(assessedBy), AssessedAt: assessedAt.Time, RowVersion: rowVersion, CreatedAt: createdAt.Time}, nil
}

// AssessFramework is the literal acceptance criterion's natural next step — answering the checklist that
// was just made available — following DPO-09's own Assess pattern exactly (forms.Service.Record, scored by
// the form engine, one assessment row kept). Deliberately doesn't auto-open a remediation task the way
// DPO-09's own failing items do: a related-law checklist is informational (BE S's own literal scope is
// "pick a framework, get a checklist"), not a security-control register this codebase already tracks
// elsewhere.
func (s *Service) AssessFramework(ctx context.Context, code string, answers forms.Answers) (FrameworkAssessment, error) {
	if !has(ctx, FrameworkPermExecute) {
		return FrameworkAssessment{}, ErrForbidden
	}
	f, err := s.GetFrameworkByCode(ctx, code)
	if err != nil {
		return FrameworkAssessment{}, err
	}
	v, err := s.frameworkChecklistVersion(ctx, f)
	if err != nil {
		return FrameworkAssessment{}, err
	}
	me := currentUser(ctx)
	subID, res, err := s.Forms.Record(ctx, v.ID, answers, "user", me, "", nil)
	var ae *forms.AnswerErrors
	if errors.As(err, &ae) {
		ve := &ValidationError{}
		for _, fe := range ae.Fields {
			ve.Fields = append(ve.Fields, FieldError{"answers." + fe.Question, fe.Code})
		}
		return FrameworkAssessment{}, ve
	}
	if err != nil {
		return FrameworkAssessment{}, err
	}
	factors := forms.Contributions(v.Schema, res.Answers)
	raw, _ := json.Marshal(factors)
	id, err := uuid.NewV7()
	if err != nil {
		return FrameworkAssessment{}, err
	}
	now := s.now()
	var score pgtype.Numeric
	if err := score.Scan(strconv.FormatFloat(res.Score, 'f', 2, 64)); err != nil {
		return FrameworkAssessment{}, err
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).InsertFrameworkAssessment(ctx, govstore.InsertFrameworkAssessmentParams{
		ID: id, FrameworkID: f.ID, FormSubmissionID: subID, Score: score, Result: res.Band, Factors: raw,
		AssessedBy: pgUUIDPtr(me), AssessedAt: pgtype.Timestamptz{Time: now, Valid: true}})
	if err != nil {
		return FrameworkAssessment{}, err
	}
	if err := s.audit(ctx, "dpx.framework.assess", FrameworkAssessmentType, row.ID, nil,
		map[string]any{"framework_code": f.Code, "score": res.Score, "result": res.Band}); err != nil {
		return FrameworkAssessment{}, err
	}
	return toFrameworkAssessment(row.ID, f.ID, subID, score, res.Band, raw, pgUUIDPtr(me), pgtype.Timestamptz{Time: now, Valid: true}, row.RowVersion,
		pgtype.Timestamptz{Time: now, Valid: true})
}

// ListFrameworkAssessments lists this tenant's own past runs, optionally narrowed to one framework by its
// code, newest first.
func (s *Service) ListFrameworkAssessments(ctx context.Context, code *string) ([]FrameworkAssessment, error) {
	if !has(ctx, FrameworkPermRead) {
		return nil, ErrForbidden
	}
	var frameworkID pgtype.UUID
	if code != nil {
		f, err := s.GetFrameworkByCode(ctx, *code)
		if err != nil {
			return nil, err
		}
		frameworkID = pgtype.UUID{Bytes: f.ID, Valid: true}
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListFrameworkAssessments(ctx, frameworkID)
	if err != nil {
		return nil, err
	}
	out := make([]FrameworkAssessment, 0, len(rows))
	for _, r := range rows {
		a, err := toFrameworkAssessment(r.ID, r.FrameworkID, r.FormSubmissionID, r.Score, r.Result, r.Factors, r.AssessedBy, r.AssessedAt, r.RowVersion, r.CreatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// GetFrameworkAssessment reads one past run by id (tenant-scoped via RLS).
func (s *Service) GetFrameworkAssessment(ctx context.Context, id uuid.UUID) (FrameworkAssessment, error) {
	if !has(ctx, FrameworkPermRead) {
		return FrameworkAssessment{}, ErrForbidden
	}
	r, err := govstore.New(pdb.MustTxFromContext(ctx)).GetFrameworkAssessment(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FrameworkAssessment{}, ErrNotFound
		}
		return FrameworkAssessment{}, err
	}
	return toFrameworkAssessment(r.ID, r.FrameworkID, r.FormSubmissionID, r.Score, r.Result, r.Factors, r.AssessedBy, r.AssessedAt, r.RowVersion, r.CreatedAt)
}
