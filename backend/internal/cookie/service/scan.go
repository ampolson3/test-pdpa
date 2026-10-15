package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	cookiescan "pdpa-platform/internal/cookie/scan"
	cookiestore "pdpa-platform/internal/cookie/store"
	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// ScanEntityType is this module's own audit entity type for cookie.scans.
const ScanEntityType = "cookie_scan"

var scanTriggers = []string{"manual", "schedule", "publish"}

// Scan is one round of CON-04's own automated scanner.
type Scan struct {
	ID           uuid.UUID
	DomainID     uuid.UUID
	Trigger      string
	Status       string // queued | running | done | failed
	StartedAt    *time.Time
	FinishedAt   *time.Time
	PagesScanned *int32
	CookiesFound *int32
	NewCookies   *int32
	RowVersion   int32
	CreatedAt    time.Time
}

func toScan(id, domainID uuid.UUID, trigger, status string, startedAt, finishedAt pgtype.Timestamptz, pages, found, newC *int32, rowVersion int32, createdAt pgtype.Timestamptz) Scan {
	s := Scan{ID: id, DomainID: domainID, Trigger: trigger, Status: status, PagesScanned: pages, CookiesFound: found, NewCookies: newC, RowVersion: rowVersion, CreatedAt: createdAt.Time}
	if startedAt.Valid {
		t := startedAt.Time
		s.StartedAt = &t
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		s.FinishedAt = &t
	}
	return s
}

// ScanFinding is one cookie observed on one page of one scan round.
type ScanFinding struct {
	ID                uuid.UUID
	ScanID            uuid.UUID
	CookieName        string
	CookieDomain      string
	PageURL           string
	SetBeforeConsent  bool
	SuggestedCategory *string
	MatchedCookieID   *uuid.UUID
	RowVersion        int32
}

func toScanFinding(id, scanID uuid.UUID, name, domain, pageURL string, before bool, suggested *string, matched pgtype.UUID, rowVersion int32) ScanFinding {
	f := ScanFinding{ID: id, ScanID: scanID, CookieName: name, CookieDomain: domain, PageURL: pageURL, SetBeforeConsent: before, SuggestedCategory: suggested, RowVersion: rowVersion}
	if matched.Valid {
		id := uuid.UUID(matched.Bytes)
		f.MatchedCookieID = &id
	}
	return f
}

// StartScan registers a new scan round and — when this Service has a River client — enqueues cookie.scan
// to actually run it (cmd/api's own insert-only client; cmd/worker's ScanWorker does the crawling).
func (s *Service) StartScan(ctx context.Context, domainID uuid.UUID, trigger string) (Scan, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.scan.create") {
		return Scan{}, ErrForbidden
	}
	return s.startScan(ctx, domainID, trigger)
}

// startScan is StartScan without the permission check — used by the scheduled-scan job below, which runs
// as system work with no caller to authorize (the same "jobs are system work" reasoning CLAUDE.md rule 1
// already states for TenantTxMiddleware).
func (s *Service) startScan(ctx context.Context, domainID uuid.UUID, trigger string) (Scan, error) {
	if !contains(scanTriggers, trigger) {
		return Scan{}, fmt.Errorf("%w: trigger", ErrInvalid)
	}
	if _, err := s.GetDomain(ctx, domainID); err != nil {
		return Scan{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Scan{}, err
	}
	row, err := cookiestore.New(pdb.MustTxFromContext(ctx)).InsertScan(ctx, cookiestore.InsertScanParams{ID: id, DomainID: domainID, Trigger: trigger})
	if err != nil {
		return Scan{}, err
	}
	out := toScan(row.ID, row.DomainID, row.Trigger, row.Status, row.StartedAt, row.FinishedAt, row.PagesScanned, row.CookiesFound, row.NewCookies, row.RowVersion, row.CreatedAt)
	if err := s.audit(ctx, "cookie.scan.create", ScanEntityType, id, nil, map[string]any{"domain_id": domainID, "trigger": trigger}); err != nil {
		return Scan{}, err
	}
	if s.River != nil {
		tenant, err := tenantID(ctx)
		if err != nil {
			return Scan{}, err
		}
		if _, err := jobs.Enqueue(ctx, s.River, ScanArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, ScanID: id.String()}, nil); err != nil {
			return Scan{}, err
		}
	}
	return out, nil
}

