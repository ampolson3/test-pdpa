package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
	vendorstore "pdpa-platform/internal/vendormgmt/store"
)

// IntakeFormType is the PLT-06 form type VEN-02 uses — "intake", already in platform.form_definitions'
// form_type CHECK constraint and already named "reserved but unregistered" by internal/wiring.Forms's own
// comment, unused until this feature.
const IntakeFormType = "intake"

// IntakeTemplateCode is the seeded global intake form's code (migration 00058, docs/decisions.md Q-33). A
// tenant may publish its own "intake" form with the same code to override it (publishedIntakeVersion prefers
// the tenant's own form over the global default) — the same override pattern DPIA-01's screening template
// already established.
const IntakeTemplateCode = "vendor_intake"

// ErrNoIntakeForm means no published "intake" form with IntakeTemplateCode is visible to this tenant (the
// seed migration was rolled back, or every published version was somehow retired).
var ErrNoIntakeForm = errors.New("vendor: no published intake form")

// Intake is one VEN-02 tiering round: the inherent-risk score and resulting tier from answering the intake
// questionnaire once (forms.Service.Record — a one-shot submission, the same mechanism breach risk
// assessments and DPO-09's security checklist already use).
type Intake struct {
	ID               uuid.UUID
	VendorID         uuid.UUID
	FormSubmissionID uuid.UUID
	InherentScore    float64
	TierResult       string
	RowVersion       int32
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// RequiredAssessmentCodes names the VEN-04 assessment templates (migration 00053: vendor_pdpa/
// vendor_security/vendor_transfer) a vendor's tier calls for before its first assessment cycle — the
// acceptance criterion's "กำหนดแบบประเมินที่ต้องส่ง". This mapping is a tunable business default, not a
// legally-mandated rule (no docs/decisions.md entry — changing which tier requires which template changes
// no data model or legally-required behaviour): every tier gets the PDPA compliance questionnaire except
// "low", every tier at "high" or above also gets the security assessment, and the cross-border transfer
// assessment is required whenever the intake's own cross_border_transfer answer was "yes", regardless of
// tier (an otherwise low-risk engagement that still moves data abroad still needs that specific check).
func RequiredAssessmentCodes(tier string, crossBorderTransfer bool) []string {
	var codes []string
	if tier != "low" {
		codes = append(codes, "vendor_pdpa")
	}
	if tier == "high" || tier == "critical" {
		codes = append(codes, "vendor_security")
	}
	if crossBorderTransfer {
		codes = append(codes, "vendor_transfer")
	}
	return codes
}

// reassessmentInterval is how soon a tenant should re-run the vendor's assessment cycle, by tier — another
// tunable default (no docs/decisions.md entry: a scheduling convenience, not a legal deadline the way
// breach/DSAR's own clocks are). Higher risk is reviewed more often.
func reassessmentInterval(tier string) time.Duration {
	switch tier {
	case "critical":
		return 6 * 30 * 24 * time.Hour
	case "high":
		return 12 * 30 * 24 * time.Hour
	case "medium":
		return 18 * 30 * 24 * time.Hour
	default: // "low"
		return 24 * 30 * 24 * time.Hour
	}
}

// publishedIntakeVersion resolves IntakeTemplateCode to its form's current *published* version — tiering
// must only ever run against a live, reviewed form (the same rule DPIA-01's own screeningVersion enforces),
// preferring a tenant's own override over the global default.
func (s *Service) publishedIntakeVersion(ctx context.Context) (forms.Form, forms.Version, error) {
	list, err := s.Forms.ListForms(ctx, IntakeFormType)
	if err != nil {
		return forms.Form{}, forms.Version{}, err
	}
	var match *forms.Form
	for i := range list {
		if list[i].Code != IntakeTemplateCode {
			continue
		}
		if match == nil || !list[i].Global {
			f := list[i]
			match = &f
		}
		if !list[i].Global {
			break
		}
	}
	if match == nil {
		return forms.Form{}, forms.Version{}, ErrNoIntakeForm
	}
	f, err := s.Forms.GetForm(ctx, match.ID)
	if errors.Is(err, forms.ErrNotFound) || errors.Is(err, forms.ErrForbidden) {
		return forms.Form{}, forms.Version{}, ErrNoIntakeForm
	}
	if err != nil {
		return forms.Form{}, forms.Version{}, err
	}
	if f.CurrentVersionID == nil {
		return forms.Form{}, forms.Version{}, ErrNoIntakeForm
	}
	for _, v := range f.Versions {
		if v.ID == *f.CurrentVersionID {
			return f, v, nil
		}
	}
	return forms.Form{}, forms.Version{}, ErrNoIntakeForm
}

// RecordIntake is VEN-02's acceptance criterion: answer the intake questionnaire once for a vendor and the
// system computes its tier (the form's own band, seeded low/medium/high/critical so it maps straight onto
// vendor.intakes.tier_result/vendor.vendors.tier with no translation step) and the assessment templates that
// tier now calls for — never left to a human's own reading of the answers.
func (s *Service) RecordIntake(ctx context.Context, vendorID uuid.UUID, answers forms.Answers) (Intake, []string, error) {
	before, err := s.GetVendor(ctx, vendorID)
	if err != nil {
		return Intake{}, nil, err
	}
	_, ver, err := s.publishedIntakeVersion(ctx)
	if err != nil {
		return Intake{}, nil, err
	}

	g, _ := authz.FromContext(ctx)
	var submitter *uuid.UUID
	if uid, err := uuid.Parse(g.UserID); err == nil {
		submitter = &uid
	}
	submissionID, res, err := s.Forms.Record(ctx, ver.ID, answers, "user", submitter, VendorEntityType, &vendorID)
	if err != nil {
		var ae *forms.AnswerErrors
		if errors.As(err, &ae) {
			return Intake{}, nil, fmt.Errorf("%w: %d answer(s) invalid", ErrInvalid, len(ae.Fields))
		}
		return Intake{}, nil, err
	}
	if res.Band == "" {
		return Intake{}, nil, fmt.Errorf("%w: answers did not resolve to a tier", ErrInvalid)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Intake{}, nil, err
	}
	row, err := vendorstore.New(pdb.MustTxFromContext(ctx)).InsertIntake(ctx, vendorstore.InsertIntakeParams{
		ID: id, VendorID: vendorID, FormSubmissionID: submissionID, InherentScore: numeric(res.Score), TierResult: res.Band,
	})
	if err != nil {
		return Intake{}, nil, err
	}

	tier := res.Band
	nextAt := time.Now().UTC().Add(reassessmentInterval(tier))
	if _, err := vendorstore.New(pdb.MustTxFromContext(ctx)).UpdateVendorTier(ctx, vendorstore.UpdateVendorTierParams{
		ID: vendorID, Tier: &tier, NextAssessmentAt: pgtype.Date{Time: nextAt, Valid: true},
	}); err != nil {
		return Intake{}, nil, err
	}

	crossBorder, _ := res.Answers["cross_border_transfer"].(string)
	required := RequiredAssessmentCodes(tier, crossBorder == "yes")

	if err := s.audit(ctx, "vendor.vendor.tier", vendorID, map[string]any{"tier": before.Tier},
		map[string]any{"tier": tier, "score": res.Score, "required_assessments": required}); err != nil {
		return Intake{}, nil, err
	}
	return toIntake(row), required, nil
}

// ListIntakes lists a vendor's tiering rounds, newest first.
func (s *Service) ListIntakes(ctx context.Context, vendorID uuid.UUID) ([]Intake, error) {
	if _, err := s.GetVendor(ctx, vendorID); err != nil {
		return nil, err
	}
	rows, err := vendorstore.New(pdb.MustTxFromContext(ctx)).ListIntakes(ctx, vendorID)
	if err != nil {
		return nil, err
	}
	out := make([]Intake, 0, len(rows))
	for _, r := range rows {
		out = append(out, toIntake(vendorstore.InsertIntakeRow(r)))
	}
	return out, nil
}

func toIntake(r vendorstore.InsertIntakeRow) Intake {
	score, _ := r.InherentScore.Float64Value()
	return Intake{
		ID: r.ID, VendorID: r.VendorID, FormSubmissionID: r.FormSubmissionID, InherentScore: score.Float64,
		TierResult: r.TierResult, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func numeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(f, 'f', 2, 64))
	return n
}
