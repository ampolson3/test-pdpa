package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	cookie "pdpa-platform/internal/cookie/service"
)

// TestPublicCookiePolicy is CON-05's own acceptance criterion ("ตารางในหน้า Cookie Policy ตรงกับผลสแกน
// ล่าสุด"): the policy table is a live read of this domain's active, catalogued cookies joined with their
// category names — so a newly catalogued cookie (whether entered by hand, CON-02, or auto-created by a
// CON-04 scan) appears immediately and a deleted one disappears immediately, with no separate snapshot
// that could ever go stale.
func TestPublicCookiePolicy(t *testing.T) {
	e := setup(t, "con05policy")
	var domainID, categoryID, gaCookieID uuid.UUID

	e.in(t, func(ctx context.Context) error {
		domainID, categoryID = e.withDomainAndCategory(t, ctx, "policy.example", "analytics")
		cookieEntry, err := e.svc.CreateCookie(ctx, domainID, cookie.CreateCookieInput{
			Name: "_ga", ProviderDomain: "google.com", CategoryID: &categoryID, StorageType: "http_cookie",
			Duration: "2 ปี", DescriptionTh: "ใช้วิเคราะห์การเข้าชม", DescriptionEn: "Used for traffic analysis",
		})
		if err != nil {
			return err
		}
		gaCookieID = cookieEntry.ID
		return nil
	})

	// Right after catalogueing it, the policy table already carries the cookie with its category's name.
	e.in(t, func(ctx context.Context) error {
		entries, err := e.svc.PublicCookiePolicy(ctx, domainID)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("entries = %+v, want exactly 1", entries)
		}
		got := entries[0]
		if got.Name != "_ga" || got.ProviderDomain != "google.com" || got.StorageType != "http_cookie" ||
			got.CategoryCode != "analytics" || got.CategoryNameEn != "Analytics" ||
			got.DescriptionTh != "ใช้วิเคราะห์การเข้าชม" || got.DescriptionEn != "Used for traffic analysis" {
			t.Errorf("entry = %+v, want it to match what was just catalogued", got)
		}
		return nil
	})

	// Deleting the cookie (the same effect a re-scan that no longer finds it would have) drops it from
	// the table immediately — no stale row left behind.
	e.in(t, func(ctx context.Context) error {
		if err := e.svc.DeleteCookie(ctx, gaCookieID); err != nil {
			return err
		}
		entries, err := e.svc.PublicCookiePolicy(ctx, domainID)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("entries after delete = %+v, want none", entries)
		}
		return nil
	})
}

// TestPublicCookiePolicy_TwoTenantIsolation proves the public read never leaks a cookie cataloged by a
// different tenant's own domain, even given its raw domain id.
func TestPublicCookiePolicy_TwoTenantIsolation(t *testing.T) {
	e1 := setup(t, "con05policya")
	e2 := setup(t, "con05policyb")
	var domainID uuid.UUID

	e1.in(t, func(ctx context.Context) error {
		dID, catID := e1.withDomainAndCategory(t, ctx, "tenant-a.example", "analytics")
		domainID = dID
		_, err := e1.svc.CreateCookie(ctx, dID, cookie.CreateCookieInput{
			Name: "_ga", ProviderDomain: "google.com", CategoryID: &catID, StorageType: "http_cookie",
		})
		return err
	})

	e2.in(t, func(ctx context.Context) error {
		entries, err := e2.svc.PublicCookiePolicy(ctx, domainID)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("tenant B must never see tenant A's own cookies: entries = %+v", entries)
		}
		return nil
	})
}
