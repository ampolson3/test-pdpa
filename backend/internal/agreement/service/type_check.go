package service

import "fmt"

// AgreementTypeRecommendation is DSA-01's whole feature: ask one question — the data recipient's role —
// and recommend which kind of agreement is required, with the PDPA section that requires it. A
// processor acting on our instructions needs a DPA (ม.40); another independent controller receiving or
// sharing data with us needs a DSA (ม.27); a joint-controller arrangement gets its own agreement_type
// (already a real value in agreement.agreements' own CHECK constraint, migration 00015) rather than
// being folded into "dsa" — the two are different legal relationships even though both arise under ม.27.
type AgreementTypeRecommendation struct {
	AgreementType string
	LegalRef      string
	ReasonCode    string
}

// RecommendAgreementType is a pure function — no transaction, no database lookup, no tenant context —
// so the acceptance criterion ("ผลแนะนำถูกต้องตามบทบาททุกกรณีทดสอบ": the correct recommendation for every
// test case of counterparty role) is a direct table test with no fixture needed.
func RecommendAgreementType(counterpartyRole string) (AgreementTypeRecommendation, error) {
	switch counterpartyRole {
	case "processor":
		return AgreementTypeRecommendation{AgreementType: "dpa", LegalRef: "ม.40", ReasonCode: "processor"}, nil
	case "controller":
		return AgreementTypeRecommendation{AgreementType: "dsa", LegalRef: "ม.27", ReasonCode: "controller"}, nil
	case "joint_controller":
		return AgreementTypeRecommendation{AgreementType: "joint_controller", LegalRef: "ม.27", ReasonCode: "joint_controller"}, nil
	}
	return AgreementTypeRecommendation{}, fmt.Errorf("%w: counterparty_role", ErrInvalid)
}
