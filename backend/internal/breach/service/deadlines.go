package service

import "time"

// Legal time limits of a breach (PDPA s.37(4), PDPC breach notification rules 2022 — docs/legal/pdpa-rules.md).
const (
	// NoticeWindow: notify the PDPC without delay and within 72 hours of becoming aware of the breach.
	NoticeWindow = 72 * time.Hour
	// LateLimit: a late notice, with its reason, within 15 days of awareness (PDPC clarification — decisions Q-07,
	// pending legal confirmation).
	LateLimit = 15 * 24 * time.Hour
)

// Checkpoint is a moment the 72-hour clock alerts people (BRE-07): reminders at 24, 48 and 66 hours, escalation to
// executives from 66 hours, and overdue at 72.
type Checkpoint struct {
	Hours    int
	At       time.Time
	Escalate bool // executives are alerted too
	Overdue  bool
}

// CheckpointHours are the elapsed hours of the alerts.
var CheckpointHours = []int{24, 48, 66, 72}

// PDPCDue is when the PDPC notice is due: 72 hours after awareness.
func PDPCDue(awareAt time.Time) time.Time { return awareAt.UTC().Add(NoticeWindow) }

// LateDeadline is the last moment for a late notice (15 days after awareness).
func LateDeadline(awareAt time.Time) time.Time { return awareAt.UTC().Add(LateLimit) }

// Checkpoints are the alert moments of an incident, in order.
func Checkpoints(awareAt time.Time) []Checkpoint {
	out := make([]Checkpoint, 0, len(CheckpointHours))
	for _, h := range CheckpointHours {
		out = append(out, Checkpoint{Hours: h, At: awareAt.UTC().Add(time.Duration(h) * time.Hour), Escalate: h >= 66, Overdue: h >= 72})
	}
	return out
}

// CheckpointAt returns the checkpoint of hours h.
func CheckpointAt(awareAt time.Time, h int) (Checkpoint, bool) {
	for _, c := range Checkpoints(awareAt) {
		if c.Hours == h {
			return c, true
		}
	}
	return Checkpoint{}, false
}

// ToSchedule are the checkpoints to set up at now: every future one, plus the latest one already passed (so an
// incident recorded late still alerts, and escalates, at once).
func ToSchedule(awareAt, now time.Time) []Checkpoint {
	var out []Checkpoint
	var passed *Checkpoint
	for _, c := range Checkpoints(awareAt) {
		if c.At.After(now) {
			out = append(out, c)
		} else {
			cc := c
			passed = &cc
		}
	}
	if passed != nil {
		passed.At = now
		out = append([]Checkpoint{*passed}, out...)
	}
	return out
}

// Clock describes where an incident stands against the 72-hour limit at now.
type Clock struct {
	DueAt        time.Time
	LateBy       time.Time // LateDeadline
	State        string    // on_track | due_soon (66 h or more elapsed) | overdue | stopped (no notice needed or incident closed)
	HoursElapsed float64
}

// ClockAt computes the clock. running is false once the timer no longer applies (decision "no notification",
// PDPC notified, or closed).
func ClockAt(awareAt, now time.Time, running bool) Clock {
	c := Clock{DueAt: PDPCDue(awareAt), LateBy: LateDeadline(awareAt), HoursElapsed: now.Sub(awareAt).Hours()}
	switch {
	case !running:
		c.State = "stopped"
	case !now.Before(c.DueAt):
		c.State = "overdue"
	case now.Sub(awareAt) >= 66*time.Hour:
		c.State = "due_soon"
	default:
		c.State = "on_track"
	}
	return c
}

// LateReasonRequired: a PDPC notice submitted after the 72 hours needs a reason for the delay (BRE-08).
func LateReasonRequired(awareAt, submittedAt time.Time) bool {
	return submittedAt.After(PDPCDue(awareAt))
}
