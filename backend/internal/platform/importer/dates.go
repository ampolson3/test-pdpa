package importer

import (
	"errors"
	"regexp"
	"strconv"
	"time"
)

// ErrBadDate is returned by ParseDate for text that is not a date in an accepted form.
var ErrBadDate = errors.New("importer: not a date (use YYYY-MM-DD or D/M/YYYY)")

var dmy = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})$`)

// ParseDate reads a date cell as import types commonly receive it: ISO "2026-04-13"; day first
// "13/4/2026" (also with "-" or "."), where a year of 2400 or more is Buddhist Era ("13/4/2569");
// or an Excel serial date ("46125"), which is how an .xlsx date cell arrives. Month-first forms are
// not accepted — they are ambiguous with day-first ones. The result is midnight UTC of that date.
func ParseDate(v string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, nil
	}
	if m := dmy.FindStringSubmatch(v); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		if y >= 2400 {
			y -= 543
		}
		t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
		if t.Day() != d || int(t.Month()) != mo { // 31/2 would roll over into March
			return time.Time{}, ErrBadDate
		}
		return t, nil
	}
	// Excel serials count days from 1899-12-30; 1 January 1950 … 31 December 2199.
	if n, err := strconv.Atoi(v); err == nil && n >= 18264 && n <= 109939 {
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n), nil
	}
	return time.Time{}, ErrBadDate
}
