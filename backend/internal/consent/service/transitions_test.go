package service

import (
	"errors"
	"testing"
)

func TestDecide(t *testing.T) {
	for _, c := range []struct {
		current, decision string
		prefs, guardian   bool
		tx, status        string
		err               error
	}{
		{"", TxConsented, false, false, TxConsented, StatusActive, nil},
		{"", TxNotConsented, false, false, TxNotConsented, StatusNotGiven, nil},
		{StatusNotGiven, TxConsented, false, false, TxConsented, StatusActive, nil},
		{StatusNotGiven, TxNotConsented, false, false, TxNotConsented, StatusNotGiven, nil},
		{StatusActive, TxConsented, false, false, TxExtended, StatusActive, nil},
		{StatusActive, TxConsented, true, false, TxChangedPreferences, StatusActive, nil},
		{StatusActive, TxNotConsented, false, false, "", StatusActive, nil}, // unticked on a form: no change (withdrawal is explicit)
		{StatusActive, TxWithdrawn, false, false, TxWithdrawn, StatusWithdrawn, nil},
		{StatusWithdrawn, TxConsented, false, false, TxConsented, StatusActive, nil},
		{StatusWithdrawn, TxNotConsented, false, false, TxNotConsented, StatusWithdrawn, nil},
		{StatusExpired, TxConsented, false, false, TxConsented, StatusActive, nil},
		{StatusWithdrawn, TxWithdrawn, false, false, "", "", ErrInvalidTransition},
		{"", TxWithdrawn, false, false, "", "", ErrInvalidTransition},
		{StatusNotGiven, TxWithdrawn, false, false, "", "", ErrInvalidTransition},
		{StatusPending, TxConsented, false, false, "", "", ErrInvalidTransition},
		{"", "MAYBE", false, false, "", "", ErrInvalidRequest},
		// CON-11 (ม.20): a brand-new subject's first CONSENTED decision on a guardian-gated purpose records
		// PENDING, not ACTIVE.
		{"", TxConsented, false, true, TxPending, StatusPending, nil},
		// A subject with any prior status skips the gate (ST-01 has no edge back into PENDING from these).
		{StatusNotGiven, TxConsented, false, true, TxConsented, StatusActive, nil},
		{StatusWithdrawn, TxConsented, false, true, TxConsented, StatusActive, nil},
		{StatusExpired, TxConsented, false, true, TxConsented, StatusActive, nil},
		{StatusActive, TxConsented, false, true, TxExtended, StatusActive, nil},
	} {
		tx, st, err := Decide(c.current, c.decision, c.prefs, c.guardian)
		if (c.err == nil) != (err == nil) || (c.err != nil && !errors.Is(err, c.err)) || tx != c.tx || st != c.status {
			t.Errorf("%s + %s (prefs %v, guardian %v) = %s %s %v, want %s %s %v", c.current, c.decision, c.prefs, c.guardian, tx, st, err, c.tx, c.status, c.err)
		}
	}
}
