package service

import (
	"testing"
	"time"
)

var bkk = time.FixedZone("ICT", 7*3600)

func TestPDPCDue_Is72HoursFromAwareness(t *testing.T) {
	aware := time.Date(2026, 4, 12, 23, 30, 0, 0, bkk) // a Songkran weekend: calendar hours, no business days
	if got, want := PDPCDue(aware), time.Date(2026, 4, 15, 16, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("due %v, want %v", got, want)
	}
	if got, want := LateDeadline(aware), time.Date(2026, 4, 27, 16, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("late limit %v, want %v", got, want)
	}
}

func TestCheckpoints_24_48_66_72(t *testing.T) {
	aware := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	cs := Checkpoints(aware)
	if len(cs) != 4 {
		t.Fatalf("%d checkpoints", len(cs))
	}
	for i, h := range []int{24, 48, 66, 72} {
		if cs[i].Hours != h || !cs[i].At.Equal(aware.Add(time.Duration(h)*time.Hour)) {
			t.Errorf("checkpoint %d: %+v", i, cs[i])
		}
		if cs[i].Escalate != (h >= 66) || cs[i].Overdue != (h == 72) {
			t.Errorf("checkpoint %d flags: %+v", h, cs[i])
		}
	}
}

func TestToSchedule_FutureOnesPlusTheLatestPassed(t *testing.T) {
	aware := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	now := aware.Add(50 * time.Hour) // recorded late: 24 and 48 have passed
	got := ToSchedule(aware, now)
	if len(got) != 3 || got[0].Hours != 48 || !got[0].At.Equal(now) || got[1].Hours != 66 || got[2].Hours != 72 {
		t.Errorf("%+v", got)
	}
	if got := ToSchedule(aware, aware); len(got) != 4 || got[0].Hours != 24 {
		t.Errorf("fresh incident: %+v", got)
	}
	if got := ToSchedule(aware, aware.Add(100*time.Hour)); len(got) != 1 || got[0].Hours != 72 {
		t.Errorf("recorded after 72 h: %+v", got)
	}
}

func TestClockAt(t *testing.T) {
	aware := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		after   time.Duration
		running bool
		want    string
	}{
		{0, true, "on_track"}, {65*time.Hour + 59*time.Minute, true, "on_track"}, {66 * time.Hour, true, "due_soon"},
		{72*time.Hour - time.Second, true, "due_soon"}, {72 * time.Hour, true, "overdue"}, {100 * time.Hour, false, "stopped"},
	} {
		if got := ClockAt(aware, aware.Add(c.after), c.running); got.State != c.want {
			t.Errorf("%v running=%v: %s, want %s", c.after, c.running, got.State, c.want)
		}
	}
}

func TestLateReasonRequired(t *testing.T) {
	aware := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	if LateReasonRequired(aware, aware.Add(72*time.Hour)) {
		t.Error("exactly at 72 h is on time")
	}
	if !LateReasonRequired(aware, aware.Add(72*time.Hour+time.Minute)) {
		t.Error("after 72 h needs a reason")
	}
}
