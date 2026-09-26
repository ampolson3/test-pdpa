package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	breachstore "pdpa-platform/internal/breach/store"
	iamservice "pdpa-platform/internal/iam/service"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// BreachTypes: confidentiality, integrity, availability (BRE-02).
var BreachTypes = []string{"confidentiality", "integrity", "availability"}

// ReportedVia values staff can record in the admin app (public_form / processor come with BRE-01 / BRE-03).
var ReportedVia = []string{"employee_form", "email", "phone", "system"}

// Incident is a breach in the register.
type Incident struct {
	ID                 uuid.UUID
	No                 string
	LegalEntityID      uuid.UUID
	ReportedVia        string
	ReporterID         *uuid.UUID
	Title              string
	Description        string
	BreachTypes        []string
	IncidentType       string
	OccurredAt         *time.Time
	AwareAt            time.Time
	ContainedAt        *time.Time
	AffectedSubjects   *int32
	AffectedCategories []uuid.UUID
	RiskLevel          string
	Decision           string
	DecisionReason     string
	DecidedBy          *uuid.UUID
	DueAt              time.Time
	IsDrill            bool
	Status             string
	OwnerID            *uuid.UUID
	CloseReason        string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	RowVersion         int32
	Clock              Clock
	OwnerName          string
	ReporterName       string
}

// Input is what staff record about an incident.
type Input struct {
	LegalEntityID       uuid.UUID
	ReportedVia         string
	Title               string
	Description         string
	BreachTypes         []string
	IncidentType        string
	OccurredAt          *time.Time
	AwareAt             time.Time
	ContainedAt         *time.Time
	AffectedSubjects    *int32
	AffectedCategoryIDs []uuid.UUID
	OwnerID             *uuid.UUID
	IsDrill             bool
	// AwareAtReason is required to change aware_at after the incident was recorded (SEQ-06: DPO only, audited).
	AwareAtReason string
}

func (s *Service) toIncident(r breachstore.GetIncidentRow) Incident {
	in := Incident{ID: r.ID, No: r.IncidentNo, LegalEntityID: r.LegalEntityID, ReportedVia: r.ReportedVia, ReporterID: uuidPtr(r.ReporterUserID),
		Title: r.Title, Description: r.Description, BreachTypes: r.BreachTypes, IncidentType: deref(r.IncidentType), OccurredAt: timePtr(r.OccurredAt),
		AwareAt: r.AwareAt.Time.UTC(), ContainedAt: timePtr(r.ContainedAt), AffectedSubjects: r.AffectedSubjects, AffectedCategories: r.AffectedCategories,
		RiskLevel: deref(r.RiskLevel), Decision: deref(r.Decision), DecisionReason: deref(r.DecisionReason), DecidedBy: uuidPtr(r.DecidedBy),
		DueAt: r.PdpcDueAt.Time.UTC(), IsDrill: r.IsDrill, Status: r.Status, OwnerID: uuidPtr(r.OwnerUserID), CloseReason: deref(r.CloseReason),
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(), RowVersion: r.RowVersion}
	if in.BreachTypes == nil {
		in.BreachTypes = []string{}
	}
	if in.AffectedCategories == nil {
		in.AffectedCategories = []uuid.UUID{}
	}
	in.Clock = ClockAt(in.AwareAt, s.now(), timerRunning(in.Status, r.Decision))
	return in
}

// canSee: readers see every incident; people who may only report see the ones they reported (permissions.yaml note).
func canSee(ctx context.Context, in Incident) bool {
	if has(ctx, PermRead) {
		return true
	}
	me := currentUser(ctx)
	return has(ctx, PermCreate) && me != nil && in.ReporterID != nil && *in.ReporterID == *me
}

