package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	cookiescan "pdpa-platform/internal/cookie/scan"
	cookie "pdpa-platform/internal/cookie/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

var scanPerms = []string{"cookie.banner.create", "cookie.banner.read", "cookie.scan.create", "cookie.scan.read", "cookie.scan.update"}

// fakeCrawler lets tests drive RunScan deterministically without a real browser.
type fakeCrawler struct {
	result cookiescan.Result
	err    error
}

func (f fakeCrawler) Crawl(_ context.Context, _ cookiescan.Config) (cookiescan.Result, error) {
	return f.result, f.err
}

func scanEnv(t *testing.T, suffix string, crawler cookiescan.Crawler) env {
	t.Helper()
	e := setup(t, suffix)
	e.svc.Scanner = crawler
	return e
}

func (e env) inWithPerms(t *testing.T, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: perms}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Acceptance CON-04: a scan round that finds cookies catalogues genuinely new ones (source=scan) and
// reports the right counters; a cookie already catalogued for this domain is recognized, not duplicated.
func TestRunScan_CataloguesNewCookiesAndReportsCounters(t *testing.T) {
	crawler := fakeCrawler{result: cookiescan.Result{
		PagesScanned: 3,
		Findings: []cookiescan.Finding{
			{PageURL: "https://scan.example/", CookieName: "_ga", CookieDomain: "scan.example"},
			{PageURL: "https://scan.example/about", CookieName: "_ga", CookieDomain: "scan.example"}, // same cookie, second page
			{PageURL: "https://scan.example/", CookieName: "session_id", CookieDomain: "scan.example"},
		},
	}}
	e := scanEnv(t, "cookiescan1", crawler)
	var domainID uuid.UUID
	var scanID uuid.UUID
	e.inWithPerms(t, scanPerms, func(ctx context.Context) error {
		d, err := e.svc.RegisterDomain(ctx, "scan.example", e.legalEntity)
		if err != nil {
			return err
		}
		domainID = d.ID

		sc, err := e.svc.StartScan(ctx, domainID, "manual")
		if err != nil {
			return err
		}
		scanID = sc.ID
		if sc.Status != "queued" {
			t.Errorf("new scan status = %q, want queued", sc.Status)
		}

		if err := e.svc.RunScan(ctx, scanID); err != nil {
			return err
		}

		done, err := e.svc.GetScan(ctx, scanID)
		if err != nil {
			return err
		}
		if done.Status != "done" {
			t.Errorf("scan status = %q, want done", done.Status)
		}
		if done.PagesScanned == nil || *done.PagesScanned != 3 {
			t.Errorf("pages_scanned = %v, want 3", done.PagesScanned)
		}
		if done.CookiesFound == nil || *done.CookiesFound != 2 {
			t.Errorf("cookies_found = %v, want 2 (distinct names)", done.CookiesFound)
		}
		if done.NewCookies == nil || *done.NewCookies != 2 {
			t.Errorf("new_cookies = %v, want 2 (both genuinely new)", done.NewCookies)
		}

		findings, err := e.svc.ListScanFindings(ctx, scanID, 0, 50)
		if err != nil {
			return err
		}
		if len(findings) != 3 {
			t.Fatalf("findings = %d, want 3 (one row per page observation)", len(findings))
		}
		for _, f := range findings {
			if f.MatchedCookieID == nil {
				t.Errorf("finding %+v has no matched cookie", f)
			}
		}

		// A second scan re-crawling the exact same cookies finds nothing new.
		sc2, err := e.svc.StartScan(ctx, domainID, "manual")
		if err != nil {
			return err
		}
		if err := e.svc.RunScan(ctx, sc2.ID); err != nil {
			return err
		}
		done2, err := e.svc.GetScan(ctx, sc2.ID)
		if err != nil {
			return err
		}
		if done2.NewCookies == nil || *done2.NewCookies != 0 {
			t.Errorf("second scan new_cookies = %v, want 0", done2.NewCookies)
		}
		return nil
	})
}

