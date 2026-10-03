// Package versioning is the versioning & approval framework (PLT-08): snapshots of a record's content as
// numbered versions (platform.record_versions), each moving draft → in_review → approved → published,
// with multi-level maker-checker approval (platform.approvals). A published version is never edited: a
// change is a new draft. Modules register a record type with Register and save drafts from their own
// services (SaveDraft); the publish hook copies the approved content into the module's record.
package versioning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/notify"
	versioningstore "pdpa-platform/internal/platform/versioning/store"
)

// EntityType is the audit entity type of a version.
const EntityType = "record_version"

var (
	ErrNotFound          = errors.New("versioning: not found")
	ErrForbidden         = errors.New("versioning: forbidden")
	ErrInvalidTransition = errors.New("versioning: not allowed in the version's current status")
	ErrLocked            = errors.New("versioning: a version is under review or approved; it can't be edited")
	ErrVersionMismatch   = errors.New("versioning: version mismatch")
	ErrSelfApproval      = errors.New("versioning: the author, the requester or an earlier approver can't approve")
	ErrInvalidRequest    = errors.New("versioning: invalid request")
)

// Step is one approval level: someone holding Role (a role code, e.g. DPO or LEGAL) decides it.
type Step struct {
	Role string
}

// Policy is registered by the module owning a record type.
type Policy struct {
	ReadPermission    string
	EditPermission    string // save drafts and submit them
	PublishPermission string
	Steps             []Step // at least one: every version is checked by someone other than its maker
	// Title names the record in the approval inbox and notifications (optional).
	Title func(ctx context.Context, entityID uuid.UUID) (string, error)
	// OnPublish applies the published snapshot to the module's record, in the same transaction.
	OnPublish func(ctx context.Context, entityID uuid.UUID, snapshot json.RawMessage) error
}

// Service runs versions and approvals for the tenant of the transaction in ctx.
type Service struct {
	Notify *notify.Service
	Audit  *audit.Service
	Now    func() time.Time

	mu       sync.RWMutex
	policies map[string]Policy
}

// Register sets the policy of a record type (at start-up). It panics on a policy without approval steps.
func (s *Service) Register(entityType string, p Policy) {
	if len(p.Steps) == 0 {
		panic("versioning: " + entityType + " needs at least one approval step")
	}
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

// Version is one version of a record.
type Version struct {
	ID         uuid.UUID
	EntityType string
	EntityID   uuid.UUID
	No         int32
	Snapshot   json.RawMessage
	Diff       []Change // against the version published when it was submitted
	Status     string   // draft | in_review | approved | published | superseded
	AuthorID   *uuid.UUID
	AuthorName string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	RowVersion int32
	Approvals  []Approval
}

// Approval is one approval step of a version.
type Approval struct {
	ID           uuid.UUID
	VersionID    uuid.UUID
	Step         int32
	Role         string
	RequestedBy  uuid.UUID
	Requester    string
	ApproverID   *uuid.UUID
	ApproverName string
	Decision     string // pending | approved | rejected | returned
	Reason       string
	DecidedAt    *time.Time
	RowVersion   int32
}

func (s *Service) authorize(ctx context.Context, entityType, need string) (Policy, authz.Grants, error) {
	p, ok := s.policy(entityType)
	if !ok {
		return p, authz.Grants{}, ErrNotFound
	}
	g, _ := authz.FromContext(ctx)
	var perm string
	switch need {
	case "read":
		if g.Has(p.EditPermission) || g.Has(p.PublishPermission) || slices.ContainsFunc(p.Steps, func(st Step) bool { return slices.Contains(g.Roles, st.Role) }) {
			return p, g, nil
		}
		perm = p.ReadPermission
	case "edit":
		perm = p.EditPermission
	case "publish":
		perm = p.PublishPermission
	}
	if perm == "" || !g.Has(perm) {
		if need == "read" {
			return p, g, ErrNotFound
		}
		return p, g, ErrForbidden
	}
	return p, g, nil
}

// SaveDraft stores snapshot as the record's open draft: the existing draft is replaced, or a new version
// (latest number + 1) is started. A version under review or approved blocks edits (ErrLocked); a published
// one is never touched — changing published content always means a new version (acceptance, PLT-08).
func (s *Service) SaveDraft(ctx context.Context, entityType string, entityID uuid.UUID, snapshot any) (Version, error) {
	if _, _, err := s.authorize(ctx, entityType, "edit"); err != nil {
		return Version{}, err
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return Version{}, fmt.Errorf("%w: snapshot: %v", ErrInvalidRequest, err)
	}
	q := versioningstore.New(pdb.MustTxFromContext(ctx))
	open, err := q.OpenVersion(ctx, versioningstore.OpenVersionParams{EntityType: entityType, EntityID: entityID})
	switch {
	case err == nil && open.Status != "draft":
		return Version{}, ErrLocked
	case err == nil:
		r, err := q.UpdateVersion(ctx, versioningstore.UpdateVersionParams{ID: open.ID, Snapshot: body, Diff: open.Diff, Status: "draft"})
		if err != nil {
			return Version{}, err
		}
		v := toVersion(versioningstore.GetVersionRow(r))
		return v, s.audit(ctx, "platform.version.save_draft", v.ID, nil, map[string]any{"entity_type": entityType, "entity_id": entityID, "version": v.No})
	case !errors.Is(err, pgx.ErrNoRows):
		return Version{}, err
	}
	no, err := q.NextVersionNo(ctx, versioningstore.NextVersionNoParams{EntityType: entityType, EntityID: entityID})
	if err != nil {
		return Version{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Version{}, err
	}
	var r versioningstore.InsertVersionRow
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		r, err = versioningstore.New(pdb.MustTxFromContext(ctx)).InsertVersion(ctx, versioningstore.InsertVersionParams{
			ID: id, EntityType: entityType, EntityID: entityID, VersionNo: no, Snapshot: body})
		return err
	})
	if isUnique(err) { // a concurrent SaveDraft opened the draft first
		return Version{}, ErrLocked
	}
	if err != nil {
		return Version{}, err
	}
	v := toVersion(versioningstore.GetVersionRow(r))
	return v, s.audit(ctx, "platform.version.create", v.ID, nil, map[string]any{"entity_type": entityType, "entity_id": entityID, "version": v.No})
}

