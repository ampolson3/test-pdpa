// Package collab is the shared collaboration layer for records of every module (PLT-07): comments with
// replies and @mentions, file attachments (PLT-09) and an activity feed (PLT-12 audit rows), keyed
// polymorphically by (entity_type, entity_id).
//
// A module opts a record type in by registering a Policy: the permission to see its conversation, the
// permission to take part, and an existence check under RLS (there is no foreign key to a polymorphic id,
// so the check is what stops comments on another tenant's or a made-up record — decisions D-20).
// Unregistered types are refused.
package collab

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	collabstore "pdpa-platform/internal/platform/collab/store"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/notify"
)

// Policy is what a module registers for one of its record types.
type Policy struct {
	ReadPermission  string // see comments, attachments, activity
	WritePermission string // comment, reply, resolve, attach
	// Exists reports whether id is a record of this type visible in the transaction in ctx (RLS).
	Exists func(ctx context.Context, id uuid.UUID) (bool, error)
}

var (
	ErrUnknownType     = errors.New("collab: record type not registered")
	ErrNotFound        = errors.New("collab: not found")
	ErrForbidden       = errors.New("collab: not allowed")
	ErrInvalid         = errors.New("collab: invalid")
	ErrVersionMismatch = errors.New("collab: version mismatch")
	ErrHasReplies      = errors.New("collab: comment has replies")
)

// MaxBody is the longest comment accepted (runes).
const MaxBody = 5000

// Service is the collaboration service of one process.
type Service struct {
	Policies map[string]Policy
	Notify   *notify.Service
	Files    *files.Service
	Audit    *audit.Service
}

// Register adds a record type. Call at start-up, before serving.
func (s *Service) Register(entityType string, p Policy) {
	if s.Policies == nil {
		s.Policies = map[string]Policy{}
	}
	s.Policies[entityType] = p
}

// FilePermissions is the read permission per registered type, for files.Service.EntityPermissions, so a
// record's attachments are downloadable by whoever may see the record.
func (s *Service) FilePermissions() map[string]string {
	out := map[string]string{}
	for t, p := range s.Policies {
		out[t] = p.ReadPermission
	}
	return out
}

// Comment is a comment with its author's name resolved.
type Comment struct {
	ID         uuid.UUID
	ParentID   *uuid.UUID
	AuthorID   uuid.UUID
	AuthorName string
	Body       string
	Mentions   []uuid.UUID
	Resolved   bool
	RowVersion int32
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// mentionRE matches @[Display name](user-uuid), the markup the comment box inserts when a user is picked.
var mentionRE = regexp.MustCompile(`@\[[^\]\n]{1,100}\]\(([0-9a-fA-F-]{36})\)`)

// authorize resolves the policy, checks the caller's permission and that the record exists.
func (s *Service) authorize(ctx context.Context, entityType string, entityID uuid.UUID, write bool) (Policy, authz.Grants, error) {
	p, ok := s.Policies[entityType]
	if !ok {
		return p, authz.Grants{}, ErrUnknownType
	}
	g, _ := authz.FromContext(ctx)
	perm := p.ReadPermission
	if write {
		perm = p.WritePermission
	}
	if !g.Has(perm) {
		return p, g, ErrForbidden
	}
	ok, err := p.Exists(ctx, entityID)
	if err != nil {
		return p, g, err
	}
	if !ok {
		return p, g, ErrNotFound
	}
	return p, g, nil
}

func userOf(g authz.Grants) (uuid.UUID, error) {
	id, err := uuid.Parse(g.UserID)
	if err != nil {
		return uuid.Nil, ErrForbidden
	}
	return id, nil
}

// Comments lists a record's comments, oldest first (threads are rebuilt by the client from parent ids).
func (s *Service) Comments(ctx context.Context, entityType string, entityID uuid.UUID) ([]Comment, error) {
	if _, _, err := s.authorize(ctx, entityType, entityID, false); err != nil {
		return nil, err
	}
	rows, err := collabstore.New(pdb.MustTxFromContext(ctx)).ListComments(ctx, collabstore.ListCommentsParams{EntityType: entityType, EntityID: entityID})
	if err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toComment(collabstore.GetCommentRow(r)))
	}
	return out, s.withNames(ctx, out)
}

