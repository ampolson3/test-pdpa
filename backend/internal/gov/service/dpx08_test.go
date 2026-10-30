package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	breachservice "pdpa-platform/internal/breach/service"
	govservice "pdpa-platform/internal/gov/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// fakeBreach is DPX-08's own rule-9 Breach interface stand-in — a regulator letter's optional
// incident_id must be visible, the same FK-visibility pattern every other cross-module check in this
// codebase uses, without actually depending on the breach module's own real fixture (S3/clamd).
type fakeBreach struct {
	known map[uuid.UUID]bool
}

func (f *fakeBreach) Get(ctx context.Context, id uuid.UUID) (breachservice.Incident, error) {
	if !f.known[id] {
		return breachservice.Incident{}, errors.New("not found")
	}
	return breachservice.Incident{ID: id}, nil
}

// dpx08Setup wires govEnv's own svc (govSetup, retention_test.go) with a fake Breach and a real
// notify.Service so this feature's own due-date reminder can be scheduled and fired for real.
func dpx08Setup(t *testing.T, suffix string) (govEnv, *fakeBreach) {
	t.Helper()
	e := govSetup(t, suffix)
	fb := &fakeBreach{known: map[uuid.UUID]bool{}}
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Breach = fb
	e.svc.Notify = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.QuietHours{}}
	e.svc.River = client
	return e, fb
}

var dpx08Perms = append(append([]string{}, govPerms...), "dpx.regulator.read", "dpx.regulator.create", "dpx.regulator.update", "dpx.regulator.delete")

func (e govEnv) inDpx08(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: dpx08Perms}))
	}); err != nil {
		t.Fatal(err)
	}
}

func countRegulatorLetterJobs(t *testing.T, e govEnv, letterID uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.app.QueryRow(context.Background(),
		`SELECT count(*)::int FROM river_job WHERE kind = 'gov.regulator_letter_due' AND args->>'letter_id' = $1`, letterID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func countRegulatorLetterNotifications(t *testing.T, e govEnv, letterID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(DISTINCT recipient_user_id)::int FROM platform.notifications WHERE entity_id = $1 AND channel = 'in_app'`, letterID).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestSaveRegulatorLetter_SchedulesDueReminder is the acceptance criterion's write half: a real due_at
// schedules exactly one checkpoint.
func TestSaveRegulatorLetter_SchedulesDueReminder(t *testing.T) {
	e, _ := dpx08Setup(t, "dpx08sched")
	due := time.Now().UTC().AddDate(0, 0, 14)
	var letterID uuid.UUID
	e.inDpx08(t, func(ctx context.Context) error {
		l, err := e.svc.SaveRegulatorLetter(ctx, govservice.RegulatorLetterInput{Direction: "inbound", Subject: "คำสั่งขอข้อมูลเพิ่มเติม", DueAt: &due})
		if err != nil {
			return err
		}
		letterID = l.ID
		return nil
	})
	if n := countRegulatorLetterJobs(t, e, letterID); n != 1 {
		t.Fatalf("expected 1 scheduled reminder, got %d", n)
	}
}

// TestFireRegulatorLetterDueReminder_NotifiesWhileOpen is the acceptance criterion's read/fire half: the
// reminder notifies DPO/LEGAL while the letter is still open, and is a harmless no-op once it is answered.
func TestFireRegulatorLetterDueReminder_NotifiesWhileOpen(t *testing.T) {
	e, _ := dpx08Setup(t, "dpx08fire")
	due := time.Now().UTC().AddDate(0, 0, 3)
	var letter govservice.RegulatorLetter
	e.inDpx08(t, func(ctx context.Context) error {
		var err error
		letter, err = e.svc.SaveRegulatorLetter(ctx, govservice.RegulatorLetterInput{Direction: "inbound", Subject: "คำสั่งตรวจสอบ", DueAt: &due})
		return err
	})

	e.inDpx08(t, func(ctx context.Context) error {
		return e.svc.FireRegulatorLetterDueReminder(ctx, letter.ID, due)
	})
	if n := countRegulatorLetterNotifications(t, e, letter.ID); n != 1 {
		t.Fatalf("expected 1 recipient notified while open, got %d", n)
	}

	// Responding closes the letter — a later fire against the same schedule is now a harmless no-op.
	e.inDpx08(t, func(ctx context.Context) error {
		_, err := e.svc.SetRegulatorLetterStatus(ctx, letter.ID, letter.RowVersion, "responded")
		return err
	})
	e.inDpx08(t, func(ctx context.Context) error {
		return e.svc.FireRegulatorLetterDueReminder(ctx, letter.ID, due)
	})
	if n := countRegulatorLetterNotifications(t, e, letter.ID); n != 1 {
		t.Fatalf("expected still 1 notification after responding (the fire was a no-op), got %d", n)
	}
}

// TestSaveRegulatorLetter_IncidentFKVisibility is the FK-visibility half (rule 1): an unknown incident_id
// is refused, a known one is accepted.
func TestSaveRegulatorLetter_IncidentFKVisibility(t *testing.T) {
	e, fb := dpx08Setup(t, "dpx08fk")
	unknown := uuid.Must(uuid.NewV7())
	e.inDpx08(t, func(ctx context.Context) error {
		_, err := e.svc.SaveRegulatorLetter(ctx, govservice.RegulatorLetterInput{Direction: "outbound", Subject: "x", IncidentID: &unknown})
		if !errors.Is(err, govservice.ErrInvalid) {
			t.Fatalf("expected ErrInvalid for an unknown incident_id, got %v", err)
		}
		return nil
	})

	known := uuid.Must(uuid.NewV7())
	fb.known[known] = true
	e.inDpx08(t, func(ctx context.Context) error {
		l, err := e.svc.SaveRegulatorLetter(ctx, govservice.RegulatorLetterInput{Direction: "outbound", Subject: "y", IncidentID: &known})
		if err != nil {
			t.Fatalf("expected a known incident_id to be accepted, got %v", err)
		}
		if l.IncidentID == nil || *l.IncidentID != known {
			t.Fatalf("expected incident_id to round-trip, got %v", l.IncidentID)
		}
		return nil
	})
}

// TestListRegulatorLetters_TwoTenantIsolation is rule 1's own two-tenant isolation proof.
func TestListRegulatorLetters_TwoTenantIsolation(t *testing.T) {
	eA, _ := dpx08Setup(t, "dpx08isoA")
	eB, _ := dpx08Setup(t, "dpx08isoB")
	eA.inDpx08(t, func(ctx context.Context) error {
		_, err := eA.svc.SaveRegulatorLetter(ctx, govservice.RegulatorLetterInput{Direction: "inbound", Subject: "tenant A letter"})
		return err
	})
	eB.inDpx08(t, func(ctx context.Context) error {
		list, err := eB.svc.ListRegulatorLetters(ctx, govservice.RegulatorLetterFilter{})
		if err != nil {
			return err
		}
		for _, l := range list {
			if l.Subject == "tenant A letter" {
				t.Fatal("tenant B must not see tenant A's regulator letter")
			}
		}
		return nil
	})
}
