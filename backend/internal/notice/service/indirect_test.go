package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	noticeservice "pdpa-platform/internal/notice/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
)

func count(t *testing.T, app *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := app.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// seedFile inserts a minimal platform.files row so a test can use its id as evidence_file_id (a real FK) —
// svc.Files stays nil in these unit tests (no S3/clamd here), so RecordNotice skips the Get/AttachSystem
// checks and only the database constraint needs satisfying.
func seedFile(t *testing.T, ctx context.Context) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `INSERT INTO platform.files (id, tenant_id, bucket, object_key, file_name, mime_type, size_bytes, sha256)
		VALUES ($1, current_setting('app.tenant_id')::uuid, 'b', $2, 'evidence.pdf', 'application/pdf', 1, repeat('0', 64))`, id, id.String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestIndirectCollectionCheckpoints_ReminderThenOverdue(t *testing.T) {
	obtained := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	cps := noticeservice.Checkpoints(obtained)
	if len(cps) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(cps))
	}
	want := []struct {
		days    int
		overdue bool
	}{{20, false}, {25, false}, {30, true}}
	for i, w := range want {
		if cps[i].Days != w.days || cps[i].Overdue != w.overdue {
			t.Errorf("checkpoint %d: got %+v, want days=%d overdue=%v", i, cps[i], w.days, w.overdue)
		}
		if !cps[i].At.Equal(obtained.AddDate(0, 0, w.days)) {
			t.Errorf("checkpoint %d: wrong time %v", i, cps[i].At)
		}
	}
}

func TestIndirectCollectionToSchedule_LateEntryStillAlerts(t *testing.T) {
	obtained := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	// Recorded 22 days after the fact: day-20 checkpoint has already passed.
	now := obtained.AddDate(0, 0, 22)
	sched := noticeservice.ToSchedule(obtained, now)
	if len(sched) != 3 {
		t.Fatalf("expected 3 scheduled checkpoints (1 immediate + 2 future), got %d: %+v", len(sched), sched)
	}
	if sched[0].Days != 20 || !sched[0].At.Equal(now) {
		t.Errorf("the passed checkpoint should fire immediately: %+v", sched[0])
	}
	if sched[1].Days != 25 || sched[2].Days != 30 {
		t.Errorf("expected day 25 then day 30 next, got %+v", sched[1:])
	}
}

// TestRegisterCollection_ValidatesAndSchedules is the acceptance criterion's setup half: registering an
// indirect-collection event computes its 30-day deadline and schedules the reminder/overdue checkpoints
// (notice.indirect_due).
func TestRegisterCollection_ValidatesAndSchedules(t *testing.T) {
	e := setup(t, "noticeindirect")
	var le uuid.UUID
	var party orgservice.ExternalParty
	e.in(t, func(ctx context.Context) error {
		l, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		le = l.ID
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "นายหน้าจัดหางาน", CountryCode: "TH"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		bogus := uuid.New()
		if _, err := e.svc.RegisterCollection(ctx, noticeservice.IndirectCollection{SourcePartyID: bogus, ObtainedAt: time.Now().UTC().AddDate(0, 0, -1)}); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("unknown source_party_id: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.RegisterCollection(ctx, noticeservice.IndirectCollection{SourcePartyID: party.ID, ObtainedAt: time.Now().UTC().AddDate(0, 0, 1)}); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("future obtained_at: %v, want ErrInvalid", err)
		}
		return nil
	})

	obtained := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	var ic noticeservice.IndirectCollection
	e.in(t, func(ctx context.Context) error {
		var err error
		ic, err = e.svc.RegisterCollection(ctx, noticeservice.IndirectCollection{SourcePartyID: party.ID, ObtainedAt: obtained})
		if err != nil {
			return err
		}
		if ic.Status != "pending" {
			t.Errorf("expected pending, got %q", ic.Status)
		}
		wantDue := obtained.AddDate(0, 0, 30)
		if !ic.NotifyDueAt.Equal(wantDue) {
			t.Errorf("notify_due_at = %v, want %v", ic.NotifyDueAt, wantDue)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		n := count(t, e.app, `SELECT count(*)::int FROM river_job WHERE kind = 'notice.indirect_due' AND args->>'collection_id' = $1`, ic.ID.String())
		if n != 3 {
			t.Errorf("expected 3 scheduled checkpoints, got %d", n)
		}
		return nil
	})
	_ = le
}

func TestRecordNotice_RequiresMethodAndEvidence(t *testing.T) {
	e := setup(t, "noticeindirectrecord")
	var party orgservice.ExternalParty
	e.in(t, func(ctx context.Context) error {
		var err error
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "แหล่งข้อมูล", CountryCode: "TH"}, 0)
		return err
	})
	var ic noticeservice.IndirectCollection
	e.in(t, func(ctx context.Context) error {
		var err error
		ic, err = e.svc.RegisterCollection(ctx, noticeservice.IndirectCollection{SourcePartyID: party.ID, ObtainedAt: time.Now().UTC().AddDate(0, 0, -2)})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordNotice(ctx, ic.ID, ic.RowVersion, "bogus", uuid.New()); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("bad method: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.RecordNotice(ctx, ic.ID, ic.RowVersion, "email", uuid.Nil); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("missing evidence: %v, want ErrInvalid", err)
		}
		return nil
	})

	var notified noticeservice.IndirectCollection
	e.in(t, func(ctx context.Context) error {
		evidence := seedFile(t, ctx)
		var err error
		notified, err = e.svc.RecordNotice(ctx, ic.ID, ic.RowVersion, "email", evidence)
		if err != nil {
			return err
		}
		if notified.Status != "notified" || notified.Method != "email" || notified.NotifiedAt == nil {
			t.Errorf("expected notified with method/notified_at set: %+v", notified)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordNotice(ctx, ic.ID, notified.RowVersion, "email", uuid.New()); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("recording notice twice: %v, want ErrInvalid", err)
		}
		if _, err := e.svc.RecordNotice(ctx, ic.ID, 999, "email", uuid.New()); !errors.Is(err, noticeservice.ErrInvalid) && !errors.Is(err, noticeservice.ErrVersionMismatch) {
			t.Errorf("stale version after already-notified: %v", err)
		}
		return nil
	})
}

