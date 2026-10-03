// Package workflow is the workflow & SLA engine (PLT-05): tenant-configurable state machines (JSON
// definitions, versioned), tasks assigned to users or groups, and SLA timers counted in calendar days,
// business days (on the tenant's ORG-20 calendar) or hours, with reminders before the due time and
// escalation when it passes — driven by River jobs scheduled for exactly those moments.
//
// Modules own their records and their legally defined statuses (docs/states); the engine tracks the
// work around a record. A module starts an instance for its record (Service.Start), registers who may
// see and move it (Service.Register), and can react to SLA events and transitions through hooks.
package workflow

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

// Definition is platform.workflow_definitions.definition — the JSON an admin edits (docs/data/platform.md).
type Definition struct {
	Initial     string       `json:"initial"`
	States      []State      `json:"states"`
	Transitions []Transition `json:"transitions"`
	SLA         *SLASpec     `json:"sla,omitempty"`
}

// Text is a label by language; "th" is required (the primary UI language), "en" optional.
type Text map[string]string

// State is one step. Terminal states end the instance (its SLA is then met or stays breached).
type State struct {
	Key      string    `json:"key"`
	Label    Text      `json:"label"`
	Terminal bool      `json:"terminal,omitempty"`
	PauseSLA bool      `json:"pause_sla,omitempty"` // the SLA clock stops while the instance is here
	Task     *TaskSpec `json:"task,omitempty"`      // work created when the instance enters the state
}

// TaskSpec describes the task opened on entering a state: assigned to a user or to a group (whose
// members may claim it). Due is optional and counted like the SLA.
type TaskSpec struct {
	Title           Text       `json:"title"`
	AssigneeUserID  *uuid.UUID `json:"assignee_user_id,omitempty"`
	AssigneeGroupID *uuid.UUID `json:"assignee_group_id,omitempty"`
	Due             *Duration  `json:"due,omitempty"`
}

// Transition allows moving from one state to another.
type Transition struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label Text   `json:"label,omitempty"`
}

// Duration is an amount of calendar days, business days or hours.
type Duration struct {
	Mode   string `json:"mode"` // calendar_days | business_days | hours
	Amount int    `json:"amount"`
}

// SLASpec is the instance's deadline, counted from the start. RemindBefore lists how long before the due
// time to remind, in the SLA's own unit (days, business days or hours) — e.g. [10, 5] on a 30-day SLA
// reminds on day 20 and day 25 (BP-06). Escalation goes to the listed users and group when it is breached.
type SLASpec struct {
	Code            string      `json:"code"`
	Duration                    // mode, amount
	CalendarID      *uuid.UUID  `json:"calendar_id,omitempty"` // nil: the tenant's default calendar
	RemindBefore    []int       `json:"remind_before,omitempty"`
	EscalateUserIDs []uuid.UUID `json:"escalate_user_ids,omitempty"`
	EscalateGroupID *uuid.UUID  `json:"escalate_group_id,omitempty"`
}

const (
	ModeCalendarDays = "calendar_days"
	ModeBusinessDays = "business_days"
	ModeHours        = "hours"
)

// ErrInvalidDefinition wraps every definition validation failure.
var ErrInvalidDefinition = errors.New("workflow: invalid definition")

var keyRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,59}$`)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidDefinition, fmt.Sprintf(format, a...))
}

// Validate checks the definition's structure. References to users, groups and calendars are checked by
// the service against the tenant's data.
func (d Definition) Validate() error {
	if len(d.States) == 0 || len(d.States) > 50 {
		return invalid("1 to 50 states")
	}
	states := map[string]State{}
	terminal := 0
	for _, s := range d.States {
		if !keyRE.MatchString(s.Key) {
			return invalid("state key %q", s.Key)
		}
		if _, dup := states[s.Key]; dup {
			return invalid("duplicate state %q", s.Key)
		}
		if err := s.Label.check("label of " + s.Key); err != nil {
			return err
		}
		if s.Terminal {
			terminal++
			if s.Task != nil || s.PauseSLA {
				return invalid("terminal state %q cannot have a task or pause the SLA", s.Key)
			}
		}
		if t := s.Task; t != nil {
			if err := t.Title.check("task title of " + s.Key); err != nil {
				return err
			}
			if (t.AssigneeUserID == nil) == (t.AssigneeGroupID == nil) {
				return invalid("task of %q needs exactly one assignee (user or group)", s.Key)
			}
			if t.Due != nil {
				if err := t.Due.check(); err != nil {
					return err
				}
			}
		}
		states[s.Key] = s
	}
	if _, ok := states[d.Initial]; !ok {
		return invalid("initial state %q is not a state", d.Initial)
	}
	if states[d.Initial].Terminal {
		return invalid("initial state cannot be terminal")
	}
	if terminal == 0 {
		return invalid("at least one terminal state")
	}
	seen := map[[2]string]bool{}
	for _, t := range d.Transitions {
		from, ok1 := states[t.From]
		_, ok2 := states[t.To]
		if !ok1 || !ok2 || t.From == t.To {
			return invalid("transition %s → %s", t.From, t.To)
		}
		if from.Terminal {
			return invalid("terminal state %q has an outgoing transition", t.From)
		}
		if seen[[2]string{t.From, t.To}] {
			return invalid("duplicate transition %s → %s", t.From, t.To)
		}
		seen[[2]string{t.From, t.To}] = true
		if len(t.Label) > 0 {
			if err := t.Label.check("transition label"); err != nil {
				return err
			}
		}
	}
	for _, s := range d.States {
		if !s.Terminal && !hasOutgoing(d.Transitions, s.Key) {
			return invalid("state %q is a dead end (no transition and not terminal)", s.Key)
		}
	}
	if sla := d.SLA; sla != nil {
		if !keyRE.MatchString(sla.Code) || len(sla.Code) > 40 {
			return invalid("SLA code %q", sla.Code)
		}
		if err := sla.Duration.check(); err != nil {
			return err
		}
		prev := -1
		for i, r := range sla.RemindBefore {
			if r <= 0 || r >= sla.Amount || (i > 0 && r == prev) || len(sla.RemindBefore) > 5 {
				return invalid("reminders must be 1 to 5 distinct amounts between 0 and the SLA")
			}
			prev = r
		}
	}
	return nil
}

func hasOutgoing(ts []Transition, key string) bool {
	for _, t := range ts {
		if t.From == key {
			return true
		}
	}
	return false
}

func (t Text) check(what string) error {
	if len([]rune(t["th"])) == 0 || len([]rune(t["th"])) > 200 || len([]rune(t["en"])) > 200 {
		return invalid("%s needs a Thai text (up to 200 characters)", what)
	}
	for k := range t {
		if k != "th" && k != "en" {
			return invalid("%s: language %q", what, k)
		}
	}
	return nil
}

func (d Duration) check() error {
	switch d.Mode {
	case ModeCalendarDays, ModeBusinessDays:
		if d.Amount < 1 || d.Amount > 3650 {
			return invalid("days must be 1 to 3650")
		}
	case ModeHours:
		if d.Amount < 1 || d.Amount > 8760 {
			return invalid("hours must be 1 to 8760")
		}
	default:
		return invalid("mode %q", d.Mode)
	}
	return nil
}

// state returns the state with key.
func (d Definition) state(key string) (State, bool) {
	for _, s := range d.States {
		if s.Key == key {
			return s, true
		}
	}
	return State{}, false
}

// allowed reports whether from → to is a transition.
func (d Definition) allowed(from, to string) bool {
	for _, t := range d.Transitions {
		if t.From == from && t.To == to {
			return true
		}
	}
	return false
}

// next lists the transitions out of a state.
func (d Definition) next(from string) []Transition {
	var out []Transition
	for _, t := range d.Transitions {
		if t.From == from {
			out = append(out, t)
		}
	}
	return out
}
