package service_test

import (
	"context"
	"testing"
)

var wantRequestTypeCodes = []string{"access", "portability", "objection", "erasure", "restriction",
	"rectification", "withdraw_consent", "complaint", "inquiry"}

// TestListRequestTypes_CoversAllNineCodes: the 9 fixed dsar.requests.request_type_id codes are all seeded
// (migration 00043) and visible to every tenant.
func TestListRequestTypes_CoversAllNineCodes(t *testing.T) {
	e := setup(t, "dsartypes")
	var codes []string
	e.in(t, func(ctx context.Context) error {
		types, err := e.svc.ListRequestTypes(ctx)
		if err != nil {
			return err
		}
		for _, rt := range types {
			codes = append(codes, rt.Code)
			if rt.NameTh == "" || rt.NameEn == "" {
				t.Errorf("type %s missing a th/en name", rt.Code)
			}
			if rt.SLADays != 30 {
				t.Errorf("type %s sla_days = %d, want 30", rt.Code, rt.SLADays)
			}
		}
		return nil
	})
	for _, want := range wantRequestTypeCodes {
		found := false
		for _, c := range codes {
			if c == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing request type code %q", want)
		}
	}
}
