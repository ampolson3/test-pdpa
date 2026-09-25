package service

import (
	"errors"
	"testing"
)

func TestDecide(t *testing.T) {
	for _, c := range []struct {
		current, decision string
		prefs             bool
		tx, status        string
		err               error
	}{
		{"", TxConsented, false, TxConsented, StatusActive, nil},
		{"", TxNotConsented, false, TxNotConsented, StatusNotGiven, nil},
		{StatusNotGiven, TxConsented, false, TxConsented, StatusActive, nil},
		{StatusNotGiven, TxNotConsented, false, TxNotConsented, StatusNotGiven, nil},
		{StatusActive, TxConsented, false, TxExtended, StatusActive, nil},
		{StatusActive, TxConsented, true, TxChangedPreferences, StatusActive, nil},
		{StatusActive, TxNotConsented, false, TxWithdrawn, StatusWithdrawn, nil}, // unticking = withdrawing
		{StatusActive, TxWithdrawn, false, TxWithdrawn, StatusWithdrawn, nil},
		{StatusWithdrawn, TxConsented, false, TxConsented, StatusActive, nil},
		{StatusWithdrawn, TxNotConsented, false, TxNotConsented, StatusWithdrawn, nil},
		{StatusExpired, TxConsented, false, TxConsented, StatusActive, nil},
		{StatusWithdrawn, TxWithdrawn, false, "", "", ErrInvalidTransition},
		{"", TxWithdrawn, false, "", "", ErrInvalidTransition},
		{StatusNotGiven, TxWithdrawn, false, "", "", ErrInvalidTransition},
		{StatusPending, TxConsented, false, "", "", ErrInvalidTransition},
		{"", "MAYBE", false, "", "", ErrInvalidRequest},
	} {
		tx, st, err := Decide(c.current, c.decision, c.prefs)
		if (c.err == nil) != (err == nil) || (c.err != nil && !errors.Is(err, c.err)) || tx != c.tx || st != c.status {
			t.Errorf("%s + %s (prefs %v) = %s %s %v, want %s %s %v", c.current, c.decision, c.prefs, tx, st, err, c.tx, c.status, c.err)
		}
	}
}
