package forms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
	formsstore "pdpa-platform/internal/platform/forms/store"
	"pdpa-platform/internal/platform/notify"
)

const (
	FormEntityType     = "form"
	ResponseEntityType = "form_response"
)

var (
	ErrNotFound        = errors.New("forms: not found")
	ErrForbidden       = errors.New("forms: forbidden")
	ErrUnknownType     = errors.New("forms: no module offers this form type")
	ErrVersionMismatch = errors.New("forms: version mismatch")
	ErrInvalidState    = errors.New("forms: not allowed in the current state")
	ErrInvalidRequest  = errors.New("forms: invalid request")
)

// AnswerErrors is returned when answers don't pass the schema; Fields says which and why.
type AnswerErrors struct{ Fields []FieldError }

func (e *AnswerErrors) Error() string {
	return fmt.Sprintf("forms: %d answer(s) invalid", len(e.Fields))
}

// Policy is registered per form type by the module that uses it: who may read, create, change and publish
// its forms, and who may start a response inside the admin app ("" = only through the module, e.g. the
// public consent API).
type Policy struct {
	Read, Create, Update, Publish string
	Respond                       string
}

// Service runs forms for the tenant of the transaction in ctx.
type Service struct {
	Notify *notify.Service
	Audit  *audit.Service

	mu       sync.RWMutex
	policies map[string]Policy
}

// Register sets the policy of a form type (at start-up, in internal/wiring).
func (s *Service) Register(formType string, p Policy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.policies == nil {
		s.policies = map[string]Policy{}
	}
	s.policies[formType] = p
}

func (s *Service) policy(formType string) (Policy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policies[formType]
	return p, ok
}

