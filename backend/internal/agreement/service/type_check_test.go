package service_test

import (
	"errors"
	"testing"

	agreementservice "pdpa-platform/internal/agreement/service"
)

// TestRecommendAgreementType_CorrectForEveryRole is DSA-01's acceptance criterion directly: the
// recommended agreement type (and its legal basis) is correct for every counterparty-role test case.
func TestRecommendAgreementType_CorrectForEveryRole(t *testing.T) {
	cases := []struct {
		role     string
		wantType string
		wantRef  string
	}{
		{"processor", "dpa", "ม.40"},
		{"controller", "dsa", "ม.27"},
		{"joint_controller", "joint_controller", "ม.27"},
	}
	for _, c := range cases {
		rec, err := agreementservice.RecommendAgreementType(c.role)
		if err != nil {
			t.Fatalf("role=%q: unexpected error %v", c.role, err)
		}
		if rec.AgreementType != c.wantType {
			t.Errorf("role=%q: agreement_type = %q, want %q", c.role, rec.AgreementType, c.wantType)
		}
		if rec.LegalRef != c.wantRef {
			t.Errorf("role=%q: legal_ref = %q, want %q", c.role, rec.LegalRef, c.wantRef)
		}
		if rec.ReasonCode == "" {
			t.Errorf("role=%q: reason_code is empty", c.role)
		}
	}
}

func TestRecommendAgreementType_UnknownRoleRefused(t *testing.T) {
	if _, err := agreementservice.RecommendAgreementType("vendor"); !errors.Is(err, agreementservice.ErrInvalid) {
		t.Errorf("unknown role: %v, want ErrInvalid", err)
	}
	if _, err := agreementservice.RecommendAgreementType(""); !errors.Is(err, agreementservice.ErrInvalid) {
		t.Errorf("empty role: %v, want ErrInvalid", err)
	}
}
