// DPO-02 (docs/decisions.md Q-37): the ม.41(2) / ประกาศ สคส. พ.ศ. 2566 appointment-requirement screening.
// Reuses dpia's own generic, non-activity screening primitive (ScreenSubject, added for this feature) rather
// than building a second scoring engine here — dpo owns none of the assess schema (rule 9), only the OR-based
// decision rule this feature's own module doc calls for.
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
	dpostore "pdpa-platform/internal/dpo/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
)

// RequirementCheckEntityType is the audit/entity tag for a DPO-02 requirement check.
const RequirementCheckEntityType = "dpo_requirement_check"

// DpoRequirementAssessmentType/DpoRequirementTemplateCode name migration 00077's seeded global checklist
// (docs/decisions.md Q-37) — assess.templates.assessment_type's own CHECK already listed 'dpo_check' before
// this feature, unused until now (the same "schema already anticipated this" pattern DPX-06/Q-36 found).
const (
	DpoRequirementAssessmentType = "dpo_check"
	DpoRequirementTemplateCode   = "dpo_requirement_check"
)

// RequirementCheck is one ม.41(2) screening round for a legal entity.
type RequirementCheck struct {
	ID            uuid.UUID
	LegalEntityID uuid.UUID
	AssessmentID  uuid.UUID
	Result        string // required | not_required
	Basis         string
	AssessedAt    time.Time
	RowVersion    int32
	CreatedAt     time.Time
}

// AssessDpoRequirement is the acceptance criterion itself: answer the ม.41(2)/2023-notice checklist for a
// legal entity and get a required/not_required result with a reason citing the notice — never left to a
// human's own reading of the criteria, the same "the system decides" rule DPIA-01 already established for
// its own screening.
func (s *Service) AssessDpoRequirement(ctx context.Context, legalEntityID uuid.UUID, answers forms.Answers) (RequirementCheck, error) {
	if _, err := s.Org.GetLegalEntity(ctx, legalEntityID); err != nil {
		return RequirementCheck{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	if s.Dpia == nil {
		return RequirementCheck{}, fmt.Errorf("dpo: dpia screening not wired")
	}
	title := "ประเมินหน้าที่ต้องแต่งตั้ง DPO (ม.41(2))"
	res, err := s.Dpia.ScreenSubject(ctx, DpoRequirementAssessmentType, DpoRequirementTemplateCode, "legal_entity",
		legalEntityID, title, answers, decideDpoRequirement)
	if err != nil {
		var dve *dpiaservice.ValidationError
		switch {
		case errors.Is(err, dpiaservice.ErrBadTemplate):
			return RequirementCheck{}, fmt.Errorf("dpo: no published DPO requirement-check template")
		case errors.As(err, &dve):
			ve := &ValidationError{}
			for _, f := range dve.Fields {
				ve.Fields = append(ve.Fields, FieldError{Field: f.Field, Code: f.Code})
			}
			return RequirementCheck{}, ve
		default:
			return RequirementCheck{}, err
		}
	}

	q := dpostore.New(pdb.MustTxFromContext(ctx))
	id, err := uuid.NewV7()
	if err != nil {
		return RequirementCheck{}, err
	}
	row, err := q.InsertRequirementCheck(ctx, dpostore.InsertRequirementCheckParams{
		ID: id, LegalEntityID: legalEntityID, AssessmentID: res.AssessmentID, Result: res.ScreeningResult,
		Basis: res.ScreeningReason, AssessedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		return RequirementCheck{}, err
	}
	if err := s.auditRequirementCheck(ctx, "dpo.requirement_check.assess", id, nil,
		map[string]any{"legal_entity_id": legalEntityID, "result": res.ScreeningResult, "basis": res.ScreeningReason}); err != nil {
		return RequirementCheck{}, err
	}
	return toRequirementCheck(row), nil
}

// decideDpoRequirement is ม.41(2)'s own OR rule: any one of the four criteria (public authority, large-scale
// processing, regular/systematic monitoring, sensitive/criminal-record core processing) already triggers the
// duty to appoint a DPO — unlike DPIA-02's own multi-factor threshold, there is no "recommended" middle tier
// here; the module doc's own acceptance criterion asks only for required-or-not plus a reason citing the
// notice.
func decideDpoRequirement(factorCount int, factors []forms.Contribution) (result, reason string) {
	if factorCount == 0 {
		return "not_required", "ไม่พบเกณฑ์ตามมาตรา 41(2) หรือประกาศ สคส. เรื่อง DPO พ.ศ. 2566 ข้อใดที่เข้าเงื่อนไของค์กรนี้"
	}
	var flagged []string
	for _, f := range factors {
		if f.Points > 0 {
			label := f.Label["th"]
			if label == "" {
				label = f.Question
			}
			flagged = append(flagged, label)
		}
	}
	return "required", fmt.Sprintf("เข้าเกณฑ์ตามมาตรา 41(2) และประกาศ สคส. เรื่อง DPO พ.ศ. 2566: %v", flagged)
}

// GetLatestRequirementCheck returns a legal entity's most recent ม.41(2) screening round, if any.
func (s *Service) GetLatestRequirementCheck(ctx context.Context, legalEntityID uuid.UUID) (RequirementCheck, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetLatestRequirementCheckForLegalEntity(ctx, legalEntityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RequirementCheck{}, ErrNotFound
	}
	if err != nil {
		return RequirementCheck{}, err
	}
	return toRequirementCheck(row), nil
}

// ListRequirementChecks returns every ม.41(2) screening round for a legal entity, newest first.
func (s *Service) ListRequirementChecks(ctx context.Context, legalEntityID uuid.UUID) ([]RequirementCheck, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListRequirementChecks(ctx, legalEntityID)
	if err != nil {
		return nil, err
	}
	out := make([]RequirementCheck, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRequirementCheck(r))
	}
	return out, nil
}

func (s *Service) auditRequirementCheck(ctx context.Context, action string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		var t string
		if terr := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); terr != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("dpo: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: RequirementCheckEntityType, EntityID: &id, Before: before, After: after}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}

func toRequirementCheck(r dpostore.DpoRequirementCheck) RequirementCheck {
	return RequirementCheck{
		ID: r.ID, LegalEntityID: r.LegalEntityID, AssessmentID: r.AssessmentID, Result: r.Result, Basis: r.Basis,
		AssessedAt: r.AssessedAt.Time, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time,
	}
}
