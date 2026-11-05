package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	breachstore "pdpa-platform/internal/breach/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

// Assessment is one risk assessment of an incident (BRE-05): the answers of a published breach form, the score, the
// risk level from the form's band, and each factor's contribution — the reasons behind the level.
type Assessment struct {
	ID               uuid.UUID
	FormSubmissionID uuid.UUID
	Score            float64
	RiskLevel        string
	Factors          []forms.Contribution
	AssessedBy       uuid.UUID
	AssessedByName   string
	AssessedAt       time.Time
}

var riskLevels = []string{RiskNone, RiskLow, RiskHigh}

// assessmentVersion is the current published version of a breach assessment form, checked usable: its bands must be
// exactly the risk levels, so every result maps to one.
func (s *Service) assessmentVersion(ctx context.Context, formID uuid.UUID) (forms.Version, error) {
	f, err := s.Forms.GetForm(ctx, formID)
	if errors.Is(err, forms.ErrNotFound) || errors.Is(err, forms.ErrForbidden) {
		return forms.Version{}, ErrBadForm
	}
	if err != nil {
		return forms.Version{}, err
	}
	if f.Type != FormType || f.CurrentVersionID == nil {
		return forms.Version{}, ErrBadForm
	}
	for _, v := range f.Versions {
		if v.ID != *f.CurrentVersionID {
			continue
		}
		if v.Scoring == nil || len(v.Scoring.Bands) == 0 {
			return forms.Version{}, ErrBadForm
		}
		for _, b := range v.Scoring.Bands {
			if !slices.Contains(riskLevels, b.Key) {
				return forms.Version{}, ErrBadForm
			}
		}
		return v, nil
	}
	return forms.Version{}, ErrBadForm
}

// Assess records a risk assessment of an incident in assessing: the answers are validated and scored by the form
// engine (PLT-06), the band gives the risk level, which becomes the incident's; breach.assessed is published.
func (s *Service) Assess(ctx context.Context, id, formID uuid.UUID, answers forms.Answers) (Assessment, error) {
	if !has(ctx, PermUpdate) {
		return Assessment{}, ErrForbidden
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	in, err := s.load(ctx, q, id, true)
	if err != nil {
		return Assessment{}, err
	}
	if !canSee(ctx, in) {
		return Assessment{}, ErrNotFound
	}
	if in.Status != StatusAssessing {
		return Assessment{}, ErrInvalidTransition
	}
	v, err := s.assessmentVersion(ctx, formID)
	if err != nil {
		return Assessment{}, err
	}
	me := currentUser(ctx)
	subID, res, err := s.Forms.Record(ctx, v.ID, answers, "user", me, IncidentType, &id)
	var ae *forms.AnswerErrors
	if errors.As(err, &ae) {
		ve := &ValidationError{}
		for _, f := range ae.Fields {
			ve.Fields = append(ve.Fields, FieldError{"answers." + f.Question, f.Code})
		}
		return Assessment{}, ve
	}
	if err != nil {
		return Assessment{}, err
	}
	if !slices.Contains(riskLevels, res.Band) {
		return Assessment{}, ErrBadForm // a score outside every band
	}
	factors := forms.Contributions(v.Schema, res.Answers)
	raw, _ := json.Marshal(factors)
	aid, err := uuid.NewV7()
	if err != nil {
		return Assessment{}, err
	}
	now := s.now()
	var score pgtype.Numeric
	if err := score.Scan(strconv.FormatFloat(res.Score, 'f', 2, 64)); err != nil {
		return Assessment{}, err
	}
	if err := q.InsertAssessment(ctx, breachstore.InsertAssessmentParams{ID: aid, IncidentID: id, FormSubmissionID: subID, Score: score,
		RiskLevel: res.Band, Factors: raw, AssessedBy: *me, AssessedAt: ts(now)}); err != nil {
		return Assessment{}, err
	}
	risk := res.Band
	if err := q.SetIncidentRisk(ctx, breachstore.SetIncidentRiskParams{RiskLevel: &risk, Actor: pgUUID(me), ID: id}); err != nil {
		return Assessment{}, err
	}
	in.RiskLevel = risk
	if err := s.timeline(ctx, id, "decision", "assessment:"+risk+":"+strconv.FormatFloat(res.Score, 'f', -1, 64), "", true); err != nil {
		return Assessment{}, err
	}
	if err := s.publish(ctx, "breach.assessed", in); err != nil {
		return Assessment{}, err
	}
	if err := s.audit(ctx, "breach.incident.assess", IncidentType, id, nil, map[string]any{"risk_level": risk, "score": res.Score, "form_submission_id": subID}); err != nil {
		return Assessment{}, err
	}
	return Assessment{ID: aid, FormSubmissionID: subID, Score: res.Score, RiskLevel: risk, Factors: factors, AssessedBy: *me, AssessedAt: now}, nil
}

// Assessments lists an incident's assessments, newest first.
func (s *Service) Assessments(ctx context.Context, id uuid.UUID) ([]Assessment, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := breachstore.New(pdb.MustTxFromContext(ctx)).ListAssessments(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]Assessment, 0, len(rows))
	var ids []uuid.UUID
	for _, r := range rows {
		a := Assessment{ID: r.ID, FormSubmissionID: r.FormSubmissionID, RiskLevel: r.RiskLevel, AssessedBy: r.AssessedBy, AssessedAt: r.AssessedAt.Time.UTC()}
		if f, err := r.Score.Float64Value(); err == nil {
			a.Score = f.Float64
		}
		_ = json.Unmarshal(r.Factors, &a.Factors)
		if a.Factors == nil {
			a.Factors = []forms.Contribution{}
		}
		ids = append(ids, r.AssessedBy)
		out = append(out, a)
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].AssessedByName = names[out[i].AssessedBy]
	}
	return out, nil
}

