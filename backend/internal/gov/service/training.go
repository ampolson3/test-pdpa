// DPX-01 (awareness training & quiz, ม.37(1)/ม.42): gov.courses/gov.training_assignments/
// gov.training_attempts were already fully specified in the baseline migrations — a small built-in LMS
// rather than the module doc's own alternative ("หรือเชื่อม LMS ผ่าน SCORM / xAPI"), scoped to the literal
// acceptance criterion: an employee can study a course and take its quiz, and a report shows results
// per individual. A course's content is the caller's own clean, still-unattached PLT-09 upload
// (content_type video/slides — scorm/xapi are already in the schema's own CHECK for a future external-LMS
// integration this pass doesn't build); its quiz is an ordinary "quiz"-type PLT-06 form the DPO designs
// and publishes (rule 8 — never hard-code the questions here).
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	govstore "pdpa-platform/internal/gov/store"
	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

var courseContentTypes = []string{"video", "slides", "scorm", "xapi"}
var courseStatuses = []string{"draft", "published", "retired"}
var assignmentTargetTypes = []string{"all", "org_unit", "group", "user"}

// ErrBadQuizForm is returned when a course names a form that isn't a published "quiz"-type form.
var ErrBadQuizForm = errors.New("gov: not a published quiz form")

// FieldError/ValidationError report a per-question quiz-answer problem (the same shape dpo's own
// security-assessment feature already established, mirrored here since each module owns its own error
// types per rule 9).
type FieldError struct{ Field, Code string }
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return fmt.Sprintf("gov: invalid: %v", e.Fields) }
func (e *ValidationError) Unwrap() error { return ErrInvalid }