// AddComment posts a comment (or a reply, with parentID) on a record, notifies the users it mentions and
// the author of the comment it replies to, and records it in the record's activity.
func (s *Service) AddComment(ctx context.Context, entityType string, entityID uuid.UUID, parentID *uuid.UUID, body string) (Comment, error) {
	_, g, err := s.authorize(ctx, entityType, entityID, true)
	if err != nil {
		return Comment{}, err
	}
	author, err := userOf(g)
	if err != nil {
		return Comment{}, err
	}
	body, err = cleanBody(body)
	if err != nil {
		return Comment{}, err
	}
	q := collabstore.New(pdb.MustTxFromContext(ctx))

	var parent *collabstore.GetCommentRow
	if parentID != nil {
		p, err := q.GetComment(ctx, *parentID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (p.EntityType != entityType || p.EntityID != entityID)) {
			return Comment{}, fmt.Errorf("%w: reply to a comment of another record", ErrInvalid)
		}
		if err != nil {
			return Comment{}, err
		}
		if p.ParentID.Valid {
			return Comment{}, fmt.Errorf("%w: replies are one level deep", ErrInvalid)
		}
		parent = &p
	}

	mentions, err := s.mentions(ctx, body, author)
	if err != nil {
		return Comment{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Comment{}, err
	}
	params := collabstore.InsertCommentParams{ID: id, EntityType: entityType, EntityID: entityID, AuthorID: author, Body: body, Mentions: mentions}
	if parentID != nil {
		params.ParentID = pgtype.UUID{Bytes: *parentID, Valid: true}
	}
	row, err := q.InsertComment(ctx, params)
	if err != nil {
		return Comment{}, fmt.Errorf("collab: insert comment: %w", err)
	}
	c := toComment(collabstore.GetCommentRow(row))

	names, err := iamservice.Names(ctx, []uuid.UUID{author})
	if err != nil {
		return Comment{}, err
	}
	c.AuthorName = names[author]
	excerpt := plainExcerpt(body)
	for _, m := range mentions {
		if err := s.notify(ctx, "collab.mention", m, c, entityType, entityID, excerpt); err != nil {
			return Comment{}, err
		}
	}
	if parent != nil && parent.AuthorID != author && !contains(mentions, parent.AuthorID) {
		if err := s.notify(ctx, "collab.reply", parent.AuthorID, c, entityType, entityID, excerpt); err != nil {
			return Comment{}, err
		}
	}
	return c, s.audit(ctx, g, "platform.comment.create", entityType, entityID, nil, map[string]any{"comment_id": c.ID, "mentions": len(mentions)})
}

// EditComment changes the body of the caller's own comment at version (If-Match). Newly mentioned users
// are notified; the edit is in the record's activity.
func (s *Service) EditComment(ctx context.Context, id uuid.UUID, version int32, body string) (Comment, error) {
	q := collabstore.New(pdb.MustTxFromContext(ctx))
	cur, err := s.loadForWrite(ctx, q, id)
	if err != nil {
		return Comment{}, err
	}
	g, _ := authz.FromContext(ctx)
	author, err := userOf(g)
	if err != nil {
		return Comment{}, err
	}
	if cur.AuthorID != author {
		return Comment{}, ErrForbidden
	}
	if cur.RowVersion != version {
		return Comment{}, ErrVersionMismatch
	}
	body, err = cleanBody(body)
	if err != nil {
		return Comment{}, err
	}
	mentions, err := s.mentions(ctx, body, author)
	if err != nil {
		return Comment{}, err
	}
	row, err := q.UpdateCommentBody(ctx, collabstore.UpdateCommentBodyParams{ID: id, Body: body, Mentions: mentions, RowVersion: version, AuthorID: author})
	if errors.Is(err, pgx.ErrNoRows) {
		return Comment{}, ErrVersionMismatch
	}
	if err != nil {
		return Comment{}, err
	}
	c := toComment(collabstore.GetCommentRow(row))
	names, err := iamservice.Names(ctx, []uuid.UUID{author})
	if err != nil {
		return Comment{}, err
	}
	c.AuthorName = names[author]
	for _, m := range mentions {
		if !contains(cur.Mentions, m) {
			if err := s.notify(ctx, "collab.mention", m, c, cur.EntityType, cur.EntityID, plainExcerpt(body)); err != nil {
				return Comment{}, err
			}
		}
	}
	return c, s.audit(ctx, g, "platform.comment.edit", cur.EntityType, cur.EntityID, nil, map[string]any{"comment_id": id})
}

// DeleteComment removes the caller's own comment at version, unless it has replies.
func (s *Service) DeleteComment(ctx context.Context, id uuid.UUID, version int32) error {
	q := collabstore.New(pdb.MustTxFromContext(ctx))
	cur, err := s.loadForWrite(ctx, q, id)
	if err != nil {
		return err
	}
	g, _ := authz.FromContext(ctx)
	author, err := userOf(g)
	if err != nil {
		return err
	}
	if cur.AuthorID != author {
		return ErrForbidden
	}
	n, err := q.DeleteComment(ctx, collabstore.DeleteCommentParams{ID: id, RowVersion: version, AuthorID: author})
	if err != nil {
		return err
	}
	if n == 0 {
		if cur.RowVersion != version {
			return ErrVersionMismatch
		}
		return ErrHasReplies
	}
	return s.audit(ctx, g, "platform.comment.delete", cur.EntityType, cur.EntityID, map[string]any{"comment_id": id}, nil)
}

// SetResolved marks a top-level comment thread resolved or open again; anyone who may comment can.
func (s *Service) SetResolved(ctx context.Context, id uuid.UUID, resolved bool) (Comment, error) {
	q := collabstore.New(pdb.MustTxFromContext(ctx))
	cur, err := s.loadForWrite(ctx, q, id)
	if err != nil {
		return Comment{}, err
	}
	row, err := q.SetCommentResolved(ctx, collabstore.SetCommentResolvedParams{ID: id, Resolved: resolved})
	if errors.Is(err, pgx.ErrNoRows) {
		return Comment{}, fmt.Errorf("%w: only a thread's first comment can be resolved", ErrInvalid)
	}
	if err != nil {
		return Comment{}, err
	}
	c := toComment(collabstore.GetCommentRow(row))
	if err := s.withNames(ctx, []Comment{c}); err != nil {
		return Comment{}, err
	}
	g, _ := authz.FromContext(ctx)
	return c, s.audit(ctx, g, "platform.comment.resolve", cur.EntityType, cur.EntityID,
		map[string]any{"resolved": cur.ResolvedAt.Valid}, map[string]any{"comment_id": id, "resolved": resolved})
}

// loadForWrite loads a comment and checks the caller may write on its record.
func (s *Service) loadForWrite(ctx context.Context, q *collabstore.Queries, id uuid.UUID) (collabstore.GetCommentRow, error) {
	cur, err := q.GetComment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return cur, ErrNotFound
	}
	if err != nil {
		return cur, err
	}
	if _, _, err := s.authorize(ctx, cur.EntityType, cur.EntityID, true); err != nil {
		if errors.Is(err, ErrForbidden) {
			return cur, ErrNotFound // don't confirm a comment exists to someone who can't see its record
		}
		return cur, err
	}
	return cur, nil
}

