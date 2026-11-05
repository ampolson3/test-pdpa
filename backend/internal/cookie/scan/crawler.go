// Package scan is CON-04's own crawler: a headless-Chromium walk of a tenant's website that reports every
// cookie it finds, before any consent decision is ever made (the literal pre-consent state a real first-
// time visitor sees). It knows nothing about tenants, categories or persistence — service.RunScan is the
// one caller that turns a Result into cookie.scan_findings rows; this package only crawls and observes.
package scan

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// Finding is one cookie observed on one page — the exact shape cookie.scan_findings needs, kept free of any
// tenant/category concern so this package stays reusable from both the worker and a future cmd/scanner.
type Finding struct {
	PageURL      string
	CookieName   string
	CookieDomain string
}

// Result is one full crawl.
type Result struct {
	PagesScanned int
	Findings     []Finding
}

// Config controls one crawl. ExecPath lets the caller point at a specific Chromium build (this
// environment's own pre-installed one, in tests); empty uses chromedp's own default lookup.
type Config struct {
	RootURL     string
	MaxPages    int
	Concurrency int // default 4 — real tab parallelism is what keeps 200 pages inside the 30-minute acceptance window
	ExecPath    string
	PageTimeout time.Duration // default 15s per page
}

// Crawler is what service.Service depends on (rule 9's own "small local interface" shape) so unit tests can
// fake a crawl without a real browser; ChromedpCrawler below is the only real implementation.
type Crawler interface {
	Crawl(ctx context.Context, cfg Config) (Result, error)
}

// ChromedpCrawler is the real implementation: a same-origin, breadth-first walk starting at RootURL, using
// a small pool of concurrent tabs so a 200-page scan finishes in minutes, not the better part of an hour.
type ChromedpCrawler struct{}

func (ChromedpCrawler) Crawl(ctx context.Context, cfg Config) (Result, error) {
	if cfg.MaxPages <= 0 {
		cfg.MaxPages = 200
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.PageTimeout <= 0 {
		cfg.PageTimeout = 15 * time.Second
	}
	root, err := url.Parse(cfg.RootURL)
	if err != nil {
		return Result{}, fmt.Errorf("scan: bad root url: %w", err)
	}

	opts := chromedp.DefaultExecAllocatorOptions[:]
	if cfg.ExecPath != "" {
		opts = append(opts, chromedp.ExecPath(cfg.ExecPath))
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	f := &frontier{host: root.Host, maxPages: cfg.MaxPages}
	f.push(root.String())

	var (
		mu     sync.Mutex
		result Result
		wg     sync.WaitGroup
	)

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tabCtx, tabCancel := chromedp.NewContext(allocCtx)
			defer tabCancel()
			// chromedp ties the browser's lifetime to the context of its *first* call against a new
			// chromedp context (its own doc: "do not set a timeout on the context of that first call,
			// because the timeout will stop the browser"). Warming the tab up with an untimed action
			// here, before scanPage ever wraps tabCtx with a per-page deadline, is what keeps the tab
			// alive across every later page in this worker's loop.
			if _, err := chromedp.Run(tabCtx, chromedp.Navigate("about:blank")); err != nil {
				return
			}
			for {
				pageURL, ok := f.pop()
				if !ok {
					return
				}
				findings, links, err := scanPage(tabCtx, pageURL, cfg.PageTimeout)
				for _, l := range links {
					f.push(l)
				}
				f.done()
				if err != nil {
					// A single page failing to load (timeout, DNS, 404) doesn't abort the whole scan —
					// the acceptance criterion is about the scan as a whole completing, not every page
					// existing.
					continue
				}
				mu.Lock()
				result.PagesScanned++
				result.Findings = append(result.Findings, findings...)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if result.PagesScanned == 0 {
		return Result{}, fmt.Errorf("scan: no page of %s could be loaded", cfg.RootURL)
	}
	return result, nil
}

func scanPage(ctx context.Context, pageURL string, timeout time.Duration) ([]Finding, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := chromedp.Do(ctx,
		chromedp.Navigate(pageURL),
		chromedp.Sleep(300*time.Millisecond), // let same-page scripts/pixels fire before reading cookies
	); err != nil {
		return nil, nil, err
	}

	cookieRes, err := chromedp.Call(ctx, storage.GetCookies, storage.GetCookiesParams{})
	if err != nil {
		return nil, nil, err
	}
	links, err := chromedp.Run(ctx, chromedp.Evaluate[[]string](`Array.from(document.querySelectorAll('a[href]')).map(a => a.href)`))
	if err != nil {
		return nil, nil, err
	}

	findings := make([]Finding, 0, len(cookieRes.Cookies))
	for _, c := range cookieRes.Cookies {
		findings = append(findings, Finding{PageURL: pageURL, CookieName: c.Name, CookieDomain: strings.TrimPrefix(c.Domain, ".")})
	}
	return findings, links, nil
}

// frontier is a bounded, same-host BFS queue shared by every tab. Pages are discovered only by finishing a
// page that's already being crawled, so an empty queue doesn't mean the crawl is done — another tab might
// still be mid-page and about to push new links. `active` tracks tabs currently processing a page; pop()
// only reports "no more work" once the queue is empty *and* nothing is still in flight to refill it, and
// waits (via cond, not a busy loop) otherwise.
type frontier struct {
	mu       sync.Mutex
	cond     *sync.Cond
	host     string
	maxPages int
	queued   []string
	seen     map[string]bool
	enqueued int
	active   int
}

func (f *frontier) push(raw string) {
	u, err := url.Parse(raw)
	if err != nil || u.Host != f.host || (u.Scheme != "http" && u.Scheme != "https") {
		return
	}
	u.Fragment = ""
	norm := u.String()

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cond == nil {
		f.cond = sync.NewCond(&f.mu)
	}
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	if f.seen[norm] || f.enqueued >= f.maxPages {
		return
	}
	f.seen[norm] = true
	f.enqueued++
	f.queued = append(f.queued, norm)
	f.cond.Broadcast()
}

// pop blocks while the queue is empty but another tab is still active (it might push more work), and
// returns (_, false) once the queue is empty and every tab is idle — the crawl is genuinely finished.
func (f *frontier) pop() (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cond == nil {
		f.cond = sync.NewCond(&f.mu)
	}
	for len(f.queued) == 0 {
		if f.active == 0 {
			return "", false
		}
		f.cond.Wait()
	}
	u := f.queued[0]
	f.queued = f.queued[1:]
	f.active++
	return u, true
}

// done marks the current page's work finished; called whether the page succeeded or failed, after any
// links it found have already been pushed.
func (f *frontier) done() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active--
	f.cond.Broadcast()
}
