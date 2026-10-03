package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/forms"
)

func score(v float64) *float64 { return &v }

// checklistForm publishes a two-item security-measures checklist: an access-control question the caller
// always answers "yes" (passes) and a backup question the tests answer either way (the failing one).
func checklistForm(t *testing.T, ctx context.Context, e env) uuid.UUID {
	t.Helper()
	d := forms.Draft{Languages: []string{"th", "en"}, Schema: forms.Schema{Sections: []forms.Section{{Key: "controls",
		Title: forms.Text{"th": "มาตรการ", "en": "Controls"}, Questions: []forms.Question{
			{Key: "access_control", Type: forms.TypeYesNo, Label: forms.Text{"th": "มีการควบคุมการเข้าถึงหรือไม่", "en": "Access control in place?"}, Required: true,
				Options: []forms.Option{{Value: "yes", Score: score(1)}, {Value: "no", Score: score(0)}}},
			{Key: "backup", Type: forms.TypeYesNo, Label: forms.Text{"th": "มีการสำรองข้อมูลหรือไม่", "en": "Backups in place?"}, Required: true,
				Options: []forms.Option{{Value: "yes", Score: score(1)}, {Value: "no", Score: score(0)}}},
		}}}},
		Scoring: &forms.Scoring{Bands: []forms.Band{{Key: "fail", Label: forms.Text{"th": "ไม่ผ่าน"}, Min: 0, Max: score(1)},
			{Key: "pass", Label: forms.Text{"th": "ผ่าน"}, Min: 2}}}}
	form, err := e.forms.CreateForm(ctx, "security_"+uuid.NewString()[:8], "ประเมินมาตรการความปลอดภัย", "security", d)
	if err != nil {
		t.Fatal(err)
	}
	form, err = e.forms.Publish(ctx, form.ID, form.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}
	return form.ID
}

// TestAssess_FailedItemOpensRemediationTask is DPO-09's acceptance criterion: an unmet checklist item
// automatically opens a dpo.tasks remediation task; a fully-passing run opens none.
func TestAssess_FailedItemOpensRemediationTask(t *testing.T) {
	e := setup(t, "dposecassess")
	var le orgservice.LegalEntity
	var formID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		formID = checklistForm(t, ctx, e)
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.Assess(ctx, le.ID, formID, forms.Answers{"access_control": "yes", "backup": "no"})
		if err != nil {
			return err
		}
		if a.Result != "fail" {
			t.Errorf("result: %q, want fail", a.Result)
		}
		if len(a.Tasks) != 1 {
			t.Fatalf("expected exactly one remediation task, got %+v", a.Tasks)
		}
		if a.Tasks[0].Status != "created" || a.Tasks[0].Priority != "high" {
			t.Errorf("task: %+v", a.Tasks[0])
		}
		got, err := e.svc.GetAssessment(ctx, a.ID)
		if err != nil {
			return err
		}
		if len(got.Tasks) != 1 || got.Tasks[0].TaskNo == "" {
			t.Errorf("GetAssessment tasks: %+v", got.Tasks)
		}
		return nil
	})

	// A fully-passing run opens no tasks.
	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.Assess(ctx, le.ID, formID, forms.Answers{"access_control": "yes", "backup": "yes"})
		if err != nil {
			return err
		}
		if a.Result != "pass" {
			t.Errorf("result: %q, want pass", a.Result)
		}
		if len(a.Tasks) != 0 {
			t.Errorf("expected no remediation tasks, got %+v", a.Tasks)
		}
		return nil
	})
}

func TestAssess_Validation(t *testing.T) {
	e := setup(t, "dposecvalid")
	var le orgservice.LegalEntity
	var formID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		formID = checklistForm(t, ctx, e)
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		bogus := uuid.New()
		if _, err := e.svc.Assess(ctx, bogus, formID, forms.Answers{"access_control": "yes", "backup": "yes"}); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("unknown legal entity: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.Assess(ctx, le.ID, uuid.New(), forms.Answers{"access_control": "yes", "backup": "yes"}); !errors.Is(err, dposervice.ErrBadForm) {
			t.Errorf("unknown form: %v, want ErrBadForm", err)
		}
		var ve *dposervice.ValidationError
		if _, err := e.svc.Assess(ctx, le.ID, formID, forms.Answers{"access_control": "yes"}); !errors.As(err, &ve) {
			t.Errorf("missing required answer: %v, want ValidationError", err)
		}
		return nil
	})
}

// TestAssess_RejectsNonSecurityForm makes sure a published form of a different type (e.g. "breach") can't be
// used — assessmentVersion checks the form's own type, not just that it's published.
func TestAssess_RejectsNonSecurityForm(t *testing.T) {
	e := setup(t, "dposecwrongtype")
	var le orgservice.LegalEntity
	var otherFormID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		var err error
		le, err = e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		d := forms.Draft{Languages: []string{"th"}, Schema: forms.Schema{Sections: []forms.Section{{Key: "s", Title: forms.Text{"th": "s"},
			Questions: []forms.Question{{Key: "q", Type: forms.TypeYesNo, Label: forms.Text{"th": "q"}, Required: true,
				Options: []forms.Option{{Value: "yes", Score: score(1)}, {Value: "no", Score: score(0)}}}}}}}}
		form, err := e.forms.CreateForm(ctx, "questionnaire_"+uuid.NewString()[:8], "x", "questionnaire", d)
		if err != nil {
			return err
		}
		form, err = e.forms.Publish(ctx, form.ID, form.LatestVersion)
		otherFormID = form.ID
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.Assess(ctx, le.ID, otherFormID, forms.Answers{"q": "yes"}); !errors.Is(err, dposervice.ErrBadForm) {
			t.Errorf("wrong form type: %v, want ErrBadForm", err)
		}
		return nil
	})
}

// TestAssessments_Isolation is CLAUDE.md rule 1's per-repository requirement.
func TestAssessments_Isolation(t *testing.T) {
	a := setup(t, "dposeciso_a")
	b := setup(t, "dposeciso_b")
	var created dposervice.Assessment
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "เอ", IsController: true}, 0)
		if err != nil {
			return err
		}
		formID := checklistForm(t, ctx, a)
		created, err = a.svc.Assess(ctx, le.ID, formID, forms.Answers{"access_control": "yes", "backup": "yes"})
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetAssessment(ctx, created.ID); !errors.Is(err, dposervice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's assessment: %v", err)
		}
		list, _, err := b.svc.ListAssessments(ctx, dposervice.AssessmentFilter{})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("tenant B's list should be empty, got %+v", list)
		}
		return nil
	})
}
