package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	"pdpa-platform/internal/pkg/bizcal"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/notify"
	workflowstore "pdpa-platform/internal/platform/workflow/store"
)

// EntityType is the audit entity type of an instance: its history is the audit trail under this type.
const (
	EntityType           = "workflow_instance"
	DefinitionEntityType = "workflow_definition"
)

var (
	ErrNotFound          = errors.New("workflow: not found")
	ErrForbidden         = errors.New("workflow: forbidden")
	ErrInvalidTransition = errors.New("workflow: transition not allowed")
	ErrVersionMismatch   = errors.New("workflow: version mismatch")
	ErrInvalidRequest    = errors.New("workflow: invalid request")
)

// Calendars is what the engine needs from ORG-20 (org/service.Service implements it): a business calendar
// by id, or the tenant's default for nil, and the id actually used (nil for the built-in default).
type Calendars interface {
	BusinessCalendar(ctx context.Context, id *uuid.UUID) (bizcal.Calendar, *uuid.UUID, error)
}

// Policy is registered by the module that owns a record type: who may see and move its workflows
// besides the people the work is assigned to, and optional hooks, e.g. for the module's own events
// (dsar.sla_warning) — they run in the same transaction.
type Policy struct {
	ReadPermission  string
	WritePermission string
	OnSLA           func(ctx context.Context, e SLAEvent) error
	OnTransition    func(ctx context.Context, e TransitionEvent) error
}

// SLAEvent is a reminder or a breach of an instance's SLA timer.
type SLAEvent struct {
	Kind       string // reminder | breached
	InstanceID uuid.UUID
	EntityType string
	EntityID   uuid.UUID
	Code       string
	DueAt      time.Time
}

// TransitionEvent is a completed transition.
type TransitionEvent struct {
	InstanceID uuid.UUID
	EntityType string
	EntityID   uuid.UUID
	From, To   string
	Completed  bool
}

// Service runs workflows for the tenant of the transaction in ctx.
type Service struct {
	Calendars Calendars
	Notify    *notify.Service
	River     *river.Client[pgx.Tx]
	Audit     *audit.Service
	// Now is the clock (injectable for deadline tests); nil means time.Now.
	Now func() time.Time

	mu       sync.RWMutex
	policies map[string]Policy
}

// Register sets the policy for workflows of an entity type (call at start-up, in api and worker alike).
func (s *Service) Register(entityType string, p Policy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.policies == nil {
		s.policies = map[string]Policy{}
	}
	s.policies[entityType] = p
}

