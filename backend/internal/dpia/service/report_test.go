package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestReport_ShowsScoreMeasuresAndApprovers is DPIA-15's own acceptance criterion
// ("รายงานแสดงคะแนน มาตรการ และผู้อนุมัติครบ"): the rendered report carries the screening score, every
// security measure (ROPA-09) linked to the activity, and every DPO opinion/approver recorded on the round.
func TestReport_ShowsScoreMeasuresAndApprovers(t *testing.T) {
	e := setup(t, "dpiareport")
	var assessmentID uuid.UUID
	var controlName string

	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "RPT-01")
		answers := allAnswers("no")
		answers["sensitive_data"] = "yes"
		answers["large_scale"] = "yes"
		a, err := e.svc.Screen(ctx, act.ID, answers)
		if err != nil {
			return err
		}
		assessmentID = a.ID

		controls, err := e.ropa.ListControls(ctx)
		if err != nil {
			return err
		}
		if len(controls) == 0 {
			t.Fatal("expected the seeded ม.37(1) control catalog (ROPA-09) to be non-empty")
		}
		controlName = controls[0].Name
		if _, err := e.ropa.AddActivityControl(ctx, ropaservice.ActivityControl{ActivityID: act.ID, ControlID: controls[0].ID, Description: "ปรับใช้กับกิจกรรมนี้"}); err != nil {
			return err
		}

		if _, err := e.svc.RecordOpinion(ctx, a.ID, "เห็นควรดำเนินการ", "proceed"); err != nil {
			return err
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		b, name, err := e.svc.Report(ctx, assessmentID, "th", "html")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(name, ".html") {
			t.Errorf("name: %q", name)
		}
		html := string(b)
		if !strings.Contains(html, "RPT-01") {
			t.Errorf("report missing activity code: %s", html)
		}
		if !strings.Contains(html, controlName) {
			t.Errorf("report missing linked security measure %q: %s", controlName, html)
		}
		if !strings.Contains(html, "เห็นควรดำเนินการ") || !strings.Contains(html, "proceed") {
			t.Errorf("report missing DPO opinion/recommendation: %s", html)
		}
		if !strings.Contains(html, "required") {
			t.Errorf("report missing screening result: %s", html)
		}
		return nil
	})

	// English report carries the same data under its own labels.
	e.in(t, func(ctx context.Context) error {
		b, _, err := e.svc.Report(ctx, assessmentID, "en", "html")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "Security measures") {
			t.Errorf("english report missing its own heading: %s", string(b))
		}
		return nil
	})

	// docx must at least render without error (byte-exactness isn't this feature's acceptance criterion).
	e.in(t, func(ctx context.Context) error {
		b, name, err := e.svc.Report(ctx, assessmentID, "th", "docx")
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 || !strings.HasSuffix(name, ".docx") {
			t.Errorf("docx: %d bytes, name %q", len(b), name)
		}
		return nil
	})
}

// TestReport_NoMeasuresOrApprovals: an activity with neither still produces a report, with the "none yet"
// placeholders rather than an error — the report must always render, complete or not.
func TestReport_NoMeasuresOrApprovals(t *testing.T) {
	e := setup(t, "dpiareportempty")
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "RPT-02")
		a, err := e.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		assessmentID = a.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		b, _, err := e.svc.Report(ctx, assessmentID, "th", "html")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "ยังไม่มีมาตรการความปลอดภัย") || !strings.Contains(string(b), "ยังไม่มีความเห็น") {
			t.Errorf("expected both placeholders: %s", string(b))
		}
		return nil
	})
}

// TestReport_TwoTenantIsolation proves tenant B can't render tenant A's report.
func TestReport_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpiareportisoA")
	b := setup(t, "dpiareportisoB")
	var assessmentID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		act := a.activity(t, ctx, "RPT-ISO")
		got, err := a.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		assessmentID = got.ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, _, err := b.svc.Report(ctx, assessmentID, "th", "html"); err == nil {
			t.Error("expected tenant B's report render to fail for tenant A's assessment")
		}
		return nil
	})
}
