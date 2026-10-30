// Package service — ROPA-11: a department-facing data mapping questionnaire. The questionnaire's own
// questions are an ordinary PLT-06 form of the already-registered "questionnaire" type
// (internal/wiring/forms.go reserves it with Respond: "ropa.activity.update" — this feature is that
// reservation's first real consumer, CLAUDE.md rule 8: never hard-code the questions here). This file
// is new code against ropa.questionnaires, a table already fully specified in the baseline migrations
// (00008_ropa.sql) with no queries against it until now.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/platform/notify"
	ropastore "pdpa-platform/internal/ropa/store"
)

// QuestionnaireFormType is the PLT-06 form type ROPA-11 answers against — already registered in
// internal/wiring/forms.go ahead of this feature (Respond: "ropa.activity.update").
const QuestionnaireFormType = "questionnaire"

// QuestionnaireEntityType is the PLT-07 collaboration record type this feature registers (ROPA-12's
// own pattern): a DPO's rejection reason, or any back-and-forth with the respondent, is a comment on
// the assignment itself — ropa.questionnaires has no reason column of its own to add one for.
const QuestionnaireEntityType = "ropa_questionnaire"

var questionnaireStatuses = []string{"sent", "answered", "approved", "rejected"}

// Questionnaire is one department's own round of the data mapping questionnaire.
type Questionnaire struct {
	ID               uuid.UUID
	OrgUnitID        uuid.UUID
	ActivityID       *uuid.UUID
	FormSubmissionID *uuid.UUID
	RespondentUserID uuid.UUID
	RespondentName   string
	DueAt            *time.Time
	Status           string
	ApprovedBy       *uuid.UUID
	RowVersion       int32
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ErrBadQuestionnaireForm is returned when the given form version isn't a published "questionnaire"-type
// form — the same sentinel shape ErrBadChecklistForm/ErrBadTemplate already use elsewhere in this codebase.
var ErrBadQuestionnaireForm = errors.New("ropa: not a published questionnaire form")

func toQuestionnaire(r ropastore.RopaQuestionnaire) Questionnaire {
	q := Questionnaire{ID: r.ID, OrgUnitID: r.OrgUnitID, RespondentUserID: r.RespondentUserID, Status: r.Status,
		ActivityID: uuidPtr(r.ActivityID), FormSubmissionID: uuidPtr(r.FormSubmissionID), ApprovedBy: uuidPtr(r.ApprovedBy),
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.DueAt.Valid {
		t := r.DueAt.Time
		q.DueAt = &t
	}
	return q
}

// SendQuestionnaire is BP-05's own first step ("ส่งแบบสอบถามให้แผนกกรอก"): a check that the org unit is
// visible under RLS (rule 1) and the respondent is a real, active user of this tenant (the same
// iamservice.Names FK-visibility check ROPA-02/DPO-01 already use), then one new "sent" round. Best-effort
// notifies the respondent (PLT-04) — a lost notification never fails the real assignment.
func (s *Service) SendQuestionnaire(ctx context.Context, orgUnitID, respondentUserID uuid.UUID, dueAt *time.Time) (Questionnaire, error) {
	unit, err := s.Org.GetOrgUnit(ctx, orgUnitID)
	if err != nil {
		return Questionnaire{}, fmt.Errorf("%w: org_unit_id", ErrInvalid)
	}
	names, err := iamservice.Names(ctx, []uuid.UUID{respondentUserID})
	if err != nil {
		return Questionnaire{}, err
	}
	name, ok := names[respondentUserID]
	if !ok {
		return Questionnaire{}, fmt.Errorf("%w: respondent_user_id", ErrInvalid)
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		return Questionnaire{}, fmt.Errorf("%w: no tenant in context", ErrInvalid)
	}
	var due pgtype.Timestamptz
	if dueAt != nil {
		due = pgtype.Timestamptz{Time: *dueAt, Valid: true}
	}
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).InsertQuestionnaire(ctx, ropastore.InsertQuestionnaireParams{
		TenantID: tenant, OrgUnitID: orgUnitID, RespondentUserID: respondentUserID, DueAt: due,
	})
	if err != nil {
		return Questionnaire{}, err
	}
	out := toQuestionnaire(row)
	out.RespondentName = name
	if err := s.audit(ctx, "ropa.questionnaire.send", out.ID, nil, map[string]any{"org_unit_id": orgUnitID, "respondent_user_id": respondentUserID}); err != nil {
		return Questionnaire{}, err
	}
	s.notifyQuestionnaireSent(ctx, out, unit.NameTh)
	return out, nil
}

func (s *Service) notifyQuestionnaireSent(ctx context.Context, q Questionnaire, orgUnitName string) {
	if s.Notify == nil {
		return
	}
	for _, ch := range []string{"in_app", "email"} {
		uid := q.RespondentUserID
		_ = pdb.Savepoint(ctx, func(ctx context.Context) error {
			_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "ropa.questionnaire_sent", Channel: ch,
				RecipientUserID: &uid, Vars: map[string]any{"org_unit": orgUnitName},
				EntityType: QuestionnaireEntityType, EntityID: &q.ID})
			return err
		})
	}
}