func (s *Service) check(ctx context.Context, in *Input, creating bool) error {
	var errs []FieldError
	in.Title, in.Description = strings.TrimSpace(in.Title), strings.TrimSpace(in.Description)
	if in.Title == "" || len([]rune(in.Title)) > 300 {
		errs = append(errs, FieldError{"title", "invalid"})
	}
	if in.Description == "" {
		errs = append(errs, FieldError{"description", "required"})
	}
	if len(in.BreachTypes) == 0 {
		errs = append(errs, FieldError{"breach_types", "required"})
	}
	for _, t := range in.BreachTypes {
		if !slices.Contains(BreachTypes, t) {
			errs = append(errs, FieldError{"breach_types", "invalid"})
			break
		}
	}
	slices.Sort(in.BreachTypes)
	in.BreachTypes = slices.Compact(in.BreachTypes)
	if creating && !slices.Contains(ReportedVia, in.ReportedVia) {
		errs = append(errs, FieldError{"reported_via", "invalid"})
	}
	now := s.now().Add(5 * time.Minute) // clock skew
	if in.AwareAt.IsZero() || in.AwareAt.After(now) {
		errs = append(errs, FieldError{"aware_at", "invalid"})
	}
	if in.OccurredAt != nil && (in.OccurredAt.After(now) || (!in.AwareAt.IsZero() && in.OccurredAt.After(in.AwareAt))) {
		errs = append(errs, FieldError{"occurred_at", "after_awareness"})
	}
	if in.ContainedAt != nil && in.ContainedAt.After(now) {
		errs = append(errs, FieldError{"contained_at", "invalid"})
	}
	if in.AffectedSubjects != nil && *in.AffectedSubjects < 0 {
		errs = append(errs, FieldError{"affected_subjects", "invalid"})
	}
	if len([]rune(in.IncidentType)) > 40 {
		errs = append(errs, FieldError{"incident_type", "invalid"})
	}
	// References must be visible under RLS before their ids are written (rule 1: FKs bypass RLS).
	if creating && s.Org != nil {
		if _, err := s.Org.GetLegalEntity(ctx, in.LegalEntityID); err != nil {
			if !errors.Is(err, orgservice.ErrNotFound) {
				return err
			}
			errs = append(errs, FieldError{"legal_entity_id", "not_found"})
		}
	}
	if len(in.AffectedCategoryIDs) > 0 && s.Org != nil {
		cats, err := s.Org.ListMaster(ctx, "data_categories")
		if err != nil {
			return err
		}
		for _, id := range in.AffectedCategoryIDs {
			if !slices.ContainsFunc(cats, func(c orgservice.MasterItem) bool { return c.ID != nil && *c.ID == id }) {
				errs = append(errs, FieldError{"affected_category_ids", "not_found"})
				break
			}
		}
	}
	if in.OwnerID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*in.OwnerID})
		if err != nil {
			return err
		}
		if _, ok := names[*in.OwnerID]; !ok {
			errs = append(errs, FieldError{"owner_user_id", "not_found"})
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// Create records an incident: status reported, the PDPC notice due 72 hours after awareness, timeline entry,
// breach.reported, the deadline alerts scheduled, and the owner and DPOs alerted at once (BRE-02, BRE-07).
func (s *Service) Create(ctx context.Context, in Input) (Incident, error) {
	if !has(ctx, PermCreate) {
		return Incident{}, ErrForbidden
	}
	me := currentUser(ctx)
	if err := s.check(ctx, &in, true); err != nil {
		return Incident{}, err
	}
	if in.OwnerID == nil {
		in.OwnerID = me
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	year := s.now().Year()
	if err := q.LockIncidentNumbering(ctx, fmt.Sprint(year)); err != nil {
		return Incident{}, err
	}
	prefix := fmt.Sprintf("BR-%d-", year)
	n, err := q.CountIncidentsInYear(ctx, prefix)
	if err != nil {
		return Incident{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Incident{}, err
	}
	aware := in.AwareAt.UTC()
	if _, err := q.InsertIncident(ctx, breachstore.InsertIncidentParams{ID: id, IncidentNo: fmt.Sprintf("%s%04d", prefix, n+1), LegalEntityID: in.LegalEntityID,
		ReportedVia: in.ReportedVia, ReporterUserID: pgUUID(me), Title: in.Title, Description: in.Description, BreachTypes: in.BreachTypes,
		IncidentType: optText(in.IncidentType), OccurredAt: tsPtr(in.OccurredAt), AwareAt: ts(aware), ContainedAt: tsPtr(in.ContainedAt),
		AffectedSubjects: in.AffectedSubjects, AffectedCategories: nonNil(in.AffectedCategoryIDs), PdpcDueAt: ts(PDPCDue(aware)), IsDrill: in.IsDrill,
		OwnerUserID: pgUUID(in.OwnerID)}); err != nil {
		return Incident{}, err
	}
	inc, err := s.load(ctx, q, id, false)
	if err != nil {
		return Incident{}, err
	}
	if err := s.timeline(ctx, id, "system", "reported:"+in.ReportedVia, "", true); err != nil {
		return Incident{}, err
	}
	if err := s.publish(ctx, "breach.reported", inc); err != nil {
		return Incident{}, err
	}
	if err := s.audit(ctx, "breach.incident.create", IncidentType, id, nil, auditView(inc)); err != nil {
		return Incident{}, err
	}
	if err := s.scheduleTimers(ctx, inc); err != nil {
		return Incident{}, err
	}
	if err := s.alert(ctx, inc, "breach.reported", map[string]any{"incident_no": inc.No, "title": inc.Title, "deadline": bangkok(inc.DueAt)}, false); err != nil {
		return Incident{}, err
	}
	return s.Get(ctx, id)
}

func nonNil(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

func (s *Service) load(ctx context.Context, q *breachstore.Queries, id uuid.UUID, lock bool) (Incident, error) {
	var r breachstore.GetIncidentRow
	var err error
	if lock {
		var l breachstore.LockIncidentRow
		l, err = q.LockIncident(ctx, id)
		r = breachstore.GetIncidentRow(l)
	} else {
		r, err = q.GetIncident(ctx, id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrNotFound
	}
	if err != nil {
		return Incident{}, err
	}
	return s.toIncident(r), nil
}

// Get returns an incident the caller may see, with the names of its owner and reporter.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Incident, error) {
	in, err := s.load(ctx, breachstore.New(pdb.MustTxFromContext(ctx)), id, false)
	if err != nil {
		return Incident{}, err
	}
	if !canSee(ctx, in) {
		return Incident{}, ErrNotFound
	}
	if err := s.name(ctx, []*Incident{&in}); err != nil {
		return Incident{}, err
	}
	return in, nil
}

func (s *Service) name(ctx context.Context, ins []*Incident) error {
	var ids []uuid.UUID
	for _, in := range ins {
		for _, u := range []*uuid.UUID{in.OwnerID, in.ReporterID} {
			if u != nil {
				ids = append(ids, *u)
			}
		}
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return err
	}
	for _, in := range ins {
		if in.OwnerID != nil {
			in.OwnerName = names[*in.OwnerID]
		}
		if in.ReporterID != nil {
			in.ReporterName = names[*in.ReporterID]
		}
	}
	return nil
}

// Filter narrows the register (BRE-13).
type Filter struct {
	Status   string
	Risk     string
	Owner    *uuid.UUID
	Query    string
	From, To *time.Time
	OpenOnly bool
	Cursor   string // "<aware_at RFC3339Nano>|<id>"
	Limit    int
}

// List searches the register, newest awareness first. People who may only report see their own incidents.
func (s *Service) List(ctx context.Context, f Filter) ([]Incident, string, error) {
	p := breachstore.ListIncidentsParams{Status: optText(f.Status), Risk: optText(f.Risk), Owner: pgUUID(f.Owner), Q: optText(escapeLike(f.Query)),
		FromAt: tsPtr(f.From), ToAt: tsPtr(f.To), OpenOnly: f.OpenOnly}
	switch {
	case has(ctx, PermRead):
	case has(ctx, PermCreate) && currentUser(ctx) != nil:
		p.Reporter = pgUUID(currentUser(ctx))
	default:
		return nil, "", ErrForbidden
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	p.Lim = int32(f.Limit + 1)
	if f.Cursor != "" {
		at, id, ok := strings.Cut(f.Cursor, "|")
		t, err1 := time.Parse(time.RFC3339Nano, at)
		u, err2 := uuid.Parse(id)
		if !ok || err1 != nil || err2 != nil {
			return nil, "", invalid("cursor", "invalid")
		}
		p.CursorAt, p.CursorID = ts(t), pgUUID(&u)
	}
	rows, err := breachstore.New(pdb.MustTxFromContext(ctx)).ListIncidents(ctx, p)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
		last := rows[len(rows)-1]
		next = last.AwareAt.Time.UTC().Format(time.RFC3339Nano) + "|" + last.ID.String()
	}
	out := make([]Incident, 0, len(rows))
	ptrs := make([]*Incident, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.toIncident(breachstore.GetIncidentRow(r)))
	}
	for i := range out {
		ptrs = append(ptrs, &out[i])
	}
	if err := s.name(ctx, ptrs); err != nil {
		return nil, "", err
	}
	return out, next, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(s))
}

// Update changes the facts of an open incident (If-Match). Moving aware_at — and so the 72-hour deadline — needs
// breach.incident.approve and a reason (SEQ-06); the alerts are rescheduled.
func (s *Service) Update(ctx context.Context, id uuid.UUID, version int32, in Input) (Incident, error) {
	if !has(ctx, PermUpdate) {
		return Incident{}, ErrForbidden
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	cur, err := s.load(ctx, q, id, true)
	if err != nil {
		return Incident{}, err
	}
	if cur.RowVersion != version {
		return Incident{}, ErrVersionMismatch
	}
	if cur.Status == StatusClosed {
		return Incident{}, ErrClosed
	}
	in.LegalEntityID, in.ReportedVia = cur.LegalEntityID, cur.ReportedVia
	if in.OwnerID == nil {
		in.OwnerID = cur.OwnerID
	}
	if err := s.check(ctx, &in, false); err != nil {
		return Incident{}, err
	}
	aware := in.AwareAt.UTC()
	awareMoved := !aware.Equal(cur.AwareAt)
	if awareMoved {
		if !has(ctx, PermApprove) {
			return Incident{}, ErrForbidden
		}
		if strings.TrimSpace(in.AwareAtReason) == "" {
			return Incident{}, invalid("aware_at_reason", "required")
		}
	}
	n, err := q.UpdateIncidentFacts(ctx, breachstore.UpdateIncidentFactsParams{Title: in.Title, Description: in.Description, BreachTypes: in.BreachTypes,
		IncidentType: optText(in.IncidentType), OccurredAt: tsPtr(in.OccurredAt), AwareAt: ts(aware), PdpcDueAt: ts(PDPCDue(aware)),
		ContainedAt: tsPtr(in.ContainedAt), AffectedSubjects: in.AffectedSubjects, AffectedCategories: nonNil(in.AffectedCategoryIDs),
		OwnerUserID: pgUUID(in.OwnerID), Actor: pgUUID(currentUser(ctx)), ID: id, RowVersion: version})
	if err != nil {
		return Incident{}, err
	}
	if n == 0 {
		return Incident{}, ErrVersionMismatch
	}
	after, err := s.load(ctx, q, id, false)
	if err != nil {
		return Incident{}, err
	}
	if awareMoved {
		if err := s.timeline(ctx, id, "decision", "aware_at:"+cur.AwareAt.Format(time.RFC3339)+":"+aware.Format(time.RFC3339), in.AwareAtReason, true); err != nil {
			return Incident{}, err
		}
		if err := s.scheduleTimers(ctx, after); err != nil {
			return Incident{}, err
		}
	}
	if !samePtr(cur.OwnerID, after.OwnerID) {
		if err := s.timeline(ctx, id, "action", "owner:"+after.OwnerID.String(), "", true); err != nil {
			return Incident{}, err
		}
	}
	if !cur.ContainedAtEqual(after) && after.ContainedAt != nil {
		if err := s.timeline(ctx, id, "action", "contained:"+after.ContainedAt.Format(time.RFC3339), "", true); err != nil {
			return Incident{}, err
		}
	}
	if err := s.audit(ctx, "breach.incident.update", IncidentType, id, auditView(cur), auditView(after)); err != nil {
		return Incident{}, err
	}
	return s.Get(ctx, id)
}

// ContainedAtEqual compares containment times.
func (in Incident) ContainedAtEqual(o Incident) bool {
	if in.ContainedAt == nil || o.ContainedAt == nil {
		return in.ContainedAt == nil && o.ContainedAt == nil
	}
	return in.ContainedAt.Equal(*o.ContainedAt)
}

func samePtr(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Transition moves an incident along ST-03 where a person decides it (If-Match): take it up (reported → triage,
// optionally assigning an owner), confirm it (triage → assessing), close a non-breach (triage → closed) or a finished
// one (remediating → closed) with a reason — closing needs breach.incident.approve —, or reopen the assessment on new
// facts (remediating → assessing, with a reason). assessing → notifying / remediating is the notification decision
// (Decide); notifying → remediating needs the recorded PDPC notice (BRE-09, not built yet).
func (s *Service) Transition(ctx context.Context, id uuid.UUID, version int32, to, reason string, owner *uuid.UUID) (Incident, error) {
	if !has(ctx, PermUpdate) {
		return Incident{}, ErrForbidden
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	cur, err := s.load(ctx, q, id, true)
	if err != nil {
		return Incident{}, err
	}
	if cur.RowVersion != version {
		return Incident{}, ErrVersionMismatch
	}
	if !Allowed(cur.Status, to) {
		return Incident{}, ErrInvalidTransition
	}
	reason = strings.TrimSpace(reason)
	switch {
	case cur.Status == StatusAssessing:
		return Incident{}, ErrInvalidTransition // through Decide
	case cur.Status == StatusNotifying && to == StatusRemediating:
		return Incident{}, ErrPDPCNoticeMissing
	case to == StatusClosed && !has(ctx, PermApprove):
		return Incident{}, ErrForbidden
	case (to == StatusClosed || (cur.Status == StatusRemediating && to == StatusAssessing)) && reason == "":
		return Incident{}, invalid("reason", "required")
	}
	if owner != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*owner})
		if err != nil {
			return Incident{}, err
		}
		if _, ok := names[*owner]; !ok {
			return Incident{}, invalid("owner_user_id", "not_found")
		}
	}
	p := breachstore.SetIncidentStatusParams{Status: to, OwnerUserID: pgUUID(owner), Actor: pgUUID(currentUser(ctx)), ID: id}
	if to == StatusClosed {
		p.CloseReason = &reason
	}
	if err := q.SetIncidentStatus(ctx, p); err != nil {
		return Incident{}, err
	}
	after, err := s.load(ctx, q, id, false)
	if err != nil {
		return Incident{}, err
	}
	if err := s.timeline(ctx, id, "decision", "status:"+cur.Status+":"+to, reason, true); err != nil {
		return Incident{}, err
	}
	if owner != nil && !samePtr(cur.OwnerID, owner) {
		if err := s.timeline(ctx, id, "action", "owner:"+owner.String(), "", true); err != nil {
			return Incident{}, err
		}
	}
	if to == StatusClosed {
		if err := s.publish(ctx, "breach.closed", after); err != nil {
			return Incident{}, err
		}
	}
	if err := s.audit(ctx, "breach.incident.transition", IncidentType, id, map[string]any{"status": cur.Status}, map[string]any{"status": to, "reason": reason}); err != nil {
		return Incident{}, err
	}
	return s.Get(ctx, id)
}

// TimelineItem is one entry of an incident's timeline. Automatic entries carry a token (e.g. "status:triage:assessing",
// "deadline:24") the UI localizes, plus any free text a person gave (a reason); notes are free text.
type TimelineItem struct {
	ID         uuid.UUID
	OccurredAt time.Time
	Type       string // decision | action | communication | system | note
	Token      string
	Text       string
	ActorID    *uuid.UUID
	ActorName  string
	Auto       bool
}

// timeline appends to breach.timeline_events (insert-only, rule 4).
func (s *Service) timeline(ctx context.Context, incident uuid.UUID, typ, token, text string, auto bool) error {
	return s.timelineAt(ctx, incident, s.now(), typ, token, text, auto)
}

func (s *Service) timelineAt(ctx context.Context, incident uuid.UUID, at time.Time, typ, token, text string, auto bool) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	desc := token
	if !auto {
		desc = text
	} else if strings.TrimSpace(text) != "" {
		desc = token + "\n" + strings.TrimSpace(text)
	}
	return breachstore.New(pdb.MustTxFromContext(ctx)).InsertTimelineEvent(ctx, breachstore.InsertTimelineEventParams{ID: id, IncidentID: incident,
		OccurredAt: ts(at), EventType: typ, Description: desc, ActorID: pgUUID(currentUser(ctx)), IsAuto: auto})
}

// Timeline returns an incident's timeline in order, with actor names (BRE-12).
func (s *Service) Timeline(ctx context.Context, id uuid.UUID) ([]TimelineItem, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := breachstore.New(pdb.MustTxFromContext(ctx)).ListTimeline(ctx, id)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	out := make([]TimelineItem, 0, len(rows))
	for _, r := range rows {
		it := TimelineItem{ID: r.ID, OccurredAt: r.OccurredAt.Time.UTC(), Type: r.EventType, ActorID: uuidPtr(r.ActorID), Auto: r.IsAuto}
		if r.IsAuto {
			it.Token, it.Text, _ = strings.Cut(r.Description, "\n")
		} else {
			it.Text = r.Description
		}
		if it.ActorID != nil {
			ids = append(ids, *it.ActorID)
		}
		out = append(out, it)
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].ActorID != nil {
			out[i].ActorName = names[*out[i].ActorID]
		}
	}
	return out, nil
}

// AddNote adds a person's entry to the timeline, at the time given (something that happened earlier) or now.
func (s *Service) AddNote(ctx context.Context, id uuid.UUID, at *time.Time, text string) error {
	if !has(ctx, PermUpdate) {
		return ErrForbidden
	}
	in, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if in.Status == StatusClosed {
		return ErrClosed
	}
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > 4000 {
		return invalid("text", "invalid")
	}
	when := s.now()
	if at != nil {
		if at.After(when.Add(5 * time.Minute)) {
			return invalid("occurred_at", "invalid")
		}
		when = at.UTC()
	}
	if err := s.timelineAt(ctx, id, when, "note", "", text, false); err != nil {
		return err
	}
	return s.audit(ctx, "breach.timeline.note", IncidentType, id, nil, map[string]any{"occurred_at": when})
}

// Evidence is a file kept as evidence of an incident, with the SHA-256 recorded at upload.
type Evidence struct {
	ID              uuid.UUID
	FileID          uuid.UUID
	FileName        string
	SHA256          string
	SizeBytes       int64
	AVStatus        string
	Description     string
	CollectedBy     *uuid.UUID
	CollectedByName string
	CollectedAt     time.Time
}

// AddEvidence keeps one of the caller's clean uploads (PLT-09) as evidence: attached to the incident (downloadable with
// breach.incident.read), its hash on the timeline (BRE-12).
func (s *Service) AddEvidence(ctx context.Context, id, fileID uuid.UUID, description string) (Evidence, error) {
	if !has(ctx, PermUpdate) {
		return Evidence{}, ErrForbidden
	}
	in, err := s.Get(ctx, id)
	if err != nil {
		return Evidence{}, err
	}
	if in.Status == StatusClosed {
		return Evidence{}, ErrClosed
	}
	f, err := s.Files.Get(ctx, fileID)
	if err != nil || f.EntityType != "" || f.AVStatus != "clean" {
		return Evidence{}, ErrFileNotUsable
	}
	if err := s.Files.AttachSystem(ctx, fileID, IncidentType, id); err != nil {
		return Evidence{}, err
	}
	eid, err := uuid.NewV7()
	if err != nil {
		return Evidence{}, err
	}
	now := s.now()
	me := currentUser(ctx)
	if err := breachstore.New(pdb.MustTxFromContext(ctx)).InsertEvidence(ctx, breachstore.InsertEvidenceParams{ID: eid, IncidentID: id, FileID: fileID,
		Description: optText(description), CollectedBy: pgUUID(me), CollectedAt: ts(now)}); err != nil {
		return Evidence{}, err
	}
	if err := s.timeline(ctx, id, "action", "evidence:"+f.SHA256, f.FileName, true); err != nil {
		return Evidence{}, err
	}
	if err := s.audit(ctx, "breach.evidence.add", IncidentType, id, nil, map[string]any{"file_id": fileID, "sha256": f.SHA256}); err != nil {
		return Evidence{}, err
	}
	return Evidence{ID: eid, FileID: fileID, FileName: f.FileName, SHA256: f.SHA256, SizeBytes: f.SizeBytes, AVStatus: f.AVStatus,
		Description: strings.TrimSpace(description), CollectedBy: me, CollectedAt: now}, nil
}

// ListEvidence returns an incident's evidence files.
func (s *Service) ListEvidence(ctx context.Context, id uuid.UUID) ([]Evidence, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := breachstore.New(pdb.MustTxFromContext(ctx)).ListEvidence(ctx, id)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	out := make([]Evidence, 0, len(rows))
	for _, r := range rows {
		e := Evidence{ID: r.ID, FileID: r.FileID, Description: deref(r.Description), CollectedBy: uuidPtr(r.CollectedBy), CollectedAt: r.CollectedAt.Time.UTC()}
		if _, f, err := s.Files.Open(ctx, r.FileID); err == nil {
			e.FileName, e.SHA256, e.SizeBytes, e.AVStatus = f.FileName, f.SHA256, f.SizeBytes, f.AVStatus
		}
		if e.CollectedBy != nil {
			ids = append(ids, *e.CollectedBy)
		}
		out = append(out, e)
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].CollectedBy != nil {
			out[i].CollectedByName = names[*out[i].CollectedBy]
		}
	}
	return out, nil
}

