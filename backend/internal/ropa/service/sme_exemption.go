// ROPA-20: SME exemption screening (ประกาศ สคส. ยกเว้นกิจการขนาดเล็ก พ.ศ. 2565 (ผู้ควบคุม) และ พ.ศ. 2567
// (ผู้ประมวลผล)). ropa.sme_exemption_checks was already fully specified in the baseline migrations — no
// migration for the table itself. Answers a published, global "sme_exemption" PLT-06 form (migration
// 00090) in one shot via forms.Service.Record (the same one-step-record pattern DPO-09/BRE-05/RRA-05's own
// checklists already use), then applies this feature's own business rule to the raw answers.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
	ropastore "pdpa-platform/internal/ropa/store"
)

const SmeExemptionFormCode = "sme_exemption"

// smeControllerKeys / smeProcessorKeys are the seeded screening form's own known question keys per
// section — every "yes" in a section means that section's own exemption applies.
var smeControllerKeys = []string{"is_sme_or_community", "not_large_scale", "no_sensitive_data", "no_high_risk_profiling"}
var smeProcessorKeys = []string{"acts_only_as_processor", "no_subcontracted_high_risk"}

var ErrBadSmeExemptionForm = errors.New("ropa: no published sme_exemption screening form")

// SmeExemptionCheck is one screening round against a legal entity.
type SmeExemptionCheck struct {
	ID               uuid.UUID
	LegalEntityID    uuid.UUID
	FormSubmissionID uuid.UUID
	Result           string // exempt | not_exempt | partial
	Basis            string
	AssessedAt       time.Time
	RowVersion       int32
	CreatedAt        time.Time
}

func toSmeExemptionCheck(id, legalEntityID, formSubmissionID uuid.UUID, result, basis string, assessedAt, createdAt pgtype.Timestamptz, rowVersion int32) SmeExemptionCheck {
	return SmeExemptionCheck{
		ID: id, LegalEntityID: legalEntityID, FormSubmissionID: formSubmissionID, Result: result, Basis: basis,
		AssessedAt: assessedAt.Time.UTC(), RowVersion: rowVersion, CreatedAt: createdAt.Time.UTC(),
	}
}

func allYes(answers forms.Answers, keys []string) bool {
	for _, k := range keys {
		v, ok := answers[k]
		if !ok || v != "yes" {
			return false
		}
	}
	return true
}

// SmeExemptionResult is the acceptance criterion's own business rule, pure and directly testable: every
// controller-section answer "yes" qualifies for the 2565 controller exemption; every processor-section
// answer "yes" qualifies for the 2567 processor exemption (only meaningful when isProcessor). Neither
// qualifying is not_exempt; both (or the one that applies) qualifying is exempt; qualifying for one but
// not the other (when both are in scope) is partial.
func SmeExemptionResult(isProcessor bool, answers forms.Answers) (result, basis string) {
	controllerOK := allYes(answers, smeControllerKeys)
	if !isProcessor {
		if controllerOK {
			return "exempt", "เข้าเกณฑ์ยกเว้นสำหรับผู้ควบคุมข้อมูลขนาดเล็กตามประกาศ สคส. พ.ศ. 2565 ทุกข้อ"
		}
		return "not_exempt", "ไม่เข้าเกณฑ์ยกเว้นผู้ควบคุมข้อมูลขนาดเล็กตามประกาศ สคส. พ.ศ. 2565 อย่างน้อยหนึ่งข้อ"
	}
	processorOK := allYes(answers, smeProcessorKeys)
	switch {
	case controllerOK && processorOK:
		return "exempt", "เข้าเกณฑ์ยกเว้นทั้งผู้ควบคุมข้อมูล (ประกาศ พ.ศ. 2565) และผู้ประมวลผลข้อมูล (ประกาศ พ.ศ. 2567) ขนาดเล็ก"
	case controllerOK && !processorOK:
		return "partial", "เข้าเกณฑ์ยกเว้นผู้ควบคุมข้อมูลขนาดเล็ก (ประกาศ พ.ศ. 2565) แต่ไม่เข้าเกณฑ์ยกเว้นผู้ประมวลผลข้อมูล (ประกาศ พ.ศ. 2567)"
	case !controllerOK && processorOK:
		return "partial", "เข้าเกณฑ์ยกเว้นผู้ประมวลผลข้อมูลขนาดเล็ก (ประกาศ พ.ศ. 2567) แต่ไม่เข้าเกณฑ์ยกเว้นผู้ควบคุมข้อมูล (ประกาศ พ.ศ. 2565)"
	default:
		return "not_exempt", "ไม่เข้าเกณฑ์ยกเว้นทั้งผู้ควบคุมข้อมูล (ประกาศ พ.ศ. 2565) และผู้ประมวลผลข้อมูล (ประกาศ พ.ศ. 2567)"
	}
}

