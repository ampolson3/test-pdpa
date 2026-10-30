package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	noticestore "pdpa-platform/internal/notice/store"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// IndirectCollectionType is the PLT-09/audit entity type of an indirect-collection record (PNG-04).
const IndirectCollectionType = "notice_indirect_collection"

var indirectMethods = []string{"email", "sms", "letter", "website", "other"}

// NotifyWindow is the ม.25 deadline: 30 calendar days from when the data was obtained.
const NotifyWindow = 30 * 24 * time.Hour

// IndirectCollection is notice.indirect_collections (PNG-04, ม.25): personal data obtained from a source
// other than the data subject, which must be notified to the data subject within 30 days.
type IndirectCollection struct {
	ID             uuid.UUID
	SourcePartyID  uuid.UUID
	ActivityID     *uuid.UUID
	ObtainedAt     time.Time
	SubjectCount   *int
	NotifyDueAt    time.Time
	Method         string
	NotifiedAt     *time.Time
	EvidenceFileID *uuid.UUID
	Status         string // pending | notified | overdue | exempted
	RowVersion     int32
}

// Checkpoint is a moment the 30-day clock alerts staff (PNG-04): reminders at 20 and 25 days elapsed,
// overdue at 30 (BP-04's own job name, notice.indirect_due).
type Checkpoint struct {
	Days    int
	At      time.Time
	Overdue bool
}

var CheckpointDays = []int{20, 25, 30}

// Checkpoints are the alert moments of an indirect-collection record, in order.
func Checkpoints(obtainedAt time.Time) []Checkpoint {
	out := make([]Checkpoint, 0, len(CheckpointDays))
	for _, d := range CheckpointDays {
		out = append(out, Checkpoint{Days: d, At: obtainedAt.UTC().AddDate(0, 0, d), Overdue: d >= 30})
	}
	return out
}

// CheckpointAt returns the checkpoint of d elapsed days.
func CheckpointAt(obtainedAt time.Time, d int) (Checkpoint, bool) {
	for _, c := range Checkpoints(obtainedAt) {
		if c.Days == d {
			return c, true
		}
	}
	return Checkpoint{}, false
}

// ToSchedule are the checkpoints to set up at now: every future one, plus the latest one already passed (so a
// record entered late still alerts, or is marked overdue, at once).
func ToSchedule(obtainedAt, now time.Time) []Checkpoint {
	var out []Checkpoint
	var passed *Checkpoint
	for _, c := range Checkpoints(obtainedAt) {
		if c.At.After(now) {
			out = append(out, c)
		} else {
			cc := c
			passed = &cc
		}
	}
	if passed != nil {
		passed.At = now
		out = append([]Checkpoint{*passed}, out...)
	}
	return out
}

func (in *IndirectCollection) normalize() error {
	if in.ObtainedAt.IsZero() {
		return fmt.Errorf("%w: obtained_at", ErrInvalid)
	}
	if in.ObtainedAt.After(time.Now().UTC()) {
		return fmt.Errorf("%w: obtained_at cannot be in the future", ErrInvalid)
	}
	if in.SubjectCount != nil && *in.SubjectCount < 0 {
		return fmt.Errorf("%w: subject_count", ErrInvalid)
	}
	return nil
}