// publish writes a breach.* event (docs/architecture/events.yaml) to the outbox in the same transaction.
func (s *Service) publish(ctx context.Context, typ string, in Incident) error {
	if s.Events == nil {
		return nil
	}
	_, err := s.Events.Publish(ctx, events.Event{Type: typ, AggregateType: IncidentType, AggregateID: in.ID, OccurredAt: s.now(),
		Data: map[string]any{"incident_ref": in.No, "aware_at": in.AwareAt.Format(time.RFC3339), "risk_level": in.RiskLevel, "deadline_at": in.DueAt.Format(time.RFC3339)}})
	return err
}

// auditView is what the audit log keeps of an incident: facts and state, no free text beyond the title.
func auditView(in Incident) map[string]any {
	return map[string]any{"incident_no": in.No, "title": in.Title, "status": in.Status, "breach_types": in.BreachTypes, "aware_at": in.AwareAt,
		"affected_subjects": in.AffectedSubjects, "owner_user_id": in.OwnerID, "risk_level": in.RiskLevel, "decision": in.Decision}
}

// alert tells the incident's owner and the DPOs — and, when escalate, the executives — in the app and by e-mail. A
// person without an e-mail address still gets the in-app message.
func (s *Service) alert(ctx context.Context, in Incident, template string, vars map[string]any, escalate bool) error {
	if s.Notify == nil {
		return nil
	}
	var to []uuid.UUID
	if in.OwnerID != nil {
		to = append(to, *in.OwnerID)
	}
	roles := []string{"DPO"}
	if escalate {
		roles = append(roles, "EXEC")
	}
	for _, r := range roles {
		ids, err := iamservice.UsersWithRole(ctx, r)
		if err != nil {
			return err
		}
		to = append(to, ids...)
	}
	slices.SortFunc(to, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	to = slices.Compact(to)
	for _, u := range to {
		for _, ch := range []string{"in_app", "email"} {
			uid := u
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: template, Channel: ch, RecipientUserID: &uid, Vars: vars,
					EntityType: IncidentType, EntityID: &in.ID, Urgent: true})
				return err
			})
			if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
				return err
			}
		}
	}
	return nil
}

