// Package service is the vendor module's business logic. First feature: VEN-01, the vendor/processor
// registry that every later VEN feature (tiering, assessments, approval, sub-processors, offboarding —
// ST-06) extends. vendor.vendors is already fully specified in the baseline migrations, so this package
// is CRUD plus the FK-visibility checks rule 1 requires (party_id → org.external_parties,
// relationship_owner_id → iam.users) — no migration, no transition logic: this feature never moves the
// vendor off its default status, "prospect" (ST-06's own [*] → prospect edge); every other edge belongs
// to a sibling feature not built yet.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
	riskservice "pdpa-platform/internal/risk/service"
)

var (
	ErrNotFound        = errors.New("vendor: not found")
	ErrInvalid         = errors.New("vendor: invalid input")
	ErrVersionMismatch = errors.New("vendor: version mismatch")
)

// Org is what vendor reads from the organization module (rule 9): FK visibility (rule 1) for party_id.
type Org interface {
	GetExternalParty(ctx context.Context, id uuid.UUID) (orgservice.ExternalParty, error)
}

type Service struct {
	Audit *audit.Service
	Org   Org
	Forms *forms.Service
	// Dpia/Risk are direct concrete dependencies, not local interfaces: no module in either direction
	// imports vendormgmt, so there is no cycle to route around. VEN-07 uses them to answer one of
	// VEN-04's own assess.assessments templates (dpia owns that schema, rule 9) and classify the
	// resulting score against the tenant's own RRA-02 risk matrix.
	Dpia *dpiaservice.Service
	Risk *riskservice.Service
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
			return fmt.Errorf("vendor: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: VendorEntityType, EntityID: &id, Before: before, After: after}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}
