// Package bizcal counts working days on a tenant's business calendar (ORG-20): which weekdays are worked,
// in which time zone, and which dates are holidays. It is pure — the org module loads a Calendar
// (org/service.Calendars) and the SLA engine does the arithmetic here, so every rule is unit-testable.
package bizcal

import (
	"errors"
	"time"
)

// Calendar is one business calendar. The zero value is not usable; use Default or build one with New.
type Calendar struct {
	loc      *time.Location
	workdays [7]bool            // by time.Weekday
	holidays map[civilDate]bool // local dates
}

type civilDate struct {
	y int
	m time.Month
	d int
}

// ErrNoWorkdays is returned for a calendar without any working weekday.
var ErrNoWorkdays = errors.New("bizcal: a calendar needs at least one working weekday")

// ErrTooFar is returned when a count would run past maxSpan, e.g. a calendar that is all holidays.
var ErrTooFar = errors.New("bizcal: no working day within range")

// maxSpan bounds every walk so a calendar made (almost) entirely of holidays cannot loop forever.
const maxSpan = 3660 // days, about ten years

// Bangkok is the platform's display and default calendar zone (CLAUDE.md rule 11).
var Bangkok = mustLoad("Asia/Bangkok")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// New builds a calendar. isoWorkdays are ISO weekdays (1 = Monday … 7 = Sunday), as stored in
// org.business_calendars.workdays; holidays are dates in loc (only their year, month and day count).
func New(loc *time.Location, isoWorkdays []int, holidays []time.Time) (Calendar, error) {
	c := Calendar{loc: loc, holidays: make(map[civilDate]bool, len(holidays))}
	for _, d := range isoWorkdays {
		if d < 1 || d > 7 {
			return Calendar{}, errors.New("bizcal: workday out of range 1..7")
		}
		c.workdays[time.Weekday(d%7)] = true
	}
	if c.workdays == [7]bool{} {
		return Calendar{}, ErrNoWorkdays
	}
	for _, h := range holidays {
		c.holidays[civilDate{h.Year(), h.Month(), h.Day()}] = true
	}
	return c, nil
}

// Default is the calendar used when a tenant has none: Monday to Friday in Asia/Bangkok, no holidays.
func Default() Calendar {
	c, _ := New(Bangkok, []int{1, 2, 3, 4, 5}, nil)
	return c
}

// Location is the calendar's time zone.
func (c Calendar) Location() *time.Location { return c.loc }

// IsBusinessDay reports whether the local date of t is a working weekday and not a holiday.
func (c Calendar) IsBusinessDay(t time.Time) bool {
	l := t.In(c.loc)
	return c.workdays[l.Weekday()] && !c.holidays[civilDate{l.Year(), l.Month(), l.Day()}]
}

// AddBusinessDays returns the moment n business days after t: the local date moves forward one business
// day at a time (days off are skipped) and keeps t's local time of day. Starting on a day off counts
// from the next business day, so Saturday 10:00 + 1 is Monday 10:00 on a Monday–Friday calendar.
// n = 0 returns t moved to the start of the next business day if t falls on a day off, else t.
func (c Calendar) AddBusinessDays(t time.Time, n int) (time.Time, error) {
	if n < 0 {
		return time.Time{}, errors.New("bizcal: negative day count")
	}
	l := t.In(c.loc)
	if n == 0 {
		if c.IsBusinessDay(l) {
			return t, nil
		}
		next, err := c.nextBusinessDay(l)
		if err != nil {
			return time.Time{}, err
		}
		y, m, d := next.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, c.loc), nil
	}
	for i := 0; n > 0; i++ {
		if i > maxSpan {
			return time.Time{}, ErrTooFar
		}
		l = addLocalDays(l, 1, c.loc)
		if c.IsBusinessDay(l) {
			n--
		}
	}
	return l, nil
}

// SubBusinessDays is AddBusinessDays backwards: the moment n business days before t, keeping t's local
// time of day. Reminders "N business days before the due date" use it.
func (c Calendar) SubBusinessDays(t time.Time, n int) (time.Time, error) {
	if n < 0 {
		return time.Time{}, errors.New("bizcal: negative day count")
	}
	l := t.In(c.loc)
	for i := 0; n > 0; i++ {
		if i > maxSpan {
			return time.Time{}, ErrTooFar
		}
		l = addLocalDays(l, -1, c.loc)
		if c.IsBusinessDay(l) {
			n--
		}
	}
	return l, nil
}

// AddCalendarDays returns t moved by n local calendar days, keeping the wall clock in the calendar's zone.
func (c Calendar) AddCalendarDays(t time.Time, n int) time.Time {
	return addLocalDays(t.In(c.loc), n, c.loc)
}

// BusinessDaysBetween counts the business days whose local date is after from's and not after to's —
// the number of AddBusinessDays steps from one to the other. It is negative when to is before from.
func (c Calendar) BusinessDaysBetween(from, to time.Time) int {
	a, b, sign := from.In(c.loc), to.In(c.loc), 1
	if dateOf(b).before(dateOf(a)) {
		a, b, sign = b, a, -1
	}
	n := 0
	for i, d := 0, a; dateOf(d).before(dateOf(b)) && i <= maxSpan; i++ {
		d = addLocalDays(d, 1, c.loc)
		if c.IsBusinessDay(d) {
			n++
		}
	}
	return sign * n
}

func (c Calendar) nextBusinessDay(l time.Time) (time.Time, error) {
	for i := 0; i <= maxSpan; i++ {
		l = addLocalDays(l, 1, c.loc)
		if c.IsBusinessDay(l) {
			return l, nil
		}
	}
	return time.Time{}, ErrTooFar
}

// addLocalDays moves a local time by whole calendar days, keeping the wall clock (DST-safe).
func addLocalDays(l time.Time, days int, loc *time.Location) time.Time {
	y, m, d := l.Date()
	return time.Date(y, m, d+days, l.Hour(), l.Minute(), l.Second(), l.Nanosecond(), loc)
}

func dateOf(l time.Time) civilDate {
	y, m, d := l.Date()
	return civilDate{y, m, d}
}

func (a civilDate) before(b civilDate) bool {
	if a.y != b.y {
		return a.y < b.y
	}
	if a.m != b.m {
		return a.m < b.m
	}
	return a.d < b.d
}
