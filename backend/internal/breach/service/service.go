// Package service is the breach module (BRE): the incident register and its ST-03 lifecycle (BRE-02), the 72-hour
// clock with reminders and escalation (BRE-07), risk assessment on a PLT-06 form (BRE-05), the notification decision
// (BRE-06), notices to affected data subjects (BRE-10), evidence and an append-only timeline (BRE-12), and search
// (BRE-13). It works in the transaction of the context (CLAUDE.md rule 1) and reads other modules only through their
// services (rule 9).
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/platform/notify"
)

// Entity types (audit, files, notifications, forms).
const (
	IncidentType            = "breach_incident"
	SubjectNotificationType = "breach_subject_notification"
	// FormType is the PLT-06 form type of breach risk assessments; its scoring bands must be none / low / high.
	FormType = "breach"
)

// Permission codes (docs/security/permissions.yaml).
const (
	PermRead         = "breach.incident.read"
	PermCreate       = "breach.incident.create"
	PermUpdate       = "breach.incident.update"
	PermApprove      = "breach.incident.approve"
	PermNoticeRead   = "breach.notification.read"
	PermNoticeCreate = "breach.notification.create"
	PermNoticeUpdate = "breach.notification.update"
	PermNoticeSend   = "breach.notification.approve"
)

var (
	ErrNotFound          = errors.New("breach: not found")
	ErrForbidden         = errors.New("breach: forbidden")
	ErrVersionMismatch   = errors.New("breach: version mismatch")
	ErrInvalidTransition = errors.New("breach: invalid transition")
	ErrInvalidRequest    = errors.New("breach: invalid request")
	ErrClosed            = errors.New("breach: incident is closed")
	ErrNoAssessment      = errors.New("breach: no risk assessment")
	ErrDecisionTooWeak   = errors.New("breach: decision is weaker than the assessed risk requires")
	ErrPDPCNoticeMissing = errors.New("breach: the PDPC notice has not been recorded")
	ErrNoticeNotRequired = errors.New("breach: data subjects are not to be notified for this incident")
	ErrSelfApproval      = errors.New("breach: the maker of a notice can't approve it")
	ErrBadForm           = errors.New("breach: form is not a published breach assessment with none/low/high bands")
	ErrFileNotUsable     = errors.New("breach: file not usable")
)

// FieldError is one problem of a request, by field or CSV line (codes the UI localizes).
type FieldError struct {
	Field string
	Code  string
}

// ValidationError lists field problems (422).
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return fmt.Sprintf("breach: invalid: %v", e.Fields) }
func (e *ValidationError) Unwrap() error { return ErrInvalidRequest }

// Org is what the breach module reads from the organization module.
type Org interface {
	GetLegalEntity(ctx context.Context, id uuid.UUID) (orgservice.LegalEntity, error)
	ListMaster(ctx context.Context, kind string) ([]orgservice.MasterItem, error)
}

// Files is what the breach module needs from file storage (PLT-09).
type Files interface {
	Get(ctx context.Context, id uuid.UUID) (files.File, error)
	Open(ctx context.Context, id uuid.UUID) (io.ReadCloser, files.File, error)
	AttachSystem(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error
}

// Service runs the breach module for the tenant of the transaction in ctx.
type Service struct {
	Events  *events.Publisher
	Notify  *notify.Service
	Forms   *forms.Service
	Files   Files
	Keyring *crypto.Keyring
	Audit   *audit.Service
	Org     Org
	River   *river.Client[pgx.Tx]
	Now     func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// tenantID is the tenant of the transaction (requests and worker jobs alike).
func tenantID(ctx context.Context) (uuid.UUID, error) {
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(t)
}

func currentUser(ctx context.Context) *uuid.UUID {
	g, _ := authz.FromContext(ctx)
	if u, err := uuid.Parse(g.UserID); err == nil {
		return &u
	}
	return nil
}

func has(ctx context.Context, perm string) bool {
	g, _ := authz.FromContext(ctx)
	return g.Has(perm)
}

func (s *Service) audit(ctx context.Context, action, entityType string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	tenant, err := tenantID(ctx)
	if err != nil {
		return err
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: entityType, EntityID: &id, Before: before, After: after}
	if u := currentUser(ctx); u != nil {
		e.ActorType, e.ActorID = "user", u
	}
	return s.Audit.Write(ctx, e)
}

func invalid(field, code string) error { return &ValidationError{Fields: []FieldError{{field, code}}} }