// GetScan returns one scan round.
func (s *Service) GetScan(ctx context.Context, id uuid.UUID) (Scan, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.scan.read") {
		return Scan{}, ErrForbidden
	}
	row, err := cookiestore.New(pdb.MustTxFromContext(ctx)).GetScan(ctx, id)
	if pgErrNoRows(err) {
		return Scan{}, ErrNotFound
	}
	if err != nil {
		return Scan{}, err
	}
	return toScan(row.ID, row.DomainID, row.Trigger, row.Status, row.StartedAt, row.FinishedAt, row.PagesScanned, row.CookiesFound, row.NewCookies, row.RowVersion, row.CreatedAt), nil
}

// ScanCursor is ListScans' own opaque cursor (created_at, id) — the same shape every other module's
// cursor-paginated list already uses (e.g. vendorservice.VendorCursor).
type ScanCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

const scanPageSize = 20

// ListScans returns scan rounds for a domain, newest first, cursor-paginated (the N+1-row trick every
// other cursor list in this codebase uses to decide whether a next page exists).
func (s *Service) ListScans(ctx context.Context, domainID uuid.UUID, after *ScanCursor, limit int32) ([]Scan, *ScanCursor, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.scan.read") {
		return nil, nil, ErrForbidden
	}
	if _, err := s.GetDomain(ctx, domainID); err != nil {
		return nil, nil, err
	}
	if limit <= 0 || limit > scanPageSize {
		limit = scanPageSize
	}
	var at pgtype.Timestamptz
	var id pgtype.UUID
	if after != nil {
		at = pgtype.Timestamptz{Time: after.CreatedAt, Valid: true}
		id = pgtype.UUID{Bytes: after.ID, Valid: true}
	}
	rows, err := cookiestore.New(pdb.MustTxFromContext(ctx)).ListScansByDomain(ctx, cookiestore.ListScansByDomainParams{DomainID: domainID, CursorAt: at, CursorID: id, Lim: limit + 1})
	if err != nil {
		return nil, nil, err
	}
	out := make([]Scan, 0, len(rows))
	for i, r := range rows {
		if int32(i) == limit {
			last := out[len(out)-1]
			return out, &ScanCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
		}
		out = append(out, toScan(r.ID, r.DomainID, r.Trigger, r.Status, r.StartedAt, r.FinishedAt, r.PagesScanned, r.CookiesFound, r.NewCookies, r.RowVersion, r.CreatedAt))
	}
	return out, nil, nil
}

// ListScanFindings returns every cookie finding of one scan round, paginated.
func (s *Service) ListScanFindings(ctx context.Context, scanID uuid.UUID, offset, limit int32) ([]ScanFinding, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.scan.read") {
		return nil, ErrForbidden
	}
	if _, err := s.GetScan(ctx, scanID); err != nil {
		return nil, err
	}
	rows, err := cookiestore.New(pdb.MustTxFromContext(ctx)).ListScanFindingsByScan(ctx, cookiestore.ListScanFindingsByScanParams{ScanID: scanID, Offs: offset, Lim: limit})
	if err != nil {
		return nil, err
	}
	out := make([]ScanFinding, 0, len(rows))
	for _, r := range rows {
		out = append(out, toScanFinding(r.ID, r.ScanID, r.CookieName, r.CookieDomain, r.PageUrl, r.SetBeforeConsent, r.SuggestedCategory, r.MatchedCookieID, r.RowVersion))
	}
	return out, nil
}

// ClassifyFinding is the module doc's own "จัดหมวดคุกกี้ที่ไม่รู้จัก" (classify an unknown scanned cookie):
// the staff-chosen category is written straight to the cookie.cookies row the scanner already created for
// this cookie name (RunScan always creates one, categorized or not) and the finding keeps pointing at it —
// nothing new to insert here, this feature just corrects what the scanner guessed (or didn't).
func (s *Service) ClassifyFinding(ctx context.Context, findingID, categoryID uuid.UUID) (ScanFinding, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.scan.update") {
		return ScanFinding{}, ErrForbidden
	}
	q := cookiestore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetScanFinding(ctx, findingID)
	if pgErrNoRows(err) {
		return ScanFinding{}, ErrNotFound
	}
	if err != nil {
		return ScanFinding{}, err
	}
	if _, err := s.GetScan(ctx, row.ScanID); err != nil {
		return ScanFinding{}, err
	}
	if !row.MatchedCookieID.Valid {
		return ScanFinding{}, fmt.Errorf("%w: finding has no matched cookie to classify", ErrInvalid)
	}
	if _, err := s.GetCategory(ctx, categoryID); err != nil {
		return ScanFinding{}, fmt.Errorf("%w: category_id", ErrInvalid)
	}
	cookieID := uuid.UUID(row.MatchedCookieID.Bytes)
	if _, err := q.SetCookieCategory(ctx, cookiestore.SetCookieCategoryParams{ID: cookieID, CategoryID: pgtype.UUID{Bytes: categoryID, Valid: true}}); err != nil {
		return ScanFinding{}, err
	}
	if err := s.audit(ctx, "cookie.scan_finding.classify", ScanEntityType, findingID, nil, map[string]any{"cookie_id": cookieID, "category_id": categoryID}); err != nil {
		return ScanFinding{}, err
	}
	return toScanFinding(row.ID, row.ScanID, row.CookieName, row.CookieDomain, row.PageUrl, row.SetBeforeConsent, row.SuggestedCategory, row.MatchedCookieID, row.RowVersion), nil
}

