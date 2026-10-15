package service

import "testing"

func TestFuzzyBestMatch(t *testing.T) {
	candidates := []string{"ฝ่ายทรัพยากรบุคคล", "ฝ่ายการเงิน", "ฝ่ายขาย"}

	if idx, ok := fuzzyBestMatch("ฝ่ายทรัพยากรบุคคล", candidates); !ok || idx != 0 {
		t.Fatalf("exact match: got idx=%d ok=%v, want 0/true", idx, ok)
	}
	// A near-miss (one character off) should still resolve via edit distance.
	if idx, ok := fuzzyBestMatch("ฝายทรัพยากรบุคคล", candidates); !ok || idx != 0 {
		t.Fatalf("near match: got idx=%d ok=%v, want 0/true", idx, ok)
	}
	// Case/spacing-insensitive exact match.
	if idx, ok := fuzzyBestMatch("  ฝ่ายขาย  ", candidates); !ok || idx != 2 {
		t.Fatalf("normalized exact match: got idx=%d ok=%v, want 2/true", idx, ok)
	}
	// Nothing in common with any candidate must not silently pick one.
	if _, ok := fuzzyBestMatch("completely unrelated text", candidates); ok {
		t.Fatal("unrelated query matched a candidate, want no match")
	}
	// Empty query never matches.
	if _, ok := fuzzyBestMatch("", candidates); ok {
		t.Fatal("empty query matched, want no match")
	}
	// Empty candidate list never matches.
	if _, ok := fuzzyBestMatch("anything", nil); ok {
		t.Fatal("empty candidate list matched, want no match")
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"same", "same", 0},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q, %q): got %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
