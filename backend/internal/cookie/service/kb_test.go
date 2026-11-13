package service

import "testing"

func TestMatchesKBPattern(t *testing.T) {
	cases := []struct {
		name, pattern string
		want          bool
	}{
		{"_ga", "_ga", true},
		{"_ga_ABC123", "_ga", true},
		{"session_id", "_ga", false},
		{"", "_ga", false},
		{"_ga", "", false},
		{"fbp_123", `/^fbp/`, true},
		{"not_fbp", `/^fbp/`, false},
		{"anything", `/[/`, false}, // an invalid regex never matches, never panics
	}
	for _, c := range cases {
		if got := matchesKBPattern(c.name, c.pattern); got != c.want {
			t.Errorf("matchesKBPattern(%q, %q) = %v, want %v", c.name, c.pattern, got, c.want)
		}
	}
}
