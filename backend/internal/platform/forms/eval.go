package forms

import (
	"encoding/json"
	"math"
	"net/mail"
	"slices"
	"strings"
	"time"
)

// Answers maps question keys to values: strings (text, textarea, date YYYY-MM-DD, email, single_choice,
// yes_no "yes"/"no"), numbers (number) and string lists (multi_choice).
type Answers map[string]any

// FieldError is one problem with an answer. Codes: required, invalid_type, invalid_option, out_of_range,
// too_long, invalid_email, invalid_date.
type FieldError struct {
	Question string `json:"question"`
	Code     string `json:"code"`
}

// Result is the outcome of evaluating answers against a schema.
type Result struct {
	Visible  []string     `json:"visible"`   // question keys shown, in order
	Answers  Answers      `json:"answers"`   // the valid answers of visible questions only
	Errors   []FieldError `json:"errors"`    // empty when the answers can be submitted
	Score    float64      `json:"score"`     // weighted sum of the chosen options' scores
	MaxScore float64      `json:"max_score"` // the best score reachable with these visible questions
	Band     string       `json:"band,omitempty"`
}

// Evaluate applies a schema to answers: which questions are visible (in order — a hidden question counts
// as unanswered for later conditions), which answers are valid, the score and its band. With requireAll,
// visible required questions must be answered (final submission); without it only given answers are
// checked (saving a draft). sections, when not nil, limits checking and scoring to those sections.
func Evaluate(s Schema, sc *Scoring, in Answers, requireAll bool, sections []string) Result {
	res := Result{Visible: []string{}, Answers: Answers{}, Errors: []FieldError{}}
	shown := Answers{} // answers of visible questions so far, as conditions see them
	for _, sec := range s.Sections {
		if sec.VisibleIf != nil && !holds(*sec.VisibleIf, shown) {
			continue
		}
		inScope := sections == nil || slices.Contains(sections, sec.Key)
		for _, q := range sec.Questions {
			if q.VisibleIf != nil && !holds(*q.VisibleIf, shown) {
				continue
			}
			res.Visible = append(res.Visible, q.Key)
			raw, given := in[q.Key]
			if given && isEmpty(raw) {
				given = false
			}
			if !given {
				if requireAll && q.Required && inScope {
					res.Errors = append(res.Errors, FieldError{q.Key, "required"})
				}
				if inScope {
					res.MaxScore += maxScore(q)
				}
				continue
			}
			v, code := normalize(q, raw)
			if code != "" {
				if inScope {
					res.Errors = append(res.Errors, FieldError{q.Key, code})
					res.MaxScore += maxScore(q)
				}
				continue
			}
			shown[q.Key] = v
			if inScope {
				res.Answers[q.Key] = v
				res.Score += score(q, v)
				res.MaxScore += maxScore(q)
			}
		}
	}
	res.Score, res.MaxScore = round(res.Score), round(res.MaxScore)
	if sc != nil {
		for _, b := range sc.Bands {
			if res.Score >= b.Min && (b.Max == nil || res.Score <= *b.Max) {
				res.Band = b.Key
				break
			}
		}
	}
	return res
}

// SectionOf returns the key of the section a question belongs to.
func (s Schema) SectionOf(question string) string {
	for _, sec := range s.Sections {
		for _, q := range sec.Questions {
			if q.Key == question {
				return sec.Key
			}
		}
	}
	return ""
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	}
	return false
}

