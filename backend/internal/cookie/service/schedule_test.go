package service

import (
	"testing"
	"time"
)

func TestScheduleInterval(t *testing.T) {
	cases := []struct {
		schedule string
		want     time.Duration
		ok       bool
	}{
		{"daily", 24 * time.Hour, true},
		{"weekly", 7 * 24 * time.Hour, true},
		{"monthly", 30 * 24 * time.Hour, true},
		{"", 0, false},
		{"hourly", 0, false},
	}
	for _, c := range cases {
		got, ok := scheduleInterval(c.schedule)
		if got != c.want || ok != c.ok {
			t.Errorf("scheduleInterval(%q) = %v, %v; want %v, %v", c.schedule, got, ok, c.want, c.ok)
		}
	}
}

func TestScanDue(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)

	// No prior scan at all: always due, whatever the schedule.
	if !scanDue("daily", nil, now) {
		t.Error("no prior scan should always be due")
	}

	// Unknown/empty schedule never triggers, even with no prior scan.
	if scanDue("", nil, now) {
		t.Error("empty schedule should never be due")
	}
	if scanDue("hourly", nil, now) {
		t.Error("unrecognized schedule should never be due")
	}

	recent := now.Add(-1 * time.Hour)
	if scanDue("daily", &recent, now) {
		t.Error("a daily scan finished 1h ago should not be due yet")
	}

	stale := now.Add(-25 * time.Hour)
	if !scanDue("daily", &stale, now) {
		t.Error("a daily scan finished 25h ago should be due")
	}

	exactlyDue := now.Add(-24 * time.Hour)
	if !scanDue("daily", &exactlyDue, now) {
		t.Error("a daily scan finished exactly 24h ago should be due")
	}

	recentWeekly := now.Add(-3 * 24 * time.Hour)
	if scanDue("weekly", &recentWeekly, now) {
		t.Error("a weekly scan finished 3 days ago should not be due yet")
	}
}