// Types lists the registered form types the caller may read.
func (s *Service) Types(ctx context.Context) []string {
	g, _ := authz.FromContext(ctx)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for t, p := range s.policies {
		if g.Has(p.Read) {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out
}

// TypeInfo is a form type the caller may read and what else they may do with it.
type TypeInfo struct {
	Type                                         string
	CanCreate, CanUpdate, CanPublish, CanRespond bool
}

// TypeInfos lists the form types the caller may read.
func (s *Service) TypeInfos(ctx context.Context) []TypeInfo {
	g, _ := authz.FromContext(ctx)
	has := func(code string) bool { return code != "" && g.Has(code) }
	var out []TypeInfo
	for _, t := range s.Types(ctx) {
		p, _ := s.policy(t)
		out = append(out, TypeInfo{Type: t, CanCreate: has(p.Create), CanUpdate: has(p.Update), CanPublish: has(p.Publish), CanRespond: has(p.Respond)})
	}
	return out
}

func (s *Service) allow(ctx context.Context, formType string, perm func(Policy) string) (Policy, error) {
	p, ok := s.policy(formType)
	if !ok {
		return p, ErrUnknownType
	}
	g, _ := authz.FromContext(ctx)
	if code := perm(p); code == "" || !g.Has(code) {
		return p, ErrForbidden
	}
	return p, nil
}

// ---- definitions and versions ----

// Form is a form definition with its versions (newest first).
type Form struct {
	ID               uuid.UUID
	Global           bool
	Code             string
	Name             string
	Type             string
	Status           string // draft | published | retired
	CurrentVersionID *uuid.UUID
	RowVersion       int32
	UpdatedAt        time.Time
	LatestVersion    int32
	Versions         []Version
}

// Version is one version of a form's schema. Draft versions (not published) are edited in place.
type Version struct {
	ID          uuid.UUID
	FormID      uuid.UUID
	No          int32
	Schema      Schema
	Scoring     *Scoring
	Languages   []string
	PublishedAt *time.Time
	RowVersion  int32
	UpdatedAt   time.Time
}

// Draft is what the builder saves.
type Draft struct {
	Schema    Schema
	Scoring   *Scoring
	Languages []string
}

var formCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,59}$`)

func (d *Draft) check() error {
	if len(d.Languages) == 0 {
		d.Languages = []string{"th", "en"}
	}
	for _, l := range d.Languages {
		if l != "th" && l != "en" {
			return invalid("language %q", l)
		}
	}
	if !slices.Contains(d.Languages, "th") {
		return invalid("Thai is always a language of the form")
	}
	if d.Scoring != nil && len(d.Scoring.Bands) == 0 {
		d.Scoring = nil
	}
	return Validate(d.Schema, d.Scoring)
}

// ListForms returns the forms of the types the caller may read (optionally one type).
func (s *Service) ListForms(ctx context.Context, formType string) ([]Form, error) {
	types := s.Types(ctx)
	if formType != "" {
		if !slices.Contains(types, formType) {
			return []Form{}, nil
		}
		types = []string{formType}
	}
	if types == nil {
		types = []string{}
	}
	rows, err := formsstore.New(pdb.MustTxFromContext(ctx)).ListForms(ctx, types)
	if err != nil {
		return nil, err
	}
	out := make([]Form, 0, len(rows))
	for _, r := range rows {
		f := toForm(formsstore.GetFormRow{ID: r.ID, TenantID: r.TenantID, Code: r.Code, Name: r.Name, FormType: r.FormType, Status: r.Status,
			CurrentVersionID: r.CurrentVersionID, RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt})
		f.LatestVersion = r.LatestVersion
		out = append(out, f)
	}
	return out, nil
}

// CreateForm adds a form with its first draft version.
func (s *Service) CreateForm(ctx context.Context, code, name, formType string, d Draft) (Form, error) {
	if _, err := s.allow(ctx, formType, func(p Policy) string { return p.Create }); err != nil {
		return Form{}, err
	}
	name = strings.TrimSpace(name)
	if !formCode.MatchString(code) || name == "" || len([]rune(name)) > 200 {
		return Form{}, fmt.Errorf("%w: code or name", ErrInvalidRequest)
	}
	if err := d.check(); err != nil {
		return Form{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Form{}, err
	}
	var row formsstore.GetFormRow
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		r, err := formsstore.New(pdb.MustTxFromContext(ctx)).InsertForm(ctx, formsstore.InsertFormParams{ID: id, Code: code, Name: name, FormType: formType})
		row = formsstore.GetFormRow(r)
		return err
	})
	if isUnique(err) {
		return Form{}, fmt.Errorf("%w: code exists", ErrInvalidRequest)
	}
	if err != nil {
		return Form{}, err
	}
	if _, err := s.insertVersion(ctx, id, d); err != nil {
		return Form{}, err
	}
	if err := s.audit(ctx, "platform.form.create", FormEntityType, id, nil, map[string]any{"code": code, "form_type": formType}); err != nil {
		return Form{}, err
	}
	return s.GetForm(ctx, row.ID)
}

// GetForm returns a form with all its versions.
func (s *Service) GetForm(ctx context.Context, id uuid.UUID) (Form, error) {
	q := formsstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetForm(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Form{}, ErrNotFound
	}
	if err != nil {
		return Form{}, err
	}
	if _, err := s.allow(ctx, r.FormType, func(p Policy) string { return p.Read }); err != nil {
		return Form{}, ErrNotFound
	}
	f := toForm(r)
	rows, err := q.ListFormVersions(ctx, id)
	if err != nil {
		return Form{}, err
	}
	for _, v := range rows {
		ver, err := toVersion(formsstore.GetFormVersionRow(v))
		if err != nil {
			return Form{}, err
		}
		f.Versions = append(f.Versions, ver)
	}
	if len(f.Versions) > 0 {
		f.LatestVersion = f.Versions[0].No
	}
	return f, nil
}

// SaveDraft stores the builder's work as the form's draft version: the existing draft (If-Match on its
// row version) or, when the latest version is published, a new draft numbered after it (version 0 = none).
func (s *Service) SaveDraft(ctx context.Context, formID uuid.UUID, draftVersion int32, d Draft) (Version, error) {
	f, err := s.lockForm(ctx, formID, func(p Policy) string { return p.Update })
	if err != nil {
		return Version{}, err
	}
	if f.Status == "retired" {
		return Version{}, ErrInvalidState
	}
	if err := d.check(); err != nil {
		return Version{}, err
	}
	q := formsstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.DraftFormVersion(ctx, formID)
	if errors.Is(err, pgx.ErrNoRows) {
		if draftVersion != 0 {
			return Version{}, ErrVersionMismatch
		}
		v, err := s.insertVersion(ctx, formID, d)
		if err != nil {
			return Version{}, err
		}
		return v, s.audit(ctx, "platform.form.new_version", FormEntityType, formID, nil, map[string]any{"version": v.No})
	}
	if err != nil {
		return Version{}, err
	}
	if cur.RowVersion != draftVersion {
		return Version{}, ErrVersionMismatch
	}
	schema, scoring := marshal(d)
	r, err := q.UpdateDraftVersion(ctx, formsstore.UpdateDraftVersionParams{ID: cur.ID, RowVersion: draftVersion, Schema: schema, Scoring: scoring, Languages: d.Languages})
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrVersionMismatch
	}
	if err != nil {
		return Version{}, err
	}
	return toVersion(formsstore.GetFormVersionRow(r))
}

// Publish makes the draft the form's current version; responses started from now on use it, running
// responses keep theirs. The published version can never change again.
func (s *Service) Publish(ctx context.Context, formID uuid.UUID, draftVersion int32) (Form, error) {
	f, err := s.lockForm(ctx, formID, func(p Policy) string { return p.Publish })
	if err != nil {
		return Form{}, err
	}
	q := formsstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.DraftFormVersion(ctx, formID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Form{}, ErrInvalidState
	}
	if err != nil {
		return Form{}, err
	}
	v, err := toVersion(formsstore.GetFormVersionRow(cur))
	if err != nil {
		return Form{}, err
	}
	if err := Validate(v.Schema, v.Scoring); err != nil {
		return Form{}, err
	}
	pub, err := q.PublishVersion(ctx, formsstore.PublishVersionParams{ID: cur.ID, RowVersion: draftVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return Form{}, ErrVersionMismatch
	}
	if err != nil {
		return Form{}, err
	}
	if err := q.UpdateFormState(ctx, formsstore.UpdateFormStateParams{ID: formID, Name: f.Name, Status: "published", CurrentVersionID: pgtype.UUID{Bytes: pub.ID, Valid: true}}); err != nil {
		return Form{}, err
	}
	if err := s.audit(ctx, "platform.form.publish", FormEntityType, formID, map[string]any{"status": f.Status}, map[string]any{"status": "published", "version": pub.VersionNo}); err != nil {
		return Form{}, err
	}
	return s.GetForm(ctx, formID)
}

func (s *Service) lockForm(ctx context.Context, id uuid.UUID, perm func(Policy) string) (Form, error) {
	r, err := formsstore.New(pdb.MustTxFromContext(ctx)).LockForm(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Form{}, ErrNotFound
	}
	if err != nil {
		return Form{}, err
	}
	if _, err := s.allow(ctx, r.FormType, func(p Policy) string { return p.Read }); err != nil {
		return Form{}, ErrNotFound
	}
	if _, err := s.allow(ctx, r.FormType, perm); err != nil {
		return Form{}, err
	}
	if !r.TenantID.Valid {
		return Form{}, ErrForbidden // platform forms are changed by the provider only
	}
	return toForm(formsstore.GetFormRow(r)), nil
}

func (s *Service) insertVersion(ctx context.Context, formID uuid.UUID, d Draft) (Version, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Version{}, err
	}
	schema, scoring := marshal(d)
	r, err := formsstore.New(pdb.MustTxFromContext(ctx)).InsertFormVersion(ctx, formsstore.InsertFormVersionParams{ID: id, FormID: formID, Schema: schema, Scoring: scoring, Languages: d.Languages})
	if err != nil {
		return Version{}, err
	}
	return toVersion(formsstore.GetFormVersionRow(r))
}

// ---- responses ----

// Assignment is one section of a response given to someone to answer.
type Assignment struct {
	ID           uuid.UUID
	Section      string
	AssigneeID   uuid.UUID
	AssigneeName string
	Status       string // open | done
	CompletedAt  *time.Time
	RowVersion   int32
}

// Response is a (draft or submitted) set of answers to one form version.
type Response struct {
	ID          uuid.UUID
	Form        Form // without versions
	Version     Version
	EntityType  string
	EntityID    *uuid.UUID
	OwnerID     *uuid.UUID
	OwnerName   string
	Status      string // draft | submitted
	Answers     Answers
	Result      Result
	SubmittedAt *time.Time
	RowVersion  int32
	Assignments []Assignment
	// What the caller may do: answer these sections (all for the owner, else their own open ones).
	CanAnswer []string
	IsOwner   bool
}

// StartResponse opens a draft response to the form's current published version, owned by the caller.
func (s *Service) StartResponse(ctx context.Context, formID uuid.UUID, entityType string, entityID *uuid.UUID) (Response, error) {
	q := formsstore.New(pdb.MustTxFromContext(ctx))
	fr, err := q.GetForm(ctx, formID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Response{}, ErrNotFound
	}
	if err != nil {
		return Response{}, err
	}
	if _, err := s.allow(ctx, fr.FormType, func(p Policy) string { return p.Read }); err != nil {
		return Response{}, ErrNotFound
	}
	if _, err := s.allow(ctx, fr.FormType, func(p Policy) string { return p.Respond }); err != nil {
		return Response{}, err
	}
	if fr.Status != "published" || !fr.CurrentVersionID.Valid {
		return Response{}, ErrInvalidState
	}
	me, _, _ := currentUser(ctx)
	id, err := uuid.NewV7()
	if err != nil {
		return Response{}, err
	}
	p := formsstore.InsertSubmissionParams{ID: id, FormVersionID: fr.CurrentVersionID.Bytes, SubmittedByType: "user", SubmittedBy: pgtype.UUID{Bytes: me, Valid: true},
		Answers: []byte("{}"), Status: "draft"}
	if entityType != "" {
		p.EntityType, p.EntityID = &entityType, pgUUID(entityID)
	}
	if _, err := q.InsertSubmission(ctx, p); err != nil {
		return Response{}, err
	}
	if err := s.audit(ctx, "platform.form.response_start", ResponseEntityType, id, nil, map[string]any{"form_id": formID}); err != nil {
		return Response{}, err
	}
	return s.GetResponse(ctx, id)
}

// GetResponse returns a response the caller owns, is assigned a section of, or may respond to as a
// holder of the form type's respond permission.
func (s *Service) GetResponse(ctx context.Context, id uuid.UUID) (Response, error) {
	r, _, err := s.loadResponse(ctx, id, false)
	return r, err
}

func (s *Service) loadResponse(ctx context.Context, id uuid.UUID, lock bool) (Response, formsstore.GetSubmissionRow, error) {
	q := formsstore.New(pdb.MustTxFromContext(ctx))
	var row formsstore.GetSubmissionRow
	var err error
	if lock {
		var r formsstore.LockSubmissionRow
		r, err = q.LockSubmission(ctx, id)
		row = formsstore.GetSubmissionRow(r)
	} else {
		row, err = q.GetSubmission(ctx, id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Response{}, row, ErrNotFound
	}
	if err != nil {
		return Response{}, row, err
	}
	vr, err := q.GetFormVersion(ctx, row.FormVersionID)
	if err != nil {
		return Response{}, row, err
	}
	ver, err := toVersion(vr)
	if err != nil {
		return Response{}, row, err
	}
	fr, err := q.GetForm(ctx, ver.FormID)
	if err != nil {
		return Response{}, row, err
	}
	resp := Response{ID: row.ID, Form: toForm(fr), Version: ver, Status: row.Status, RowVersion: row.RowVersion, OwnerID: uuidPtr(row.SubmittedBy), EntityID: uuidPtr(row.EntityID)}
	if row.EntityType != nil {
		resp.EntityType = *row.EntityType
	}
	if row.SubmittedAt.Valid && row.Status == "submitted" {
		t := row.SubmittedAt.Time
		resp.SubmittedAt = &t
	}
	if err := json.Unmarshal(row.Answers, &resp.Answers); err != nil {
		return Response{}, row, err
	}
	arows, err := q.ListAssignments(ctx, id)
	if err != nil {
		return Response{}, row, err
	}
	me, g, _ := currentUser(ctx)
	resp.IsOwner = resp.OwnerID != nil && *resp.OwnerID == me
	p, _ := s.policy(resp.Form.Type)
	respondent := p.Respond != "" && g.Has(p.Respond)
	var ids []uuid.UUID
	assigned := map[string]bool{}
	for _, a := range arows {
		as := Assignment{ID: a.ID, Section: a.SectionKey, AssigneeID: a.AssigneeUserID, Status: a.Status, RowVersion: a.RowVersion}
		if a.CompletedAt.Valid {
			t := a.CompletedAt.Time
			as.CompletedAt = &t
		}
		ids = append(ids, a.AssigneeUserID)
		resp.Assignments = append(resp.Assignments, as)
		assigned[a.SectionKey] = true
		if a.AssigneeUserID == me && a.Status == "open" && resp.Status == "draft" {
			resp.CanAnswer = append(resp.CanAnswer, a.SectionKey)
		}
	}
	if !resp.IsOwner && !respondent && len(slices.DeleteFunc(slices.Clone(resp.Assignments), func(a Assignment) bool { return a.AssigneeID != me })) == 0 {
		return Response{}, row, ErrNotFound
	}
	if resp.IsOwner && resp.Status == "draft" {
		for _, sec := range ver.Schema.Sections {
			if !assigned[sec.Key] && !slices.Contains(resp.CanAnswer, sec.Key) {
				resp.CanAnswer = append(resp.CanAnswer, sec.Key)
			}
		}
	}
	if resp.OwnerID != nil {
		ids = append(ids, *resp.OwnerID)
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return Response{}, row, err
	}
	for i := range resp.Assignments {
		resp.Assignments[i].AssigneeName = names[resp.Assignments[i].AssigneeID]
	}
	if resp.OwnerID != nil {
		resp.OwnerName = names[*resp.OwnerID]
	}
	if len(row.Result) > 0 && resp.Status == "submitted" {
		_ = json.Unmarshal(row.Result, &resp.Result)
	} else {
		resp.Result = Evaluate(ver.Schema, ver.Scoring, resp.Answers, false, nil)
	}
	return resp, row, nil
}

// SaveAnswers merges answers into a draft response: the owner answers any section not assigned to someone
// else, an assignee only their open sections. Invalid answers are refused (AnswerErrors); answers to
// questions that end up hidden are dropped.
func (s *Service) SaveAnswers(ctx context.Context, id uuid.UUID, version int32, answers Answers) (Response, error) {
	resp, row, err := s.loadResponse(ctx, id, true)
	if err != nil {
		return Response{}, err
	}
	if resp.RowVersion != version {
		return Response{}, ErrVersionMismatch
	}
	if resp.Status != "draft" {
		return Response{}, ErrInvalidState
	}
	for key := range answers {
		sec := resp.Version.Schema.SectionOf(key)
		if sec == "" {
			return Response{}, &AnswerErrors{Fields: []FieldError{{Question: key, Code: "unknown_question"}}}
		}
		if !slices.Contains(resp.CanAnswer, sec) {
			return Response{}, ErrForbidden
		}
	}
	merged := Answers{}
	for k, v := range resp.Answers {
		merged[k] = v
	}
	for k, v := range answers {
		if isEmpty(v) {
			delete(merged, k)
		} else {
			merged[k] = v
		}
	}
	res := Evaluate(resp.Version.Schema, resp.Version.Scoring, merged, false, resp.CanAnswer)
	if len(res.Errors) > 0 {
		return Response{}, &AnswerErrors{Fields: res.Errors}
	}
	all := Evaluate(resp.Version.Schema, resp.Version.Scoring, merged, false, nil)
	body, _ := json.Marshal(all.Answers)
	if _, err := formsstore.New(pdb.MustTxFromContext(ctx)).UpdateSubmission(ctx, formsstore.UpdateSubmissionParams{ID: id, Answers: body, Status: "draft",
		Score: row.Score, Result: row.Result, SubmittedAt: row.SubmittedAt}); err != nil {
		return Response{}, err
	}
	return s.GetResponse(ctx, id)
}

// Assign gives a section of a draft response to a user (or takes it back with a nil user). Owner only.
func (s *Service) Assign(ctx context.Context, id uuid.UUID, version int32, section string, user *uuid.UUID) (Response, error) {
	resp, _, err := s.loadResponse(ctx, id, true)
	if err != nil {
		return Response{}, err
	}
	if !resp.IsOwner {
		return Response{}, ErrForbidden
	}
	if resp.RowVersion != version {
		return Response{}, ErrVersionMismatch
	}
	if resp.Status != "draft" || !slices.ContainsFunc(resp.Version.Schema.Sections, func(sc Section) bool { return sc.Key == section }) {
		return Response{}, fmt.Errorf("%w: section", ErrInvalidRequest)
	}
	q := formsstore.New(pdb.MustTxFromContext(ctx))
	if user == nil {
		if _, err := q.DeleteAssignment(ctx, formsstore.DeleteAssignmentParams{SubmissionID: id, SectionKey: section}); err != nil {
			return Response{}, err
		}
	} else {
		names, err := iamservice.Names(ctx, []uuid.UUID{*user})
		if err != nil {
			return Response{}, err
		}
		if _, ok := names[*user]; !ok {
			return Response{}, fmt.Errorf("%w: user", ErrInvalidRequest)
		}
		aid, err := uuid.NewV7()
		if err != nil {
			return Response{}, err
		}
		if _, err := q.UpsertAssignment(ctx, formsstore.UpsertAssignmentParams{ID: aid, SubmissionID: id, SectionKey: section, AssigneeUserID: *user}); err != nil {
			return Response{}, err
		}
		if err := s.notifyAssigned(ctx, resp, section, *user); err != nil {
			return Response{}, err
		}
	}
	if err := s.touch(ctx, id); err != nil {
		return Response{}, err
	}
	if err := s.audit(ctx, "platform.form.assign", ResponseEntityType, id, nil, map[string]any{"section": section, "assignee_user_id": user}); err != nil {
		return Response{}, err
	}
	return s.GetResponse(ctx, id)
}

// CompleteSection marks the caller's assigned section done once its visible required questions are answered.
func (s *Service) CompleteSection(ctx context.Context, id uuid.UUID, section string) (Response, error) {
	resp, _, err := s.loadResponse(ctx, id, true)
	if err != nil {
		return Response{}, err
	}
	me, _, _ := currentUser(ctx)
	if resp.Status != "draft" {
		return Response{}, ErrInvalidState
	}
	res := Evaluate(resp.Version.Schema, resp.Version.Scoring, resp.Answers, true, []string{section})
	if len(res.Errors) > 0 {
		return Response{}, &AnswerErrors{Fields: res.Errors}
	}
	n, err := formsstore.New(pdb.MustTxFromContext(ctx)).CompleteAssignment(ctx, formsstore.CompleteAssignmentParams{SubmissionID: id, SectionKey: section, AssigneeUserID: me})
	if err != nil {
		return Response{}, err
	}
	if n == 0 {
		return Response{}, ErrForbidden
	}
	if err := s.touch(ctx, id); err != nil {
		return Response{}, err
	}
	if err := s.audit(ctx, "platform.form.section_done", ResponseEntityType, id, nil, map[string]any{"section": section}); err != nil {
		return Response{}, err
	}
	return s.GetResponse(ctx, id)
}

// Submit validates the whole response (visible required questions, every assigned section done), scores
// it and freezes it. Owner only.
func (s *Service) Submit(ctx context.Context, id uuid.UUID, version int32) (Response, error) {
	resp, _, err := s.loadResponse(ctx, id, true)
	if err != nil {
		return Response{}, err
	}
	if !resp.IsOwner {
		return Response{}, ErrForbidden
	}
	if resp.RowVersion != version {
		return Response{}, ErrVersionMismatch
	}
	if resp.Status != "draft" {
		return Response{}, ErrInvalidState
	}
	for _, a := range resp.Assignments {
		if a.Status != "done" {
			return Response{}, fmt.Errorf("%w: section %s is still with %s", ErrInvalidState, a.Section, a.AssigneeName)
		}
	}
	res := Evaluate(resp.Version.Schema, resp.Version.Scoring, resp.Answers, true, nil)
	if len(res.Errors) > 0 {
		return Response{}, &AnswerErrors{Fields: res.Errors}
	}
	if err := s.freeze(ctx, id, res); err != nil {
		return Response{}, err
	}
	if err := s.audit(ctx, "platform.form.submit", ResponseEntityType, id, map[string]any{"status": "draft"}, map[string]any{"status": "submitted", "score": res.Score, "band": res.Band}); err != nil {
		return Response{}, err
	}
	return s.GetResponse(ctx, id)
}

// Record stores a complete, validated submission in one step — for modules whose forms are filled outside
// the admin app (the public consent or DSAR forms). The module has authorized the submitter itself.
func (s *Service) Record(ctx context.Context, versionID uuid.UUID, answers Answers, submitterType string, submitter *uuid.UUID, entityType string, entityID *uuid.UUID) (uuid.UUID, Result, error) {
	q := formsstore.New(pdb.MustTxFromContext(ctx))
	vr, err := q.GetFormVersion(ctx, versionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, Result{}, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, Result{}, err
	}
	if !vr.PublishedAt.Valid {
		return uuid.Nil, Result{}, ErrInvalidState
	}
	ver, err := toVersion(vr)
	if err != nil {
		return uuid.Nil, Result{}, err
	}
	res := Evaluate(ver.Schema, ver.Scoring, answers, true, nil)
	if len(res.Errors) > 0 {
		return uuid.Nil, res, &AnswerErrors{Fields: res.Errors}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, Result{}, err
	}
	body, _ := json.Marshal(res.Answers)
	result, _ := json.Marshal(res)
	p := formsstore.InsertSubmissionParams{ID: id, FormVersionID: versionID, SubmittedByType: submitterType, SubmittedBy: pgUUID(submitter), Answers: body,
		Status: "submitted", Score: numeric(res.Score), Result: result, SubmittedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}}
	if entityType != "" {
		p.EntityType, p.EntityID = &entityType, pgUUID(entityID)
	}
	if _, err := q.InsertSubmission(ctx, p); err != nil {
		return uuid.Nil, Result{}, err
	}
	return id, res, nil
}

// ListResponses lists a form's responses for holders of its read permission.
func (s *Service) ListResponses(ctx context.Context, formID uuid.UUID) ([]Response, error) {
	if _, err := s.GetForm(ctx, formID); err != nil {
		return nil, err
	}
	rows, err := formsstore.New(pdb.MustTxFromContext(ctx)).ListResponses(ctx, formID)
	if err != nil {
		return nil, err
	}
	var owners []uuid.UUID
	out := make([]Response, 0, len(rows))
	for _, r := range rows {
		resp := Response{ID: r.ID, Status: r.Status, RowVersion: r.RowVersion, OwnerID: uuidPtr(r.SubmittedBy)}
		if r.SubmittedAt.Valid && r.Status == "submitted" {
			t := r.SubmittedAt.Time
			resp.SubmittedAt = &t
		}
		if len(r.Result) > 0 {
			_ = json.Unmarshal(r.Result, &resp.Result)
		}
		if resp.OwnerID != nil {
			owners = append(owners, *resp.OwnerID)
		}
		out = append(out, resp)
	}
	names, err := iamservice.AllNames(ctx, owners)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].OwnerID != nil {
			out[i].OwnerName = names[*out[i].OwnerID]
		}
	}
	return out, nil
}

// AssignmentItem is a section waiting for the caller.
type AssignmentItem struct {
	ResponseID   uuid.UUID
	FormID       uuid.UUID
	FormName     string
	FormType     string
	Section      string
	SectionTitle Text
	CreatedAt    time.Time
}

// MyAssignments lists the open sections assigned to the caller.
func (s *Service) MyAssignments(ctx context.Context) ([]AssignmentItem, error) {
	me, _, ok := currentUser(ctx)
	if !ok {
		return nil, ErrForbidden
	}
	rows, err := formsstore.New(pdb.MustTxFromContext(ctx)).MyFormAssignments(ctx, me)
	if err != nil {
		return nil, err
	}
	out := make([]AssignmentItem, 0, len(rows))
	for _, r := range rows {
		var sch Schema
		_ = json.Unmarshal(r.Schema, &sch)
		it := AssignmentItem{ResponseID: r.SubmissionID, FormID: r.FormID, FormName: r.FormName, FormType: r.FormType, Section: r.SectionKey, CreatedAt: r.CreatedAt.Time}
		for _, sec := range sch.Sections {
			if sec.Key == r.SectionKey {
				it.SectionTitle = sec.Title
			}
		}
		out = append(out, it)
	}
	return out, nil
}

// ---- helpers ----

func (s *Service) freeze(ctx context.Context, id uuid.UUID, res Result) error {
	body, _ := json.Marshal(res.Answers)
	result, _ := json.Marshal(res)
	_, err := formsstore.New(pdb.MustTxFromContext(ctx)).UpdateSubmission(ctx, formsstore.UpdateSubmissionParams{ID: id, Answers: body, Status: "submitted",
		Score: numeric(res.Score), Result: result, SubmittedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
	return err
}

// touch bumps a response's row version (assignments change what the response is).
func (s *Service) touch(ctx context.Context, id uuid.UUID) error {
	row, err := formsstore.New(pdb.MustTxFromContext(ctx)).GetSubmission(ctx, id)
	if err != nil {
		return err
	}
	_, err = formsstore.New(pdb.MustTxFromContext(ctx)).UpdateSubmission(ctx, formsstore.UpdateSubmissionParams{ID: id, Answers: row.Answers, Status: row.Status,
		Score: row.Score, Result: row.Result, SubmittedAt: row.SubmittedAt})
	return err
}

func (s *Service) notifyAssigned(ctx context.Context, resp Response, section string, user uuid.UUID) error {
	me, _, _ := currentUser(ctx)
	if s.Notify == nil || user == me {
		return nil
	}
	title := section
	for _, sec := range resp.Version.Schema.Sections {
		if sec.Key == section {
			title = sec.Title["th"]
		}
	}
	id := resp.ID
	_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "form.section_assigned", Channel: notify.ChannelInApp, RecipientUserID: &user,
		Vars: map[string]any{"form": resp.Form.Name, "section": title}, EntityType: ResponseEntityType, EntityID: &id})
	return err
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

func currentUser(ctx context.Context) (uuid.UUID, authz.Grants, bool) {
	g, _ := authz.FromContext(ctx)
	u, err := uuid.Parse(g.UserID)
	return u, g, err == nil
}

func marshal(d Draft) ([]byte, []byte) {
	schema, _ := json.Marshal(d.Schema)
	var scoring []byte
	if d.Scoring != nil {
		scoring, _ = json.Marshal(d.Scoring)
	}
	return schema, scoring
}

func toForm(r formsstore.GetFormRow) Form {
	return Form{ID: r.ID, Global: !r.TenantID.Valid, Code: r.Code, Name: r.Name, Type: r.FormType, Status: r.Status, CurrentVersionID: uuidPtr(r.CurrentVersionID),
		RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}
}

func toVersion(r formsstore.GetFormVersionRow) (Version, error) {
	v := Version{ID: r.ID, FormID: r.FormID, No: r.VersionNo, Languages: r.Languages, RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}
	dec := json.NewDecoder(bytes.NewReader(r.Schema))
	if err := dec.Decode(&v.Schema); err != nil {
		return Version{}, fmt.Errorf("forms: version %s schema: %w", r.ID, err)
	}
	if len(r.Scoring) > 0 && string(r.Scoring) != "null" {
		v.Scoring = &Scoring{}
		if err := json.Unmarshal(r.Scoring, v.Scoring); err != nil {
			return Version{}, err
		}
	}
	if r.PublishedAt.Valid {
		t := r.PublishedAt.Time
		v.PublishedAt = &t
	}
	return v, nil
}

func numeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%.2f", f))
	return n
}

func pgUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	u := uuid.UUID(v.Bytes)
	return &u
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
