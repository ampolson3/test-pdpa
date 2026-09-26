package service

// Consent status (docs/states/state-machines.yaml ST-01#1) and transaction types.
const (
	StatusActive    = "ACTIVE"
	StatusNotGiven  = "NOT_GIVEN"
	StatusWithdrawn = "WITHDRAWN"
	StatusExpired   = "EXPIRED"
	StatusPending   = "PENDING"

	TxConsented          = "CONSENTED"
	TxNotConsented       = "NOT_CONSENTED"
	TxWithdrawn          = "WITHDRAWN"
	TxExtended           = "EXTENDED"
	TxChangedPreferences = "CHANGED_PREFERENCES"
)

// allowed is ST-01#1: from → to. Initial states are reached from "" (no status yet).
var allowed = map[[2]string]bool{
	{"", StatusActive}: true, {"", StatusNotGiven}: true, {"", StatusPending}: true,
	{StatusPending, StatusActive}: true, {StatusActive, StatusPending}: true, {StatusPending, StatusNotGiven}: true,
	{StatusNotGiven, StatusActive}: true, {StatusActive, StatusWithdrawn}: true, {StatusWithdrawn, StatusActive}: true,
	{StatusActive, StatusExpired}: true, {StatusExpired, StatusActive}: true, {StatusActive, StatusActive}: true,
}

// Decide maps a data subject's decision on a purpose to the transaction recorded and the resulting status,
// given the current status ("" when there is none). A decision that repeats a refusal changes nothing but is
// still recorded. Leaving an ACTIVE purpose unticked is no decision at all (txType ""): ST-01 has no
// NOT_CONSENTED transition out of ACTIVE, and an unverified web form must not withdraw someone's consent —
// withdrawal is its own WITHDRAWN decision (decisions.md Q-21). Confirming an ACTIVE consent (same or newer
// version) renews it (EXTENDED), or records the new preferences. Withdrawing what isn't given is
// ErrInvalidTransition.
func Decide(current, decision string, preferencesChanged bool) (txType, status string, err error) {
	switch decision {
	case TxConsented:
		switch current {
		case "", StatusNotGiven, StatusWithdrawn, StatusExpired:
			txType, status = TxConsented, StatusActive
		case StatusActive:
			txType, status = TxExtended, StatusActive
			if preferencesChanged {
				txType = TxChangedPreferences
			}
		default: // PENDING waits for its confirmation flow (double opt-in / re-consent), not built yet
			return "", "", ErrInvalidTransition
		}
	case TxNotConsented:
		switch current {
		case "":
			txType, status = TxNotConsented, StatusNotGiven
		case StatusActive:
			return "", StatusActive, nil
		case StatusNotGiven, StatusWithdrawn, StatusExpired:
			txType, status = TxNotConsented, current
		default:
			return "", "", ErrInvalidTransition
		}
	case TxWithdrawn:
		if current != StatusActive {
			return "", "", ErrInvalidTransition
		}
		txType, status = TxWithdrawn, StatusWithdrawn
	default:
		return "", "", invalid("decision %q", decision)
	}
	if status != current && !allowed[[2]string{current, status}] {
		return "", "", ErrInvalidTransition
	}
	return txType, status, nil
}

// eventFor is the domain event a transaction publishes (docs/architecture/events.yaml).
func eventFor(txType string) string {
	switch txType {
	case TxConsented, TxExtended:
		return "consent.granted"
	case TxNotConsented:
		return "consent.denied"
	case TxWithdrawn:
		return "consent.withdrawn"
	case TxChangedPreferences:
		return "consent.preferences_changed"
	}
	return ""
}
