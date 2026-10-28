package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/forms"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// questionnaireSetup wires a real Forms engine with the "questionnaire" type registered exactly the way
// internal/wiring/forms.go already reserves it ahead of this feature (Respond: "ropa.activity.update").
func questionnaireSetup(t *testing.T, suffix string) env {
	t.Helper()
	e := setup(t, suffix)
	e.svc.Forms = &forms.Service{Audit: e.svc.Audit}
	e.svc.Forms.Register("questionnaire", forms.Policy{Read: "ropa.template.read", Create: "ropa.template.create",
		Update: "ropa.template.update", Publish: "ropa.template.publish", Respond: "ropa.activity.update"})
	owner := dbtest.OwnerPool(t)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, e.tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM ropa.questionnaires`, `DELETE FROM platform.form_submissions`,
				`DELETE FROM platform.form_versions`, `DELETE FROM platform.form_definitions`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	return e
}

func (e env) inQ(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: []string{
			"ropa.template.read", "ropa.template.create", "ropa.template.update", "ropa.template.publish",
			"ropa.activity.read", "ropa.activity.create", "ropa.activity.update", "ropa.activity.approve",
			"org.structure.read", "org.structure.update",
		}}))
	}); err != nil {
		t.Fatal(err)
	}
}

// publishedQuestionnaireForm publishes a questionnaire of the "questionnaire" type whose own question
// keys match the fixed set docs/modules/ROPA.md#ropa-11 documents (the DPO's own responsibility to name
// them this way when designing the form in PLT-06's builder).
func publishedQuestionnaireForm(t *testing.T, ctx context.Context, e env) uuid.UUID {
	t.Helper()
	d := forms.Draft{Languages: []string{"th", "en"}, Schema: forms.Schema{Sections: []forms.Section{{Key: "mapping",
		Title: forms.Text{"th": "ข้อมูลกิจกรรม", "en": "Activity data"}, Questions: []forms.Question{
			{Key: "activity_name", Type: forms.TypeText, Label: forms.Text{"th": "ชื่อกิจกรรม", "en": "Activity name"}, Required: true},
			{Key: "purpose_text", Type: forms.TypeText, Label: forms.Text{"th": "วัตถุประสงค์", "en": "Purpose"}},
			{Key: "lawful_basis_code", Type: forms.TypeSingle, Label: forms.Text{"th": "ฐานกฎหมาย", "en": "Lawful basis"},
				Options: []forms.Option{{Value: "CONTRACT", Label: forms.Text{"th": "สัญญา"}}}},
			{Key: "data_category_code", Type: forms.TypeSingle, Label: forms.Text{"th": "หมวดข้อมูล", "en": "Data category"},
				Options: []forms.Option{{Value: "identity", Label: forms.Text{"th": "ข้อมูลระบุตัวตน"}}}},
			{Key: "subject_type_code", Type: forms.TypeSingle, Label: forms.Text{"th": "กลุ่มเจ้าของข้อมูล", "en": "Subject type"},
				Options: []forms.Option{{Value: "job_applicant", Label: forms.Text{"th": "ผู้สมัครงาน"}}}},
			{Key: "retention_months", Type: forms.TypeNumber, Label: forms.Text{"th": "ระยะเวลาเก็บ (เดือน)", "en": "Retention (months)"}},
			{Key: "retention_basis", Type: forms.TypeText, Label: forms.Text{"th": "เหตุผลการเก็บ", "en": "Retention basis"}},
			{Key: "trigger_event", Type: forms.TypeText, Label: forms.Text{"th": "เริ่มนับเมื่อ", "en": "Trigger event"}},
			{Key: "disposal_method", Type: forms.TypeSingle, Label: forms.Text{"th": "วิธีทำลาย", "en": "Disposal method"},
				Options: []forms.Option{{Value: "delete", Label: forms.Text{"th": "ลบ"}}}},
		}}}}}
	form, err := e.svc.Forms.CreateForm(ctx, "ropa_questionnaire_"+uuid.NewString()[:8], "Data mapping questionnaire", "questionnaire", d)
	if err != nil {
		t.Fatal(err)
	}
	form, err = e.svc.Forms.Publish(ctx, form.ID, form.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}
	return form.Versions[len(form.Versions)-1].ID
}

func seedOrgUnit(t *testing.T, ctx context.Context, e env) orgservice.OrgUnit {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR11", NameTh: "HR", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	return unit
}

// TestQuestionnaireLifecycle_ApprovedAnswersEnterRopaAutomatically is ROPA-11's own acceptance criterion
// directly: send -> answer -> approve, and the approved answers land as a real RoPA activity with the
// mapped purpose/data/retention rows, no further edits.
func TestQuestionnaireLifecycle_ApprovedAnswersEnterRopaAutomatically(t *testing.T) {
	e := questionnaireSetup(t, "ropa11")
	var unit orgservice.OrgUnit
	var respondent uuid.UUID
	var formVersionID uuid.UUID
	e.inQ(t, func(ctx context.Context) error {
		unit = seedOrgUnit(t, ctx, e)
		respondent = seedUser(t, ctx, e, "Dept Owner")
		formVersionID = publishedQuestionnaireForm(t, ctx, e)
		return nil
	})

	var q ropaservice.Questionnaire
	e.inQ(t, func(ctx context.Context) error {
		var err error
		q, err = e.svc.SendQuestionnaire(ctx, unit.ID, respondent, nil)
		if err != nil {
			t.Fatalf("SendQuestionnaire: %v", err)
		}
		if q.Status != "sent" {
			t.Errorf("expected status sent, got %q", q.Status)
		}
		return nil
	})

	answers := forms.Answers{
		"activity_name": "ประกาศรับสมัครงาน", "purpose_text": "สรรหาบุคลากร", "lawful_basis_code": "CONTRACT",
		"data_category_code": "identity", "subject_type_code": "job_applicant",
		"retention_months": float64(12), "retention_basis": "เก็บไว้พิจารณา", "trigger_event": "วันที่ปิดรับสมัคร", "disposal_method": "delete",
	}
	e.inQ(t, func(ctx context.Context) error {
		var err error
		q, err = e.svc.AnswerQuestionnaire(ctx, q.ID, formVersionID, answers)
		if err != nil {
			t.Fatalf("AnswerQuestionnaire: %v", err)
		}
		if q.Status != "answered" || q.FormSubmissionID == nil {
			t.Errorf("expected status answered with a submission id, got %+v", q)
		}
		return nil
	})

	e.inQ(t, func(ctx context.Context) error {
		var activity ropaservice.Activity
		var err error
		q, activity, err = e.svc.ApproveQuestionnaire(ctx, q.ID, unit.LegalEntityID, "HR11-Q-01", nil, q.RowVersion)
		if err != nil {
			t.Fatalf("ApproveQuestionnaire: %v", err)
		}
		if q.Status != "approved" || q.ActivityID == nil || *q.ActivityID != activity.ID {
			t.Errorf("expected status approved pointing at the new activity, got %+v", q)
		}
		if activity.Name != "ประกาศรับสมัครงาน" || activity.Code != "HR11-Q-01" {
			t.Errorf("activity core fields not mapped from the answers: %+v", activity)
		}
		purposes, err := e.svc.ListActivityPurposes(ctx, activity.ID)
		if err != nil {
			return err
		}
		if len(purposes) != 1 || purposes[0].LawfulBasisCode != "CONTRACT" {
			t.Errorf("expected the answered purpose to be mapped, got %+v", purposes)
		}
		data, err := e.svc.ListActivityData(ctx, activity.ID)
		if err != nil {
			return err
		}
		if len(data) != 1 {
			t.Errorf("expected the answered data row to be mapped, got %+v", data)
		}
		retention, err := e.svc.ListRetentionRules(ctx, activity.ID)
		if err != nil {
			return err
		}
		if len(retention) != 1 || retention[0].DisposalMethod != "delete" {
			t.Errorf("expected the answered retention rule to be mapped, got %+v", retention)
		}
		return nil
	})
}

// TestQuestionnaireLifecycle_RejectedRoundCanBeReAnswered proves a DPO's reject sends it back to the
// respondent, who can answer again (rejected -> answered), rather than being stuck.
func TestQuestionnaireLifecycle_RejectedRoundCanBeReAnswered(t *testing.T) {
	e := questionnaireSetup(t, "ropa11reject")
	var unit orgservice.OrgUnit
	var respondent uuid.UUID
	var formVersionID uuid.UUID
	e.inQ(t, func(ctx context.Context) error {
		unit = seedOrgUnit(t, ctx, e)
		respondent = seedUser(t, ctx, e, "Dept Owner")
		formVersionID = publishedQuestionnaireForm(t, ctx, e)
		return nil
	})

	var q ropaservice.Questionnaire
	e.inQ(t, func(ctx context.Context) error {
		var err error
		q, err = e.svc.SendQuestionnaire(ctx, unit.ID, respondent, nil)
		return err
	})
	e.inQ(t, func(ctx context.Context) error {
		var err error
		q, err = e.svc.AnswerQuestionnaire(ctx, q.ID, formVersionID, forms.Answers{"activity_name": "First draft"})
		return err
	})
	e.inQ(t, func(ctx context.Context) error {
		var err error
		q, err = e.svc.RejectQuestionnaire(ctx, q.ID, q.RowVersion)
		if err != nil {
			t.Fatalf("RejectQuestionnaire: %v", err)
		}
		if q.Status != "rejected" {
			t.Errorf("expected status rejected, got %q", q.Status)
		}
		return nil
	})
	e.inQ(t, func(ctx context.Context) error {
		var err error
		q, err = e.svc.AnswerQuestionnaire(ctx, q.ID, formVersionID, forms.Answers{"activity_name": "Fixed draft"})
		if err != nil {
			t.Fatalf("re-answer after rejection: %v", err)
		}
		if q.Status != "answered" {
			t.Errorf("expected status answered after re-answering a rejected round, got %q", q.Status)
		}
		return nil
	})
}

// TestApproveQuestionnaire_RefusesWrongStatusAndStaleVersion covers the guard rails: approving before
// the round is answered, and a stale row_version, are both refused distinctly.
func TestApproveQuestionnaire_RefusesWrongStatusAndStaleVersion(t *testing.T) {
	e := questionnaireSetup(t, "ropa11guard")
	var unit orgservice.OrgUnit
	var respondent uuid.UUID
	e.inQ(t, func(ctx context.Context) error {
		unit = seedOrgUnit(t, ctx, e)
		respondent = seedUser(t, ctx, e, "Dept Owner")
		return nil
	})

	var q ropaservice.Questionnaire
	e.inQ(t, func(ctx context.Context) error {
		var err error
		q, err = e.svc.SendQuestionnaire(ctx, unit.ID, respondent, nil)
		return err
	})

	e.inQ(t, func(ctx context.Context) error {
		if _, _, err := e.svc.ApproveQuestionnaire(ctx, q.ID, unit.LegalEntityID, "X", nil, q.RowVersion); err != ropaservice.ErrInvalidTransition {
			t.Errorf("approving a still-sent round: got %v, want ErrInvalidTransition", err)
		}
		return nil
	})

	formVersionID := e.inQformVersion(t)
	e.inQ(t, func(ctx context.Context) error {
		var err error
		q, err = e.svc.AnswerQuestionnaire(ctx, q.ID, formVersionID, forms.Answers{"activity_name": "A"})
		return err
	})

	e.inQ(t, func(ctx context.Context) error {
		if _, _, err := e.svc.ApproveQuestionnaire(ctx, q.ID, unit.LegalEntityID, "X", nil, q.RowVersion+1); err != ropaservice.ErrVersionMismatch {
			t.Errorf("stale version: got %v, want ErrVersionMismatch", err)
		}
		return nil
	})
}

// inQformVersion publishes a fresh questionnaire form inside its own transaction and returns its version
// id — a small helper so TestApproveQuestionnaire_RefusesWrongStatusAndStaleVersion can interleave it with
// assertions in separate transactions.
func (e env) inQformVersion(t *testing.T) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	e.inQ(t, func(ctx context.Context) error {
		id = publishedQuestionnaireForm(t, ctx, e)
		return nil
	})
	return id
}

// TestListQuestionnaires_TwoTenantIsolation proves RLS scopes the list to the caller's own tenant.
func TestListQuestionnaires_TwoTenantIsolation(t *testing.T) {
	a := questionnaireSetup(t, "ropa11isoa")
	b := questionnaireSetup(t, "ropa11isob")

	var unitA orgservice.OrgUnit
	var respondentA uuid.UUID
	a.inQ(t, func(ctx context.Context) error {
		unitA = seedOrgUnit(t, ctx, a)
		respondentA = seedUser(t, ctx, a, "A Owner")
		_, err := a.svc.SendQuestionnaire(ctx, unitA.ID, respondentA, nil)
		return err
	})

	b.inQ(t, func(ctx context.Context) error {
		list, err := b.svc.ListQuestionnaires(ctx, nil, "")
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B must not see tenant A's own questionnaire rounds, got %d", len(list))
		}
		return nil
	})
}
