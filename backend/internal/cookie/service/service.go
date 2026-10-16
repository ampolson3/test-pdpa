// Package service is the cookie module (CON-01): the consent banner's own domains, categories and
// versioned banner configs, plus the append-only evidence of each visitor's decision
// (cookie.consent_records). It works in the transaction of the context (CLAUDE.md rule 1) and reads other
// modules only through their services (rule 9).
//
// Deliberately scoped to CON-01's own literal acceptance criterion (no pre-ticked category; reject-all in
// one click; the SDK itself ≤ 30 KB gzip) and nothing past it: cookie.apps (mobile/LIFF SDKs, CON-07),
// cookie.scans/scan_findings/cookie_kb (the automated scanner, CON-04), cookie.script_rules (prior script
// blocking, CON-03) and cookie.ab_variants (A/B testing, CON-08) are all already in the baseline schema,
// unused — sibling features layered on the same tables later, the same "build only the minimal slice this
// feature needs" pattern this codebase uses throughout.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"pdpa-platform/internal/cookie/scan"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/notify"
)

var (
	ErrInvalid         = errors.New("cookie: invalid input")
	ErrNotFound        = errors.New("cookie: not found")
	ErrVersionMismatch = errors.New("cookie: version mismatch")
	ErrForbidden       = errors.New("cookie: forbidden")
)

// Org is what cookie reads from the organization module (rule 9): FK visibility (rule 1) for a domain's
// legal_entity_id.
type Org interface {
	GetLegalEntity(ctx context.Context, id uuid.UUID) (orgservice.LegalEntity, error)
}

type Service struct {
	Audit  *audit.Service
	Org    Org
	Notify *notify.Service
	// River enqueues cookie.scan / cookie.scan_schedule_* jobs; nil (e.g. in cmd/worker's own job
	// handlers, which already run inside the job they were given) skips enqueuing — the same "worker
	// builds only what its own jobs need" pattern dsarservice.Service's own ReminderWorker fields use.
	River *river.Client[pgx.Tx]
	// Scanner is CON-04's own crawler, a rule-9 "small local interface" so tests can fake a crawl
	// without a real browser; cmd/worker wires scan.ChromedpCrawler{}, the only real implementation.
	Scanner scan.Crawler
	// Now is the clock (injectable for tests); nil means time.Now.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

// audit resolves the tenant from AuthZ grants when present (every /admin/v1 call), falling back to the
// request transaction's own setting when not (a /public/v1 consent submission carries no Grants — the same
// fallback CON-11's own guardian-approval audit call needed, documented there as a pattern every module's
// public-route audit helper should use).
func (s *Service) audit(ctx context.Context, action, entityType string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		var t string
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("cookie: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: entityType, EntityID: &id, Before: before, After: after}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}
