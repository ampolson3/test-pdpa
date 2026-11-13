package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/forms"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

// lowAnswers/criticalAnswers exercise the seeded migration 00058 bands directly: low = 0 points total,
// critical = max points on every question (9 or above, no upper bound).
var lowAnswers = forms.Answers{"data_volume": "small", "sensitive_data": "no", "system_access_level": "none", "cross_border_transfer": "no"}
var criticalAnswers = forms.Answers{"data_volume": "very_large", "sensitive_data": "yes", "system_access_level": "admin", "cross_border_transfer": "yes"}

func TestRecordIntake_ComputesTierAndRequiredAssessments(t *testing.T) {
	e := setup(t, "venintake")
	var party orgservice.ExternalParty
	var vendor vendorservice.Vendor
	e.in(t, func(ctx context.Context) error {
		var err error
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "คู่ค้าทดสอบ", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		vendor, err = e.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "ทดสอบการประเมินความเสี่ยง"}, 0)
		return err
	})

	e.in(t, func(ctx context.Context) error {
		in, required, err := e.svc.RecordIntake(ctx, vendor.ID, criticalAnswers)
		if err != nil {
			return err
		}
		if in.TierResult != "critical" {
			t.Errorf("tier = %q, want critical", in.TierResult)
		}
		if in.InherentScore < 9 {
			t.Errorf("score = %v, want >= 9", in.InherentScore)
		}
		wantCodes := map[string]bool{"vendor_pdpa": true, "vendor_security": true, "vendor_transfer": true}
		if len(required) != len(wantCodes) {
			t.Errorf("required codes = %v, want %v", required, wantCodes)
		}
		for _, c := range required {
			if !wantCodes[c] {
				t.Errorf("unexpected required code %q", c)
			}
		}
		got, err := e.svc.GetVendor(ctx, vendor.ID)
		if err != nil {
			return err
		}
		if got.Tier == nil || *got.Tier != "critical" {
			t.Errorf("vendor.tier = %v, want critical", got.Tier)
		}
		if got.NextAssessmentAt == nil {
			t.Error("next_assessment_at was not set")
		}
		return nil
	})

	// A low-risk answer set lands "low" and needs no assessment template at all.
	var vendor2 vendorservice.Vendor
	e.in(t, func(ctx context.Context) error {
		party2, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "คู่ค้าความเสี่ยงต่ำ", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		vendor2, err = e.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party2.ID, ServiceDescription: "ความเสี่ยงต่ำ"}, 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		in, required, err := e.svc.RecordIntake(ctx, vendor2.ID, lowAnswers)
		if err != nil {
			return err
		}
		if in.TierResult != "low" {
			t.Errorf("tier = %q, want low", in.TierResult)
		}
		if len(required) != 0 {
			t.Errorf("required codes = %v, want none", required)
		}
		return nil
	})
}

func TestRecordIntake_ValidationAndUnknownVendor(t *testing.T) {
	e := setup(t, "venintakeval")
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.RecordIntake(ctx, uuid.New(), lowAnswers); !errors.Is(err, vendorservice.ErrNotFound) {
			t.Errorf("unknown vendor: %v, want ErrNotFound", err)
		}
		return nil
	})

	var party orgservice.ExternalParty
	var vendor vendorservice.Vendor
	e.in(t, func(ctx context.Context) error {
		var err error
		party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "คู่ค้า", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		vendor, err = e.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "x"}, 0)
		return err
	})
	e.in(t, func(ctx context.Context) error {
		incomplete := forms.Answers{"data_volume": "small"}
		if _, _, err := e.svc.RecordIntake(ctx, vendor.ID, incomplete); !errors.Is(err, vendorservice.ErrInvalid) {
			t.Errorf("missing required answers: %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestListIntakes_NewestFirstAndIsolation(t *testing.T) {
	a := setup(t, "venintakea")
	b := setup(t, "venintakeb")
	var vendor vendorservice.Vendor
	a.in(t, func(ctx context.Context) error {
		party, err := a.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "เอ", CountryCode: "US"}, 0)
		if err != nil {
			return err
		}
		vendor, err = a.svc.SaveVendor(ctx, vendorservice.Vendor{PartyID: party.ID, ServiceDescription: "เอ"}, 0)
		if err != nil {
			return err
		}
		if _, _, err := a.svc.RecordIntake(ctx, vendor.ID, lowAnswers); err != nil {
			return err
		}
		_, _, err = a.svc.RecordIntake(ctx, vendor.ID, criticalAnswers)
		return err
	})
	a.in(t, func(ctx context.Context) error {
		list, err := a.svc.ListIntakes(ctx, vendor.ID)
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Fatalf("len(list) = %d, want 2", len(list))
		}
		if list[0].TierResult != "critical" || list[1].TierResult != "low" {
			t.Errorf("not newest-first: %+v", list)
		}
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.ListIntakes(ctx, vendor.ID); !errors.Is(err, vendorservice.ErrNotFound) {
			t.Errorf("tenant B should not see tenant A's vendor's intakes: %v", err)
		}
		return nil
	})
}

func TestRequiredAssessmentCodes(t *testing.T) {
	cases := []struct {
		tier        string
		crossBorder bool
		want        []string
	}{
		{"low", false, nil},
		{"low", true, []string{"vendor_transfer"}},
		{"medium", false, []string{"vendor_pdpa"}},
		{"high", false, []string{"vendor_pdpa", "vendor_security"}},
		{"critical", true, []string{"vendor_pdpa", "vendor_security", "vendor_transfer"}},
	}
	for _, c := range cases {
		got := vendorservice.RequiredAssessmentCodes(c.tier, c.crossBorder)
		if len(got) != len(c.want) {
			t.Errorf("tier=%s crossBorder=%v: got %v, want %v", c.tier, c.crossBorder, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("tier=%s crossBorder=%v: got %v, want %v", c.tier, c.crossBorder, got, c.want)
				break
			}
		}
	}
}