// Published returns the record's published version, or ErrNotFound.
func (s *Service) Published(ctx context.Context, entityType string, entityID uuid.UUID) (Version, error) {
	r, err := versioningstore.New(pdb.MustTxFromContext(ctx)).PublishedVersion(ctx, versioningstore.PublishedVersionParams{EntityType: entityType, EntityID: entityID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	return toVersion(versioningstore.GetVersionRow(r)), nil
}

// List returns a record's versions, newest first.
func (s *Service) List(ctx context.Context, entityType string, entityID uuid.UUID) ([]Version, error) {
	if _, _, err := s.authorize(ctx, entityType, "read"); err != nil {
		return nil, err
	}
	rows, err := versioningstore.New(pdb.MustTxFromContext(ctx)).ListVersions(ctx, versioningstore.ListVersionsParams{EntityType: entityType, EntityID: entityID})
	if err != nil {
		return nil, err
	}
	out := make([]Version, 0, len(rows))
	for _, r := range rows {
		out = append(out, toVersion(versioningstore.GetVersionRow(r)))
	}
	return out, s.name(ctx, out)
}

// Get returns one version with its approval steps.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Version, error) {
	q := versioningstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetVersion(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	if _, _, err := s.authorize(ctx, r.EntityType, "read"); err != nil {
		return Version{}, ErrNotFound
	}
	v := toVersion(r)
	if v.Approvals, err = s.approvals(ctx, id); err != nil {
		return Version{}, err
	}
	vs := []Version{v}
	return vs[0], s.name(ctx, vs)
}

// Compare diffs two versions of the same record (from → to).
func (s *Service) Compare(ctx context.Context, fromID, toID uuid.UUID) ([]Change, error) {
	a, err := s.Get(ctx, fromID)
	if err != nil {
		return nil, err
	}
	b, err := s.Get(ctx, toID)
	if err != nil {
		return nil, err
	}
	if a.EntityType != b.EntityType || a.EntityID != b.EntityID {
		return nil, fmt.Errorf("%w: versions of different records", ErrInvalidRequest)
	}
	return Diff(a.Snapshot, b.Snapshot)
}

// Submit sends a draft for review: its diff against the published version is fixed and one pending
// approval per policy step is opened, for the submitter as requester.
func (s *Service) Submit(ctx context.Context, id uuid.UUID, rowVersion int32) (Version, error) {
	v, p, g, err := s.lock(ctx, id, rowVersion, "edit")
	if err != nil {
		return Version{}, err
	}
	if v.Status != "draft" {
		return Version{}, ErrInvalidTransition
	}
	var base json.RawMessage
	if pub, err := s.Published(ctx, v.EntityType, v.EntityID); err == nil {
		base = pub.Snapshot
	} else if !errors.Is(err, ErrNotFound) {
		return Version{}, err
	}
	changes, err := Diff(base, v.Snapshot)
	if err != nil {
		return Version{}, err
	}
	diff, _ := json.Marshal(changes)
	requester, err := uuid.Parse(g.UserID)
	if err != nil {
		return Version{}, ErrForbidden
	}
	q := versioningstore.New(pdb.MustTxFromContext(ctx))
	for i, st := range p.Steps {
		aid, err := uuid.NewV7()
		if err != nil {
			return Version{}, err
		}
		role := st.Role
		if _, err := q.InsertApproval(ctx, versioningstore.InsertApprovalParams{ID: aid, EntityType: v.EntityType, EntityID: v.EntityID,
			RecordVersionID: pgtype.UUID{Bytes: v.ID, Valid: true}, StepNo: int32(i + 1), RequestedBy: requester, ApproverRole: &role}); err != nil {
			return Version{}, err
		}
	}
	out, err := s.setStatus(ctx, v, "in_review", diff)
	if err != nil {
		return Version{}, err
	}
	return out, s.audit(ctx, "platform.version.submit", v.ID, map[string]any{"status": "draft"}, map[string]any{"status": "in_review", "steps": len(p.Steps)})
}

// Decide records an approver's decision on the current step. Approving the last step approves the
// version; returning or rejecting sends it back to draft (later steps are dropped; a resubmission starts
// a new round). Maker-checker: the version's author, its requester and anyone who approved an earlier step
// can't decide (acceptance, PLT-08).
func (s *Service) Decide(ctx context.Context, approvalID uuid.UUID, rowVersion int32, decision, reason string) (Version, error) {
	if decision != "approved" && decision != "rejected" && decision != "returned" {
		return Version{}, fmt.Errorf("%w: decision", ErrInvalidRequest)
	}
	reason = strings.TrimSpace(reason)
	if decision != "approved" && reason == "" {
		return Version{}, fmt.Errorf("%w: a reason is required to reject or return", ErrInvalidRequest)
	}
	if len([]rune(reason)) > 2000 {
		return Version{}, fmt.Errorf("%w: reason too long", ErrInvalidRequest)
	}
	q := versioningstore.New(pdb.MustTxFromContext(ctx))
	a, err := q.LockApproval(ctx, approvalID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !a.RecordVersionID.Valid) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	g, _ := authz.FromContext(ctx)
	me, err := uuid.Parse(g.UserID)
	if err != nil {
		return Version{}, ErrForbidden
	}
	if _, _, err := s.authorize(ctx, a.EntityType, "read"); err != nil {
		return Version{}, ErrNotFound
	}
	vr, err := q.LockVersion(ctx, a.RecordVersionID.Bytes)
	if err != nil {
		return Version{}, err
	}
	v := toVersion(versioningstore.GetVersionRow(vr))
	if a.RowVersion != rowVersion {
		return Version{}, ErrVersionMismatch
	}
	if a.Decision != "pending" || v.Status != "in_review" {
		return Version{}, ErrInvalidTransition
	}
	if a.ApproverRole == nil || !slices.Contains(g.Roles, *a.ApproverRole) {
		return Version{}, ErrForbidden
	}
	steps, err := s.approvals(ctx, v.ID)
	if err != nil {
		return Version{}, err
	}
	last := true
	for _, st := range steps {
		if st.Decision == "pending" && st.Step < a.StepNo {
			return Version{}, ErrInvalidTransition // an earlier level decides first
		}
		if st.Decision == "pending" && st.Step > a.StepNo {
			last = false
		}
		if st.Decision == "approved" && st.ApproverID != nil && *st.ApproverID == me {
			return Version{}, ErrSelfApproval
		}
	}
	if me == a.RequestedBy || (v.AuthorID != nil && *v.AuthorID == me) {
		return Version{}, ErrSelfApproval
	}
	now := s.now()
	if err := q.DecideApproval(ctx, versioningstore.DecideApprovalParams{ID: a.ID, Decision: decision, Reason: optional(reason),
		ApproverUserID: pgtype.UUID{Bytes: me, Valid: true}, DecidedAt: pgtype.Timestamptz{Time: now, Valid: true}}); err != nil {
		return Version{}, err
	}
	next := v
	switch {
	case decision != "approved":
		if err := q.DropPendingApprovals(ctx, pgtype.UUID{Bytes: v.ID, Valid: true}); err != nil {
			return Version{}, err
		}
		if next, err = s.setStatus(ctx, v, "draft", nil); err != nil {
			return Version{}, err
		}
	case last:
		if next, err = s.setStatus(ctx, v, "approved", nil); err != nil {
			return Version{}, err
		}
	}
	if err := s.audit(ctx, "platform.version.decide", v.ID, map[string]any{"status": v.Status},
		map[string]any{"status": next.Status, "step": a.StepNo, "decision": decision}); err != nil {
		return Version{}, err
	}
	if decision != "approved" || last {
		if err := s.notifyDecision(ctx, v, a.RequestedBy, decision, reason, g); err != nil {
			return Version{}, err
		}
	}
	return next, nil
}

// Publish makes an approved version the record's published one: the previous published version is
// superseded and the module's OnPublish hook applies the snapshot, all in one transaction.
func (s *Service) Publish(ctx context.Context, id uuid.UUID, rowVersion int32) (Version, error) {
	v, p, _, err := s.lock(ctx, id, rowVersion, "publish")
	if err != nil {
		return Version{}, err
	}
	if v.Status != "approved" {
		return Version{}, ErrInvalidTransition
	}
	if prev, err := s.Published(ctx, v.EntityType, v.EntityID); err == nil {
		if _, err := s.setStatus(ctx, prev, "superseded", nil); err != nil {
			return Version{}, err
		}
	} else if !errors.Is(err, ErrNotFound) {
		return Version{}, err
	}
	out, err := s.setStatus(ctx, v, "published", nil)
	if err != nil {
		return Version{}, err
	}
	if p.OnPublish != nil {
		if err := p.OnPublish(ctx, v.EntityID, v.Snapshot); err != nil {
			return Version{}, err
		}
	}
	return out, s.audit(ctx, "platform.version.publish", v.ID, map[string]any{"status": "approved"}, map[string]any{"status": "published", "version": v.No})
}

// InboxItem is an approval waiting on the caller.
type InboxItem struct {
	Approval
	EntityType string
	EntityID   uuid.UUID
	VersionNo  int32
	Title      string
	AuthorName string
}

// Inbox lists the approvals the caller may decide now (their roles, the current step, maker-checker).
func (s *Service) Inbox(ctx context.Context) ([]InboxItem, error) {
	g, _ := authz.FromContext(ctx)
	me, err := uuid.Parse(g.UserID)
	if err != nil {
		return nil, ErrForbidden
	}
	roles := g.Roles
	if roles == nil {
		roles = []string{}
	}
	rows, err := versioningstore.New(pdb.MustTxFromContext(ctx)).MyPendingApprovals(ctx, versioningstore.MyPendingApprovalsParams{Roles: roles, UserID: me})
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	authors := make([]*uuid.UUID, 0, len(rows))
	out := make([]InboxItem, 0, len(rows))
	for _, r := range rows {
		p, ok := s.policy(r.EntityType)
		if !ok {
			continue
		}
		it := InboxItem{Approval: Approval{ID: r.ID, VersionID: r.RecordVersionID.Bytes, Step: r.StepNo, RequestedBy: r.RequestedBy, Decision: "pending", RowVersion: r.RowVersion},
			EntityType: r.EntityType, EntityID: r.EntityID, VersionNo: r.VersionNo}
		if r.ApproverRole != nil {
			it.Role = *r.ApproverRole
		}
		if p.Title != nil {
			if it.Title, err = p.Title(ctx, r.EntityID); err != nil {
				return nil, err
			}
		}
		ids = append(ids, r.RequestedBy)
		var author *uuid.UUID
		if r.Author.Valid {
			a := uuid.UUID(r.Author.Bytes)
			author = &a
			ids = append(ids, a)
		}
		authors = append(authors, author)
		out = append(out, it)
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Requester = names[out[i].RequestedBy]
		if authors[i] != nil {
			out[i].AuthorName = names[*authors[i]]
		}
	}
	return out, nil
}

// ---- helpers ----

func (s *Service) lock(ctx context.Context, id uuid.UUID, rowVersion int32, need string) (Version, Policy, authz.Grants, error) {
	r, err := versioningstore.New(pdb.MustTxFromContext(ctx)).LockVersion(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, Policy{}, authz.Grants{}, ErrNotFound
	}
	if err != nil {
		return Version{}, Policy{}, authz.Grants{}, err
	}
	if _, _, err := s.authorize(ctx, r.EntityType, "read"); err != nil {
		return Version{}, Policy{}, authz.Grants{}, ErrNotFound
	}
	p, g, err := s.authorize(ctx, r.EntityType, need)
	if err != nil {
		return Version{}, p, g, err
	}
	if r.RowVersion != rowVersion {
		return Version{}, p, g, ErrVersionMismatch
	}
	return toVersion(versioningstore.GetVersionRow(r)), p, g, nil
}

func (s *Service) setStatus(ctx context.Context, v Version, status string, diff []byte) (Version, error) {
	if diff == nil {
		diff, _ = json.Marshal(v.Diff)
		if v.Diff == nil {
			diff = nil
		}
	}
	r, err := versioningstore.New(pdb.MustTxFromContext(ctx)).UpdateVersion(ctx, versioningstore.UpdateVersionParams{ID: v.ID, Snapshot: v.Snapshot, Diff: diff, Status: status})
	if err != nil {
		return Version{}, err
	}
	return toVersion(versioningstore.GetVersionRow(r)), nil
}

func (s *Service) approvals(ctx context.Context, versionID uuid.UUID) ([]Approval, error) {
	rows, err := versioningstore.New(pdb.MustTxFromContext(ctx)).ListApprovals(ctx, pgtype.UUID{Bytes: versionID, Valid: true})
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	out := make([]Approval, 0, len(rows))
	for _, r := range rows {
		a := Approval{ID: r.ID, VersionID: versionID, Step: r.StepNo, RequestedBy: r.RequestedBy, Decision: r.Decision, RowVersion: r.RowVersion}
		if r.ApproverRole != nil {
			a.Role = *r.ApproverRole
		}
		if r.ApproverUserID.Valid {
			u := uuid.UUID(r.ApproverUserID.Bytes)
			a.ApproverID = &u
			ids = append(ids, u)
		}
		if r.Reason != nil {
			a.Reason = *r.Reason
		}
		if r.DecidedAt.Valid {
			t := r.DecidedAt.Time
			a.DecidedAt = &t
		}
		ids = append(ids, r.RequestedBy)
		out = append(out, a)
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Requester = names[out[i].RequestedBy]
		if out[i].ApproverID != nil {
			out[i].ApproverName = names[*out[i].ApproverID]
		}
	}
	return out, nil
}

func (s *Service) name(ctx context.Context, vs []Version) error {
	var ids []uuid.UUID
	for _, v := range vs {
		if v.AuthorID != nil {
			ids = append(ids, *v.AuthorID)
		}
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return err
	}
	for i := range vs {
		if vs[i].AuthorID != nil {
			vs[i].AuthorName = names[*vs[i].AuthorID]
		}
	}
	return nil
}

// notifyDecision tells the requester the outcome: approved (all steps), returned or rejected.
func (s *Service) notifyDecision(ctx context.Context, v Version, to uuid.UUID, decision, reason string, g authz.Grants) error {
	if s.Notify == nil {
		return nil
	}
	title := v.EntityType
	if p, ok := s.policy(v.EntityType); ok && p.Title != nil {
		t, err := p.Title(ctx, v.EntityID)
		if err != nil {
			return err
		}
		title = t
	}
	me, _ := uuid.Parse(g.UserID)
	names, err := iamservice.AllNames(ctx, []uuid.UUID{me})
	if err != nil {
		return err
	}
	vars := map[string]any{"title": title, "version": v.No, "approver": names[me]}
	if decision != "approved" {
		vars["reason"] = reason
	}
	id := v.ID
	_, err = s.Notify.Send(ctx, notify.Request{TemplateCode: "approval." + decision, Channel: notify.ChannelInApp, RecipientUserID: &to,
		Vars: vars, EntityType: EntityType, EntityID: &id})
	return err
}

func (s *Service) audit(ctx context.Context, action string, id uuid.UUID, before, after any) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		return ErrForbidden
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: action, EntityType: EntityType, EntityID: &id, Before: before, After: after}
	if u, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &u
	}
	return s.Audit.Write(ctx, e)
}

func toVersion(r versioningstore.GetVersionRow) Version {
	v := Version{ID: r.ID, EntityType: r.EntityType, EntityID: r.EntityID, No: r.VersionNo, Snapshot: r.Snapshot, Status: r.Status,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time, RowVersion: r.RowVersion}
	if r.CreatedBy.Valid {
		u := uuid.UUID(r.CreatedBy.Bytes)
		v.AuthorID = &u
	}
	if len(r.Diff) > 0 {
		_ = json.Unmarshal(r.Diff, &v.Diff)
	}
	return v
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
