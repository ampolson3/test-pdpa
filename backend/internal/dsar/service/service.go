// Package service is the dsar module's business logic. First feature: DSAR-13, the automatic response-letter
// composer. It needs a real dsar.requests row with a status/outcome to generate from, so this package also
// carries the minimal slice of DSAR-01/02/03/06/07/08's job that DSAR-13 depends on — creating a request and
// moving it through ST-02's real state graph — deliberately scoped no further than that: full intake channels
// (DSAR-01/02), identity verification (DSAR-06), the workflow/subtask engine (DSAR-08) and SLA business-day
// countdown (DSAR-07) are sibling features layered on top later. DSAR-11 (reject with reason) added the
// approver gate and the `dsar.rejected` event on top of the `rejected` edge DSAR-13 already had.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	docsservice "pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/events"
	ropaservice "pdpa-platform/internal/ropa/service"
)

var (
	ErrNotFound          = errors.New("dsar: not found")
	ErrInvalid           = errors.New("dsar: invalid input")
	ErrVersionMismatch   = errors.New("dsar: version mismatch")
	ErrInvalidTransition = errors.New("dsar: invalid transition")
	ErrForbidden         = errors.New("dsar: forbidden")
)

// Org is what dsar reads from the organization module (rule 9): FK visibility (rule 1) for legal_entity_id.
type Org interface {
	GetLegalEntity(ctx context.Context, id uuid.UUID) (orgservice.LegalEntity, error)
}

// Docs is what dsar writes in the document composer (PLT-16, rule 9): each generated response letter is its
// own platform.documents row (doc_type "dsar_letter"), already registered with a DPO approval step in
// internal/wiring.Docs — dsar never touches platform.documents directly.
type Docs interface {
	Create(ctx context.Context, in docsservice.CreateInput) (docsservice.Document, error)
	SaveDraft(ctx context.Context, id uuid.UUID, rowVersion int32, d docsservice.Draft) (docsservice.Document, error)
}

// Ropa is what dsar reads from the RoPA module (rule 9): DSAR-11's FK visibility check (rule 1) for the
// processing activities a rejection is logged against — the actual write into `ropa.activity_rejections` is
// ROPA-10's job, done asynchronously off the `dsar.rejected` event this module publishes, not here.
type Ropa interface {
	GetActivity(ctx context.Context, id uuid.UUID) (ropaservice.Activity, error)
}

type Service struct {
	Audit   *audit.Service
	Org     Org
	Docs    Docs
	Ropa    Ropa
	Keyring *crypto.Keyring
	Events  *events.Publisher
	// Now is the clock (injectable for tests); nil means time.Now.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *Service) audit(ctx context.Context, action string, id uuid.UUID, before, after any) error {
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
			return fmt.Errorf("dsar: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", EntityType: "dsar_request", EntityID: &id, Action: action, Before: before, After: after}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}