// ScanRootURL builds the crawl's starting URL. Scanning always starts fresh, pre-consent (the acceptance
// criterion itself) — https, since that's what a real visitor's browser would default to.
func scanRootURL(domain string) string { return "https://" + domain }

// RunScan is the scanner's own body, run inside cookie.scan's job tx: crawl, classify every cookie found
// against cookie.cookie_kb + the tenant's own already-catalogued cookies, auto-create a cookie.cookies row
// for anything genuinely new, and alert IT/DPO when it does. A scan already past 'queued' (a duplicate job
// delivery, or one already running from a prior attempt) is a harmless no-op.
func (s *Service) RunScan(ctx context.Context, scanID uuid.UUID) error {
	q := cookiestore.New(pdb.MustTxFromContext(ctx))
	if n, err := q.MarkScanRunning(ctx, scanID); err != nil {
		return err
	} else if n == 0 {
		return nil
	}
	scanRow, err := q.GetScan(ctx, scanID)
	if err != nil {
		return err
	}
	domain, err := s.GetDomain(ctx, scanRow.DomainID)
	if err != nil {
		return err
	}

	res, err := s.Scanner.Crawl(ctx, cookiescan.Config{RootURL: scanRootURL(domain.Domain), MaxPages: 200, ExecPath: os.Getenv("SCANNER_CHROME_PATH")})
	if err != nil {
		return err
	}

	kb, err := q.ListCookieKB(ctx)
	if err != nil {
		return err
	}
	cats, err := q.ListCategories(ctx)
	if err != nil {
		return err
	}
	catByCode := make(map[string]uuid.UUID, len(cats))
	for _, c := range cats {
		catByCode[c.Code] = c.ID
	}

	resolved := map[string]uuid.UUID{} // cookie name -> cookie.cookies id, resolved once per name per scan
	newCount := 0
	for _, f := range res.Findings {
		cookieID, isNew, suggested, err := s.resolveCookieFromFinding(ctx, q, scanRow.DomainID, scanID, f, kb, catByCode, resolved)
		if err != nil {
			return err
		}
		if isNew {
			newCount++
		}
		findingID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if _, err := q.InsertScanFinding(ctx, cookiestore.InsertScanFindingParams{
			ID: findingID, ScanID: scanID, CookieName: f.CookieName, CookieDomain: f.CookieDomain, PageUrl: f.PageURL,
			SetBeforeConsent: true, SuggestedCategory: suggested, MatchedCookieID: pgtype.UUID{Bytes: cookieID, Valid: true},
		}); err != nil {
			return err
		}
	}

	pages := int32(res.PagesScanned)
	cookiesFound := int32(len(resolved))
	newCookies := int32(newCount)
	if _, err := q.MarkScanDone(ctx, cookiestore.MarkScanDoneParams{ID: scanID, PagesScanned: &pages, CookiesFound: &cookiesFound, NewCookies: &newCookies}); err != nil {
		return err
	}
	if err := s.audit(ctx, "cookie.scan.done", ScanEntityType, scanID, nil, map[string]any{"pages_scanned": pages, "cookies_found": cookiesFound, "new_cookies": newCookies}); err != nil {
		return err
	}
	if newCount > 0 {
		if err := s.alertNewCookies(ctx, domain, scanID, newCount); err != nil {
			return err
		}
	}
	return nil
}

