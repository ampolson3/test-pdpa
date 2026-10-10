package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dpostore "pdpa-platform/internal/dpo/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

// SecurityFormType is the PLT-06 form type DPO-09's checklist uses (migration 00039 widened
// platform.form_definitions.form_type to accept it). The checklist's own wording — the ประกาศมาตรการ
// ความปลอดภัย พ.ศ. 2565 items — is never hard-coded here (rule 8): the DPO authors it as an ordinary form.
const SecurityFormType = "security"

// ErrBadForm is returned when the given form isn't a published security-assessment form.
var ErrBadForm = errors.New("dpo: not a published security-assessment form")

type FieldError struct{ Field, Code string }
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return fmt.Sprintf("dpo: invalid: %v", e.Fields) }
func (e *ValidationError) Unwrap() error { return ErrInvalid }

// Assessment is one run of the security-measures checklist against a legal entity (DPO-09): the answers'
// score and band (whatever the checklist's own scoring config calls them — no legally-mandated band names
// exist to hard-code), each question's contribution, and the remediation tasks any failed item opened.
type Assessment struct {
	ID               uuid.UUID
	LegalEntityID    uuid.UUID
	FormSubmissionID uuid.UUID
	Score            float64
	Result           string
	Factors          []forms.Contribution
	AssessedBy       *uuid.UUID
	AssessedAt       time.Time
	Tasks            []Task
}

// Task is a dpo.tasks remediation item, most of it auto-opened by Assess for a failed checklist control
// (and, as of RRA-07, by a caller choosing to remediate a legal-gap finding directly).
type Task struct {
	ID             uuid.UUID
	TaskNo         string
	Title          string
	Description    string
	SourceType     string
	SourceID       *uuid.UUID
	Status         string
	Priority       string
	AssigneeUserID *uuid.UUID
	DueAt          *time.Time
	CompletedAt    *time.Time
	RowVersion     int32
	CreatedAt      time.Time
}

// assessmentVersion resolves a form to its current published version, checked usable: it must be DPO-09's
// own form type.
func (s *Service) assessmentVersion(ctx context.Context, formID uuid.UUID) (forms.Version, error) {
	f, err := s.Forms.GetForm(ctx, formID)
	if errors.Is(err, forms.ErrNotFound) || errors.Is(err, forms.ErrForbidden) {
		return forms.Version{}, ErrBadForm
	}
	if err != nil {
		return forms.Version{}, err
	}
	if f.Type != SecurityFormType || f.CurrentVersionID == nil {
		return forms.Version{}, ErrBadForm
	}
	for _, v := range f.Versions {
		if v.ID == *f.CurrentVersionID {
			return v, nil
		}
	}
	return forms.Version{}, ErrBadForm
}

// Assess records one run of a published security-measures checklist against a legal entity: the answers are
// validated and scored by the form engine (PLT-06), and every yes_no item answered "no" — a control the
// checklist found unmet — automatically opens a dpo.tasks remediation task (the acceptance criterion), so a
// failed item is never just a number on a report.
func (s *Service) Assess(ctx context.Context, legalEntityID, formID uuid.UUID, answers forms.Answers) (Assessment, error) {
	if _, err := s.Org.GetLegalEntity(ctx, legalEntityID); err != nil {
		return Assessment{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	v, err := s.assessmentVersion(ctx, formID)
	if err != nil {
		return Assessment{}, err
	}
	g, _ := authz.FromContext(ctx)
	var me *uuid.UUID
	if u, err := uuid.Parse(g.UserID); err == nil {
		me = &u
	}
	subID, res, err := s.Forms.Record(ctx, v.ID, answers, "user", me, "", nil)
	var ae *forms.AnswerErrors
	if errors.As(err, &ae) {
		ve := &ValidationError{}
		for _, f := range ae.Fields {
			ve.Fields = append(ve.Fields, FieldError{"answers." + f.Question, f.Code})
		}
		return Assessment{}, ve
	}
	if err != nil {
		return Assessment{}, err
	}
	factors := forms.Contributions(v.Schema, res.Answers)
	raw, _ := json.Marshal(factors)
	id, err := uuid.NewV7()
	if err != nil {
		return Assessment{}, err
	}
	now := time.Now().UTC()
	var score pgtype.Numeric
	if err := score.Scan(strconv.FormatFloat(res.Score, 'f', 2, 64)); err != nil {
		return Assessment{}, err
	}
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertSecurityAssessment(ctx, dpostore.InsertSecurityAssessmentParams{ID: id, LegalEntityID: legalEntityID,
		FormSubmissionID: subID, Score: score, Result: res.Band, Factors: raw, AssessedBy: pgUUID(me),
		AssessedAt: pgtype.Timestamptz{Time: now, Valid: true}})
	if err != nil {
		return Assessment{}, err
	}
	var tasks []Task
	for _, item := range failingItems(v.Schema, res.Answers) {
		t, err := s.openRemediationTask(ctx, item, row.ID, me)
		if err != nil {
			return Assessment{}, err
		}
		tasks = append(tasks, t)
	}
	if err := s.audit(ctx, "dpo.security_assessment.record", row.ID, nil, map[string]any{"legal_entity_id": legalEntityID,
		"score": res.Score, "result": res.Band, "tasks_created": len(tasks)}); err != nil {
		return Assessment{}, err
	}
	return Assessment{ID: row.ID, LegalEntityID: legalEntityID, FormSubmissionID: subID, Score: res.Score, Result: res.Band,
		Factors: factors, AssessedBy: me, AssessedAt: now, Tasks: tasks}, nil
}

type AssessmentCursor struct {
	AssessedAt time.Time
	ID         uuid.UUID
}

type AssessmentFilter struct {
	LegalEntityID *uuid.UUID
	After         *AssessmentCursor
	Limit         int
}

const assessmentPageSize = 50

// ListAssessments lists a tenant's (optionally one legal entity's) assessment runs, newest first — without
// their remediation tasks (GetAssessment includes those; a list row is meant to be scanned, not drilled into).
func (s *Service) ListAssessments(ctx context.Context, f AssessmentFilter) ([]Assessment, *AssessmentCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > assessmentPageSize {
		limit = assessmentPageSize
	}
	p := dpostore.ListSecurityAssessmentsParams{Lim: int32(limit + 1)}
	p.LegalEntityID = pgUUID(f.LegalEntityID)
	if f.After != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.After.AssessedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := dpostore.New(pdb.MustTxFromContext(ctx)).ListSecurityAssessments(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Assessment, 0, len(rows))
	for i, r := range rows {
		if i == limit {
			last := out[len(out)-1]
			return out, &AssessmentCursor{AssessedAt: last.AssessedAt, ID: last.ID}, nil
		}
		out = append(out, toAssessment(dpostore.InsertSecurityAssessmentRow(r), nil))
	}
	return out, nil, nil
}

// GetAssessment returns one assessment run and its remediation tasks.
func (s *Service) GetAssessment(ctx context.Context, id uuid.UUID) (Assessment, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetSecurityAssessment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assessment{}, ErrNotFound
	}
	if err != nil {
		return Assessment{}, err
	}
	tasks, err := s.remediationTasks(ctx, id)
	if err != nil {
		return Assessment{}, err
	}
	return toAssessment(dpostore.InsertSecurityAssessmentRow(r), tasks), nil
}

func (s *Service) remediationTasks(ctx context.Context, assessmentID uuid.UUID) ([]Task, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListTasksBySource(ctx, dpostore.ListTasksBySourceParams{SourceType: "risk", SourceID: pgUUID(&assessmentID)})
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTask(dpostore.InsertTaskRow(r)))
	}
	return out, nil
}