// Decide is the notification decision of an assessed incident (BRE-06), by someone with breach.incident.approve
// (the DPO) and with a reason: no notice (→ remediating; the 72-hour clock stops), notify the PDPC, or the PDPC and the
// data subjects (→ notifying). The decision may go further than the assessed risk requires, never less (s.37(4)).
func (s *Service) Decide(ctx context.Context, id uuid.UUID, version int32, decision, reason string) (Incident, error) {
	if !has(ctx, PermApprove) {
		return Incident{}, ErrForbidden
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	cur, err := s.load(ctx, q, id, true)
	if err != nil {
		return Incident{}, err
	}
	if cur.RowVersion != version {
		return Incident{}, ErrVersionMismatch
	}
	if cur.Status != StatusAssessing {
		return Incident{}, ErrInvalidTransition
	}
	if cur.RiskLevel == "" {
		return Incident{}, ErrNoAssessment
	}
	if _, ok := decisionRank[decision]; !ok {
		return Incident{}, invalid("decision", "invalid")
	}
	if !DecisionAllowed(cur.RiskLevel, decision) {
		return Incident{}, ErrDecisionTooWeak
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Incident{}, invalid("reason", "required")
	}
	to := StatusNotifying
	if decision == DecisionNone {
		to = StatusRemediating
	}
	if err := q.SetIncidentDecision(ctx, breachstore.SetIncidentDecisionParams{Decision: &decision, DecisionReason: &reason, Actor: pgUUID(currentUser(ctx)),
		Status: to, ID: id}); err != nil {
		return Incident{}, err
	}
	if err := s.timeline(ctx, id, "decision", "decision:"+decision+":"+cur.RiskLevel, reason, true); err != nil {
		return Incident{}, err
	}
	if err := s.timeline(ctx, id, "decision", "status:"+cur.Status+":"+to, "", true); err != nil {
		return Incident{}, err
	}
	if err := s.audit(ctx, "breach.incident.decide", IncidentType, id, map[string]any{"status": cur.Status, "decision": cur.Decision},
		map[string]any{"status": to, "decision": decision, "risk_level": cur.RiskLevel, "reason": reason}); err != nil {
		return Incident{}, err
	}
	return s.Get(ctx, id)
}
