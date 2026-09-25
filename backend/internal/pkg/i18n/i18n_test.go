package i18n_test

import (
	"testing"

	"pdpa-platform/internal/pkg/i18n"
)

func TestFromAcceptLanguage(t *testing.T) {
	cases := map[string]i18n.Lang{
		"":               i18n.Th, // default (decisions.md Q-17)
		"th":             i18n.Th,
		"TH":             i18n.Th,
		"en":             i18n.En,
		"En":             i18n.En,
		" en ":           i18n.En,
		"en-US,en;q=0.9": i18n.Th, // not one of the two literal values the spec declares -> fallback
		"fr":             i18n.Th,
	}
	for header, want := range cases {
		if got := i18n.FromAcceptLanguage(header); got != want {
			t.Errorf("FromAcceptLanguage(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestProblemTitle(t *testing.T) {
	title, ok := i18n.ProblemTitle("authn.required", i18n.Th)
	if !ok || title == "" {
		t.Fatalf("expected a Thai title for authn.required, got %q ok=%v", title, ok)
	}

	title, ok = i18n.ProblemTitle("authn.required", i18n.En)
	if !ok || title != "Authentication required" {
		t.Fatalf("expected English title 'Authentication required', got %q ok=%v", title, ok)
	}

	// dsar.invalid_transition isn't a real module code yet, but the *.invalid_transition
	// suffix must still resolve — every future module gets this for free.
	if _, ok := i18n.ProblemTitle("dsar.invalid_transition", i18n.En); !ok {
		t.Fatal("expected *.invalid_transition suffix to resolve for any module")
	}

	if _, ok := i18n.ProblemTitle("consent.purpose_inactive", i18n.Th); ok {
		t.Fatal("business-rule codes aren't cataloged yet — expected ok=false")
	}
}