// TestFireDue_OverdueThenClosable is the acceptance criterion directly: once 30 days pass without a
// recorded notice, the record is marked overdue; it can still be closed afterwards with evidence.
func TestFireDue_OverdueThenClosable(t *testing.T) {
	e := setup(t, "noticeindirectoverdue")
	var party orgservice.ExternalParty
	e.in(t, func(ctx context.Context) error {
		var err error
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "แหล่งข้อมูล", CountryCode: "TH"}, 0)
		return err
	})
	obtained := time.Now().UTC().AddDate(0, 0, -31).Truncate(24 * time.Hour)
	var ic noticeservice.IndirectCollection
	e.in(t, func(ctx context.Context) error {
		var err error
		ic, err = e.svc.RegisterCollection(ctx, noticeservice.IndirectCollection{SourcePartyID: party.ID, ObtainedAt: obtained})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		if err := e.svc.FireDue(ctx, ic.ID, 20, obtained); err != nil { // reminder checkpoint: no-op besides an alert
			return err
		}
		if err := e.svc.FireDue(ctx, ic.ID, 30, obtained); err != nil {
			return err
		}
		got, err := e.svc.GetCollection(ctx, ic.ID)
		if err != nil {
			return err
		}
		if got.Status != "overdue" {
			t.Errorf("expected overdue after the 30-day checkpoint, got %q", got.Status)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetCollection(ctx, ic.ID)
		if err != nil {
			return err
		}
		notified, err := e.svc.RecordNotice(ctx, ic.ID, got.RowVersion, "letter", seedFile(t, ctx))
		if err != nil {
			return err
		}
		if notified.Status != "notified" {
			t.Errorf("an overdue record should still be closable with evidence: %+v", notified)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		// A stale tick (e.g. after the record closed) is harmless.
		if err := e.svc.FireDue(ctx, ic.ID, 30, obtained); err != nil {
			t.Errorf("stale tick after closing: %v", err)
		}
		return nil
	})
}

func TestIndirectCollections_Isolation(t *testing.T) {
	a := setup(t, "noticeindirecta")
	b := setup(t, "noticeindirectb")
	var party orgservice.ExternalParty
	var ic noticeservice.IndirectCollection
	a.in(t, func(ctx context.Context) error {
		var err error
		party, err = a.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "controller", NameTh: "เอ", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		ic, err = a.svc.RegisterCollection(ctx, noticeservice.IndirectCollection{SourcePartyID: party.ID, ObtainedAt: time.Now().UTC().AddDate(0, 0, -1)})
		return err
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.GetCollection(ctx, ic.ID); !errors.Is(err, noticeservice.ErrNotFound) {
			t.Errorf("tenant B reading tenant A's indirect collection: %v, want ErrNotFound", err)
		}
		list, _, err := b.svc.ListCollections(ctx, noticeservice.IndirectCollectionFilter{})
		if err != nil {
			return err
		}
		for _, c := range list {
			if c.ID == ic.ID {
				t.Errorf("tenant B should not see tenant A's indirect collection in the list")
			}
		}
		return nil
	})
}