func (s *Service) policy(entityType string) (Policy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policies[entityType]
	return p, ok
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// ---- definitions ----

// DefinitionRecord is one stored version of a definition.
type DefinitionRecord struct {
	ID         uuid.UUID
	Global     bool
	Code       string
	Name       string
	EntityType string
	Version    int32
	Definition Definition
	Active     bool
	RowVersion int32
	UpdatedAt  time.Time
}

// DefinitionInput is what an admin saves; every save is a new version (instances keep theirs).
type DefinitionInput struct {
	Code       string
	Name       string
	EntityType string
	Definition Definition
}

func (s *Service) ListDefinitions(ctx context.Context) ([]DefinitionRecord, error) {
	rows, err := workflowstore.New(pdb.MustTxFromContext(ctx)).ListDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DefinitionRecord, 0, len(rows))
	for _, r := range rows {
		d, err := toDefinition(workflowstore.GetDefinitionRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Service) GetDefinition(ctx context.Context, id uuid.UUID) (DefinitionRecord, error) {
	r, err := workflowstore.New(pdb.MustTxFromContext(ctx)).GetDefinition(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefinitionRecord{}, ErrNotFound
	}
	if err != nil {
		return DefinitionRecord{}, err
	}
	return toDefinition(r)
}

// CreateDefinition saves version 1 of a new code (a code the tenant already has is refused — save a
// new version of it instead). A global definition of the same code is overridden for this tenant.
func (s *Service) CreateDefinition(ctx context.Context, in DefinitionInput) (DefinitionRecord, error) {
	q := workflowstore.New(pdb.MustTxFromContext(ctx))
	n, err := q.MaxOwnDefinitionVersion(ctx, in.Code)
	if err != nil {
		return DefinitionRecord{}, err
	}
	if n > 0 {
		return DefinitionRecord{}, fmt.Errorf("%w: code %q exists; save a new version of it", ErrInvalidRequest, in.Code)
	}
	return s.saveVersion(ctx, in, 1)
}

// SaveVersion stores in as the next version of the definition id belongs to; the previous versions of the
// tenant stop being used for new instances. Running instances keep the version they started with.
func (s *Service) SaveVersion(ctx context.Context, id uuid.UUID, rowVersion int32, in DefinitionInput) (DefinitionRecord, error) {
	cur, err := s.GetDefinition(ctx, id)
	if err != nil {
		return DefinitionRecord{}, err
	}
	if !cur.Global && cur.RowVersion != rowVersion {
		return DefinitionRecord{}, ErrVersionMismatch
	}
	if !cur.Global && !cur.Active {
		return DefinitionRecord{}, fmt.Errorf("%w: not the current version", ErrVersionMismatch)
	}
	in.Code = cur.Code
	n, err := workflowstore.New(pdb.MustTxFromContext(ctx)).MaxOwnDefinitionVersion(ctx, cur.Code)
	if err != nil {
		return DefinitionRecord{}, err
	}
	return s.saveVersion(ctx, in, n+1)
}

func (s *Service) saveVersion(ctx context.Context, in DefinitionInput, version int32) (DefinitionRecord, error) {
	if !keyRE.MatchString(in.Code) || !keyRE.MatchString(in.EntityType) || len([]rune(in.Name)) == 0 || len([]rune(in.Name)) > 200 {
		return DefinitionRecord{}, fmt.Errorf("%w: code, entity type or name", ErrInvalidDefinition)
	}
	if err := in.Definition.Validate(); err != nil {
		return DefinitionRecord{}, err
	}
	if err := s.checkReferences(ctx, in.Definition); err != nil {
		return DefinitionRecord{}, err
	}
	body, err := json.Marshal(in.Definition)
	if err != nil {
		return DefinitionRecord{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return DefinitionRecord{}, err
	}
	q := workflowstore.New(pdb.MustTxFromContext(ctx))
	if err := q.DeactivateOwnDefinitions(ctx, in.Code); err != nil {
		return DefinitionRecord{}, err
	}
	r, err := q.InsertDefinition(ctx, workflowstore.InsertDefinitionParams{ID: id, Code: in.Code, Name: in.Name, EntityType: in.EntityType, VersionNo: version, Definition: body})
	if err != nil {
		return DefinitionRecord{}, err
	}
	d, err := toDefinition(workflowstore.GetDefinitionRow(r))
	if err != nil {
		return DefinitionRecord{}, err
	}
	return d, s.audit(ctx, "platform.workflow_definition.save", DefinitionEntityType, d.ID, nil, map[string]any{"code": d.Code, "version": d.Version, "entity_type": d.EntityType})
}

// References lists the users and groups a definition assigns work or escalates to.
func (d Definition) References() (users, groups []uuid.UUID) {
	for _, st := range d.States {
		if st.Task != nil {
			if st.Task.AssigneeUserID != nil {
				users = append(users, *st.Task.AssigneeUserID)
			}
			if st.Task.AssigneeGroupID != nil {
				groups = append(groups, *st.Task.AssigneeGroupID)
			}
		}
	}
	if d.SLA != nil {
		users = append(users, d.SLA.EscalateUserIDs...)
		if d.SLA.EscalateGroupID != nil {
			groups = append(groups, *d.SLA.EscalateGroupID)
		}
	}
	return users, groups
}

// checkReferences verifies that the users, groups and calendar a definition names exist in the tenant.
func (s *Service) checkReferences(ctx context.Context, d Definition) error {
	users, groups := d.References()
	if d.SLA != nil {
		if d.SLA.CalendarID != nil {
			if _, _, err := s.Calendars.BusinessCalendar(ctx, d.SLA.CalendarID); err != nil {
				return invalid("calendar %s", d.SLA.CalendarID)
			}
		}
	}
	names, err := iamservice.Names(ctx, users)
	if err != nil {
		return err
	}
	for _, u := range users {
		if _, ok := names[u]; !ok {
			return invalid("user %s is not an active user", u)
		}
	}
	gnames, err := iamservice.GroupNames(ctx, groups)
	if err != nil {
		return err
	}
	for _, g := range groups {
		if _, ok := gnames[g]; !ok {
			return invalid("group %s", g)
		}
	}
	return nil
}

func toDefinition(r workflowstore.GetDefinitionRow) (DefinitionRecord, error) {
	var d Definition
	if err := json.Unmarshal(r.Definition, &d); err != nil {
		return DefinitionRecord{}, fmt.Errorf("workflow: definition %s: %w", r.ID, err)
	}
	return DefinitionRecord{ID: r.ID, Global: !r.TenantID.Valid, Code: r.Code, Name: r.Name, EntityType: r.EntityType, Version: r.VersionNo,
		Definition: d, Active: r.IsActive, RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}, nil
}

// ---- shared helpers ----

func (s *Service) audit(ctx context.Context, action, entityType string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	tenant, actor, err := actorOf(ctx)
	if err != nil {
		return err
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: entityType, EntityID: &id, Before: before, After: after}
	if actor != nil {
		e.ActorType, e.ActorID = "user", actor
	}
	return s.Audit.Write(ctx, e)
}

// actorOf is the tenant of the transaction and the signed-in user, if any (none in worker jobs).
func actorOf(ctx context.Context) (uuid.UUID, *uuid.UUID, error) {
	g, _ := authz.FromContext(ctx)
	if tenant, err := uuid.Parse(g.TenantID); err == nil {
		if u, err := uuid.Parse(g.UserID); err == nil {
			return tenant, &u, nil
		}
		return tenant, nil, nil
	}
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, nil, err
	}
	tenant, err := uuid.Parse(t)
	return tenant, nil, err
}

func currentUser(ctx context.Context) (uuid.UUID, authz.Grants, bool) {
	g, _ := authz.FromContext(ctx)
	u, err := uuid.Parse(g.UserID)
	return u, g, err == nil
}
