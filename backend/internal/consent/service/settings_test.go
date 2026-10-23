package service_test

import (
	"context"
	"testing"

	"pdpa-platform/internal/consent/consenttest"
	"pdpa-platform/internal/pkg/dbtest"
)

// TestGetSettings_ReportsRegionAndEncryption is CON-17's own test evidence (the module doc's
// "ยังไม่ทำ: ... หลักฐานทดสอบ region"): proves GetSettings reads each tenant's *own*
// platform.tenants.data_region — not a hardcoded value — and never leaks another tenant's region,
// the same way every other cross-tenant read in this codebase is proven. Deployment-level region
// enforcement (a real per-region database or bucket) stays deferred, the same way PLT-02 defers
// DB-per-tenant deployment: there is no multi-region infrastructure in this environment to test
// against (docs/decisions.md Q-02 is still open).
func TestGetSettings_ReportsRegionAndEncryption(t *testing.T) {
	f := consenttest.Setup(t)

	// Tenant A keeps the seeded default (TH); tenant B is onboarded with data kept elsewhere.
	platform := dbtest.PlatformPool(t)
	if _, err := platform.Exec(context.Background(), `UPDATE platform.tenants SET data_region = 'SG' WHERE id = $1`, f.B.ID); err != nil {
		t.Fatal(err)
	}

	f.As(t, f.A, f.Alice, nil, consenttest.Maker, func(ctx context.Context) error {
		s, err := f.Svc.GetSettings(ctx)
		if err != nil {
			return err
		}
		if s.DataRegion != "TH" {
			t.Errorf("tenant A data_region = %q, want TH", s.DataRegion)
		}
		if s.KeyManagement != "local_development" {
			t.Errorf("key_management = %q, want local_development (LocalKEK in tests)", s.KeyManagement)
		}
		return nil
	})

	f.As(t, f.B, f.B.UserID, nil, consenttest.Maker, func(ctx context.Context) error {
		s, err := f.Svc.GetSettings(ctx)
		if err != nil {
			return err
		}
		if s.DataRegion != "SG" {
			t.Errorf("tenant B data_region = %q, want SG — got tenant A's region or the wrong value", s.DataRegion)
		}
		return nil
	})
}