// Course is one PDPA awareness course, with its own quiz (if any) and passing criteria.
type Course struct {
	ID             uuid.UUID
	Title          string
	Description    *string
	ContentType    string
	ContentFileID  *uuid.UUID
	QuizFormID     *uuid.UUID
	PassingScore   int32
	ValidityMonths int32
	Language       string
	Status         string
	RowVersion     int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CourseInput is what a caller provides when creating or editing a course.
type CourseInput struct {
	Title          string
	Description    *string
	ContentType    string
	ContentFileID  *uuid.UUID
	QuizFormID     *uuid.UUID
	PassingScore   int32
	ValidityMonths int32
	Language       string
}

// TrainingAssignment names who must take a course and by when.
type TrainingAssignment struct {
	ID         uuid.UUID
	CourseID   uuid.UUID
	TargetType string
	TargetID   *uuid.UUID
	DueAt      time.Time
	AssignedBy uuid.UUID
	RowVersion int32
	CreatedAt  time.Time
}

// TrainingAssignmentInput is what a caller provides when assigning a course.
type TrainingAssignmentInput struct {
	CourseID   uuid.UUID
	TargetType string
	TargetID   *uuid.UUID
	DueAt      time.Time
}

// TrainingAttempt is one employee's own run of a course, started when they begin and completed once
// they finish the quiz (or, for a quiz-less course, simply mark it done).
type TrainingAttempt struct {
	ID                uuid.UUID
	CourseID          uuid.UUID
	AssignmentID      *uuid.UUID
	UserID            uuid.UUID
	UserName          string
	StartedAt         time.Time
	CompletedAt       *time.Time
	Score             *int32
	Passed            *bool
	CertificateFileID *uuid.UUID
	RowVersion        int32
	CreatedAt         time.Time
}

func validCourseContentType(s string) bool { return contains(courseContentTypes, s) }
func validCourseStatus(s string) bool      { return contains(courseStatuses, s) }
func validTargetType(s string) bool        { return contains(assignmentTargetTypes, s) }

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (s *Service) validateCourse(ctx context.Context, in CourseInput) error {
	if in.Title == "" {
		return fmt.Errorf("%w: title", ErrInvalid)
	}
	if !validCourseContentType(in.ContentType) {
		return fmt.Errorf("%w: content_type", ErrInvalid)
	}
	if in.ContentFileID != nil {
		f, err := s.Files.Get(ctx, *in.ContentFileID)
		if err != nil || f.EntityType != "" {
			return fmt.Errorf("%w: content_file_id", ErrInvalid)
		}
	}
	if in.QuizFormID != nil {
		if _, err := s.quizVersion(ctx, *in.QuizFormID); err != nil {
			return err
		}
	}
	if in.PassingScore < 0 || in.PassingScore > 100 {
		return fmt.Errorf("%w: passing_score", ErrInvalid)
	}
	if in.ValidityMonths < 0 {
		return fmt.Errorf("%w: validity_months", ErrInvalid)
	}
	return nil
}

// quizVersion resolves a form to its current published version, checked usable: it must be DPX-01's own
// "quiz" form type — the same assessmentVersion pattern DPO-09's own security checklist already uses.
func (s *Service) quizVersion(ctx context.Context, formID uuid.UUID) (forms.Version, error) {
	if s.Forms == nil {
		return forms.Version{}, ErrBadQuizForm
	}
	f, err := s.Forms.GetForm(ctx, formID)
	if errors.Is(err, forms.ErrNotFound) || errors.Is(err, forms.ErrForbidden) {
		return forms.Version{}, ErrBadQuizForm
	}
	if err != nil {
		return forms.Version{}, err
	}
	if f.Type != QuizFormType || f.CurrentVersionID == nil {
		return forms.Version{}, ErrBadQuizForm
	}
	for _, v := range f.Versions {
		if v.ID == *f.CurrentVersionID {
			return v, nil
		}
	}
	return forms.Version{}, ErrBadQuizForm
}

func withPassingDefault(v int32) int32 {
	if v == 0 {
		return 80
	}
	return v
}

func withValidityDefault(v int32) int32 {
	if v == 0 {
		return 12
	}
	return v
}

func withLanguageDefault(v string) string {
	if v == "" {
		return "th"
	}
	return v
}

// CreateCourse starts a new draft course (dpx.training.create).
func (s *Service) CreateCourse(ctx context.Context, in CourseInput) (Course, error) {
	if !has(ctx, TrainingPermCreate) {
		return Course{}, ErrForbidden
	}
	in.PassingScore, in.ValidityMonths, in.Language = withPassingDefault(in.PassingScore), withValidityDefault(in.ValidityMonths), withLanguageDefault(in.Language)
	if err := s.validateCourse(ctx, in); err != nil {
		return Course{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Course{}, err
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).InsertCourse(ctx, govstore.InsertCourseParams{
		ID: id, Title: in.Title, Description: in.Description, ContentType: in.ContentType,
		ContentFileID: pgUUIDPtr(in.ContentFileID), QuizFormID: pgUUIDPtr(in.QuizFormID),
		PassingScore: int16(in.PassingScore), ValidityMonths: int16(in.ValidityMonths), Language: in.Language,
	})
	if err != nil {
		return Course{}, err
	}
	if in.ContentFileID != nil {
		if err := s.Files.AttachSystem(ctx, *in.ContentFileID, CourseType, id); err != nil {
			return Course{}, err
		}
	}
	out := toCourse(courseRow(row))
	return out, s.audit(ctx, "gov.course.create", CourseType, out.ID, nil, map[string]any{"title": out.Title})
}

// GetCourse reads one course by id.
func (s *Service) GetCourse(ctx context.Context, id uuid.UUID) (Course, error) {
	if !has(ctx, TrainingPermRead) {
		return Course{}, ErrForbidden
	}
	r, err := govstore.New(pdb.MustTxFromContext(ctx)).GetCourse(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Course{}, ErrNotFound
	}
	if err != nil {
		return Course{}, err
	}
	return toCourse(courseRow(r)), nil
}

// ListCourses lists every course, optionally narrowed by status, newest first.
func (s *Service) ListCourses(ctx context.Context, status *string) ([]Course, error) {
	if !has(ctx, TrainingPermRead) {
		return nil, ErrForbidden
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListCourses(ctx, status)
	if err != nil {
		return nil, err
	}
	out := make([]Course, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCourse(courseRow(r)))
	}
	return out, nil
}

// UpdateCourse edits a still-draft course's own details (published content is frozen, the same "edit via
// a new version, not in place" discipline PLT-08 established — a course has no versioning of its own, so
// retiring and creating a new course is this feature's own equivalent).
func (s *Service) UpdateCourse(ctx context.Context, id uuid.UUID, rowVersion int32, in CourseInput) (Course, error) {
	if !has(ctx, TrainingPermUpdate) {
		return Course{}, ErrForbidden
	}
	in.PassingScore, in.ValidityMonths, in.Language = withPassingDefault(in.PassingScore), withValidityDefault(in.ValidityMonths), withLanguageDefault(in.Language)
	if err := s.validateCourse(ctx, in); err != nil {
		return Course{}, err
	}
	if in.ContentFileID != nil {
		existing, err := s.GetCourse(ctx, id)
		if err != nil {
			return Course{}, err
		}
		if existing.ContentFileID == nil || *existing.ContentFileID != *in.ContentFileID {
			if err := s.Files.AttachSystem(ctx, *in.ContentFileID, CourseType, id); err != nil {
				return Course{}, err
			}
		}
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).UpdateCourse(ctx, govstore.UpdateCourseParams{
		ID: id, RowVersion: rowVersion, Title: in.Title, Description: in.Description, ContentType: in.ContentType,
		ContentFileID: pgUUIDPtr(in.ContentFileID), QuizFormID: pgUUIDPtr(in.QuizFormID),
		PassingScore: int16(in.PassingScore), ValidityMonths: int16(in.ValidityMonths), Language: in.Language,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Course{}, ErrVersionMismatch
		}
		return Course{}, err
	}
	out := toCourse(courseRow(row))
	return out, s.audit(ctx, "gov.course.update", CourseType, out.ID, nil, map[string]any{"title": out.Title})
}

// SetCourseStatus moves a course draft -> published -> retired. Publishing needs dpx.training.publish;
// retiring is a permanent status change, gated by dpx.training.delete the same way DPIA-03's own template
// retire and ORG-06's party merge already reuse a "permanent decision" permission rather than minting a
// new code.
func (s *Service) SetCourseStatus(ctx context.Context, id uuid.UUID, rowVersion int32, status string) (Course, error) {
	if !validCourseStatus(status) {
		return Course{}, fmt.Errorf("%w: status", ErrInvalid)
	}
	perm := TrainingPermPublish
	if status == "retired" {
		perm = TrainingPermDelete
	}
	if !has(ctx, perm) {
		return Course{}, ErrForbidden
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).SetCourseStatus(ctx, govstore.SetCourseStatusParams{
		ID: id, RowVersion: rowVersion, Status: status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Course{}, ErrVersionMismatch
		}
		return Course{}, err
	}
	out := toCourse(courseRow(row))
	return out, s.audit(ctx, "gov.course.status", CourseType, out.ID, nil, map[string]any{"status": status})
}

// AssignTraining is "มอบหมายตามแผนก": names who must take a published course and by when. The target's
// own FK (an org unit, a group, or a user) is checked visible/real before the assignment is written —
// "all" carries no target_id at all.
func (s *Service) AssignTraining(ctx context.Context, in TrainingAssignmentInput) (TrainingAssignment, error) {
	if !has(ctx, TrainingPermExecute) {
		return TrainingAssignment{}, ErrForbidden
	}
	if !validTargetType(in.TargetType) {
		return TrainingAssignment{}, fmt.Errorf("%w: target_type", ErrInvalid)
	}
	course, err := s.GetCourse(ctx, in.CourseID)
	if err != nil {
		return TrainingAssignment{}, fmt.Errorf("%w: course_id", ErrInvalid)
	}
	if course.Status != "published" {
		return TrainingAssignment{}, fmt.Errorf("%w: course is not published", ErrInvalid)
	}
	switch in.TargetType {
	case "all":
		in.TargetID = nil
	case "org_unit":
		if in.TargetID == nil || s.Org == nil {
			return TrainingAssignment{}, fmt.Errorf("%w: target_id", ErrInvalid)
		}
		if _, err := s.Org.GetOrgUnit(ctx, *in.TargetID); err != nil {
			return TrainingAssignment{}, fmt.Errorf("%w: target_id", ErrInvalid)
		}
	case "group":
		if in.TargetID == nil {
			return TrainingAssignment{}, fmt.Errorf("%w: target_id", ErrInvalid)
		}
		names, err := iamservice.GroupNames(ctx, []uuid.UUID{*in.TargetID})
		if err != nil || names[*in.TargetID] == "" {
			return TrainingAssignment{}, fmt.Errorf("%w: target_id", ErrInvalid)
		}
	case "user":
		if in.TargetID == nil {
			return TrainingAssignment{}, fmt.Errorf("%w: target_id", ErrInvalid)
		}
		names, err := iamservice.Names(ctx, []uuid.UUID{*in.TargetID})
		if err != nil || names[*in.TargetID] == "" {
			return TrainingAssignment{}, fmt.Errorf("%w: target_id", ErrInvalid)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return TrainingAssignment{}, err
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).InsertTrainingAssignment(ctx, govstore.InsertTrainingAssignmentParams{
		ID: id, CourseID: in.CourseID, TargetType: in.TargetType, TargetID: pgUUIDPtr(in.TargetID), DueAt: pgDate(in.DueAt),
	})
	if err != nil {
		return TrainingAssignment{}, err
	}
	out := toAssignment(assignmentRow(row))
	return out, s.audit(ctx, "gov.training_assignment.create", AssignmentType, out.ID, nil,
		map[string]any{"course_id": in.CourseID, "target_type": in.TargetType, "due_at": out.DueAt})
}

// ListAssignments lists every assignment, optionally narrowed by course.
func (s *Service) ListAssignments(ctx context.Context, courseID *uuid.UUID) ([]TrainingAssignment, error) {
	if !has(ctx, TrainingPermRead) {
		return nil, ErrForbidden
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListTrainingAssignments(ctx, pgUUIDPtr(courseID))
	if err != nil {
		return nil, err
	}
	out := make([]TrainingAssignment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAssignment(assignmentRow(r)))
	}
	return out, nil
}

// MyAssignments resolves what is assigned to the calling employee right now: every "all" assignment, plus
// any naming their own primary org unit, one of their own groups, or them directly.
func (s *Service) MyAssignments(ctx context.Context) ([]TrainingAssignment, error) {
	if !has(ctx, TrainingPermExecute) {
		return nil, ErrForbidden
	}
	g, _ := authz.FromContext(ctx)
	me, err := uuid.Parse(g.UserID)
	if err != nil {
		return nil, fmt.Errorf("%w: no caller in context", ErrInvalid)
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	var out []TrainingAssignment
	allRows, err := q.ListAssignmentsForAllTarget(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range allRows {
		out = append(out, toAssignment(assignmentRow(r)))
	}
	unit, err := iamservice.PrimaryOrgUnit(ctx, me)
	if err != nil {
		return nil, err
	}
	if unit != nil {
		rows, err := q.ListAssignmentsForOrgUnit(ctx, pgUUIDPtr(unit))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, toAssignment(assignmentRow(r)))
		}
	}
	groups, err := iamservice.GroupIDsOf(ctx, me)
	if err != nil {
		return nil, err
	}
	if len(groups) > 0 {
		rows, err := q.ListAssignmentsForGroups(ctx, groups)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, toAssignment(assignmentRow(r)))
		}
	}
	userRows, err := q.ListAssignmentsForUser(ctx, pgUUIDPtr(&me))
	if err != nil {
		return nil, err
	}
	for _, r := range userRows {
		out = append(out, toAssignment(assignmentRow(r)))
	}
	return out, nil
}

// StartAttempt is "พนักงานเรียน...ได้": an employee begins a course — it must be published. assignmentID
// is optional (a caller may study a published course without a formal assignment).
func (s *Service) StartAttempt(ctx context.Context, courseID uuid.UUID, assignmentID *uuid.UUID) (TrainingAttempt, error) {
	if !has(ctx, TrainingPermExecute) {
		return TrainingAttempt{}, ErrForbidden
	}
	course, err := s.GetCourse(ctx, courseID)
	if err != nil {
		return TrainingAttempt{}, fmt.Errorf("%w: course_id", ErrInvalid)
	}
	if course.Status != "published" {
		return TrainingAttempt{}, fmt.Errorf("%w: course is not published", ErrInvalid)
	}
	g, _ := authz.FromContext(ctx)
	me, err := uuid.Parse(g.UserID)
	if err != nil {
		return TrainingAttempt{}, fmt.Errorf("%w: no caller in context", ErrInvalid)
	}
	if assignmentID != nil {
		if _, err := govstore.New(pdb.MustTxFromContext(ctx)).GetTrainingAssignment(ctx, *assignmentID); errors.Is(err, pgx.ErrNoRows) {
			return TrainingAttempt{}, fmt.Errorf("%w: assignment_id", ErrInvalid)
		} else if err != nil {
			return TrainingAttempt{}, err
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return TrainingAttempt{}, err
	}
	now := s.now()
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).InsertTrainingAttempt(ctx, govstore.InsertTrainingAttemptParams{
		ID: id, CourseID: courseID, AssignmentID: pgUUIDPtr(assignmentID), UserID: me, StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return TrainingAttempt{}, err
	}
	out := toAttempt(attemptRow(row))
	return out, s.audit(ctx, "gov.training_attempt.start", AttemptType, out.ID, nil, map[string]any{"course_id": courseID})
}

// SubmitAttempt is "...และทำแบบทดสอบได้": the caller's own still-open attempt is completed. A course with
// no quiz is marked done with no score (studying it is enough); a course with one is scored by the form
// engine (PLT-06) against the course's own passing_score.
func (s *Service) SubmitAttempt(ctx context.Context, attemptID uuid.UUID, rowVersion int32, answers forms.Answers) (TrainingAttempt, error) {
	if !has(ctx, TrainingPermExecute) {
		return TrainingAttempt{}, ErrForbidden
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	ar, err := q.GetTrainingAttempt(ctx, attemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TrainingAttempt{}, ErrNotFound
	}
	if err != nil {
		return TrainingAttempt{}, err
	}
	attempt := toAttempt(attemptRow(ar))
	course, err := s.GetCourse(ctx, attempt.CourseID)
	if err != nil {
		return TrainingAttempt{}, err
	}
	var scorePtr *int32
	passed := true
	if course.QuizFormID != nil {
		v, err := s.quizVersion(ctx, *course.QuizFormID)
		if err != nil {
			return TrainingAttempt{}, err
		}
		_, res, err := s.Forms.Record(ctx, v.ID, answers, "user", &attempt.UserID, AttemptType, &attemptID)
		var ae *forms.AnswerErrors
		if errors.As(err, &ae) {
			ve := &ValidationError{}
			for _, f := range ae.Fields {
				ve.Fields = append(ve.Fields, FieldError{"answers." + f.Question, f.Code})
			}
			return TrainingAttempt{}, ve
		}
		if err != nil {
			return TrainingAttempt{}, err
		}
		pct := int32(0)
		if res.MaxScore > 0 {
			pct = int32(res.Score / res.MaxScore * 100)
		}
		scorePtr = &pct
		passed = pct >= course.PassingScore
	}
	row, err := q.CompleteTrainingAttempt(ctx, govstore.CompleteTrainingAttemptParams{
		ID: attemptID, RowVersion: rowVersion, CompletedAt: pgtype.Timestamptz{Time: s.now(), Valid: true}, Score: scorePtrToInt16(scorePtr), Passed: &passed,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TrainingAttempt{}, ErrVersionMismatch
		}
		return TrainingAttempt{}, err
	}
	out := toAttempt(attemptRow(row))
	return out, s.audit(ctx, "gov.training_attempt.complete", AttemptType, out.ID, nil,
		map[string]any{"course_id": attempt.CourseID, "score": scorePtr, "passed": passed})
}

// TrainingReport is DPX-01's own other half of the acceptance criterion ("รายงานแสดงผลรายบุคคล"): every
// attempt against a course, newest first per user, with the attempting user's own name resolved.
func (s *Service) TrainingReport(ctx context.Context, courseID uuid.UUID) ([]TrainingAttempt, error) {
	if !has(ctx, TrainingPermRead) {
		return nil, ErrForbidden
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListAttemptsByCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	out := make([]TrainingAttempt, 0, len(rows))
	var ids []uuid.UUID
	for _, r := range rows {
		a := toAttempt(attemptRow(r))
		out = append(out, a)
		ids = append(ids, a.UserID)
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].UserName = names[out[i].UserID]
	}
	return out, nil
}

// MyAttempts lists the calling employee's own past attempts, newest first.
func (s *Service) MyAttempts(ctx context.Context) ([]TrainingAttempt, error) {
	if !has(ctx, TrainingPermExecute) {
		return nil, ErrForbidden
	}
	g, _ := authz.FromContext(ctx)
	me, err := uuid.Parse(g.UserID)
	if err != nil {
		return nil, fmt.Errorf("%w: no caller in context", ErrInvalid)
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListAttemptsByUser(ctx, me)
	if err != nil {
		return nil, err
	}
	out := make([]TrainingAttempt, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAttempt(attemptRow(r)))
	}
	return out, nil
}

func scorePtrToInt16(v *int32) *int16 {
	if v == nil {
		return nil
	}
	n := int16(*v)
	return &n
}

// courseRow/assignmentRow/attemptRow mirror each own query's shared column layout (sqlc generates a
// distinct named Row type per query despite the identical layout — the same workaround BRE-15/VEN-03/
// VEN-14/DPX-08 already established), so every query's own Row type converts to them directly.
type courseRow struct {
	ID             uuid.UUID
	Title          string
	Description    *string
	ContentType    string
	ContentFileID  pgtype.UUID
	QuizFormID     pgtype.UUID
	PassingScore   int16
	ValidityMonths int16
	Language       string
	Status         string
	RowVersion     int32
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

type assignmentRow struct {
	ID         uuid.UUID
	CourseID   uuid.UUID
	TargetType string
	TargetID   pgtype.UUID
	DueAt      pgtype.Date
	AssignedBy uuid.UUID
	RowVersion int32
	CreatedAt  pgtype.Timestamptz
	UpdatedAt  pgtype.Timestamptz
}

type attemptRow struct {
	ID                uuid.UUID
	CourseID          uuid.UUID
	AssignmentID      pgtype.UUID
	UserID            uuid.UUID
	StartedAt         pgtype.Timestamptz
	CompletedAt       pgtype.Timestamptz
	Score             *int16
	Passed            *bool
	CertificateFileID pgtype.UUID
	RowVersion        int32
	CreatedAt         pgtype.Timestamptz
	UpdatedAt         pgtype.Timestamptz
}

func toCourse(r courseRow) Course {
	out := Course{ID: r.ID, Title: r.Title, Description: r.Description, ContentType: r.ContentType,
		PassingScore: int32(r.PassingScore), ValidityMonths: int32(r.ValidityMonths), Language: r.Language,
		Status: r.Status, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.ContentFileID.Valid {
		u := uuid.UUID(r.ContentFileID.Bytes)
		out.ContentFileID = &u
	}
	if r.QuizFormID.Valid {
		u := uuid.UUID(r.QuizFormID.Bytes)
		out.QuizFormID = &u
	}
	return out
}

func toAssignment(r assignmentRow) TrainingAssignment {
	out := TrainingAssignment{ID: r.ID, CourseID: r.CourseID, TargetType: r.TargetType, DueAt: r.DueAt.Time,
		AssignedBy: r.AssignedBy, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.TargetID.Valid {
		u := uuid.UUID(r.TargetID.Bytes)
		out.TargetID = &u
	}
	return out
}

func toAttempt(r attemptRow) TrainingAttempt {
	out := TrainingAttempt{ID: r.ID, CourseID: r.CourseID, UserID: r.UserID, StartedAt: r.StartedAt.Time,
		Score: int16PtrToInt32Ptr(r.Score), Passed: r.Passed, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time}
	if r.AssignmentID.Valid {
		u := uuid.UUID(r.AssignmentID.Bytes)
		out.AssignmentID = &u
	}
	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time
		out.CompletedAt = &t
	}
	if r.CertificateFileID.Valid {
		u := uuid.UUID(r.CertificateFileID.Bytes)
		out.CertificateFileID = &u
	}
	return out
}

func int16PtrToInt32Ptr(v *int16) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}