// mentions extracts the mentioned user ids, keeping only active users of the tenant other than the author.
func (s *Service) mentions(ctx context.Context, body string, author uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	for _, m := range mentionRE.FindAllStringSubmatch(body, 20) {
		id, err := uuid.Parse(m[1])
		if err == nil && id != author && !contains(ids, id) {
			ids = append(ids, id)
		}
	}
	names, err := iamservice.Names(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := []uuid.UUID{}
	for _, id := range ids {
		if _, ok := names[id]; ok {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *Service) notify(ctx context.Context, template string, to uuid.UUID, c Comment, entityType string, entityID uuid.UUID, excerpt string) error {
	if s.Notify == nil {
		return nil
	}
	_, err := s.Notify.Send(ctx, notify.Request{
		TemplateCode: template, Channel: notify.ChannelInApp, RecipientUserID: &to,
		Vars:       map[string]any{"author": c.AuthorName, "excerpt": excerpt},
		EntityType: entityType, EntityID: &entityID,
	})
	return err
}

func (s *Service) audit(ctx context.Context, g authz.Grants, action, entityType string, entityID uuid.UUID, before, after map[string]any) error {
	if s.Audit == nil {
		return nil
	}
	tenant, _ := uuid.Parse(g.TenantID)
	actor, _ := uuid.Parse(g.UserID)
	e := audit.Entry{TenantID: tenant, ActorType: "user", ActorID: &actor, Action: action, EntityType: entityType, EntityID: &entityID}
	if before != nil {
		e.Before = before
	}
	if after != nil {
		e.After = after
	}
	return s.Audit.Write(ctx, e)
}

func (s *Service) withNames(ctx context.Context, cs []Comment) error {
	var ids []uuid.UUID
	for _, c := range cs {
		ids = append(ids, c.AuthorID)
	}
	names, err := iamservice.Names(ctx, ids)
	if err != nil {
		return err
	}
	for i := range cs {
		cs[i].AuthorName = names[cs[i].AuthorID]
	}
	return nil
}

func cleanBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > MaxBody || !utf8.ValidString(body) {
		return "", fmt.Errorf("%w: comment must be 1–%d characters", ErrInvalid, MaxBody)
	}
	return body, nil
}

// plainExcerpt turns @[Name](id) into @Name and cuts to 120 characters, for notifications.
func plainExcerpt(body string) string {
	s := mentionRE.ReplaceAllStringFunc(body, func(m string) string {
		return "@" + m[2:strings.Index(m, "](")]
	})
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120]) + "…"
	}
	return s
}

func toComment(r collabstore.GetCommentRow) Comment {
	c := Comment{
		ID: r.ID, AuthorID: r.AuthorID, Body: r.Body, Mentions: r.Mentions, Resolved: r.ResolvedAt.Valid,
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.ParentID.Valid {
		id := uuid.UUID(r.ParentID.Bytes)
		c.ParentID = &id
	}
	if c.Mentions == nil {
		c.Mentions = []uuid.UUID{}
	}
	return c
}

func contains(ids []uuid.UUID, id uuid.UUID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