// GetQuestionnaire reads one round by id — RLS already scopes it to this tenant.
func (s *Service) GetQuestionnaire(ctx context.Context, id uuid.UUID) (Questionnaire, error) {
	r, err := ropastore.New(pdb.MustTxFromContext(ctx)).GetQuestionnaire(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Questionnaire{}, ErrNotFound
	}
	if err != nil {
		return Questionnaire{}, err
	}
	out := toQuestionnaire(r)
	if names, err := iamservice.Names(ctx, []uuid.UUID{out.RespondentUserID}); err == nil {
		out.RespondentName = names[out.RespondentUserID]
	}
	return out, nil
}

// ListQuestionnaires filters by org unit and/or status.
func (s *Service) ListQuestionnaires(ctx context.Context, orgUnitID *uuid.UUID, status string) ([]Questionnaire, error) {
	rows, err := ropastore.New(pdb.MustTxFromContext(ctx)).ListQuestionnaires(ctx, ropastore.ListQuestionnairesParams{
		OrgUnitID: pgUUID(orgUnitID), Status: optStatus(status),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Questionnaire, 0, len(rows))
	var ids []uuid.UUID
	for _, r := range rows {
		q := toQuestionnaire(r)
		out = append(out, q)
		ids = append(ids, q.RespondentUserID)
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].RespondentName = names[out[i].RespondentUserID]
	}
	return out, nil
}