// publishedSmeExemptionForm resolves the one global, published "sme_exemption" form (migration 00090).
func (s *Service) publishedSmeExemptionForm(ctx context.Context) (forms.Form, error) {
	list, err := s.Forms.ListForms(ctx, "sme_exemption")
	if err != nil {
		return forms.Form{}, err
	}
	for _, f := range list {
		if f.Code == SmeExemptionFormCode && f.CurrentVersionID != nil {
			return f, nil
		}
	}
	return forms.Form{}, ErrBadSmeExemptionForm
}

// AssessSmeExemption is the acceptance criterion itself ("ผลประเมินระบุว่าได้รับยกเว้นหรือไม่พร้อมเหตุผล
// อ้างประกาศ"): answers the seeded screening form for one legal entity and records the result + the basis
// citing the relevant notice(s).
func (s *Service) AssessSmeExemption(ctx context.Context, legalEntityID uuid.UUID, isProcessor bool, answers forms.Answers) (SmeExemptionCheck, error) {
	if _, err := s.Org.GetLegalEntity(ctx, legalEntityID); err != nil {
		return SmeExemptionCheck{}, ErrInvalid
	}
	form, err := s.publishedSmeExemptionForm(ctx)
	if err != nil {
		return SmeExemptionCheck{}, err
	}
	g, _ := authz.FromContext(ctx)
	var me *uuid.UUID
	if u, err := uuid.Parse(g.UserID); err == nil {
		me = &u
	}
	submissionID, res, err := s.Forms.Record(ctx, *form.CurrentVersionID, answers, "user", me, "ropa_sme_exemption", &legalEntityID)
	if err != nil {
		var ae *forms.AnswerErrors
		if errors.As(err, &ae) {
			return SmeExemptionCheck{}, fmt.Errorf("%w: %v", ErrInvalid, ae.Fields)
		}
		if errors.Is(err, forms.ErrNotFound) || errors.Is(err, forms.ErrInvalidState) {
			return SmeExemptionCheck{}, ErrBadSmeExemptionForm
		}
		return SmeExemptionCheck{}, err
	}
	result, basis := SmeExemptionResult(isProcessor, res.Answers)
	id, err := uuid.NewV7()
	if err != nil {
		return SmeExemptionCheck{}, err
	}
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).InsertSmeExemptionCheck(ctx, ropastore.InsertSmeExemptionCheckParams{
		ID: id, LegalEntityID: legalEntityID, FormSubmissionID: submissionID, Result: result, Basis: basis, AssessedAt: now,
	})
	if err != nil {
		return SmeExemptionCheck{}, err
	}
	return toSmeExemptionCheck(row.ID, row.LegalEntityID, row.FormSubmissionID, row.Result, row.Basis, row.AssessedAt, now, row.RowVersion), nil
}

// GetSmeExemptionCheck reads one screening round — RLS already scopes it to this tenant.
func (s *Service) GetSmeExemptionCheck(ctx context.Context, id uuid.UUID) (SmeExemptionCheck, error) {
	r, err := ropastore.New(pdb.MustTxFromContext(ctx)).GetSmeExemptionCheck(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return SmeExemptionCheck{}, ErrNotFound
	}
	if err != nil {
		return SmeExemptionCheck{}, err
	}
	return toSmeExemptionCheck(r.ID, r.LegalEntityID, r.FormSubmissionID, r.Result, r.Basis, r.AssessedAt, r.CreatedAt, r.RowVersion), nil
}

// ListSmeExemptionChecks returns every screening round for a legal entity, newest first.
func (s *Service) ListSmeExemptionChecks(ctx context.Context, legalEntityID uuid.UUID) ([]SmeExemptionCheck, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListSmeExemptionChecksByLegalEntity(ctx, legalEntityID)
	if err != nil {
		return nil, err
	}
	out := make([]SmeExemptionCheck, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSmeExemptionCheck(r.ID, r.LegalEntityID, r.FormSubmissionID, r.Result, r.Basis, r.AssessedAt, r.CreatedAt, r.RowVersion))
	}
	return out, nil
}
