package importer

import (
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	want := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	for _, v := range []string{"2026-04-13", "13/4/2026", "13/04/2026", "13-04-2026", "13.4.2026", "13/4/2569", "46125"} {
		got, err := ParseDate(v)
		if err != nil || !got.Equal(want) {
			t.Errorf("%q: %v %v", v, got, err)
		}
	}
	for _, v := range []string{"", "2026-13-01", "31/2/2026", "4/13/26", "13 เมษายน 2569", "12", "abc"} {
		if _, err := ParseDate(v); err == nil {
			t.Errorf("%q accepted", v)
		}
	}
}
