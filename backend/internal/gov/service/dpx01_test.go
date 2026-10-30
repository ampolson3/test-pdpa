package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	govservice "pdpa-platform/internal/gov/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/wiring"
)

func fixedFuture() time.Time { return time.Now().UTC().AddDate(0, 0, 30) }

func dpx01score(v float64) *float64 { return &v }

// dpx01QuizForm publishes a one-question yes_no quiz worth 1 point, passed only by answering "yes" —
// the exact fixture shape DPO-09's own checklistForm already established, now for the "quiz" form type.
func dpx01QuizForm(t *testing.T, ctx context.Context, e govEnv) uuid.UUID {
	t.Helper()
	d := forms.Draft{Languages: []string{"th", "en"}, Schema: forms.Schema{Sections: []forms.Section{{Key: "q",
		Title: forms.Text{"th": "คำถาม"}, Questions: []forms.Question{
			{Key: "q1", Type: forms.TypeYesNo, Label: forms.Text{"th": "ข้อมูลส่วนบุคคลคืออะไร?", "en": "What is personal data?"}, Required: true,
				Options: []forms.Option{{Value: "yes", Score: dpx01score(1)}, {Value: "no", Score: dpx01score(0)}}},
		}}}}}
	form, err := e.svc.Forms.(*forms.Service).CreateForm(ctx, "quiz_"+uuid.NewString()[:8], "แบบทดสอบพื้นฐาน PDPA", "quiz", d)
	if err != nil {
		t.Fatal(err)
	}
	form, err = e.svc.Forms.(*forms.Service).Publish(ctx, form.ID, form.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}
	return form.ID
}

var dpx01Perms = append(append([]string{}, govPerms...), "dpx.training.read", "dpx.training.create",
	"dpx.training.update", "dpx.training.delete", "dpx.training.publish", "dpx.training.execute",
	"dpx.training.read", "assessment.template.read")

func dpx01Setup(t *testing.T, suffix string) govEnv {
	t.Helper()
	e := govSetup(t, suffix)
	e.svc.Org = e.org
	e.svc.Forms = wiring.Forms(nil, audit.New())
	return e
}

func (e govEnv) dpx01In(t *testing.T, userID uuid.UUID, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), userID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: userID.String(), Permissions: dpx01Perms}))
	}); err != nil {
		t.Fatal(err)
	}
}

func publishedCourse(t *testing.T, e govEnv, title string, quizFormID *uuid.UUID) govservice.Course {
	t.Helper()
	var c govservice.Course
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		var err error
		c, err = e.svc.CreateCourse(ctx, govservice.CourseInput{Title: title, ContentType: "video", QuizFormID: quizFormID})
		if err != nil {
			return err
		}
		c, err = e.svc.SetCourseStatus(ctx, c.ID, c.RowVersion, "published")
		return err
	})
	return c
}

// TestTraining_EmployeeStudiesAndTakesQuiz is the acceptance criterion directly: an employee starts a
// published course, submits the quiz, and the result is scored against the course's own passing_score.
func TestTraining_EmployeeStudiesAndTakesQuiz(t *testing.T) {
	e := dpx01Setup(t, "dpx01quiz")
	var formID uuid.UUID
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		formID = dpx01QuizForm(t, ctx, e)
		return nil
	})
	course := publishedCourse(t, e, "PDPA 101", &formID)

	var attempt govservice.TrainingAttempt
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		var err error
		attempt, err = e.svc.StartAttempt(ctx, course.ID, nil)
		return err
	})
	if attempt.CompletedAt != nil {
		t.Fatalf("a freshly started attempt should not be completed: %+v", attempt)
	}

	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		got, err := e.svc.SubmitAttempt(ctx, attempt.ID, attempt.RowVersion, forms.Answers{"q1": "yes"})
		if err != nil {
			return err
		}
		if got.CompletedAt == nil {
			t.Fatal("expected the attempt to be completed")
		}
		if got.Passed == nil || !*got.Passed {
			t.Errorf("expected passed=true, got %+v", got.Passed)
		}
		if got.Score == nil || *got.Score != 100 {
			t.Errorf("expected score 100, got %+v", got.Score)
		}
		return nil
	})

	// The acceptance criterion's other half: the report shows this individual's result.
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		report, err := e.svc.TrainingReport(ctx, course.ID)
		if err != nil {
			return err
		}
		if len(report) != 1 || report[0].UserID != e.tenant.UserID || report[0].Passed == nil || !*report[0].Passed {
			t.Errorf("training report: %+v", report)
		}
		if report[0].UserName == "" {
			t.Error("expected the attempting user's own name to be resolved")
		}
		return nil
	})
}