// ClassifyFinding corrects or confirms an unknown scanned cookie's category.
func TestClassifyFinding_SetsTheMatchedCookiesCategory(t *testing.T) {
	crawler := fakeCrawler{result: cookiescan.Result{
		PagesScanned: 1,
		Findings:     []cookiescan.Finding{{PageURL: "https://classify.example/", CookieName: "mystery_cookie", CookieDomain: "classify.example"}},
	}}
	e := scanEnv(t, "cookiescan2", crawler)
	e.inWithPerms(t, scanPerms, func(ctx context.Context) error {
		d, err := e.svc.RegisterDomain(ctx, "classify.example", e.legalEntity)
		if err != nil {
			return err
		}
		cat, err := e.svc.CreateCategory(ctx, cookie.CreateCategoryInput{Code: "custom", NameTh: "กำหนดเอง"})
		if err != nil {
			return err
		}

		sc, err := e.svc.StartScan(ctx, d.ID, "manual")
		if err != nil {
			return err
		}
		if err := e.svc.RunScan(ctx, sc.ID); err != nil {
			return err
		}
		findings, err := e.svc.ListScanFindings(ctx, sc.ID, 0, 50)
		if err != nil || len(findings) != 1 {
			t.Fatalf("findings = %+v %v", findings, err)
		}

		classified, err := e.svc.ClassifyFinding(ctx, findings[0].ID, cat.ID)
		if err != nil {
			return err
		}
		if classified.MatchedCookieID == nil {
			t.Fatal("classified finding has no matched cookie id")
		}

		rows, err := e.svc.ListCookies(ctx, d.ID)
		if err != nil {
			return err
		}
		var found bool
		for _, c := range rows {
			if c.ID == *classified.MatchedCookieID {
				found = true
				if c.CategoryID == nil || *c.CategoryID != cat.ID {
					t.Errorf("cookie category = %v, want %v", c.CategoryID, cat.ID)
				}
			}
		}
		if !found {
			t.Error("matched cookie not found in the domain's own catalogue")
		}

		if _, err := e.svc.ClassifyFinding(ctx, uuid.New(), cat.ID); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("unknown finding: %v, want ErrNotFound", err)
		}
		if _, err := e.svc.ClassifyFinding(ctx, findings[0].ID, uuid.New()); !errors.Is(err, cookie.ErrInvalid) {
			t.Errorf("unknown category: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// A crawl failure never corrupts scan state: the scan stays queryable and RunScan returns the error for
// River's own retry/alert machinery to handle (ScanWorker.Work wraps this call in a savepoint).
func TestRunScan_CrawlFailurePropagatesWithoutCorruptingState(t *testing.T) {
	crawler := fakeCrawler{err: errors.New("site unreachable")}
	e := scanEnv(t, "cookiescan3", crawler)
	e.inWithPerms(t, scanPerms, func(ctx context.Context) error {
		d, err := e.svc.RegisterDomain(ctx, "unreachable.example", e.legalEntity)
		if err != nil {
			return err
		}
		sc, err := e.svc.StartScan(ctx, d.ID, "manual")
		if err != nil {
			return err
		}
		if err := e.svc.RunScan(ctx, sc.ID); err == nil {
			t.Error("expected the crawl error to propagate")
		}
		still, err := e.svc.GetScan(ctx, sc.ID)
		if err != nil {
			return err
		}
		if still.Status != "running" {
			t.Errorf("scan status after a failed crawl = %q, want running (still mid-attempt; ScanWorker marks it failed on the final attempt)", still.Status)
		}
		return nil
	})
}

// Permission gates: cookie.scan.create/read/update are each required independently.
func TestScanPermissions_AreEnforcedIndependently(t *testing.T) {
	e := scanEnv(t, "cookiescan4", fakeCrawler{})
	var domainID uuid.UUID
	e.inWithPerms(t, scanPerms, func(ctx context.Context) error {
		d, err := e.svc.RegisterDomain(ctx, "perm.example", e.legalEntity)
		if err != nil {
			return err
		}
		domainID = d.ID
		return nil
	})

	e.inWithPerms(t, []string{"cookie.banner.read"}, func(ctx context.Context) error {
		if _, err := e.svc.StartScan(ctx, domainID, "manual"); !errors.Is(err, cookie.ErrForbidden) {
			t.Errorf("StartScan without cookie.scan.create: %v, want ErrForbidden", err)
		}
		if _, _, err := e.svc.ListScans(ctx, domainID, nil, 0); !errors.Is(err, cookie.ErrForbidden) {
			t.Errorf("ListScans without cookie.scan.read: %v, want ErrForbidden", err)
		}
		if _, err := e.svc.ClassifyFinding(ctx, uuid.New(), uuid.New()); !errors.Is(err, cookie.ErrForbidden) {
			t.Errorf("ClassifyFinding without cookie.scan.update: %v, want ErrForbidden", err)
		}
		return nil
	})
}

// Two-tenant isolation (rule 1): tenant B's own transaction can never reach tenant A's scans or findings.
func TestScanModule_TenantIsolation(t *testing.T) {
	crawler := fakeCrawler{result: cookiescan.Result{PagesScanned: 1, Findings: []cookiescan.Finding{
		{PageURL: "https://iso.example/", CookieName: "iso_cookie", CookieDomain: "iso.example"},
	}}}
	a := scanEnv(t, "cookiescaniso_a", crawler)
	b := scanEnv(t, "cookiescaniso_b", crawler)

	var scanID uuid.UUID
	a.inWithPerms(t, scanPerms, func(ctx context.Context) error {
		d, err := a.svc.RegisterDomain(ctx, "iso.example", a.legalEntity)
		if err != nil {
			return err
		}
		sc, err := a.svc.StartScan(ctx, d.ID, "manual")
		if err != nil {
			return err
		}
		scanID = sc.ID
		return a.svc.RunScan(ctx, scanID)
	})

	b.inWithPerms(t, scanPerms, func(ctx context.Context) error {
		if _, err := b.svc.GetScan(ctx, scanID); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("tenant B should not reach tenant A's scan: %v, want ErrNotFound", err)
		}
		if _, err := b.svc.ListScanFindings(ctx, scanID, 0, 50); !errors.Is(err, cookie.ErrNotFound) {
			t.Errorf("tenant B should not list tenant A's scan findings: %v, want ErrNotFound", err)
		}
		return nil
	})
}