type failingItem struct {
	Key   string
	Label forms.Text
}

// failingItems walks the schema in the same order Contributions does and collects every visible yes_no
// question whose answer is "no" — a checklist control the assessment found unmet.
func failingItems(sc forms.Schema, answers forms.Answers) []failingItem {
	var out []failingItem
	for _, sec := range sc.Sections {
		for _, q := range sec.Questions {
			if q.Type != forms.TypeYesNo {
				continue
			}
			v, ok := answers[q.Key]
			if !ok {
				continue
			}
			if a, ok := v.(string); ok && a == "no" {
				out = append(out, failingItem{Key: q.Key, Label: q.Label})
			}
		}
	}
	return out
}

func (s *Service) openRemediationTask(ctx context.Context, item failingItem, assessmentID uuid.UUID, actor *uuid.UUID) (Task, error) {
	q := dpostore.New(pdb.MustTxFromContext(ctx))
	year := strconv.Itoa(time.Now().UTC().Year())
	if err := q.LockTaskNumbering(ctx, year); err != nil {
		return Task{}, err
	}
	prefix := "SEC-" + year + "-"
	n, err := q.CountTasksInYear(ctx, prefix)
	if err != nil {
		return Task{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Task{}, err
	}
	title := item.Label["th"]
	if title == "" {
		title = item.Key
	}
	row, err := q.InsertTask(ctx, dpostore.InsertTaskParams{ID: id, TaskNo: fmt.Sprintf("%s%04d", prefix, n+1),
		Title: title, Description: opt("ไม่ผ่านการประเมินมาตรการความปลอดภัย: " + title), SourceType: "risk",
		SourceID: pgUUID(&assessmentID), Priority: "high", Actor: pgUUID(actor)})
	if err != nil {
		return Task{}, err
	}
	return toTask(row), nil
}

func toAssessment(r dpostore.InsertSecurityAssessmentRow, tasks []Task) Assessment {
	score, _ := r.Score.Float64Value()
	var factors []forms.Contribution
	_ = json.Unmarshal(r.Factors, &factors)
	return Assessment{ID: r.ID, LegalEntityID: r.LegalEntityID, FormSubmissionID: r.FormSubmissionID, Score: score.Float64,
		Result: r.Result, Factors: factors, AssessedBy: uuidPtr(r.AssessedBy), AssessedAt: r.AssessedAt.Time, Tasks: tasks}
}

func toTask(r dpostore.InsertTaskRow) Task {
	t := Task{ID: r.ID, TaskNo: r.TaskNo, Title: r.Title, Description: deref(r.Description), SourceType: r.SourceType,
		SourceID: uuidPtr(r.SourceID), Status: r.Status, Priority: r.Priority, AssigneeUserID: uuidPtr(r.AssigneeUserID),
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.DueAt.Valid {
		t.DueAt = &r.DueAt.Time
	}
	if r.CompletedAt.Valid {
		t.CompletedAt = &r.CompletedAt.Time
	}
	return t
}
