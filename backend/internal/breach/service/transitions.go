package service

// Incident statuses (docs/states/state-machines.yaml ST-03#1).
const (
	StatusReported    = "reported"
	StatusTriage      = "triage"
	StatusAssessing   = "assessing"
	StatusNotifying   = "notifying"
	StatusRemediating = "remediating"
	StatusClosed      = "closed"
)

// allowed is ST-03#1, the allow-list of status changes.
var allowed = map[[2]string]bool{
	{StatusReported, StatusTriage}:       true,
	{StatusTriage, StatusAssessing}:      true,
	{StatusAssessing, StatusNotifying}:   true,
	{StatusAssessing, StatusRemediating}: true,
	{StatusRemediating, StatusAssessing}: true,
	{StatusNotifying, StatusRemediating}: true,
	{StatusTriage, StatusClosed}:         true,
	{StatusRemediating, StatusClosed}:    true,
}

// Allowed reports whether ST-03 allows from → to.
func Allowed(from, to string) bool { return allowed[[2]string{from, to}] }

// Risk levels and notification decisions (BRE-05 / 06).
const (
	RiskNone = "none"
	RiskLow  = "low"
	RiskHigh = "high"

	DecisionNone            = "no_notification"
	DecisionPDPC            = "notify_pdpc"
	DecisionPDPCAndSubjects = "notify_pdpc_and_subjects"
)

var decisionRank = map[string]int{DecisionNone: 0, DecisionPDPC: 1, DecisionPDPCAndSubjects: 2}

// RequiredDecision is what a risk level requires by law (s.37(4)): none → no notice, any risk → the PDPC, high risk →
// the PDPC and the data subjects.
func RequiredDecision(risk string) string {
	switch risk {
	case RiskHigh:
		return DecisionPDPCAndSubjects
	case RiskLow:
		return DecisionPDPC
	default:
		return DecisionNone
	}
}

// DecisionAllowed: the DPO may notify more than the risk requires, never less.
func DecisionAllowed(risk, decision string) bool {
	r, ok := decisionRank[decision]
	return ok && r >= decisionRank[RequiredDecision(risk)]
}

// timerRunning: the 72-hour clock runs from the report until the PDPC is notified, unless the decision is that no
// notice is needed or the incident is closed.
func timerRunning(status string, decision *string) bool {
	if decision != nil && *decision == DecisionNone {
		return false
	}
	switch status {
	case StatusReported, StatusTriage, StatusAssessing, StatusNotifying:
		return true
	}
	return false
}
