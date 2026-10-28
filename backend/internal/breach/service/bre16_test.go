package service_test

import (
	"context"
	"testing"
	"time"

	"pdpa-platform/internal/breach/breachtest"
	breach "pdpa-platform/internal/breach/service"
)

// TestDrillIncident_NoRealTimersOrAlertsAndExcludedFromTheRegister is BRE-16's own acceptance criterion
// directly: a drill incident never schedules the real 72-hour timers, never sends a real notification,
// and is excluded from List by default ("เหตุซ้อมไม่ปนกับเหตุจริงในรายงาน") until explicitly asked for.
func TestDrillIncident_NoRealTimersOrAlertsAndExcludedFromTheRegister(t *testing.T) {
	f := breachtest.Setup(t)
	aware := f.Clock.Add(-2 * time.Hour)

	var drill, real breach.Incident
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		n := int32(10)
		drill, err = f.Svc.Create(ctx, breach.Input{LegalEntityID: f.EntityA, ReportedVia: "email", Title: "ซ้อมรับมือเหตุละเมิดประจำปี",
			Description: "แบบฝึกซ้อมภายใน ไม่ใช่เหตุจริง", BreachTypes: []string{"confidentiality"}, AwareAt: aware, AffectedSubjects: &n, IsDrill: true})
		if err != nil {
			return err
		}
		real, err = f.Svc.Create(ctx, breach.Input{LegalEntityID: f.EntityA, ReportedVia: "email", Title: "ไฟล์ลูกค้ารั่วจริง",
			Description: "เหตุจริง", BreachTypes: []string{"confidentiality"}, AwareAt: aware, AffectedSubjects: &n})
		return err
	})

	if !drill.IsDrill {
		t.Fatalf("expected the created incident to carry is_drill, got %+v", drill)
	}

	// No real SLA timers for the drill.
	if n := count(t, f, `SELECT count(*)::int FROM river_job WHERE kind = 'breach.sla_timer' AND args->>'incident_id' = $1`, drill.ID); n != 0 {
		t.Errorf("drill scheduled %d real SLA timers, want 0", n)
	}
	if n := count(t, f, `SELECT count(*)::int FROM river_job WHERE kind = 'breach.sla_timer' AND args->>'incident_id' = $1`, real.ID); n != 4 {
		t.Errorf("real incident scheduled %d SLA timers, want 4", n)
	}

	// No real notification for the drill.
	if n := count(t, f, `SELECT count(*)::int FROM platform.notifications WHERE entity_id = $1`, drill.ID); n != 0 {
		t.Errorf("drill sent %d notifications, want 0", n)
	}
	if n := count(t, f, `SELECT count(DISTINCT recipient_user_id)::int FROM platform.notifications WHERE entity_id = $1 AND channel = 'in_app'`, real.ID); n != 3 {
		t.Errorf("real incident alerted %d people, want 3", n)
	}

	// Excluded from the register by default; included when explicitly asked.
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		list, _, err := f.Svc.List(ctx, breach.Filter{Limit: 100})
		if err != nil {
			return err
		}
		for _, i := range list {
			if i.ID == drill.ID {
				t.Errorf("drill incident %s appeared in the default (non-drill) register", drill.ID)
			}
		}
		withDrills, _, err := f.Svc.List(ctx, breach.Filter{Limit: 100, IncludeDrills: true})
		if err != nil {
			return err
		}
		found := false
		for _, i := range withDrills {
			if i.ID == drill.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("drill incident %s missing from the register when IncludeDrills=true", drill.ID)
		}
		return nil
	})
}
