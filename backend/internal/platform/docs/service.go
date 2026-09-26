// Package docs is the document composer (PLT-16): documents stored as structured content (ProseMirror JSON from the
// TipTap editor, render.Content), merge fields filled from the organization (and, as they are built, other modules),
// a versioned clause library, and document templates. Drafts and approval run on PLT-08 (one record type per
// document type, "document_<type>"); publishing freezes the text, the merge field values and the clauses into an
// immutable platform.document_versions row and renders it to PDF and DOCX in the background (docs.render). Any two
// versions — drafts included — can be compared block by block.
//
// Permissions follow the owning module of each document type (DocTypes, registered in internal/wiring): a notice
// is read with notice.document.read, a DPA with agreement.dpa.read, and so on; the clause library uses
// agreement.clause.*.
package docs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"pdpa-platform/internal/pkg/authz"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/versioning"
)

var (
	ErrNotFound        = errors.New("docs: not found")
	ErrForbidden       = errors.New("docs: forbidden")
	ErrUnknownType     = errors.New("docs: no module offers this document type")
	ErrVersionMismatch = errors.New("docs: version mismatch")
	ErrInvalidRequest  = errors.New("docs: invalid request")
	ErrInvalidState    = errors.New("docs: not allowed in the current state")
	ErrCodeTaken       = errors.New("docs: code already used")
)

// IncompleteError is returned when a document can't be published: merge fields without a value or clauses that
// aren't in the library (published). The UI names them.
type IncompleteError struct {
	Fields  []string
	Clauses []string
}

func (e *IncompleteError) Error() string {
	return fmt.Sprintf("docs: incomplete (fields %v, clauses %v)", e.Fields, e.Clauses)
}

// Unwrap lets PLT-08's publish endpoint, which runs the publish hook, report it as an invalid request (422).
func (e *IncompleteError) Unwrap() error { return versioning.ErrInvalidRequest }

// Policy is a document type's permissions, given by the module that owns the type.
type Policy struct {
	Read, Create, Update, Publish string
	// Approver is the role that checks a version before it is published (PLT-08 maker-checker).
	Approver string
	// TemplateRead / TemplateWrite guard the type's templates; publishing a template needs Publish (legal wording
	// stays a draft until someone allowed to publish documents of the type releases it, CLAUDE.md rule 8).
	TemplateRead, TemplateWrite string
}

// Clause library permissions (DSA-06 / DPA): LEGAL maintains the library, DPO and AUDIT read it.
const (
	ClauseRead    = "agreement.clause.read"
	ClauseCreate  = "agreement.clause.create"
	ClauseUpdate  = "agreement.clause.update"
	ClausePublish = "agreement.clause.publish"
)

// EntityType is the PLT-08 record type (and files / audit entity type) of a document type's documents.
func EntityType(docType string) string { return "document_" + docType }

// VersionEntityType is the entity type the rendered files of a published version are attached to.
func VersionEntityType(docType string) string { return "document_version_" + docType }

// OrgFields is the organization's side of merge fields (ORG-01): orgservice.Service satisfies it.
type OrgFields interface {
	MergeFields(ctx context.Context, legalEntityID uuid.UUID) (map[string]string, error)
}

// Service runs the composer for the tenant of the transaction in ctx.
type Service struct {
	Versioning *versioning.Service
	Files      *files.Service
	River      *river.Client[pgx.Tx]
	Audit      *audit.Service
	Org        OrgFields
	PDF        render.PDFRenderer // nil: PDFs are not produced (the version's render_status says failed)
	Now        func() time.Time

	mu       sync.RWMutex
	policies map[string]Policy
}

// Register offers a document type (at start-up, in internal/wiring).
func (s *Service) Register(docType string, p Policy) {
	if p.Approver == "" {
		panic("docs: " + docType + " needs an approver role")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.policies == nil {
		s.policies = map[string]Policy{}
	}
	s.policies[docType] = p
}

func (s *Service) policy(docType string) (Policy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policies[docType]
	return p, ok
}

// Types lists the registered document types, sorted.
func (s *Service) Types() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.policies))
	for t := range s.policies {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// FilePermissions maps the entity types this package attaches files to onto the permission needed to download them
// (files.Service.EntityPermissions).
func (s *Service) FilePermissions() map[string]string {
	out := map[string]string{}
	for _, t := range s.Types() {
		p, _ := s.policy(t)
		out[VersionEntityType(t)] = p.Read
	}
	return out
}

// RegisterVersioning puts every document type under PLT-08 (call once, after the types are registered).
func (s *Service) RegisterVersioning() {
	for _, t := range s.Types() {
		p, _ := s.policy(t)
		docType := t
		s.Versioning.Register(EntityType(t), versioning.Policy{
			ReadPermission: p.Read, EditPermission: p.Update, PublishPermission: p.Publish,
			Steps: []versioning.Step{{Role: p.Approver}},
			Title: s.title,
			OnPublish: func(ctx context.Context, id uuid.UUID, snapshot json.RawMessage) error {
				return s.publish(ctx, docType, id, snapshot)
			},
		})
	}
}

// TypeAccess is what the caller may do with a document type.
type TypeAccess struct {
	DocType                       string
	Read, Create, Update, Publish bool
	TemplateRead, TemplateWrite   bool
}

// Access lists the document types the caller can read, with what else they may do.
func (s *Service) Access(ctx context.Context) []TypeAccess {
	g, _ := authz.FromContext(ctx)
	out := []TypeAccess{}
	for _, t := range s.Types() {
		p, _ := s.policy(t)
		if !g.Has(p.Read) {
			continue
		}
		out = append(out, TypeAccess{DocType: t, Read: true, Create: g.Has(p.Create), Update: g.Has(p.Update), Publish: g.Has(p.Publish),
			TemplateRead: g.Has(p.TemplateRead), TemplateWrite: g.Has(p.TemplateWrite)})
	}
	return out
}

func (s *Service) readableTypes(ctx context.Context) []string {
	g, _ := authz.FromContext(ctx)
	out := []string{}
	for _, t := range s.Types() {
		if p, _ := s.policy(t); g.Has(p.Read) {
			out = append(out, t)
		}
	}
	return out
}

// need checks one of a type's permissions; an unknown type is ErrUnknownType.
func (s *Service) need(ctx context.Context, docType string, pick func(Policy) string) (Policy, error) {
	p, ok := s.policy(docType)
	if !ok {
		return Policy{}, ErrUnknownType
	}
	g, _ := authz.FromContext(ctx)
	if !g.Has(pick(p)) {
		return Policy{}, ErrForbidden
	}
	return p, nil
}

func can(ctx context.Context, codes ...string) bool {
	g, _ := authz.FromContext(ctx)
	return slices.ContainsFunc(codes, g.Has)
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func currentUser(ctx context.Context) (uuid.UUID, authz.Grants, bool) {
	g, _ := authz.FromContext(ctx)
	u, err := uuid.Parse(g.UserID)
	return u, g, err == nil
}

func (s *Service) audit(ctx context.Context, action, entityType string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	me, g, ok := currentUser(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		return ErrForbidden
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: entityType, EntityID: &id, Before: before, After: after}
	if ok {
		e.ActorType, e.ActorID = "user", &me
	}
	return s.Audit.Write(ctx, e)
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, a...))
}
