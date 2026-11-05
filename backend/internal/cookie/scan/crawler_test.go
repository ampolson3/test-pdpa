package scan_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	scan "pdpa-platform/internal/cookie/scan"
)

// chromiumExecPath finds this environment's pre-installed Chromium (no network access to fetch one, and
// the pinned chromedp version would otherwise try to download an incompatible revision — the same
// SCANNER_CHROME_PATH / CHROMIUM_PATH convention PLT-16's own renderer already uses). Skips the test
// cleanly when no browser is available, the same pattern every DB-backed test in this codebase already
// uses for "no reachable Postgres".
func chromiumExecPath(t *testing.T) string {
	t.Helper()
	for _, p := range []string{os.Getenv("SCANNER_CHROME_PATH"), "/opt/pw-browsers/chromium"} {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("no Chromium available in this environment (set SCANNER_CHROME_PATH)")
	return ""
}

// fixtureSite is a tiny same-host multi-page site: each page sets its own cookie and links to the next,
// plus one off-site link the crawler must never follow (the acceptance criterion's own same-origin scope).
func fixtureSite(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	pages := map[string]struct {
		cookieName, cookieValue, next string
	}{
		"/":        {cookieName: "_ga", cookieValue: "GA1.1.1", next: "/about"},
		"/about":   {cookieName: "session_id", cookieValue: "abc123", next: "/contact"},
		"/contact": {cookieName: "_fbp", cookieValue: "fb.1.1", next: ""},
	}
	for path, p := range pages {
		p := p
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: p.cookieName, Value: p.cookieValue, Path: "/"})
			body := fmt.Sprintf(`<html><body><h1>%s</h1>`, path)
			if p.next != "" {
				body += fmt.Sprintf(`<a href="%s">next</a>`, p.next)
			}
			body += `<a href="https://example.com/off-site">off-site</a></body></html>`
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(body))
		})
	}
	return httptest.NewServer(mux)
}

// Acceptance CON-04: a real headless-Chromium crawl of a small same-origin site finds every page and every
// cookie those pages set — proving the crawler genuinely drives a browser rather than just fetching HTML —
// and never follows the off-site link.
func TestChromedpCrawler_FindsEveryPageAndCookie(t *testing.T) {
	execPath := chromiumExecPath(t)
	srv := fixtureSite(t)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := scan.ChromedpCrawler{}.Crawl(ctx, scan.Config{
		RootURL: srv.URL, MaxPages: 10, Concurrency: 2, ExecPath: execPath, PageTimeout: 15 * time.Second,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if res.PagesScanned != 3 {
		t.Errorf("PagesScanned = %d, want 3 (/, /about, /contact — never the off-site link)", res.PagesScanned)
	}

	seenCookies := map[string]bool{}
	for _, f := range res.Findings {
		seenCookies[f.CookieName] = true
	}
	for _, want := range []string{"_ga", "session_id", "_fbp"} {
		if !seenCookies[want] {
			t.Errorf("cookie %q not found among %v", want, seenCookies)
		}
	}
}

// MaxPages caps the crawl even when more same-origin pages remain to visit.
func TestChromedpCrawler_RespectsMaxPages(t *testing.T) {
	execPath := chromiumExecPath(t)
	srv := fixtureSite(t)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := scan.ChromedpCrawler{}.Crawl(ctx, scan.Config{
		RootURL: srv.URL, MaxPages: 1, Concurrency: 2, ExecPath: execPath, PageTimeout: 15 * time.Second,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if res.PagesScanned != 1 {
		t.Errorf("PagesScanned = %d, want exactly 1 (MaxPages cap)", res.PagesScanned)
	}
}
