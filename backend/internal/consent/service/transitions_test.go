package service

import (
	"errors"
	"testing"
)

func TestDecide(t *testing.T) {
	for _, c := range []struct {
		current, decision        string
		prefs, guardian, doubleO bool
		tx, status               string
		err                      error
	}{
		{"", TxConsented, false, false, false, TxConsented, StatusActive, nil},
		{"", TxNotConsented, false, false, false, TxNotConsented, StatusNotGiven, nil},
		{StatusNotGiven, TxConsented, false, false, false, TxConsented, StatusActive, nil},
		{StatusNotGiven, TxNotConsented, false, false, false, TxNotConsented, StatusNotGiven, nil},
		{StatusActive, TxConsented, false, false, false, TxExtended, StatusActive, nil},
		{StatusActive, TxConsented, true, false, false, TxChangedPreferences, StatusActive, nil},
		{StatusActive, TxNotConsented, false, false, false, "", StatusActive, nil}, // unticked on a form: no change (withdrawal is explicit)
		{StatusActive, TxWithdrawn, false, false, false, TxWithdrawn, StatusWithdrawn, nil},
		{StatusWithdrawn, TxConsented, false, false, false, TxConsented, StatusActive, nil},
		{StatusWithdrawn, TxNotConsented, false, false, false, TxNotConsented, StatusWithdrawn, nil},
		{StatusExpired, TxConsented, false, false, false, TxConsented, StatusActive, nil},
		{StatusWithdrawn, TxWithdrawn, false, false, false, "", "", ErrInvalidTransition},
		{"", TxWithdrawn, false, false, false, "", "", ErrInvalidTransition},
		{StatusNotGiven, TxWithdrawn, false, false, false, "", "", ErrInvalidTransition},
		{StatusPending, TxConsented, false, false, false, "", "", ErrInvalidTransition},
		{"", "MAYBE", false, false, false, "", "", ErrInvalidRequest},
		// CON-11 (ม.20): a brand-new subject's first CONSENTED decision on a guardian-gated purpose records
		// PENDING, not ACTIVE.
		{"", TxConsented, false, true, false, TxPending, StatusPending, nil},
		// A subject with any prior status skips the gate (ST-01 has no edge back into PENDING from these).
		{StatusNotGiven, TxConsented, false, true, false, TxConsented, StatusActive, nil},
		{StatusWithdrawn, TxConsented, false, true, false, TxConsented, StatusActive, nil},
		{StatusExpired, TxConsented, false, true, false, TxConsented, StatusActive, nil},
		{StatusActive, TxConsented, false, true, false, TxExtended, StatusActive, nil},
		// CON-23: a brand-new subject's first CONSENTED decision at a double-opt-in-gated purpose/collection
		// point also records PENDING until confirmed.
		{"", TxConsented, false, false, true, TxPending, StatusPending, nil},
		{StatusNotGiven, TxConsented, false, false, true, TxConsented, StatusActive, nil},
		// Guardian always wins when both would apply to the same first-ever decision.
		{"", TxConsented, false, true, true, TxPending, StatusPending, nil},
	} {
		tx, st, err := Decide(c.current, c.decision, c.prefs, c.guardian, c.doubleO)
		if (c.err == nil) != (err == nil) || (c.err != nil && !errors.Is(err, c.err)) || tx != c.tx || st != c.status {
			t.Errorf("%s + %s (prefs %v, guardian %v, doubleO %v) = %s %s %v, want %s %s %v", c.current, c.decision, c.prefs, c.guardian, c.doubleO, tx, st, err, c.tx, c.status, c.err)
		}
	}
}