// TestTraining_QuizLessCourseCompletesWithoutScore proves a course with no quiz can still be completed —
// studying it is enough.
func TestTraining_QuizLessCourseCompletesWithoutScore(t *testing.T) {
	e := dpx01Setup(t, "dpx01noquiz")
	course := publishedCourse(t, e, "Policy overview", nil)
	var attempt govservice.TrainingAttempt
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		var err error
		attempt, err = e.svc.StartAttempt(ctx, course.ID, nil)
		return err
	})
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		got, err := e.svc.SubmitAttempt(ctx, attempt.ID, attempt.RowVersion, nil)
		if err != nil {
			return err
		}
		if got.Score != nil {
			t.Errorf("expected no score for a quiz-less course, got %+v", got.Score)
		}
		if got.Passed == nil || !*got.Passed {
			t.Errorf("expected a quiz-less course to pass on completion, got %+v", got.Passed)
		}
		return nil
	})
}

// TestTraining_AssignToAllReachesEveryEmployee is the assignment half: a "target_type=all" assignment
// shows up in MyAssignments for any employee, with no FK target of its own.
func TestTraining_AssignToAllReachesEveryEmployee(t *testing.T) {
	e := dpx01Setup(t, "dpx01assignall")
	course := publishedCourse(t, e, "Annual refresher", nil)
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		_, err := e.svc.AssignTraining(ctx, govservice.TrainingAssignmentInput{CourseID: course.ID, TargetType: "all", DueAt: fixedFuture()})
		return err
	})
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		list, err := e.svc.MyAssignments(ctx)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].CourseID != course.ID || list[0].TargetID != nil {
			t.Errorf("my assignments: %+v", list)
		}
		return nil
	})
}

// TestTraining_AssignToOrgUnit_FKVisibility proves the org-unit target is checked visible (rule 1) and
// that an employee in that unit — and only that unit — sees it in their own MyAssignments.
func TestTraining_AssignToOrgUnit_FKVisibility(t *testing.T) {
	e := dpx01Setup(t, "dpx01assignunit")
	course := publishedCourse(t, e, "HR onboarding", nil)

	unknown := uuid.Must(uuid.NewV7())
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		_, err := e.svc.AssignTraining(ctx, govservice.TrainingAssignmentInput{CourseID: course.ID, TargetType: "org_unit", TargetID: &unknown, DueAt: fixedFuture()})
		if !errors.Is(err, govservice.ErrInvalid) {
			t.Fatalf("expected ErrInvalid for an unknown org unit, got %v", err)
		}
		return nil
	})

	var unitID uuid.UUID
	var memberID uuid.UUID
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		unitID = unit.ID
		_, err = e.svc.AssignTraining(ctx, govservice.TrainingAssignmentInput{CourseID: course.ID, TargetType: "org_unit", TargetID: &unitID, DueAt: fixedFuture()})
		if err != nil {
			return err
		}
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status, primary_org_unit_id) VALUES (current_setting('app.tenant_id')::uuid, $1, 'HR Member', 'active', $2) RETURNING id`,
			uuid.NewString()[:8]+"@dpx01.example", unitID).Scan(&memberID)
	})

	e.dpx01In(t, memberID, func(ctx context.Context) error {
		list, err := e.svc.MyAssignments(ctx)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].CourseID != course.ID {
			t.Errorf("org-unit member's own assignments: %+v", list)
		}
		return nil
	})
	// The tenant's own admin (not a member of that org unit) sees nothing for this assignment.
	e.dpx01In(t, e.tenant.UserID, func(ctx context.Context) error {
		list, err := e.svc.MyAssignments(ctx)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("expected no assignments for a user outside the org unit, got %+v", list)
		}
		return nil
	})
}

// TestTraining_TwoTenantIsolation is rule 1's own two-tenant isolation proof.
func TestTraining_TwoTenantIsolation(t *testing.T) {
	eA := dpx01Setup(t, "dpx01isoA")
	eB := dpx01Setup(t, "dpx01isoB")
	courseA := publishedCourse(t, eA, "Tenant A course", nil)
	eB.dpx01In(t, eB.tenant.UserID, func(ctx context.Context) error {
		if _, err := eB.svc.GetCourse(ctx, courseA.ID); !errors.Is(err, govservice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's course: %v", err)
		}
		list, err := eB.svc.ListCourses(ctx, nil)
		if err != nil {
			return err
		}
		for _, c := range list {
			if c.ID == courseA.ID {
				t.Error("tenant B's course list leaked tenant A's course")
			}
		}
		return nil
	})
}