// resolveCookieFromFinding maps one observed cookie name to its cookie.cookies row — creating one, source
// 'scan', the first time this scan sees that name — and reports whether it was genuinely new to this
// domain (not just new to this scan). `resolved` caches the result per name so a cookie seen on 50 pages
// in one scan is looked up / created only once.
func (s *Service) resolveCookieFromFinding(ctx context.Context, q *cookiestore.Queries, domainID, scanID uuid.UUID, f cookiescan.Finding, kb []cookiestore.ListCookieKBRow, catByCode map[string]uuid.UUID, resolved map[string]uuid.UUID) (uuid.UUID, bool, *string, error) {
	var suggestedCategory *string
	for _, k := range kb {
		if matchesKBPattern(f.CookieName, k.NamePattern) {
			code := k.CategoryCode
			suggestedCategory = &code
			break
		}
	}

	if id, ok := resolved[f.CookieName]; ok {
		return id, false, suggestedCategory, nil
	}

	existing, err := q.GetCookieByDomainAndName(ctx, cookiestore.GetCookieByDomainAndNameParams{DomainID: domainID, Name: f.CookieName})
	if err == nil {
		resolved[f.CookieName] = existing.ID
		return existing.ID, false, suggestedCategory, nil
	}
	if !pgErrNoRows(err) {
		return uuid.Nil, false, nil, err
	}

	var categoryID pgtype.UUID
	if suggestedCategory != nil {
		if cid, ok := catByCode[*suggestedCategory]; ok {
			categoryID = pgtype.UUID{Bytes: cid, Valid: true}
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	row, err := q.InsertCookieFromScan(ctx, cookiestore.InsertCookieFromScanParams{
		ID: id, DomainID: domainID, Name: f.CookieName, ProviderDomain: f.CookieDomain, CategoryID: categoryID,
		KbID: pgtype.UUID{}, FirstSeenScanID: pgtype.UUID{Bytes: scanID, Valid: true},
	})
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	resolved[f.CookieName] = row.ID
	return row.ID, true, suggestedCategory, nil
}

// alertNewCookies notifies IT/DPO that this scan found cookies the tenant's own catalogue didn't have yet
// (the acceptance criterion's own "คุกกี้ใหม่ถูกแจ้งเตือน") — the same "default recipients until real
// per-record routing exists" fallback BRE-07/DSAR-07/PNG-04 already use.
func (s *Service) alertNewCookies(ctx context.Context, domain Domain, scanID uuid.UUID, count int) error {
	if s.Notify == nil {
		return nil
	}
	to, err := iamservice.UsersWithRole(ctx, "DPO")
	if err != nil {
		return err
	}
	more, err := iamservice.UsersWithRole(ctx, "IT")
	if err != nil {
		return err
	}
	seen := map[uuid.UUID]bool{}
	recipients := make([]uuid.UUID, 0, len(to)+len(more))
	for _, u := range append(to, more...) {
		if !seen[u] {
			seen[u] = true
			recipients = append(recipients, u)
		}
	}
	vars := map[string]any{"domain": domain.Domain, "new_cookie_count": count}
	for _, u := range recipients {
		uid := u
		for _, ch := range []string{"in_app", "email"} {
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "cookie.scan_new_cookies", Channel: ch, RecipientUserID: &uid,
					Vars: vars, EntityType: ScanEntityType, EntityID: &scanID, Urgent: false})
				return err
			})
			if err != nil && !(ch == "email" && isInvalidRequest(err)) {
				return err
			}
		}
	}
	return nil
}

func isInvalidRequest(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid")
}

func pgErrNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func tenantID(ctx context.Context) (uuid.UUID, error) {
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(t)
}

// ScanArgs is cookie.scan: run one scan round.
type ScanArgs struct {
	jobs.TenantArgs
	ScanID string `json:"scan_id"`
}

func (ScanArgs) Kind() string { return "cookie.scan" }

// ScanMaxAttempts: a scan whose crawl keeps failing (site down, blocked by the target) ends in `failed`
// instead of retrying for weeks — the same bound files.ScanMaxAttempts already uses for AV scanning.
const ScanMaxAttempts = 5

func (ScanArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: ScanMaxAttempts} }

// ScanWorker works cookie.scan.
type ScanWorker struct {
	river.WorkerDefaults[ScanArgs]
	Service *Service
}

func (w *ScanWorker) Work(ctx context.Context, job *river.Job[ScanArgs]) error {
	id, err := uuid.Parse(job.Args.ScanID)
	if err != nil {
		return river.JobCancel(err)
	}
	// RunScan runs inside its own savepoint: a crawl or write failure rolls back only its own writes
	// (scan_findings/cookies partially inserted this attempt), not the whole job transaction — which is
	// what lets the final-attempt branch below still write MarkScanFailed in the very same transaction,
	// the same pdb.Savepoint pattern org.SaveLegalEntity already uses for its own unique-constraint retry.
	runErr := pdb.Savepoint(ctx, func(ctx context.Context) error {
		return w.Service.RunScan(ctx, id)
	})
	if runErr == nil {
		return nil
	}
	if job.Attempt < job.MaxAttempts {
		return runErr // retried by River; AlertHandler logs it
	}
	if _, err := cookiestore.New(pdb.MustTxFromContext(ctx)).MarkScanFailed(ctx, id); err != nil {
		return err
	}
	return nil
}
