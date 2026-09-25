package workflow

import (
	"fmt"
	"sort"
	"time"

	"pdpa-platform/internal/pkg/bizcal"
)

// Deadline functions (CLAUDE.md rule 7): pure, driven by an injected start time and calendar.

// DueAt is start + d on cal. Calendar days keep the local time of day in the calendar's zone; business
// days skip the calendar's days off and holidays; hours are elapsed hours.
func DueAt(cal bizcal.Calendar, start time.Time, d Duration) (time.Time, error) {
	switch d.Mode {
	case ModeCalendarDays:
		return cal.AddCalendarDays(start, d.Amount).UTC(), nil
	case ModeBusinessDays:
		t, err := cal.AddBusinessDays(start, d.Amount)
		return t.UTC(), err
	case ModeHours:
		return start.Add(time.Duration(d.Amount) * time.Hour).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("workflow: unknown SLA mode %q", d.Mode)
}

// ReminderTimes returns when to remind for an SLA due at due: each amount in before, counted back from
// due in the SLA's unit. Times not after start are dropped (a reminder that could never be useful);
// the rest are returned in time order.
func ReminderTimes(cal bizcal.Calendar, start, due time.Time, mode string, before []int) ([]time.Time, error) {
	var out []time.Time
	for _, n := range before {
		t, err := reminderAt(cal, due, mode, n)
		if err != nil {
			return nil, err
		}
		if t.After(start) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out, nil
}

// reminderAt is n units of mode before due.
func reminderAt(cal bizcal.Calendar, due time.Time, mode string, n int) (time.Time, error) {
	switch mode {
	case ModeCalendarDays:
		return cal.AddCalendarDays(due, -n).UTC(), nil
	case ModeBusinessDays:
		t, err := cal.SubBusinessDays(due, n)
		return t.UTC(), err
	case ModeHours:
		return due.Add(-time.Duration(n) * time.Hour).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("workflow: unknown SLA mode %q", mode)
}

// Resume returns a paused deadline pushed back by the pause: by the elapsed time for calendar days and
// hours, and by the business days the pause covered for business days (a pause over a weekend costs
// nothing on a Monday–Friday calendar).
func Resume(cal bizcal.Calendar, mode string, due, pausedAt, resumedAt time.Time) (time.Time, error) {
	if !resumedAt.After(pausedAt) {
		return due, nil
	}
	if mode == ModeBusinessDays {
		n := cal.BusinessDaysBetween(pausedAt, resumedAt)
		t, err := cal.AddBusinessDays(due, n)
		return t.UTC(), err
	}
	return due.Add(resumedAt.Sub(pausedAt)).UTC(), nil
}
