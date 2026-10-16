package bizcal

import (
	"errors"
	"testing"
	"time"
)

func bkk(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, Bangkok) }

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// Songkran 2026: Mon 13 – Wed 15 April are holidays; 11–12 April is a weekend.
func songkran(t *testing.T) Calendar {
	t.Helper()
	c, err := New(Bangkok, []int{1, 2, 3, 4, 5}, []time.Time{date(2026, 4, 13), date(2026, 4, 14), date(2026, 4, 15)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAddBusinessDays_SkipsWeekendsAndHolidays(t *testing.T) {
	c := songkran(t)
	cases := []struct {
		name  string
		start time.Time
		n     int
		want  time.Time
	}{
		{"within the week", bkk(2026, 4, 6, 9), 3, bkk(2026, 4, 9, 9)},
		{"over a weekend", bkk(2026, 4, 3, 9), 1, bkk(2026, 4, 6, 9)},
		{"into Songkran: Fri + 1 = Thu after the holidays", bkk(2026, 4, 10, 14), 1, bkk(2026, 4, 16, 14)},
		{"across Songkran", bkk(2026, 4, 9, 9), 5, bkk(2026, 4, 21, 9)},
		{"starting on a holiday", bkk(2026, 4, 14, 10), 1, bkk(2026, 4, 16, 10)},
		{"starting on a Saturday", bkk(2026, 4, 4, 10), 1, bkk(2026, 4, 6, 10)},
		{"zero on a business day", bkk(2026, 4, 8, 11), 0, bkk(2026, 4, 8, 11)},
		{"zero on a holiday moves to the next business morning", bkk(2026, 4, 13, 11), 0, bkk(2026, 4, 16, 0)},
	}
	for _, tc := range cases {
		got, err := c.AddBusinessDays(tc.start, tc.n)
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("%s: got %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}

// The day boundary is the calendar's zone, not UTC: 23:30 UTC on Sunday is already Monday in Bangkok.
func TestAddBusinessDays_UsesCalendarZone(t *testing.T) {
	c := songkran(t)
	start := time.Date(2026, 4, 5, 23, 30, 0, 0, time.UTC) // Mon 6 Apr 06:30 in Bangkok
	if !c.IsBusinessDay(start) {
		t.Fatal("Monday morning in Bangkok should be a business day")
	}
	got, _ := c.AddBusinessDays(start, 1)
	if want := bkk(2026, 4, 7, 6).Add(30 * time.Minute); !got.Equal(want) {
		t.Errorf("got %v, want %v", got.In(Bangkok), want)
	}
	// 15 April 18:00 UTC is still the holiday in UTC but already Thursday 16 April 01:00 in Bangkok.
	if !c.IsBusinessDay(time.Date(2026, 4, 15, 18, 0, 0, 0, time.UTC)) {
		t.Error("16 April in Bangkok should be a business day")
	}
}

func TestCustomWorkdays(t *testing.T) {
	// A six-day week (Mon–Sat), e.g. a factory.
	c, err := New(Bangkok, []int{1, 2, 3, 4, 5, 6}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := c.AddBusinessDays(bkk(2026, 4, 3, 9), 1) // Friday
	if want := bkk(2026, 4, 4, 9); !got.Equal(want) {
		t.Errorf("six-day week: got %v, want Saturday %v", got, want)
	}
	// ISO 7 is Sunday.
	sun, _ := New(Bangkok, []int{7}, nil)
	if !sun.IsBusinessDay(bkk(2026, 4, 5, 9)) || sun.IsBusinessDay(bkk(2026, 4, 6, 9)) {
		t.Error("ISO 7 should mean Sunday")
	}
}

func TestBusinessDaysBetween(t *testing.T) {
	c := songkran(t)
	if n := c.BusinessDaysBetween(bkk(2026, 4, 9, 9), bkk(2026, 4, 21, 9)); n != 5 {
		t.Errorf("across Songkran: %d, want 5", n)
	}
	if n := c.BusinessDaysBetween(bkk(2026, 4, 21, 9), bkk(2026, 4, 9, 9)); n != -5 {
		t.Errorf("reversed: %d, want -5", n)
	}
	if n := c.BusinessDaysBetween(bkk(2026, 4, 8, 9), bkk(2026, 4, 8, 17)); n != 0 {
		t.Errorf("same day: %d, want 0", n)
	}
	// Round trip with AddBusinessDays.
	for n := 0; n < 40; n++ {
		end, _ := c.AddBusinessDays(bkk(2026, 4, 1, 9), n)
		if got := c.BusinessDaysBetween(bkk(2026, 4, 1, 9), end); got != n {
			t.Fatalf("round trip %d: %d", n, got)
		}
	}
}

func TestInvalidCalendars(t *testing.T) {
	if _, err := New(Bangkok, nil, nil); !errors.Is(err, ErrNoWorkdays) {
		t.Errorf("no workdays: %v", err)
	}
	if _, err := New(Bangkok, []int{0}, nil); err == nil {
		t.Error("workday 0 accepted")
	}
	if _, err := New(Bangkok, []int{8}, nil); err == nil {
		t.Error("workday 8 accepted")
	}
	// A calendar whose only workday is always a holiday cannot run forever.
	var hs []time.Time
	for d := date(2026, 1, 1); d.Year() < 2040; d = d.AddDate(0, 0, 1) {
		hs = append(hs, d)
	}
	c, _ := New(Bangkok, []int{1}, hs)
	if _, err := c.AddBusinessDays(bkk(2026, 1, 1, 9), 1); !errors.Is(err, ErrTooFar) {
		t.Errorf("all holidays: %v", err)
	}
	if _, err := Default().AddBusinessDays(bkk(2026, 1, 1, 9), -1); err == nil {
		t.Error("negative count accepted")
	}
}

func TestDefault(t *testing.T) {
	d := Default()
	if d.Location() != Bangkok || !d.IsBusinessDay(bkk(2026, 4, 13, 9)) || d.IsBusinessDay(bkk(2026, 4, 12, 9)) {
		t.Error("default should be Mon–Fri in Bangkok without holidays")
	}
}

func TestSubBusinessDays(t *testing.T) {
	c := songkran(t)
	// Due Tuesday 21 April 09:00; 5 business days before skips Songkran and the weekend.
	got, err := c.SubBusinessDays(bkk(2026, 4, 21, 9), 5)
	if want := bkk(2026, 4, 9, 9); err != nil || !got.Equal(want) {
		t.Errorf("got %v %v, want %v", got, err, want)
	}
	for n := 0; n < 30; n++ {
		end, _ := c.AddBusinessDays(bkk(2026, 4, 1, 9), n)
		back, _ := c.SubBusinessDays(end, n)
		if !back.Equal(bkk(2026, 4, 1, 9)) {
			t.Fatalf("round trip %d: %v", n, back)
		}
	}
	if got := c.AddCalendarDays(bkk(2026, 4, 10, 14), 30); !got.Equal(bkk(2026, 5, 10, 14)) {
		t.Errorf("calendar days: %v", got)
	}
}
