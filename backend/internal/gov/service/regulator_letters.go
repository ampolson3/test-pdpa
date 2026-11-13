// DPX-08: gov.regulator_letters was already fully specified in the baseline migrations (00017_gov.sql,
// FK'd in 00018: file_id -> platform.files, incident_id -> breach.incidents) — a register of letters,
// orders and inspection notices to/from the PDPC office (สคส.), with its own due date. No queries or
// service code existed against it until now. The acceptance criterion itself ("กำหนดส่งถูกแจ้งเตือนก่อน
// ครบ") is a single reminder checkpoint before due_at — the exact one-time-per-record pattern VEN-06's own
// certificate-expiry reminder already established, not a recurring sweep (a due date doesn't move on its
// own the way a recurring review/re-assessment cycle does).
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	govstore "pdpa-platform/internal/gov/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

var directions = []string{"inbound", "outbound"}
var letterStatuses = []string{"open", "responded", "closed"}

// RegulatorLetterDueNoticeDays is how far ahead of due_at the reminder fires — a plain tunable default
// (CLAUDE.md's "implement the listed default as config", not a docs/decisions.md question).
const RegulatorLetterDueNoticeDays = 7

// RegulatorLetter is one letter/order/inspection notice to or from the PDPC office.
type RegulatorLetter struct {
	ID          uuid.UUID
	Direction   string
	LetterNo    *string
	Subject     string
	ReceivedAt  *time.Time
	DueAt       *time.Time
	Status      string
	FileID      *uuid.UUID
	IncidentID  *uuid.UUID
	RespondedAt *time.Time
	RowVersion  int32
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RegulatorLetterInput is what a caller provides when recording or editing a letter.
type RegulatorLetterInput struct {
	Direction  string
	LetterNo   *string
	Subject    string
	ReceivedAt *time.Time
	DueAt      *time.Time
	FileID     *uuid.UUID
	IncidentID *uuid.UUID
}

// RegulatorLetterFilter narrows ListRegulatorLetters.
type RegulatorLetterFilter struct {
	Status    *string
	Direction *string
}

func validDirection(d string) bool {
	for _, v := range directions {
		if v == d {
			return true
		}
	}
	return false
}

func validLetterStatus(s string) bool {
	for _, v := range letterStatuses {
		if v == s {
			return true
		}
	}
	return false
}

func (s *Service) validateRegulatorLetter(ctx context.Context, in RegulatorLetterInput) error {
	if !validDirection(in.Direction) {
		return fmt.Errorf("%w: direction", ErrInvalid)
	}
	if in.Subject == "" {
		return fmt.Errorf("%w: subject", ErrInvalid)
	}
	if in.FileID != nil {
		f, err := s.Files.Get(ctx, *in.FileID)
		if err != nil || f.EntityType != "" {
			return fmt.Errorf("%w: file_id", ErrInvalid)
		}
	}
	if in.IncidentID != nil {
		if s.Breach == nil {
			return fmt.Errorf("%w: incident_id", ErrInvalid)
		}
		if _, err := s.Breach.Get(ctx, *in.IncidentID); err != nil {
			return fmt.Errorf("%w: incident_id", ErrInvalid)
		}
	}
	return nil
}

// SaveRegulatorLetter records a new letter (ทะเบียนหนังสือ) — the incident_id, when given, must already
// be visible under RLS (rule 1, via the Breach interface); the file, when given, must be the caller's
// own clean, still-unattached upload. A real due_at schedules this feature's own single reminder.
func (s *Service) SaveRegulatorLetter(ctx context.Context, in RegulatorLetterInput) (RegulatorLetter, error) {
	if !has(ctx, RegulatorPermCreate) {
		return RegulatorLetter{}, ErrForbidden
	}
	if err := s.validateRegulatorLetter(ctx, in); err != nil {
		return RegulatorLetter{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return RegulatorLetter{}, err
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).InsertRegulatorLetter(ctx, govstore.InsertRegulatorLetterParams{
		ID: id, Direction: in.Direction, LetterNo: in.LetterNo, Subject: in.Subject,
		ReceivedAt: pgDate2(in.ReceivedAt), DueAt: pgDate2(in.DueAt), FileID: pgUUIDPtr(in.FileID), IncidentID: pgUUIDPtr(in.IncidentID),
	})
	if err != nil {
		return RegulatorLetter{}, err
	}
	if in.FileID != nil {
		if err := s.Files.AttachSystem(ctx, *in.FileID, RegulatorLetterType, id); err != nil {
			return RegulatorLetter{}, err
		}
	}
	out := toRegulatorLetter(regulatorLetterRow(row))
	if err := s.scheduleRegulatorLetterDueReminder(ctx, out.ID, out.DueAt); err != nil {
		return RegulatorLetter{}, err
	}
	if err := s.audit(ctx, "gov.regulator_letter.create", RegulatorLetterType, out.ID, nil,
		map[string]any{"direction": out.Direction, "subject": out.Subject, "due_at": out.DueAt}); err != nil {
		return RegulatorLetter{}, err
	}
	return out, nil
}

// GetRegulatorLetter reads one letter by id — RLS already scopes it to this tenant.
func (s *Service) GetRegulatorLetter(ctx context.Context, id uuid.UUID) (RegulatorLetter, error) {
	if !has(ctx, RegulatorPermRead) {
		return RegulatorLetter{}, ErrForbidden
	}
	r, err := govstore.New(pdb.MustTxFromContext(ctx)).GetRegulatorLetter(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return RegulatorLetter{}, ErrNotFound
	}
	if err != nil {
		return RegulatorLetter{}, err
	}
	return toRegulatorLetter(regulatorLetterRow(r)), nil
}

// ListRegulatorLetters lists every letter, soonest-due first, optionally narrowed by status/direction.
func (s *Service) ListRegulatorLetters(ctx context.Context, f RegulatorLetterFilter) ([]RegulatorLetter, error) {
	if !has(ctx, RegulatorPermRead) {
		return nil, ErrForbidden
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListRegulatorLetters(ctx, govstore.ListRegulatorLettersParams{
		Status: f.Status, Direction: f.Direction,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RegulatorLetter, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRegulatorLetter(regulatorLetterRow(r)))
	}
	return out, nil
}

// UpdateRegulatorLetter edits a still-open letter's own details (a re-set due_at reschedules this
// feature's own reminder; the prior schedule simply no-ops once it fires and finds a stale due_at).
func (s *Service) UpdateRegulatorLetter(ctx context.Context, id uuid.UUID, rowVersion int32, in RegulatorLetterInput) (RegulatorLetter, error) {
	if !has(ctx, RegulatorPermUpdate) {
		return RegulatorLetter{}, ErrForbidden
	}
	if err := s.validateRegulatorLetter(ctx, in); err != nil {
		return RegulatorLetter{}, err
	}
	if in.FileID != nil {
		existing, err := s.GetRegulatorLetter(ctx, id)
		if err != nil {
			return RegulatorLetter{}, err
		}
		if existing.FileID == nil || *existing.FileID != *in.FileID {
			if err := s.Files.AttachSystem(ctx, *in.FileID, RegulatorLetterType, id); err != nil {
				return RegulatorLetter{}, err
			}
		}
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).UpdateRegulatorLetter(ctx, govstore.UpdateRegulatorLetterParams{
		ID: id, RowVersion: rowVersion, Direction: in.Direction, LetterNo: in.LetterNo, Subject: in.Subject,
		ReceivedAt: pgDate2(in.ReceivedAt), DueAt: pgDate2(in.DueAt), FileID: pgUUIDPtr(in.FileID), IncidentID: pgUUIDPtr(in.IncidentID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RegulatorLetter{}, ErrVersionMismatch
		}
		return RegulatorLetter{}, err
	}
	out := toRegulatorLetter(regulatorLetterRow(row))
	if err := s.scheduleRegulatorLetterDueReminder(ctx, out.ID, out.DueAt); err != nil {
		return RegulatorLetter{}, err
	}
	if err := s.audit(ctx, "gov.regulator_letter.update", RegulatorLetterType, out.ID, nil,
		map[string]any{"due_at": out.DueAt}); err != nil {
		return RegulatorLetter{}, err
	}
	return out, nil
}

// SetRegulatorLetterStatus moves a letter open -> responded -> closed (responded_at is stamped
// automatically the moment status becomes "responded").
func (s *Service) SetRegulatorLetterStatus(ctx context.Context, id uuid.UUID, rowVersion int32, status string) (RegulatorLetter, error) {
	if !has(ctx, RegulatorPermUpdate) {
		return RegulatorLetter{}, ErrForbidden
	}
	if !validLetterStatus(status) {
		return RegulatorLetter{}, fmt.Errorf("%w: status", ErrInvalid)
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).SetRegulatorLetterStatus(ctx, govstore.SetRegulatorLetterStatusParams{
		ID: id, RowVersion: rowVersion, Status: status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RegulatorLetter{}, ErrVersionMismatch
		}
		return RegulatorLetter{}, err
	}
	out := toRegulatorLetter(regulatorLetterRow(row))
	return out, s.audit(ctx, "gov.regulator_letter.status", RegulatorLetterType, out.ID, nil, map[string]any{"status": status})
}

// DeleteRegulatorLetter removes a letter record (not its file — PLT-09's own retention rules apply).
func (s *Service) DeleteRegulatorLetter(ctx context.Context, id uuid.UUID) error {
	if !has(ctx, RegulatorPermDelete) {
		return ErrForbidden
	}
	n, err := govstore.New(pdb.MustTxFromContext(ctx)).DeleteRegulatorLetter(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "gov.regulator_letter.delete", RegulatorLetterType, id, nil, nil)
}

// RegulatorLetterDueArgs is gov.regulator_letter_due: the single checkpoint of one letter's own due_at.
// DueAt pins the schedule so a stale tick (the letter's date changed, it was answered, or it was deleted,
// since this job was enqueued) is harmless.
type RegulatorLetterDueArgs struct {
	jobs.TenantArgs
	LetterID string `json:"letter_id"`
	DueAt    string `json:"due_at"`
}

func (RegulatorLetterDueArgs) Kind() string { return "gov.regulator_letter_due" }

// scheduleRegulatorLetterDueReminder enqueues the single checkpoint RegulatorLetterDueNoticeDays before
// dueAt, or immediately if that moment has already passed (the same "late-recorded event still alerts
// at once" rule PNG-04/DSAR-07/DPA-10/VEN-06 already use). A nil dueAt enqueues nothing.
func (s *Service) scheduleRegulatorLetterDueReminder(ctx context.Context, id uuid.UUID, dueAt *time.Time) error {
	if s.River == nil || dueAt == nil {
		return nil
	}
	at := dueAt.AddDate(0, 0, -RegulatorLetterDueNoticeDays)
	if now := s.now(); at.Before(now) {
		at = now
	}
	tenant, err := tenantID(ctx)
	if err != nil {
		return err
	}
	args := RegulatorLetterDueArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, LetterID: id.String(), DueAt: dueAt.Format("2006-01-02")}
	_, err = jobs.Enqueue(ctx, s.River, args, &river.InsertOpts{ScheduledAt: at, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}

// FireRegulatorLetterDueReminder is the acceptance criterion itself: while the letter's own due_at still
// matches what the job was scheduled for and it is still open (not responded/closed, not deleted), it
// notifies every DPO and LEGAL user — the same "default recipients until real per-record routing exists"
// fallback BRE-07/PNG-04/VEN-06/VEN-10 already use (this record has no owner column of its own).
func (s *Service) FireRegulatorLetterDueReminder(ctx context.Context, id uuid.UUID, dueAt time.Time) error {
	q := govstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetRegulatorLetter(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	letter := toRegulatorLetter(regulatorLetterRow(row))
	if letter.DueAt == nil || !letter.DueAt.Equal(dueAt) {
		return nil // stale schedule
	}
	if letter.Status != "open" {
		return nil
	}
	if s.Notify == nil {
		return nil
	}
	recipients, err := regulatorLetterReminderRecipients(ctx)
	if err != nil {
		return err
	}
	vars := map[string]any{"subject": letter.Subject, "due_at": letter.DueAt.Format("2 Jan 2006")}
	for _, uid := range recipients {
		uid := uid
		for _, ch := range []string{"in_app", "email"} {
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "gov.regulator_letter_due", Channel: ch,
					RecipientUserID: &uid, Vars: vars, EntityType: RegulatorLetterType, EntityID: &letter.ID, Urgent: false})
				return err
			})
			if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
				return err
			}
		}
	}
	return nil
}

func regulatorLetterReminderRecipients(ctx context.Context) ([]uuid.UUID, error) {
	dpo, err := iamservice.UsersWithRole(ctx, "DPO")
	if err != nil {
		return nil, err
	}
	legal, err := iamservice.UsersWithRole(ctx, "LEGAL")
	if err != nil {
		return nil, err
	}
	seen := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(dpo)+len(legal))
	for _, id := range append(dpo, legal...) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

// RegulatorLetterDueWorker works gov.regulator_letter_due.
type RegulatorLetterDueWorker struct {
	river.WorkerDefaults[RegulatorLetterDueArgs]
	Service *Service
}

func (w *RegulatorLetterDueWorker) Work(ctx context.Context, job *river.Job[RegulatorLetterDueArgs]) error {
	id, err1 := uuid.Parse(job.Args.LetterID)
	due, err2 := time.Parse("2006-01-02", job.Args.DueAt)
	if err := errors.Join(err1, err2); err != nil {
		return river.JobCancel(err)
	}
	return w.Service.FireRegulatorLetterDueReminder(ctx, id, due)
}

// regulatorLetterRow mirrors every regulator_letters.sql query's own shared column layout (sqlc generates
// a distinct named Row type per query rather than reusing one shared struct — the same workaround
// BRE-15/VEN-03/VEN-14 already established), so every query's Row type converts to it directly.
type regulatorLetterRow struct {
	ID          uuid.UUID
	Direction   string
	LetterNo    *string
	Subject     string
	ReceivedAt  pgtype.Date
	DueAt       pgtype.Date
	Status      string
	FileID      pgtype.UUID
	IncidentID  pgtype.UUID
	RespondedAt pgtype.Date
	RowVersion  int32
	CreatedAt   pgtype.Timestamptz
	UpdatedAt   pgtype.Timestamptz
}

func toRegulatorLetter(r regulatorLetterRow) RegulatorLetter {
	out := RegulatorLetter{ID: r.ID, Direction: r.Direction, LetterNo: r.LetterNo, Subject: r.Subject,
		Status: r.Status, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.ReceivedAt.Valid {
		t := r.ReceivedAt.Time
		out.ReceivedAt = &t
	}
	if r.DueAt.Valid {
		t := r.DueAt.Time
		out.DueAt = &t
	}
	if r.RespondedAt.Valid {
		t := r.RespondedAt.Time
		out.RespondedAt = &t
	}
	if r.FileID.Valid {
		u := uuid.UUID(r.FileID.Bytes)
		out.FileID = &u
	}
	if r.IncidentID.Valid {
		u := uuid.UUID(r.IncidentID.Bytes)
		out.IncidentID = &u
	}
	return out
}

func pgDate2(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}
