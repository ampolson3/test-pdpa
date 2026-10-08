// Package service is the agreement module (DPA-02): the "agreement engine" the module doc's own note says
// to "build once, share with DSA" — creating a DPA (or, once that module exists, a DSA) from a RoPA
// activity and a counterparty, wrapping a PLT-16 document. It works in the transaction of the context
// (CLAUDE.md rule 1) and reads other modules only through their exported services (rule 9). Only
// agreement_type "dpa" is accepted today — the table's own CHECK constraint already allows dsa/
// joint_controller/inbound_dpa for when those modules exist, but there is nothing to wire them to yet.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/docs"
	ropaservice "pdpa-platform/internal/ropa/service"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

// EntityType is the audit entity type for agreement.agreements rows.
const EntityType = "agreement"

var (
	ErrNotFound = errors.New("agreement: not found")
	ErrInvalid  = errors.New("agreement: invalid")
)

// Org is what agreement reads from the org module (rule 9): the counterparty and our own legal entity must
// both be visible under the caller's RLS before their ids are ever written (FKs bypass RLS).
type Org interface {
	GetExternalParty(ctx context.Context, id uuid.UUID) (orgservice.ExternalParty, error)
	GetLegalEntity(ctx context.Context, id uuid.UUID) (orgservice.LegalEntity, error)
}

// Vendor is what agreement reads from the vendormgmt module (rule 9) — a DPA always starts from a VEN-01
// vendor record, whose own party_id names the real counterparty.
type Vendor interface {
	GetVendor(ctx context.Context, id uuid.UUID) (vendorservice.Vendor, error)
}

// Ropa is what agreement reads from the ropa module (rule 9) — every linked activity must be the caller's
// own, visible under RLS, before its id is written to agreement_activities.
type Ropa interface {
	GetActivity(ctx context.Context, id uuid.UUID) (ropaservice.Activity, error)
}

// Service runs the agreement module for the tenant of the transaction in ctx. Docs is a direct concrete
// field (not a rule-9 interface): the composed document is this module's entire reason to exist, and
// internal/platform/docs is itself the shared, type-agnostic engine PLT-16 built for every document type,
// not "another module's own schema" agreement would otherwise be reaching into.
type Service struct {
	Docs   *docs.Service
	Org    Org
	Vendor Vendor
	Ropa   Ropa
	Audit  *audit.Service
}

func (s *Service) audit(ctx context.Context, action string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		var t string
		if terr := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); terr != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("agreement: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: EntityType, EntityID: &id, Before: before, After: after}
	if u, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &u
	}
	return s.Audit.Write(ctx, e)
}