// normalize checks one answer and returns it in canonical form, or an error code.
func normalize(q Question, v any) (any, string) {
	switch q.Type {
	case TypeText, TypeTextarea:
		s, ok := v.(string)
		if !ok {
			return nil, "invalid_type"
		}
		s = strings.TrimSpace(s)
		limit := q.MaxLength
		if limit == 0 {
			limit = defaultMaxChars
		}
		if len([]rune(s)) > limit {
			return nil, "too_long"
		}
		return s, ""
	case TypeEmail:
		s, ok := v.(string)
		if !ok {
			return nil, "invalid_type"
		}
		s = strings.TrimSpace(s)
		if a, err := mail.ParseAddress(s); err != nil || a.Address != s || len(s) > 254 {
			return nil, "invalid_email"
		}
		return s, ""
	case TypeDate:
		s, ok := v.(string)
		if !ok {
			return nil, "invalid_type"
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, "invalid_date"
		}
		return s, ""
	case TypeNumber:
		n, ok := number(v)
		if !ok {
			return nil, "invalid_type"
		}
		if (q.Min != nil && n < *q.Min) || (q.Max != nil && n > *q.Max) {
			return nil, "out_of_range"
		}
		return n, ""
	case TypeSingle, TypeYesNo:
		s, ok := v.(string)
		if !ok {
			return nil, "invalid_type"
		}
		if !validOption(q, s) {
			return nil, "invalid_option"
		}
		return s, ""
	case TypeMulti:
		list, ok := stringList(v)
		if !ok {
			return nil, "invalid_type"
		}
		out := []string{}
		for _, s := range list {
			if !validOption(q, s) {
				return nil, "invalid_option"
			}
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
		return out, ""
	}
	return nil, "invalid_type"
}

func validOption(q Question, s string) bool {
	if q.Type == TypeYesNo {
		return s == "yes" || s == "no"
	}
	return slices.ContainsFunc(q.Options, func(o Option) bool { return o.Value == s })
}

func stringList(v any) ([]string, bool) {
	switch x := v.(type) {
	case []string:
		return x, true
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, !math.IsNaN(x) && !math.IsInf(x, 0)
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

func weight(q Question) float64 {
	if q.Weight != nil {
		return *q.Weight
	}
	return 1
}

func optionScore(q Question, value string) float64 {
	for _, o := range q.Options {
		if o.Value == value && o.Score != nil {
			return *o.Score
		}
	}
	return 0
}

func score(q Question, v any) float64 {
	switch x := v.(type) {
	case string:
		if q.Type == TypeSingle || q.Type == TypeYesNo {
			return weight(q) * optionScore(q, x)
		}
	case []string:
		sum := 0.0
		for _, s := range x {
			sum += optionScore(q, s)
		}
		return weight(q) * sum
	}
	return 0
}

// maxScore is the most a question can add: the best option, or all positive options of a multi choice.
func maxScore(q Question) float64 {
	best := 0.0
	for _, o := range q.Options {
		if o.Score == nil {
			continue
		}
		if q.Type == TypeMulti {
			if *o.Score > 0 {
				best += *o.Score
			}
		} else if *o.Score > best {
			best = *o.Score
		}
	}
	return weight(q) * best
}

func holds(c Condition, answers Answers) bool {
	if len(c.All) > 0 {
		for _, n := range c.All {
			if !holds(n, answers) {
				return false
			}
		}
		return true
	}
	if len(c.Any) > 0 {
		for _, n := range c.Any {
			if holds(n, answers) {
				return true
			}
		}
		return false
	}
	v, answered := answers[c.Question]
	switch c.Op {
	case "answered":
		return answered
	case "not_answered":
		return !answered
	}
	if !answered {
		return c.Op == "neq" || c.Op == "not_in"
	}
	switch c.Op {
	case "eq":
		return matches(v, c.Value)
	case "neq":
		return !matches(v, c.Value)
	case "in", "not_in":
		list, _ := c.Value.([]any)
		hit := slices.ContainsFunc(list, func(x any) bool { return matches(v, x) })
		return hit == (c.Op == "in")
	case "gt", "gte", "lt", "lte":
		a, ok1 := number(v)
		b, ok2 := number(c.Value)
		if !ok1 || !ok2 {
			return false
		}
		switch c.Op {
		case "gt":
			return a > b
		case "gte":
			return a >= b
		case "lt":
			return a < b
		}
		return a <= b
	}
	return false
}

// matches compares an answer with a condition value; a multi-choice answer matches when it includes it.
func matches(answer, value any) bool {
	if list, ok := answer.([]string); ok {
		s, _ := value.(string)
		return slices.Contains(list, s)
	}
	if a, ok := number(answer); ok {
		b, ok := number(value)
		return ok && a == b
	}
	if b, ok := value.(bool); ok {
		s, _ := answer.(string)
		return (b && s == "yes") || (!b && s == "no")
	}
	return answer == value
}

func round(f float64) float64 { return math.Round(f*100) / 100 }

// Contribution is how much one answered question added to a score — the "why" behind a result (e.g. the factors
// of a breach risk assessment, BRE-05).
type Contribution struct {
	Question string  `json:"question"`
	Label    Text    `json:"label"`
	Answer   any     `json:"answer"`
	Points   float64 `json:"points"`
}

// Contributions lists, in form order, every answered question of a result (res.Answers holds only visible, valid
// answers) with the points it added.
func Contributions(s Schema, answers Answers) []Contribution {
	out := []Contribution{}
	for _, sec := range s.Sections {
		for _, q := range sec.Questions {
			v, ok := answers[q.Key]
			if !ok {
				continue
			}
			out = append(out, Contribution{Question: q.Key, Label: q.Label, Answer: v, Points: round(score(q, v))})
		}
	}
	return out
}