// RegisterCollection records an indirect-collection event (BP-04 step t9) and schedules its 30-day
// reminders/overdue check (notice.indirect_due).
func (s *Service) RegisterCollection(ctx context.Context, in IndirectCollection) (IndirectCollection, error) {
	if err := in.normalize(); err != nil {
		return IndirectCollection{}, err
	}
	if _, err := s.Org.GetExternalParty(ctx, in.SourcePartyID); err != nil {
		return IndirectCollection{}, fmt.Errorf("%w: source_party_id", ErrInvalid)
	}
	if in.ActivityID != nil {
		if _, err := s.Ropa.GetActivity(ctx, *in.ActivityID); err != nil {
			return IndirectCollection{}, fmt.Errorf("%w: activity_id", ErrInvalid)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return IndirectCollection{}, err
	}
	dueAt := in.ObtainedAt.Add(NotifyWindow)
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertIndirectCollection(ctx, noticestore.InsertIndirectCollectionParams{
		ID: id, SourcePartyID: in.SourcePartyID, ActivityID: pgUUID(in.ActivityID),
		ObtainedAt: pgDate(&in.ObtainedAt), SubjectCount: int32Ptr(in.SubjectCount), NotifyDueAt: pgDate(&dueAt)})
	if err != nil {
		return IndirectCollection{}, err
	}
	out := toIndirectCollection(row)
	if err := s.scheduleReminders(ctx, out); err != nil {
		return IndirectCollection{}, err
	}
	return out, s.audit(ctx, "notice.indirect.register", out.ID, nil, indirectAudit(out))
}

// RecordNotice closes an indirect-collection record once the data subject has been notified (or the
// evidence of an earlier notice is filed): the acceptance criterion's "ปิดรายการได้เมื่อมีหลักฐานการแจ้ง".
func (s *Service) RecordNotice(ctx context.Context, id uuid.UUID, version int32, method string, evidenceFileID uuid.UUID) (IndirectCollection, error) {
	method = strings.TrimSpace(method)
	if !slices.Contains(indirectMethods, method) {
		return IndirectCollection{}, fmt.Errorf("%w: method", ErrInvalid)
	}
	if evidenceFileID == uuid.Nil {
		return IndirectCollection{}, fmt.Errorf("%w: evidence_file_id", ErrInvalid)
	}
	before, err := s.GetCollection(ctx, id)
	if err != nil {
		return IndirectCollection{}, err
	}
	if before.Status != "pending" && before.Status != "overdue" {
		return IndirectCollection{}, fmt.Errorf("%w: already %s", ErrInvalid, before.Status)
	}
	if s.Files != nil {
		if _, err := s.Files.Get(ctx, evidenceFileID); err != nil {
			return IndirectCollection{}, fmt.Errorf("%w: evidence_file_id", ErrInvalid)
		}
		if err := s.Files.AttachSystem(ctx, evidenceFileID, IndirectCollectionType, id); err != nil {
			return IndirectCollection{}, fmt.Errorf("%w: evidence_file_id", ErrInvalid)
		}
	}
	q := noticestore.New(pdb.MustTxFromContext(ctx))
	row, err := q.RecordIndirectNotice(ctx, noticestore.RecordIndirectNoticeParams{
		ID: id, Method: &method, EvidenceFileID: pgUUID(&evidenceFileID), RowVersion: version})
	if err != nil {
		return IndirectCollection{}, ErrVersionMismatch
	}
	out := toIndirectCollection(noticestore.InsertIndirectCollectionRow(row))
	return out, s.audit(ctx, "notice.indirect.notify", out.ID, indirectAudit(before), indirectAudit(out))
}

func (s *Service) GetCollection(ctx context.Context, id uuid.UUID) (IndirectCollection, error) {
	row, err := noticestore.New(pdb.MustTxFromContext(ctx)).GetIndirectCollection(ctx, id)
	if err != nil {
		return IndirectCollection{}, ErrNotFound
	}
	return toIndirectCollection(noticestore.InsertIndirectCollectionRow(row)), nil
}

type IndirectCollectionCursor struct {
	DueAt time.Time
	ID    uuid.UUID
}

type IndirectCollectionFilter struct {
	Status *string
	After  *IndirectCollectionCursor
	Limit  int
}

const indirectCollectionPageSize = 50

func (s *Service) ListCollections(ctx context.Context, f IndirectCollectionFilter) ([]IndirectCollection, *IndirectCollectionCursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > indirectCollectionPageSize {
		limit = indirectCollectionPageSize
	}
	p := noticestore.ListIndirectCollectionsParams{Status: f.Status, Lim: int32(limit + 1)}
	if f.After != nil {
		p.CursorAt = pgDate(&f.After.DueAt)
		p.CursorID = pgtype.UUID{Bytes: f.After.ID, Valid: true}
	}
	rows, err := noticestore.New(pdb.MustTxFromContext(ctx)).ListIndirectCollections(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	out := make([]IndirectCollection, 0, len(rows))
	for _, r := range rows {
		out = append(out, toIndirectCollection(noticestore.InsertIndirectCollectionRow(r)))
	}
	var next *IndirectCollectionCursor
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		next = &IndirectCollectionCursor{DueAt: last.NotifyDueAt, ID: last.ID}
	}
	return out, next, nil
}

// --- reminders (notice.indirect_due, per BP-04) ---

// DueArgs is notice.indirect_due: one checkpoint of an indirect-collection record's 30-day clock.
// ObtainedAt pins the schedule so a stale tick (after the record was already closed) is harmless.
type DueArgs struct {
	jobs.TenantArgs
	CollectionID string `json:"collection_id"`
	Days         int    `json:"days"`
	ObtainedAt   string `json:"obtained_at"`
}

func (DueArgs) Kind() string { return "notice.indirect_due" }

func (s *Service) scheduleReminders(ctx context.Context, in IndirectCollection) error {
	if s.River == nil {
		return nil
	}
	tenant, err := tenantID(ctx)
	if err != nil {
		return err
	}
	for _, c := range ToSchedule(in.ObtainedAt, s.now()) {
		args := DueArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, CollectionID: in.ID.String(), Days: c.Days,
			ObtainedAt: in.ObtainedAt.Format(time.RFC3339Nano)}
		if _, err := jobs.Enqueue(ctx, s.River, args, &river.InsertOpts{ScheduledAt: c.At, UniqueOpts: river.UniqueOpts{ByArgs: true}}); err != nil {
			return err
		}
	}
	return nil
}

