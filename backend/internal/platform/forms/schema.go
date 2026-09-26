// Package forms is the form & assessment engine (PLT-06): versioned form schemas (JSON) with question types,
// show/hide conditions, weighted scoring into bands, server-side validation of answers, and responses whose
// sections can be assigned to different people. The admin builder and every module render forms with the
// one renderer in packages/form-renderer, which evaluates the same rules (shared test fixtures keep the Go
// and TypeScript evaluators identical).
package forms

import (
	"errors"
	"fmt"
	"regexp"
)

// Schema is platform.form_versions.schema.
type Schema struct {
	Sections []Section `json:"sections"`
}

// Text is a label by language; "th" is required.
type Text map[string]string

// Section groups questions; the whole section can be conditional and is the unit of assignment.
type Section struct {
	Key         string     `json:"key"`
	Title       Text       `json:"title"`
	Description Text       `json:"description,omitempty"`
	VisibleIf   *Condition `json:"visible_if,omitempty"`
	Questions   []Question `json:"questions"`
}

// Question types.
const (
	TypeText        = "text"
	TypeTextarea    = "textarea"
	TypeNumber      = "number"
	TypeDate        = "date"
	TypeEmail       = "email"
	TypeSingle      = "single_choice"
	TypeMulti       = "multi_choice"
	TypeYesNo       = "yes_no"
	defaultMaxChars = 2000
)

var questionTypes = map[string]bool{TypeText: true, TypeTextarea: true, TypeNumber: true, TypeDate: true, TypeEmail: true, TypeSingle: true, TypeMulti: true, TypeYesNo: true}

// Question is one field. Options (with optional scores) apply to choice questions; yes_no takes the values
// "yes" and "no" (options only to score them). Weight multiplies the question's score (default 1).
type Question struct {
	Key       string     `json:"key"`
	Type      string     `json:"type"`
	Label     Text       `json:"label"`
	Help      Text       `json:"help,omitempty"`
	Required  bool       `json:"required,omitempty"`
	Options   []Option   `json:"options,omitempty"`
	Min       *float64   `json:"min,omitempty"`
	Max       *float64   `json:"max,omitempty"`
	MaxLength int        `json:"max_length,omitempty"`
	Weight    *float64   `json:"weight,omitempty"`
	VisibleIf *Condition `json:"visible_if,omitempty"`
}

// Option is a choice.
type Option struct {
	Value string   `json:"value"`
	Label Text     `json:"label"`
	Score *float64 `json:"score,omitempty"`
}

// Condition shows a question or section: a comparison on an earlier question's answer, or all/any of
// nested conditions. Ops: eq, neq, in, not_in, gt, gte, lt, lte, answered, not_answered. On a
// multi_choice answer, eq/in mean "includes", neq/not_in "doesn't include".
type Condition struct {
	Question string      `json:"question,omitempty"`
	Op       string      `json:"op,omitempty"`
	Value    any         `json:"value,omitempty"`
	All      []Condition `json:"all,omitempty"`
	Any      []Condition `json:"any,omitempty"`
}

// Scoring is platform.form_versions.scoring: bands the total score falls into (min ≤ score ≤ max).
type Scoring struct {
	Bands []Band `json:"bands"`
}

// Band is one result level, e.g. low / medium / high risk.
type Band struct {
	Key   string   `json:"key"`
	Label Text     `json:"label"`
	Min   float64  `json:"min"`
	Max   *float64 `json:"max,omitempty"` // nil: no upper bound
}

// ErrInvalidSchema wraps every schema problem.
var ErrInvalidSchema = errors.New("forms: invalid schema")

var keyRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,59}$`)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSchema, fmt.Sprintf(format, a...))
}

var ops = map[string]bool{"eq": true, "neq": true, "in": true, "not_in": true, "gt": true, "gte": true, "lt": true, "lte": true, "answered": true, "not_answered": true}

// Validate checks a schema (and its scoring) before it is saved: unique keys, known types, options for
// choices, sane limits, and conditions that only refer to questions asked earlier (so visibility never
// loops).
func Validate(s Schema, sc *Scoring) error {
	if len(s.Sections) == 0 || len(s.Sections) > 50 {
		return invalid("1 to 50 sections")
	}
	sections, questions := map[string]bool{}, map[string]Question{}
	count := 0
	for _, sec := range s.Sections {
		if !keyRE.MatchString(sec.Key) || sections[sec.Key] {
			return invalid("section key %q", sec.Key)
		}
		sections[sec.Key] = true
		if err := sec.Title.check("title of section " + sec.Key); err != nil {
			return err
		}
		if sec.VisibleIf != nil {
			if err := checkCondition(*sec.VisibleIf, questions, 0); err != nil {
				return err
			}
		}
		if len(sec.Questions) == 0 {
			return invalid("section %q has no questions", sec.Key)
		}
		for _, q := range sec.Questions {
			count++
			if count > 500 {
				return invalid("at most 500 questions")
			}
			if !keyRE.MatchString(q.Key) {
				return invalid("question key %q", q.Key)
			}
			if _, dup := questions[q.Key]; dup {
				return invalid("duplicate question key %q", q.Key)
			}
			if err := checkQuestion(q); err != nil {
				return err
			}
			if q.VisibleIf != nil {
				if err := checkCondition(*q.VisibleIf, questions, 0); err != nil {
					return err
				}
			}
			questions[q.Key] = q
		}
	}
	if sc != nil {
		if len(sc.Bands) > 10 {
			return invalid("at most 10 score bands")
		}
		seen := map[string]bool{}
		for i, b := range sc.Bands {
			if !keyRE.MatchString(b.Key) || seen[b.Key] {
				return invalid("band key %q", b.Key)
			}
			seen[b.Key] = true
			if err := b.Label.check("band " + b.Key); err != nil {
				return err
			}
			if b.Max != nil && *b.Max < b.Min {
				return invalid("band %q: max below min", b.Key)
			}
			if i > 0 {
				prev := sc.Bands[i-1]
				if prev.Max == nil || b.Min <= *prev.Max {
					return invalid("bands must ascend without overlapping (%s, %s)", prev.Key, b.Key)
				}
			}
		}
	}
	return nil
}

func checkQuestion(q Question) error {
	if !questionTypes[q.Type] {
		return invalid("question %q: type %q", q.Key, q.Type)
	}
	if err := q.Label.check("label of " + q.Key); err != nil {
		return err
	}
	if len(q.Help) > 0 {
		if err := q.Help.checkLen("help of "+q.Key, 1000); err != nil {
			return err
		}
	}
	switch q.Type {
	case TypeSingle, TypeMulti:
		if len(q.Options) == 0 || len(q.Options) > 100 {
			return invalid("question %q needs 1 to 100 options", q.Key)
		}
	case TypeYesNo:
		for _, o := range q.Options {
			if o.Value != "yes" && o.Value != "no" {
				return invalid("question %q: yes_no options are yes and no", q.Key)
			}
		}
	default:
		if len(q.Options) > 0 {
			return invalid("question %q: options only for choice questions", q.Key)
		}
	}
	values := map[string]bool{}
	for _, o := range q.Options {
		if o.Value == "" || len(o.Value) > 60 || values[o.Value] {
			return invalid("question %q: option value %q", q.Key, o.Value)
		}
		values[o.Value] = true
		if q.Type != TypeYesNo || len(o.Label) > 0 {
			if err := o.Label.check("option " + o.Value + " of " + q.Key); err != nil {
				return err
			}
		}
		if o.Score != nil && (*o.Score < -1000 || *o.Score > 1000) {
			return invalid("question %q: option score out of range", q.Key)
		}
	}
	if q.Min != nil && q.Max != nil && *q.Max < *q.Min {
		return invalid("question %q: max below min", q.Key)
	}
	if (q.Min != nil || q.Max != nil) && q.Type != TypeNumber {
		return invalid("question %q: min/max only for numbers", q.Key)
	}
	if q.MaxLength < 0 || q.MaxLength > 10000 || (q.MaxLength > 0 && q.Type != TypeText && q.Type != TypeTextarea) {
		return invalid("question %q: max_length", q.Key)
	}
	if q.Weight != nil && (*q.Weight < 0 || *q.Weight > 1000) {
		return invalid("question %q: weight 0 to 1000", q.Key)
	}
	return nil
}

func checkCondition(c Condition, earlier map[string]Question, depth int) error {
	if depth > 5 {
		return invalid("conditions nest at most 5 deep")
	}
	nested := len(c.All) + len(c.Any)
	if nested > 0 {
		if c.Question != "" || c.Op != "" || (len(c.All) > 0 && len(c.Any) > 0) {
			return invalid("a condition is either a comparison or all/any")
		}
		for _, n := range append(append([]Condition{}, c.All...), c.Any...) {
			if err := checkCondition(n, earlier, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	q, ok := earlier[c.Question]
	if !ok {
		return invalid("condition on %q: only questions asked earlier", c.Question)
	}
	if !ops[c.Op] {
		return invalid("condition op %q", c.Op)
	}
	switch c.Op {
	case "in", "not_in":
		if _, ok := c.Value.([]any); !ok {
			return invalid("condition on %q: %s needs a list", c.Question, c.Op)
		}
	case "gt", "gte", "lt", "lte":
		if _, ok := number(c.Value); !ok || q.Type != TypeNumber {
			return invalid("condition on %q: %s compares numbers", c.Question, c.Op)
		}
	case "eq", "neq":
		if c.Value == nil {
			return invalid("condition on %q: %s needs a value", c.Question, c.Op)
		}
	}
	return nil
}

func (t Text) check(what string) error {
	if len([]rune(t["th"])) == 0 {
		return invalid("%s needs a Thai text", what)
	}
	return t.checkLen(what, 500)
}

func (t Text) checkLen(what string, max int) error {
	for k, v := range t {
		if k != "th" && k != "en" {
			return invalid("%s: language %q", what, k)
		}
		if len([]rune(v)) > max {
			return invalid("%s: too long", what)
		}
	}
	return nil
}
