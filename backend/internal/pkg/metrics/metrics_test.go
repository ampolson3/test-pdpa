package metrics

import (
	"testing"
	"time"
)

func TestBucketKey_FormatsPerMinute(t *testing.T) {
	ts := time.Date(2026, 3, 4, 5, 6, 0, 0, time.UTC)
	got := bucketKey("api", ts)
	want := "metrics:api:202603040506"
	if got != want {
		t.Fatalf("bucketKey: got %q, want %q", got, want)
	}
	// Seconds within the same minute collapse to the same bucket.
	if got2 := bucketKey("api", ts.Add(53*time.Second)); got2 != want {
		t.Fatalf("bucketKey within same minute: got %q, want %q", got2, want)
	}
	// A different minute is a different bucket.
	if got3 := bucketKey("api", ts.Add(time.Minute)); got3 == want {
		t.Fatalf("bucketKey across minutes: got the same key %q", got3)
	}
}

func TestAtoi64(t *testing.T) {
	cases := map[string]int64{
		"":     0,
		"0":    0,
		"42":   42,
		"1000": 1000,
	}
	for in, want := range cases {
		if got := atoi64(in); got != want {
			t.Errorf("atoi64(%q): got %d, want %d", in, got, want)
		}
	}
}