// FireDue runs one checkpoint: while the record is still pending (or already overdue, for the reminders that
// preceded the overdue check), it alerts the DPO — and marks the record overdue once 30 days have passed.
func (s *Service) FireDue(ctx context.Context, collectionID uuid.UUID, days int, obtainedAt time.Time) error {
	ic, err := s.GetCollection(ctx, collectionID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	c, ok := CheckpointAt(obtainedAt, days)
	if !ok || !ic.ObtainedAt.Equal(obtainedAt.UTC()) {
		return nil // stale schedule
	}
	if ic.Status != "pending" && ic.Status != "overdue" {
		return nil // already notified or exempted
	}
	due := bangkok(ic.NotifyDueAt)
	if c.Overdue {
		if ic.Status == "pending" {
			q := noticestore.New(pdb.MustTxFromContext(ctx))
			if _, err := q.MarkIndirectCollectionOverdue(ctx, collectionID); err != nil {
				return err
			}
		}
		return s.alert(ctx, ic, "notice.indirect_overdue", map[string]any{"due": due})
	}
	daysLeft := 30 - days
	return s.alert(ctx, ic, "notice.indirect_reminder", map[string]any{"days_left": daysLeft, "due": due})
}

// DueWorker works notice.indirect_due.
type DueWorker struct {
	river.WorkerDefaults[DueArgs]
	Service *Service
}

func (w *DueWorker) Work(ctx context.Context, job *river.Job[DueArgs]) error {
	id, err1 := uuid.Parse(job.Args.CollectionID)
	obtained, err2 := time.Parse(time.RFC3339Nano, job.Args.ObtainedAt)
	if err := errors.Join(err1, err2); err != nil {
		return river.JobCancel(err)
	}
	return w.Service.FireDue(ctx, id, job.Args.Days, obtained)
}

// alert notifies the role responsible for outstanding ม.25 obligations. There is no per-record owner field
// on notice.indirect_collections (unlike ropa.processing_activities), so — the same "default recipients until
// real routing exists" fallback BRE-07 used before BRE-04 — this always goes to role DPO.
func (s *Service) alert(ctx context.Context, ic IndirectCollection, template string, vars map[string]any) error {
	if s.Notify == nil {
		return nil
	}
	to, err := iamservice.UsersWithRole(ctx, "DPO")
	if err != nil {
		return err
	}
	for _, u := range to {
		uid := u
		for _, ch := range []string{"in_app", "email"} {
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: template, Channel: ch, RecipientUserID: &uid, Vars: vars,
					EntityType: IndirectCollectionType, EntityID: &ic.ID, Urgent: false})
				return err
			})
			if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
				return err
			}
		}
	}
	return nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func tenantID(ctx context.Context) (uuid.UUID, error) {
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(t)
}

func bangkok(t time.Time) string {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		return t.Format("2006-01-02")
	}
	return t.In(loc).Format("2006-01-02")
}

func indirectAudit(ic IndirectCollection) map[string]any {
	return map[string]any{"status": ic.Status, "notify_due_at": ic.NotifyDueAt, "method": ic.Method}
}

func toIndirectCollection(r noticestore.InsertIndirectCollectionRow) IndirectCollection {
	return IndirectCollection{ID: r.ID, SourcePartyID: r.SourcePartyID, ActivityID: uuidPtr(r.ActivityID),
		ObtainedAt: dateVal(r.ObtainedAt), SubjectCount: int32Val(r.SubjectCount), NotifyDueAt: dateVal(r.NotifyDueAt),
		Method: deref(r.Method), NotifiedAt: datePtrTS(r.NotifiedAt), EvidenceFileID: uuidPtr(r.EvidenceFileID),
		Status: r.Status, RowVersion: r.RowVersion}
}

func pgDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func dateVal(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}

func datePtrTS(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func int32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	i := int32(*v)
	return &i
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func int32Val(v *int32) *int {
	if v == nil {
		return nil
	}
	i := int(*v)
	return &i
}
