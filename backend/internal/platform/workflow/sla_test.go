package workflow

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/bizcal"
)

func bkk(y int, m time.Month, d, h int) time.Time {
	return time.Date(y, m, d, h, 0, 0, 0, bizcal.Bangkok)
}

// Mon–Fri in Bangkok with Songkran (Mon 13 – Wed 15 April 2026) off.
func songkran(t *testing.T) bizcal.Calendar {
	t.Helper()
	c, err := bizcal.New(bizcal.Bangkok, []int{1, 2, 3, 4, 5}, []time.Time{
		time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Acceptance (PLT-05): SLAs count correctly across holidays.
func TestDueAt_AcrossHolidays(t *testing.T) {
	cal := songkran(t)
	start := bkk(2026, 4, 9, 9) // Thursday before Songkran
	cases := []struct {
		d    Duration
		want time.Time
	}{
		{Duration{ModeBusinessDays, 5}, bkk(2026, 4, 21, 9)}, // Fri 10, Thu 16, Fri 17, Mon 20, Tue 21
		{Duration{ModeBusinessDays, 1}, bkk(2026, 4, 10, 9)}, // Friday
		{Duration{ModeBusinessDays, 2}, bkk(2026, 4, 16, 9)}, // skips the weekend and Songkran
		{Duration{ModeCalendarDays, 30}, bkk(2026, 5, 9, 9)}, // calendar days ignore holidays (PDPA s.30)
		{Duration{ModeHours, 72}, bkk(2026, 4, 12, 9)},       // elapsed hours (breach notice)
	}
	for _, c := range cases {
		got, err := DueAt(cal, start, c.d)
		if err != nil || !got.Equal(c.want) || got.Location() != time.UTC {
			t.Errorf("%+v: got %v %v, want %v (in UTC)", c.d, got, err, c.want)
		}
	}
	if _, err := DueAt(cal, start, Duration{"weeks", 1}); err == nil {
		t.Error("unknown mode accepted")
	}
}

// Acceptance (PLT-05): reminders fire before the due time as configured, counted on the calendar.
func TestReminderTimes(t *testing.T) {
	cal := songkran(t)
	start := bkk(2026, 4, 1, 9)
	// 30 calendar days, reminders on day 20 and 25 (BP-06): 10 and 5 days before due.
	due, _ := DueAt(cal, start, Duration{ModeCalendarDays, 30})
	got, err := ReminderTimes(cal, start, due, ModeCalendarDays, []int{5, 10})
	if err != nil || len(got) != 2 || !got[0].Equal(bkk(2026, 4, 21, 9)) || !got[1].Equal(bkk(2026, 4, 26, 9)) {
		t.Errorf("calendar-day reminders: %v %v", got, err)
	}
	// 10 business days from 1 April, reminded 3 business days before: Songkran pushes both.
	due, _ = DueAt(cal, start, Duration{ModeBusinessDays, 10}) // Apr 2,3,6,7,8,9,10,16,17,20 → Mon 20 Apr
	if !due.Equal(bkk(2026, 4, 20, 9)) {
		t.Fatalf("business due %v", due)
	}
	got, _ = ReminderTimes(cal, start, due, ModeBusinessDays, []int{3})
	if len(got) != 1 || !got[0].Equal(bkk(2026, 4, 10, 9)) { // 3 business days before Mon 20: Fri 17, Thu 16, Fri 10
		t.Errorf("business-day reminder: %v", got)
	}
	// 72 hours, reminded 48 / 24 / 6 hours before (breach: T+24, T+48, T+66).
	due, _ = DueAt(cal, start, Duration{ModeHours, 72})
	got, _ = ReminderTimes(cal, start, due, ModeHours, []int{48, 24, 6})
	if len(got) != 3 || !got[0].Equal(start.Add(24*time.Hour)) || !got[2].Equal(start.Add(66*time.Hour)) {
		t.Errorf("hour reminders: %v", got)
	}
	// A reminder at or before the start is dropped.
	due, _ = DueAt(cal, start, Duration{ModeHours, 4})
	if got, _ := ReminderTimes(cal, start, due, ModeHours, []int{4, 5, 1}); len(got) != 1 {
		t.Errorf("past reminders kept: %v", got)
	}
}

func TestResume(t *testing.T) {
	cal := songkran(t)
	due := bkk(2026, 4, 30, 9)
	// Hours / calendar days: pushed back by the elapsed pause.
	if got, _ := Resume(cal, ModeCalendarDays, due, bkk(2026, 4, 1, 9), bkk(2026, 4, 4, 15)); !got.Equal(due.Add(78 * time.Hour)) {
		t.Errorf("calendar-day resume: %v", got)
	}
	// Business days: paused Friday 10 → resumed Thursday 16 April covers one business day (Thu 16)
	// — the weekend and Songkran cost nothing.
	if got, _ := Resume(cal, ModeBusinessDays, due, bkk(2026, 4, 10, 10), bkk(2026, 4, 16, 10)); !got.Equal(bkk(2026, 5, 1, 9)) {
		t.Errorf("business-day resume: %v", got)
	}
	if got, _ := Resume(cal, ModeHours, due, bkk(2026, 4, 1, 9), bkk(2026, 4, 1, 9)); !got.Equal(due) {
		t.Errorf("zero pause: %v", got)
	}
}

func validDefinition() Definition {
	u := uuid.New()
	return Definition{
		Initial: "received",
		States: []State{
			{Key: "received", Label: Text{"th": "รับคำขอ"}, Task: &TaskSpec{Title: Text{"th": "ตรวจคำขอ"}, AssigneeUserID: &u}},
			{Key: "awaiting_info", Label: Text{"th": "รอข้อมูล"}, PauseSLA: true},
			{Key: "done", Label: Text{"th": "เสร็จ", "en": "Done"}, Terminal: true},
		},
		Transitions: []Transition{{From: "received", To: "awaiting_info"}, {From: "awaiting_info", To: "received"}, {From: "received", To: "done"}},
		SLA:         &SLASpec{Code: "response", Duration: Duration{ModeCalendarDays, 30}, RemindBefore: []int{10, 5}},
	}
}

func TestDefinitionValidate(t *testing.T) {
	if err := validDefinition().Validate(); err != nil {
		t.Fatalf("valid definition: %v", err)
	}
	g := uuid.New()
	bad := map[string]func(d *Definition){
		"no states":        func(d *Definition) { d.States = nil },
		"bad key":          func(d *Definition) { d.States[0].Key = "Received" },
		"duplicate state":  func(d *Definition) { d.States[1].Key = "received" },
		"no thai label":    func(d *Definition) { d.States[0].Label = Text{"en": "x"} },
		"other language":   func(d *Definition) { d.States[0].Label["fr"] = "x" },
		"unknown initial":  func(d *Definition) { d.Initial = "nope" },
		"terminal initial": func(d *Definition) { d.Initial = "done" },
		"no terminal": func(d *Definition) {
			d.States[2].Terminal = false
			d.Transitions = append(d.Transitions, Transition{From: "done", To: "received"})
		},
		"transition to ghost": func(d *Definition) { d.Transitions[0].To = "ghost" },
		"self transition":     func(d *Definition) { d.Transitions[0].To = "received" },
		"from terminal":       func(d *Definition) { d.Transitions = append(d.Transitions, Transition{From: "done", To: "received"}) },
		"duplicate transition": func(d *Definition) {
			d.Transitions = append(d.Transitions, Transition{From: "received", To: "done"})
		},
		"dead end":           func(d *Definition) { d.Transitions = d.Transitions[:1] },
		"two assignees":      func(d *Definition) { d.States[0].Task.AssigneeGroupID = &g },
		"no assignee":        func(d *Definition) { d.States[0].Task.AssigneeUserID = nil },
		"task on terminal":   func(d *Definition) { d.States[2].Task = d.States[0].Task },
		"bad mode":           func(d *Definition) { d.SLA.Mode = "weeks" },
		"zero amount":        func(d *Definition) { d.SLA.Amount = 0 },
		"too many hours":     func(d *Definition) { d.SLA.Mode, d.SLA.Amount = ModeHours, 9000 },
		"reminder after due": func(d *Definition) { d.SLA.RemindBefore = []int{30} },
		"negative reminder":  func(d *Definition) { d.SLA.RemindBefore = []int{-1} },
		"bad SLA code":       func(d *Definition) { d.SLA.Code = "Response Time" },
		"bad task due":       func(d *Definition) { d.States[0].Task.Due = &Duration{ModeHours, 0} },
	}
	for name, mutate := range bad {
		d := validDefinition()
		mutate(&d)
		if err := d.Validate(); !errors.Is(err, ErrInvalidDefinition) {
			t.Errorf("%s: %v, want ErrInvalidDefinition", name, err)
		}
	}
}