func optStatus(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// AnswerQuestionnaire is the respondent's own one-shot submission (forms.Service.Record — the same
// one-step-record path DPO-09/BRE-05/RRA-05's own checklist already use, not PLT-06's draft/section flow):
// the whole form is filled and validated in one call against a formVersionID the caller already has
// (fetched generically through the forms engine's own published-form listing). A round still "sent" or
// "rejected" (a redo after DPO feedback) may be answered; any other status is refused.
func (s *Service) AnswerQuestionnaire(ctx context.Context, id, formVersionID uuid.UUID, answers forms.Answers) (Questionnaire, error) {
	cur, err := s.GetQuestionnaire(ctx, id)
	if err != nil {
		return Questionnaire{}, err
	}
	if cur.Status != "sent" && cur.Status != "rejected" {
		return Questionnaire{}, ErrInvalidTransition
	}
	g, _ := authz.FromContext(ctx)
	var me *uuid.UUID
	if u, err := uuid.Parse(g.UserID); err == nil {
		me = &u
	}
	subID, _, err := s.Forms.Record(ctx, formVersionID, answers, "user", me, QuestionnaireEntityType, &id)
	if err != nil {
		var ae *forms.AnswerErrors
		if errors.As(err, &ae) {
			return Questionnaire{}, fmt.Errorf("%w: %v", ErrInvalid, ae.Fields)
		}
		if errors.Is(err, forms.ErrNotFound) || errors.Is(err, forms.ErrInvalidState) {
			return Questionnaire{}, ErrBadQuestionnaireForm
		}
		return Questionnaire{}, err
	}
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).MarkQuestionnaireAnswered(ctx, ropastore.MarkQuestionnaireAnsweredParams{
		ID: id, FormSubmissionID: subID, UpdatedBy: pgUUID(me), RowVersion: cur.RowVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Questionnaire{}, ErrVersionMismatch
	}
	if err != nil {
		return Questionnaire{}, err
	}
	out := toQuestionnaire(row)
	if err := s.audit(ctx, "ropa.questionnaire.answer", out.ID, map[string]any{"status": "sent"}, map[string]any{"status": "answered"}); err != nil {
		return Questionnaire{}, err
	}
	return out, nil
}

// ApproveQuestionnaire is this feature's own acceptance criterion ("คำตอบที่อนุมัติแล้วเข้า RoPA
// อัตโนมัติ"): the submitted answers — read back through forms.Service.GetResponse, the generic
// response reader every DPO caller can already use (they hold the form type's own Respond permission,
// "ropa.activity.update", which loadResponse's own access check already accepts) — are mapped onto a
// brand-new processing activity the exact way ROPA-05's own CreateActivityFromTemplate builds one from a
// template's defaults: a purpose only when both purpose_text and lawful_basis_code were answered, a data
// row only when both data_category_code and subject_type_code were answered, a retention rule only when
// retention_months, retention_basis, trigger_event and disposal_method were all answered — each missing topic is simply left for the
// OWNER to add by hand afterward (the same "existence, not a forced guess" reasoning RTG-01's own
// template defaults already use), never an error. legalEntityID/code/ownerUserID are the caller's own
// choices for the new activity, the same three fields RTG-04's own batch-from-template wizard asks for.
func (s *Service) ApproveQuestionnaire(ctx context.Context, id, legalEntityID uuid.UUID, code string, ownerUserID *uuid.UUID, version int32) (Questionnaire, Activity, error) {
	cur, err := s.GetQuestionnaire(ctx, id)
	if err != nil {
		return Questionnaire{}, Activity{}, err
	}
	if cur.Status != "answered" {
		return Questionnaire{}, Activity{}, ErrInvalidTransition
	}
	if cur.RowVersion != version {
		return Questionnaire{}, Activity{}, ErrVersionMismatch
	}
	if cur.FormSubmissionID == nil {
		return Questionnaire{}, Activity{}, ErrBadQuestionnaireForm
	}
	resp, err := s.Forms.GetResponse(ctx, *cur.FormSubmissionID)
	if err != nil {
		return Questionnaire{}, Activity{}, err
	}
	activity, err := s.createActivityFromQuestionnaire(ctx, legalEntityID, cur.OrgUnitID, code, ownerUserID, resp.Answers)
	if err != nil {
		return Questionnaire{}, Activity{}, err
	}
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).DecideQuestionnaire(ctx, ropastore.DecideQuestionnaireParams{
		ID: id, Status: "approved", ActivityID: pgUUID(&activity.ID), DecidedBy: decidedByOf(ctx), RowVersion: version,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Questionnaire{}, Activity{}, ErrVersionMismatch
	}
	if err != nil {
		return Questionnaire{}, Activity{}, err
	}
	out := toQuestionnaire(row)
	if err := s.audit(ctx, "ropa.questionnaire.approve", out.ID, map[string]any{"status": "answered"}, map[string]any{"status": "approved", "activity_id": activity.ID}); err != nil {
		return Questionnaire{}, Activity{}, err
	}
	return out, activity, nil
}

// RejectQuestionnaire sends an answered round back to the respondent — a reason is left as a comment
// on the assignment's own PLT-07 record (QuestionnaireEntityType), not a column this table has none of.
func (s *Service) RejectQuestionnaire(ctx context.Context, id uuid.UUID, version int32) (Questionnaire, error) {
	cur, err := s.GetQuestionnaire(ctx, id)
	if err != nil {
		return Questionnaire{}, err
	}
	if cur.Status != "answered" {
		return Questionnaire{}, ErrInvalidTransition
	}
	row, err := ropastore.New(pdb.MustTxFromContext(ctx)).DecideQuestionnaire(ctx, ropastore.DecideQuestionnaireParams{
		ID: id, Status: "rejected", ActivityID: pgUUID(nil), DecidedBy: decidedByOf(ctx), RowVersion: version,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Questionnaire{}, ErrVersionMismatch
	}
	if err != nil {
		return Questionnaire{}, err
	}
	out := toQuestionnaire(row)
	return out, s.audit(ctx, "ropa.questionnaire.reject", out.ID, map[string]any{"status": "answered"}, map[string]any{"status": "rejected"})
}

func decidedByOf(ctx context.Context) pgtype.UUID {
	g, _ := authz.FromContext(ctx)
	if u, err := uuid.Parse(g.UserID); err == nil {
		return pgUUID(&u)
	}
	return pgUUID(nil)
}

// createActivityFromQuestionnaire is the field-mapping core shared by ApproveQuestionnaire — a sibling
// of CreateActivityFromTemplate (from_template.go), reading forms.Answers by fixed question key instead
// of a template's own defaults jsonb. The DPO who designs the questionnaire form in PLT-06's builder
// names its questions with these exact keys (documented in docs/modules/ROPA.md#ropa-11), the same
// "code-keyed, resolved against this tenant's own master data" convention RTG-01's own defaults already
// establish.
func (s *Service) createActivityFromQuestionnaire(ctx context.Context, legalEntityID, orgUnitID uuid.UUID, code string, ownerUserID *uuid.UUID, a forms.Answers) (Activity, error) {
	name, _ := a["activity_name"].(string)
	if name == "" {
		return Activity{}, fmt.Errorf("%w: activity_name answer is required to create a RoPA activity", ErrInvalid)
	}
	out, err := s.SaveActivity(ctx, Activity{
		LegalEntityID: legalEntityID, OrgUnitID: orgUnitID, Code: code, Name: name,
		Description: strVal(a, "activity_description"), Role: "controller", OwnerUserID: ownerUserID,
		RightsAndAccess: defaultRightsAndAccess,
	}, 0)
	if err != nil {
		return Activity{}, err
	}

	if purposeText, basisCode := strVal(a, "purpose_text"), strVal(a, "lawful_basis_code"); purposeText != "" && basisCode != "" {
		if _, err := s.AddActivityPurpose(ctx, ActivityPurpose{ActivityID: out.ID, PurposeText: purposeText, LawfulBasisCode: basisCode}); err != nil {
			return Activity{}, fmt.Errorf("questionnaire purpose: %w", err)
		}
	}

	if catCode, subCode := strVal(a, "data_category_code"), strVal(a, "subject_type_code"); catCode != "" && subCode != "" {
		catID, err := s.masterIDByCode(ctx, "data_categories", catCode)
		if err != nil {
			return Activity{}, err
		}
		subID, err := s.masterIDByCode(ctx, "data_subject_types", subCode)
		if err != nil {
			return Activity{}, err
		}
		if _, err := s.AddActivityData(ctx, ActivityData{ActivityID: out.ID, DataCategoryID: catID, SubjectTypeID: subID, Source: "direct"}); err != nil {
			return Activity{}, fmt.Errorf("questionnaire data: %w", err)
		}
	}

	months, basis, trigger, method := numVal(a, "retention_months"), strVal(a, "retention_basis"), strVal(a, "trigger_event"), strVal(a, "disposal_method")
	if months > 0 && basis != "" && trigger != "" && method != "" {
		m := int(months)
		if _, err := s.AddRetentionRule(ctx, RetentionRule{ActivityID: out.ID, RetentionMonths: &m, RetentionBasis: basis,
			TriggerEvent: trigger, DisposalMethod: method}); err != nil {
			return Activity{}, fmt.Errorf("questionnaire retention: %w", err)
		}
	}

	return s.GetActivity(ctx, out.ID)
}

func strVal(a forms.Answers, key string) string {
	s, _ := a[key].(string)
	return s
}

func numVal(a forms.Answers, key string) int32 {
	switch v := a[key].(type) {
	case float64:
		return int32(v)
	case int:
		return int32(v)
	}
	return 0
}
