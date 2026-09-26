// Package service is the consent module (CON): purposes and their versioned consent text (CON-12, approved through
// PLT-08), collection points with the s.19 / s.26 publish checks (CON-09, CON-10), recording decisions as
// hash-chained receipts, append-only transactions and a status projection (CON-15), and withdrawal (CON-13).
// It works in the transaction of the context (CLAUDE.md rule 1) and reads other modules only through their services.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/versioning"
)

// Entity types (audit, versioning, notifications).
const (
	PurposeType         = "consent_purpose"
	CollectionPointType = "consent_collection_point"
	SubjectType         = "consent_subject"
)

var (
	ErrNotFound          = errors.New("consent: not found")
	ErrForbidden         = errors.New("consent: forbidden")
	ErrVersionMismatch   = errors.New("consent: version mismatch")
	ErrInvalidTransition = errors.New("consent: invalid transition")
	ErrInvalidRequest    = errors.New("consent: invalid request")
	ErrStaleVersion      = errors.New("consent: purpose version is not the current one")
	ErrPublishChecks     = errors.New("consent: publish checks failed")
	ErrSubjectConflict   = errors.New("consent: identifiers belong to different data subjects")
	ErrInUse             = errors.New("consent: in use")
)

// CheckError lists which publish checks failed (codes the UI localizes).
type CheckError struct{ Failed []string }

func (e *CheckError) Error() string {
	return fmt.Sprintf("consent: publish checks failed: %v", e.Failed)
}
func (e *CheckError) Unwrap() error { return ErrPublishChecks }

// Org is what consent reads from the organization module (rule 9).
type Org interface {
	GetLegalEntity(ctx context.Context, id uuid.UUID) (orgservice.LegalEntity, error)
	ListMaster(ctx context.Context, kind string) ([]orgservice.MasterItem, error)
}

// Service runs the consent module for the tenant of the transaction in ctx.
type Service struct {
	Versioning *versioning.Service
	Events     *events.Publisher
	Notify     *notify.Service
	Keyring    *crypto.Keyring
	Audit      *audit.Service
	Org        Org
	Now        func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) audit(ctx context.Context, action, entityType string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		return ErrForbidden
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: entityType, EntityID: &id, Before: before, After: after}
	if u, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &u
	}
	return s.Audit.Write(ctx, e)
}

func currentUser(ctx context.Context) *uuid.UUID {
	g, _ := authz.FromContext(ctx)
	if u, err := uuid.Parse(g.UserID); err == nil {
		return &u
	}
	return nil
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, a...))
}
