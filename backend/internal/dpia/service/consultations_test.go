package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// TestAddConsultation_ListedForReport is DPIA-11's acceptance criterion directly: a recorded consultation
// (who, when, the issue, the explanation) is on record and comes back in order when read for the report.
func TestAddConsultation_ListedForReport(t *testing.T) {
	e := setup(t, "dpiaconsult")
	var act ropaservice.Activity
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act = e.activity(t, ctx, "CON-01")
		a, err := e.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		assessmentID = a.ID
		return nil
	})

	day1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddConsultation(ctx, assessmentID, "คุณสมชาย", "data_subject", day2, "กังวลเรื่องการเก็บข้อมูลไบโอเมตริก", "จะจำกัดระยะเวลาเก็บและเข้ารหัสข้อมูล"); err != nil {
			return err
		}
		_, err := e.svc.AddConsultation(ctx, assessmentID, "ผู้ประมวลผล ABC", "processor", day1, "ขอบเขตการประมวลผลข้อมูลย่อย", "")
		return err
	})

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListConsultations(ctx, assessmentID)
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Fatalf("got %d consultations, want 2", len(list))
		}
		// oldest first
		if list[0].Stakeholder != "ผู้ประมวลผล ABC" || list[0].StakeholderType != "processor" || list[0].Response != "" {
			t.Errorf("first row = %+v", list[0])
		}
		if list[1].Stakeholder != "คุณสมชาย" || list[1].StakeholderType != "data_subject" || list[1].Response == "" {
			t.Errorf("second row = %+v", list[1])
		}
		if !list[0].ConsultedAt.Equal(day1) || !list[1].ConsultedAt.Equal(day2) {
			t.Errorf("dates = %v, %v", list[0].ConsultedAt, list[1].ConsultedAt)
		}
		return nil
	})

	// The DPIA-15 report surfaces the same log.
	e.in(t, func(ctx context.Context) error {
		html, _, err := e.svc.Report(ctx, assessmentID, "th", "html")
		if err != nil {
			return err
		}
		s := string(html)
		for _, want := range []string{"คุณสมชาย", "ผู้ประมวลผล ABC", "กังวลเรื่องการเก็บข้อมูลไบโอเมตริก", "จะจำกัดระยะเวลาเก็บและเข้ารหัสข้อมูล"} {
			if !contains(s, want) {
				t.Errorf("report missing %q", want)
			}
		}
		return nil
	})
}

// TestAddConsultation_Validation covers the field-level rules.
func TestAddConsultation_Validation(t *testing.T) {
	e := setup(t, "dpiaconsultval")
	var assessmentID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "CON-02")
		a, err := e.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		assessmentID = a.ID
		return nil
	})

	cases := []struct {
		name            string
		stakeholder     string
		stakeholderType string
		consultedAt     time.Time
		summary         string
	}{
		{"empty stakeholder", "", "expert", time.Now(), "summary"},
		{"bad stakeholder type", "someone", "vendor", time.Now(), "summary"},
		{"empty summary", "someone", "expert", time.Now(), ""},
		{"zero date", "someone", "expert", time.Time{}, "summary"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e.in(t, func(ctx context.Context) error {
				_, err := e.svc.AddConsultation(ctx, assessmentID, c.stakeholder, c.stakeholderType, c.consultedAt, c.summary, "")
				var ve *dpiaservice.ValidationError
				if err == nil {
					t.Fatal("expected a validation error")
				}
				if !asValidationError(err, &ve) {
					t.Fatalf("got %v, want *ValidationError", err)
				}
				return nil
			})
		})
	}

	// Unknown assessment.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.AddConsultation(ctx, uuid.Must(uuid.NewV7()), "x", "expert", time.Now(), "y", ""); err == nil {
			t.Fatal("expected an error for an unknown assessment")
		}
		return nil
	})
}

// TestRemoveConsultation_DeletesTheRow is the ROPA-03-style "no update; delete+recreate" convention —
// removing a consultation drops it from the list and reading back an unknown id is ErrNotFound.
func TestRemoveConsultation_DeletesTheRow(t *testing.T) {
	e := setup(t, "dpiaconsultrm")
	var assessmentID uuid.UUID
	var consultationID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		act := e.activity(t, ctx, "CON-03")
		a, err := e.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		assessmentID = a.ID
		c, err := e.svc.AddConsultation(ctx, assessmentID, "x", "internal", time.Now(), "y", "")
		if err != nil {
			return err
		}
		consultationID = c.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.RemoveConsultation(ctx, assessmentID, consultationID); err != nil {
			return err
		}
		list, err := e.svc.ListConsultations(ctx, assessmentID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Fatalf("got %d consultations after remove, want 0", len(list))
		}
		if err := e.svc.RemoveConsultation(ctx, assessmentID, consultationID); err == nil {
			t.Fatal("expected ErrNotFound removing an already-removed consultation")
		}
		return nil
	})
}

// TestConsultations_TwoTenantIsolation proves RLS: a second tenant's own assessment id is simply not found.
func TestConsultations_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpiaconsultisoa")
	b := setup(t, "dpiaconsultisob")
	var assessmentID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		act := a.activity(t, ctx, "CON-04")
		as, err := a.svc.Screen(ctx, act.ID, allAnswers("no"))
		if err != nil {
			return err
		}
		assessmentID = as.ID
		_, err = a.svc.AddConsultation(ctx, assessmentID, "x", "internal", time.Now(), "y", "")
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.ListConsultations(ctx, assessmentID); err == nil {
			t.Fatal("tenant B must not read tenant A's assessment consultations")
		}
		return nil
	})
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func asValidationError(err error, target **dpiaservice.ValidationError) bool {
	ve, ok := err.(*dpiaservice.ValidationError)
	if ok {
		*target = ve
	}
	return ok
}
