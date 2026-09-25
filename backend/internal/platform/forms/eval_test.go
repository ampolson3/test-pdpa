package forms

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sort"
	"testing"
)

type fixture struct {
	Schema  Schema   `json:"schema"`
	Scoring *Scoring `json:"scoring"`
	Cases   []struct {
		Name       string       `json:"name"`
		Answers    Answers      `json:"answers"`
		RequireAll bool         `json:"require_all"`
		Sections   []string     `json:"sections"`
		Visible    []string     `json:"visible"`
		Errors     []FieldError `json:"errors"`
		AnswerKeys []string     `json:"answer_keys"`
		Score      float64      `json:"score"`
		MaxScore   float64      `json:"max_score"`
		Band       string       `json:"band"`
	} `json:"cases"`
}

func loadFixture(t *testing.T) fixture {
	t.Helper()
	raw, err := os.ReadFile("../../../../packages/form-renderer/src/fixtures/engine-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// Acceptance (PLT-06): conditions show and hide questions and sections, and the score is computed —
// the same results the TypeScript renderer gets from this fixture.
func TestEvaluate_SharedFixture(t *testing.T) {
	f := loadFixture(t)
	if err := Validate(f.Schema, f.Scoring); err != nil {
		t.Fatalf("fixture schema: %v", err)
	}
	for _, c := range f.Cases {
		r := Evaluate(f.Schema, f.Scoring, c.Answers, c.RequireAll, c.Sections)
		keys := make([]string, 0, len(r.Answers))
		for k := range r.Answers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if !reflect.DeepEqual(r.Visible, c.Visible) {
			t.Errorf("%s: visible %v, want %v", c.Name, r.Visible, c.Visible)
		}
		if !reflect.DeepEqual(r.Errors, append([]FieldError{}, c.Errors...)) {
			t.Errorf("%s: errors %v, want %v", c.Name, r.Errors, c.Errors)
		}
		if !reflect.DeepEqual(keys, append([]string{}, c.AnswerKeys...)) {
			t.Errorf("%s: answers %v, want %v", c.Name, keys, c.AnswerKeys)
		}
		if r.Score != c.Score || r.MaxScore != c.MaxScore || r.Band != c.Band {
			t.Errorf("%s: score %v/%v band %q, want %v/%v %q", c.Name, r.Score, r.MaxScore, r.Band, c.Score, c.MaxScore, c.Band)
		}
	}
}

func TestValidate_RejectsBadSchemas(t *testing.T) {
	f := loadFixture(t)
	raw, _ := json.Marshal(f.Schema)
	clone := func() Schema {
		var s Schema
		_ = json.Unmarshal(raw, &s)
		return s
	}
	five := 5.0
	cases := map[string]func(s *Schema, sc *Scoring){
		"no sections":            func(s *Schema, _ *Scoring) { s.Sections = nil },
		"duplicate section":      func(s *Schema, _ *Scoring) { s.Sections[1].Key = "basic" },
		"duplicate question":     func(s *Schema, _ *Scoring) { s.Sections[2].Questions[0].Key = "org_name" },
		"bad key":                func(s *Schema, _ *Scoring) { s.Sections[0].Questions[0].Key = "Org Name" },
		"unknown type":           func(s *Schema, _ *Scoring) { s.Sections[0].Questions[0].Type = "signature" },
		"no thai label":          func(s *Schema, _ *Scoring) { s.Sections[0].Questions[0].Label = Text{"en": "x"} },
		"choice without options": func(s *Schema, _ *Scoring) { s.Sections[2].Questions[1].Options = nil },
		"duplicate option":       func(s *Schema, _ *Scoring) { s.Sections[2].Questions[1].Options[1].Value = "internal" },
		"options on text": func(s *Schema, _ *Scoring) {
			s.Sections[0].Questions[0].Options = []Option{{Value: "a", Label: Text{"th": "a"}}}
		},
		"yes_no other value": func(s *Schema, _ *Scoring) { s.Sections[0].Questions[1].Options[0].Value = "maybe" },
		"min above max": func(s *Schema, _ *Scoring) {
			s.Sections[0].Questions[3].Min = &five
			m := 1.0
			s.Sections[0].Questions[3].Max = &m
		},
		"range on text": func(s *Schema, _ *Scoring) { s.Sections[0].Questions[0].Min = &five },
		"forward reference": func(s *Schema, _ *Scoring) {
			s.Sections[0].Questions[0].VisibleIf = &Condition{Question: "subjects", Op: "answered"}
		},
		"self reference": func(s *Schema, _ *Scoring) {
			s.Sections[0].Questions[3].VisibleIf = &Condition{Question: "subjects", Op: "answered"}
		},
		"unknown op":      func(s *Schema, _ *Scoring) { s.Sections[1].VisibleIf.Op = "like" },
		"in without list": func(s *Schema, _ *Scoring) { s.Sections[2].Questions[2].VisibleIf.Value = "cloud" },
		"gt on text": func(s *Schema, _ *Scoring) {
			s.Sections[1].VisibleIf = &Condition{Question: "org_name", Op: "gt", Value: 1.0}
		},
		"mixed condition":   func(s *Schema, _ *Scoring) { s.Sections[2].Questions[1].VisibleIf.Op = "eq" },
		"overlapping bands": func(_ *Schema, sc *Scoring) { sc.Bands[1].Min = 5 },
	}
	for name, mutate := range cases {
		s := clone()
		sc := &Scoring{}
		scRaw, _ := json.Marshal(f.Scoring)
		_ = json.Unmarshal(scRaw, sc)
		mutate(&s, sc)
		if err := Validate(s, sc); !errors.Is(err, ErrInvalidSchema) {
			t.Errorf("%s: %v, want ErrInvalidSchema", name, err)
		}
	}
}