// TimerArgs is breach.sla_timer: one checkpoint of an incident's 72-hour clock (BRE-07), scheduled at its moment.
// AwareAt pins the schedule: when aware_at moves, new timers are set and the old ones find it changed and stop.
type TimerArgs struct {
	jobs.TenantArgs
	IncidentID string `json:"incident_id"`
	Hours      int    `json:"hours"`
	AwareAt    string `json:"aware_at"`
}

func (TimerArgs) Kind() string { return "breach.sla_timer" }

func (s *Service) scheduleTimers(ctx context.Context, in Incident) error {
	if s.River == nil {
		return nil
	}
	tenant, err := tenantID(ctx)
	if err != nil {
		return err
	}
	for _, c := range ToSchedule(in.AwareAt, s.now()) {
		args := TimerArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, IncidentID: in.ID.String(), Hours: c.Hours, AwareAt: in.AwareAt.Format(time.RFC3339Nano)}
		if _, err := jobs.Enqueue(ctx, s.River, args, &river.InsertOpts{ScheduledAt: c.At, UniqueOpts: river.UniqueOpts{ByArgs: true}}); err != nil {
			return err
		}
	}
	return nil
}

// FireTimer runs one checkpoint: while the clock still runs for this awareness time, it records the checkpoint on the
// timeline and alerts the owner and DPOs — plus the executives from 66 hours, and at 72 hours as overdue (BRE-07).
func (s *Service) FireTimer(ctx context.Context, incidentID uuid.UUID, hours int, awareAt time.Time) error {
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	in, err := s.load(ctx, q, incidentID, true)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	c, ok := CheckpointAt(in.AwareAt, hours)
	if !ok || !in.AwareAt.Equal(awareAt.UTC()) {
		return nil // aware_at moved: this schedule is stale
	}
	dec := &in.Decision
	if in.Decision == "" {
		dec = nil
	}
	if !timerRunning(in.Status, dec) {
		return nil
	}
	if err := s.timelineAt(ctx, in.ID, s.now(), "system", fmt.Sprintf("deadline:%d", hours), "", true); err != nil {
		return err
	}
	if c.Overdue {
		return s.alert(ctx, in, "breach.deadline_overdue", map[string]any{"incident_no": in.No}, true)
	}
	return s.alert(ctx, in, "breach.deadline_reminder", map[string]any{"incident_no": in.No, "hours": hours, "deadline": bangkok(in.DueAt)}, c.Escalate)
}
