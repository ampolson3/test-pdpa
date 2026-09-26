package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	breachstore "pdpa-platform/internal/breach/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// Notice channels built so far (letter / website / LINE need their own delivery and aren't offered yet).
var NoticeChannels = []string{"email", "sms"}

// NoticeTemplate is the platform template of the notice (a marked DRAFT seeded by migration 00036 — the tenant's
// Legal/DPO replaces it with their own approved wording in the notification-template editor, rule 8).
const NoticeTemplate = "breach.subject_notice"

// MaxRecipients bounds one batch (the acceptance test sends 10,000).
const MaxRecipients = 100_000

const (
	cryptoClass   = "breach"
	addressColumn = "breach.notification_recipients.address_enc"
)

// NoticeVars is the incident-specific content filled into the notice template.
type NoticeVars struct {
	Organization string `json:"organization"`
	Summary      string `json:"summary"`
	Remedy       string `json:"remedy"`
	Contact      string `json:"contact"`
}

// Notice is a batch notifying affected data subjects of a high-risk breach (BRE-10): draft (content and recipients,
// by the maker) → sending (approved by another person with breach.notification.approve) → done / failed.
type Notice struct {
	ID             uuid.UUID
	IncidentID     uuid.UUID
	Channel        string
	TemplateCode   string
	Vars           NoticeVars
	Total          int32
	Handed         int32 // handed to the notification service
	Failed         int32 // could not be handed over (e.g. an address it refused)
	Status         string
	StartedAt      *time.Time
	CompletedAt    *time.Time
	CreatedBy      *uuid.UUID
	CreatedByName  string
	ApprovedBy     *uuid.UUID
	ApprovedByName string
	ApprovedAt     *time.Time
	RowVersion     int32
	CreatedAt      time.Time
	// Delivery counts the handed-over messages by their delivery status: queued, sent, failed.
	Delivery map[string]int
}

func toNotice(r breachstore.GetSubjectNotificationRow) Notice {
	n := Notice{ID: r.ID, IncidentID: r.IncidentID, Channel: r.Channel, TemplateCode: r.TemplateCode, Total: r.TotalRecipients, Handed: r.SentCount,
		Failed: r.FailedCount, Status: r.Status, StartedAt: timePtr(r.StartedAt), CompletedAt: timePtr(r.CompletedAt), CreatedBy: uuidPtr(r.CreatedBy),
		ApprovedBy: uuidPtr(r.ApprovedBy), ApprovedAt: timePtr(r.ApprovedAt), RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time.UTC(),
		Delivery: map[string]int{}}
	_ = json.Unmarshal(r.Variables, &n.Vars)
	return n
}

func (v *NoticeVars) check() error {
	var errs []FieldError
	for _, f := range []struct {
		name string
		val  *string
	}{{"organization", &v.Organization}, {"summary", &v.Summary}, {"remedy", &v.Remedy}, {"contact", &v.Contact}} {
		*f.val = strings.TrimSpace(*f.val)
		if *f.val == "" || len([]rune(*f.val)) > 2000 {
			errs = append(errs, FieldError{"variables." + f.name, "required"})
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// CreateNotice starts a draft notice for an incident whose decision includes the data subjects.
func (s *Service) CreateNotice(ctx context.Context, incidentID uuid.UUID, channel string, vars NoticeVars) (Notice, error) {
	if !has(ctx, PermNoticeCreate) {
		return Notice{}, ErrForbidden
	}
	in, err := s.Get(ctx, incidentID)
	if err != nil {
		return Notice{}, err
	}
	if in.Status == StatusClosed {
		return Notice{}, ErrClosed
	}
	if in.Decision != DecisionPDPCAndSubjects {
		return Notice{}, ErrNoticeNotRequired
	}
	if !slices.Contains(NoticeChannels, channel) {
		return Notice{}, invalid("channel", "invalid")
	}
	if err := vars.check(); err != nil {
		return Notice{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Notice{}, err
	}
	raw, _ := json.Marshal(vars)
	if _, err := breachstore.New(pdb.MustTxFromContext(ctx)).InsertSubjectNotification(ctx, breachstore.InsertSubjectNotificationParams{ID: id,
		IncidentID: incidentID, Channel: channel, TemplateCode: NoticeTemplate, Variables: raw, Actor: pgUUID(currentUser(ctx))}); err != nil {
		return Notice{}, err
	}
	if err := s.timeline(ctx, incidentID, "communication", "notice_draft:"+channel, "", true); err != nil {
		return Notice{}, err
	}
	if err := s.audit(ctx, "breach.notice.create", SubjectNotificationType, id, nil, map[string]any{"incident_id": incidentID, "channel": channel}); err != nil {
		return Notice{}, err
	}
	return s.GetNotice(ctx, id)
}

// UpdateNotice changes a draft's content (If-Match).
func (s *Service) UpdateNotice(ctx context.Context, id uuid.UUID, version int32, vars NoticeVars) (Notice, error) {
	if !has(ctx, PermNoticeUpdate) {
		return Notice{}, ErrForbidden
	}
	if err := vars.check(); err != nil {
		return Notice{}, err
	}
	cur, err := s.lockNotice(ctx, id)
	if err != nil {
		return Notice{}, err
	}
	if cur.Status != "draft" {
		return Notice{}, ErrInvalidTransition
	}
	raw, _ := json.Marshal(vars)
	n, err := breachstore.New(pdb.MustTxFromContext(ctx)).UpdateSubjectNotificationDraft(ctx, breachstore.UpdateSubjectNotificationDraftParams{
		Variables: raw, TemplateCode: cur.TemplateCode, Actor: pgUUID(currentUser(ctx)), ID: id, RowVersion: version})
	if err != nil {
		return Notice{}, err
	}
	if n == 0 {
		return Notice{}, ErrVersionMismatch
	}
	if err := s.audit(ctx, "breach.notice.update", SubjectNotificationType, id, nil, map[string]any{"channel": cur.Channel}); err != nil {
		return Notice{}, err
	}
	return s.GetNotice(ctx, id)
}

func (s *Service) lockNotice(ctx context.Context, id uuid.UUID) (Notice, error) {
	r, err := breachstore.New(pdb.MustTxFromContext(ctx)).LockSubjectNotification(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Notice{}, ErrNotFound
	}
	if err != nil {
		return Notice{}, err
	}
	n := toNotice(breachstore.GetSubjectNotificationRow(r))
	if _, err := s.Get(ctx, n.IncidentID); err != nil { // the incident must be visible too
		return Notice{}, err
	}
	return n, nil
}

// GetNotice returns a notice with its delivery counts.
func (s *Service) GetNotice(ctx context.Context, id uuid.UUID) (Notice, error) {
	if !has(ctx, PermNoticeRead) && !has(ctx, PermNoticeCreate) {
		return Notice{}, ErrForbidden
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetSubjectNotification(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Notice{}, ErrNotFound
	}
	if err != nil {
		return Notice{}, err
	}
	n := toNotice(r)
	if _, err := s.Get(ctx, n.IncidentID); err != nil {
		return Notice{}, err
	}
	ns := []*Notice{&n}
	if err := s.noticeDetails(ctx, q, ns); err != nil {
		return Notice{}, err
	}
	return n, nil
}

// Notices lists an incident's notices.
func (s *Service) Notices(ctx context.Context, incidentID uuid.UUID) ([]Notice, error) {
	if !has(ctx, PermNoticeRead) && !has(ctx, PermNoticeCreate) {
		return nil, ErrForbidden
	}
	if _, err := s.Get(ctx, incidentID); err != nil {
		return nil, err
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListSubjectNotifications(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	out := make([]Notice, 0, len(rows))
	for _, r := range rows {
		out = append(out, toNotice(breachstore.GetSubjectNotificationRow(r)))
	}
	ptrs := make([]*Notice, len(out))
	for i := range out {
		ptrs[i] = &out[i]
	}
	return out, s.noticeDetails(ctx, q, ptrs)
}

func (s *Service) noticeDetails(ctx context.Context, q *breachstore.Queries, ns []*Notice) error {
	var people []uuid.UUID
	for _, n := range ns {
		for _, u := range []*uuid.UUID{n.CreatedBy, n.ApprovedBy} {
			if u != nil {
				people = append(people, *u)
			}
		}
		if n.Status == "draft" || s.Notify == nil {
			continue
		}
		raw, err := q.RecipientNotificationIDs(ctx, n.ID)
		if err != nil {
			return err
		}
		ids := make([]uuid.UUID, 0, len(raw))
		for _, r := range raw {
			if r.Valid {
				ids = append(ids, uuid.UUID(r.Bytes))
			}
		}
		st, err := s.Notify.Statuses(ctx, ids)
		if err != nil {
			return err
		}
		for _, v := range st {
			n.Delivery[v.Status]++
		}
	}
	names, err := iamservice.AllNames(ctx, people)
	if err != nil {
		return err
	}
	for _, n := range ns {
		if n.CreatedBy != nil {
			n.CreatedByName = names[*n.CreatedBy]
		}
		if n.ApprovedBy != nil {
			n.ApprovedByName = names[*n.ApprovedBy]
		}
	}
	return nil
}

// LoadRecipients replaces a draft's recipients with a CSV the caller uploaded (a clean PLT-09 file): a column
// address (or email / phone / อีเมล / โทรศัพท์) and an optional language (th / en). Addresses are normalized, checked
// for the channel, de-duplicated and stored encrypted (PLT-13); any bad line rejects the whole file, reported by line
// number and code only (no values, rule 3).
func (s *Service) LoadRecipients(ctx context.Context, id uuid.UUID, version int32, fileID uuid.UUID) (Notice, error) {
	if !has(ctx, PermNoticeUpdate) {
		return Notice{}, ErrForbidden
	}
	cur, err := s.lockNotice(ctx, id)
	if err != nil {
		return Notice{}, err
	}
	if cur.RowVersion != version {
		return Notice{}, ErrVersionMismatch
	}
	if cur.Status != "draft" {
		return Notice{}, ErrInvalidTransition
	}
	f, err := s.Files.Get(ctx, fileID)
	if err != nil || f.EntityType != "" || f.AVStatus != "clean" {
		return Notice{}, ErrFileNotUsable
	}
	rc, _, err := s.Files.Open(ctx, fileID)
	if err != nil {
		return Notice{}, ErrFileNotUsable
	}
	defer rc.Close()
	recs, err := parseRecipients(rc, cur.Channel)
	if err != nil {
		return Notice{}, err
	}
	var p breachstore.InsertRecipientsParams
	p.SubjectNotificationID = id
	for _, r := range recs {
		enc, err := s.Keyring.Encrypt(ctx, cryptoClass, addressColumn, []byte(r.address))
		if err != nil {
			return Notice{}, err
		}
		rid, err := uuid.NewV7()
		if err != nil {
			return Notice{}, err
		}
		p.Ids, p.Addresses, p.Languages, p.LineNos = append(p.Ids, rid), append(p.Addresses, enc), append(p.Languages, r.language), append(p.LineNos, int32(r.line))
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	if err := q.DeleteQueuedRecipients(ctx, id); err != nil {
		return Notice{}, err
	}
	const batch = 5000
	for i := 0; i < len(p.Ids); i += batch {
		j := min(i+batch, len(p.Ids))
		if err := q.InsertRecipients(ctx, breachstore.InsertRecipientsParams{SubjectNotificationID: id, Ids: p.Ids[i:j], Addresses: p.Addresses[i:j],
			Languages: p.Languages[i:j], LineNos: p.LineNos[i:j]}); err != nil {
			return Notice{}, err
		}
	}
	if err := q.SetRecipientTotal(ctx, breachstore.SetRecipientTotalParams{Total: int32(len(p.Ids)), ID: id}); err != nil {
		return Notice{}, err
	}
	if err := s.Files.AttachSystem(ctx, fileID, SubjectNotificationType, id); err != nil {
		return Notice{}, err
	}
	if err := s.audit(ctx, "breach.notice.recipients", SubjectNotificationType, id, nil, map[string]any{"recipients": len(p.Ids), "file_id": fileID}); err != nil {
		return Notice{}, err
	}
	return s.GetNotice(ctx, id)
}

type recipient struct {
	line     int
	address  string
	language string
}

func parseRecipients(r io.Reader, channel string) ([]recipient, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 64<<20))
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	first, _, _ := bytes.Cut(raw, []byte("\n"))
	cr := csv.NewReader(bytes.NewReader(raw))
	if bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, invalid("file", "empty")
	}
	addrCol, langCol := -1, -1
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "address", "email", "e-mail", "phone", "mobile", "อีเมล", "โทรศัพท์", "เบอร์โทรศัพท์", "ที่อยู่":
			if addrCol < 0 {
				addrCol = i
			}
		case "language", "lang", "ภาษา":
			langCol = i
		}
	}
	if addrCol < 0 {
		return nil, invalid("file", "no_address_column")
	}
	kind := crypto.KindEmail
	if channel == "sms" {
		kind = crypto.KindPhone
	}
	var out []recipient
	var errs []FieldError
	seen := map[string]bool{}
	line := 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err != nil {
			errs = append(errs, FieldError{fmt.Sprintf("line %d", line), "unreadable"})
			continue
		}
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue
		}
		val := ""
		if addrCol < len(rec) {
			val = rec[addrCol]
		}
		addr, err := crypto.Normalize(kind, val)
		if err != nil {
			errs = append(errs, FieldError{fmt.Sprintf("line %d", line), "invalid_address"})
			continue
		}
		lang := "th"
		if langCol >= 0 && langCol < len(rec) && strings.TrimSpace(rec[langCol]) != "" {
			lang = strings.ToLower(strings.TrimSpace(rec[langCol]))
			if lang != "th" && lang != "en" {
				errs = append(errs, FieldError{fmt.Sprintf("line %d", line), "invalid_language"})
				continue
			}
		}
		if seen[addr] {
			continue // the same person once
		}
		seen[addr] = true
		out = append(out, recipient{line: line, address: addr, language: lang})
		if len(out) > MaxRecipients {
			return nil, invalid("file", "too_many_rows")
		}
	}
	if len(errs) > 0 {
		if len(errs) > 100 {
			errs = errs[:100]
		}
		return nil, &ValidationError{Fields: errs}
	}
	if len(out) == 0 {
		return nil, invalid("file", "empty")
	}
	return out, nil
}

// NoticeArgs is breach.subject_notice: hand the next chunk of a notice's recipients to the notification service
// (PLT-04, which delivers and retries per message), then queue the next chunk or finish the batch.
type NoticeArgs struct {
	jobs.TenantArgs
	NoticeID string `json:"notice_id"`
	Chunk    int    `json:"chunk"`
}

func (NoticeArgs) Kind() string { return "breach.subject_notice" }

// NoticeChunk is how many recipients one job hands over.
const NoticeChunk = 1000

// Send approves a notice and starts sending it (If-Match): maker-checker — the person who drafted it can't approve
// it (docs/legal/pdpa-rules.md s.37(4): the DPO approves what is sent).
func (s *Service) Send(ctx context.Context, id uuid.UUID, version int32) (Notice, error) {
	if !has(ctx, PermNoticeSend) {
		return Notice{}, ErrForbidden
	}
	cur, err := s.lockNotice(ctx, id)
	if err != nil {
		return Notice{}, err
	}
	if cur.RowVersion != version {
		return Notice{}, ErrVersionMismatch
	}
	if cur.Status != "draft" || cur.Total == 0 {
		return Notice{}, ErrInvalidTransition
	}
	me := currentUser(ctx)
	if me == nil || (cur.CreatedBy != nil && *cur.CreatedBy == *me) {
		return Notice{}, ErrSelfApproval
	}
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	if err := q.StartSubjectNotification(ctx, breachstore.StartSubjectNotificationParams{Actor: pgUUID(me), ID: id}); err != nil {
		return Notice{}, err
	}
	tenant, err := tenantID(ctx)
	if err != nil {
		return Notice{}, err
	}
	if s.River != nil {
		if _, err := jobs.Enqueue(ctx, s.River, NoticeArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, NoticeID: id.String()}, nil); err != nil {
			return Notice{}, err
		}
	}
	if err := s.timeline(ctx, cur.IncidentID, "communication", fmt.Sprintf("notice_approved:%s:%d", cur.Channel, cur.Total), "", true); err != nil {
		return Notice{}, err
	}
	if err := s.audit(ctx, "breach.notice.send", SubjectNotificationType, id, map[string]any{"status": "draft"},
		map[string]any{"status": "sending", "recipients": cur.Total}); err != nil {
		return Notice{}, err
	}
	return s.GetNotice(ctx, id)
}

// SendChunk works one NoticeArgs job; it returns whether more recipients remain. The job's transaction covers the
// chunk, so a crash hands nobody over twice (the rows are marked in the same transaction as notify.Send).
func (s *Service) SendChunk(ctx context.Context, id uuid.UUID) (bool, error) {
	q := breachstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.LockSubjectNotification(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	n := toNotice(breachstore.GetSubjectNotificationRow(r))
	if n.Status != "sending" {
		return false, nil
	}
	rows, err := q.NextQueuedRecipients(ctx, breachstore.NextQueuedRecipientsParams{ID: id, Lim: NoticeChunk})
	if err != nil {
		return false, err
	}
	vars := map[string]any{"organization": n.Vars.Organization, "summary": n.Vars.Summary, "remedy": n.Vars.Remedy, "contact": n.Vars.Contact}
	var handed, failed int32
	for _, rec := range rows {
		addr, err := s.Keyring.Decrypt(ctx, cryptoClass, addressColumn, rec.AddressEnc)
		if err != nil {
			return false, err
		}
		var mid uuid.UUID
		err = pdb.Savepoint(ctx, func(ctx context.Context) error {
			var err error
			mid, err = s.Notify.Send(ctx, notify.Request{TemplateCode: n.TemplateCode, Channel: n.Channel, Language: rec.Language,
				RecipientAddress: string(addr), Vars: vars, EntityType: IncidentType, EntityID: &n.IncidentID, Urgent: true})
			return err
		})
		if err != nil {
			code := "rejected"
			if errors.Is(err, notify.ErrTemplateNotFound) {
				code = "template_not_found"
			}
			if err := q.MarkRecipientFailed(ctx, breachstore.MarkRecipientFailedParams{Error: &code, ID: rec.ID}); err != nil {
				return false, err
			}
			failed++
			continue
		}
		if err := q.MarkRecipientSent(ctx, breachstore.MarkRecipientSentParams{NotificationID: pgUUID(&mid), ID: rec.ID}); err != nil {
			return false, err
		}
		handed++
	}
	if err := q.AddSubjectNotificationProgress(ctx, breachstore.AddSubjectNotificationProgressParams{Sent: handed, Failed: failed, ID: id}); err != nil {
		return false, err
	}
	if len(rows) == NoticeChunk {
		return true, nil
	}
	status := "done"
	if n.Handed+handed == 0 {
		status = "failed"
	}
	if err := q.FinishSubjectNotification(ctx, breachstore.FinishSubjectNotificationParams{Status: status, ID: id}); err != nil {
		return false, err
	}
	if err := s.timeline(ctx, n.IncidentID, "communication", fmt.Sprintf("notice_sent:%s:%d:%d", n.Channel, n.Handed+handed, n.Failed+failed), "", true); err != nil {
		return false, err
	}
	in, err := s.load(ctx, q, n.IncidentID, false)
	if err != nil {
		return false, err
	}
	if err := s.publish(ctx, "breach.subjects_notified", in); err != nil {
		return false, err
	}
	return false, s.audit(ctx, "breach.notice.finish", SubjectNotificationType, id, map[string]any{"status": "sending"},
		map[string]any{"status": status, "handed": n.Handed + handed, "failed": n.Failed + failed})
}

// Recipient is one person of a notice, address masked (rule 3), with the delivery status of their message.
type Recipient struct {
	ID       uuid.UUID
	Line     int32
	Masked   string
	Language string
	Status   string // queued | sent (handed over) | failed
	Delivery string // the message's own status: queued | sent | failed ("" before hand-over)
	Attempts int32
	SentAt   *time.Time
	Error    string
}

// Recipients pages through a notice's recipients by CSV line, optionally by status.
func (s *Service) Recipients(ctx context.Context, id uuid.UUID, status string, afterLine *int32, limit int) ([]Recipient, error) {
	if !has(ctx, PermNoticeRead) && !has(ctx, PermNoticeCreate) {
		return nil, ErrForbidden
	}
	if _, err := s.GetNotice(ctx, id); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := breachstore.New(pdb.MustTxFromContext(ctx)).ListRecipients(ctx, breachstore.ListRecipientsParams{ID: id, Status: optText(status),
		AfterLine: afterLine, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	out := make([]Recipient, 0, len(rows))
	for _, r := range rows {
		addr, err := s.Keyring.Decrypt(ctx, cryptoClass, addressColumn, r.AddressEnc)
		if err != nil {
			return nil, err
		}
		rc := Recipient{ID: r.ID, Masked: notify.Mask(string(addr)), Language: r.Language, Status: r.Status, SentAt: timePtr(r.SentAt), Error: deref(r.Error)}
		if r.LineNo != nil {
			rc.Line = *r.LineNo
		}
		if r.NotificationID.Valid {
			ids = append(ids, uuid.UUID(r.NotificationID.Bytes))
		}
		out = append(out, rc)
	}
	st, err := s.Notify.Statuses(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i, r := range rows {
		if r.NotificationID.Valid {
			if v, ok := st[uuid.UUID(r.NotificationID.Bytes)]; ok {
				out[i].Delivery, out[i].Attempts = v.Status, v.Attempts
				if v.SentAt != nil {
					out[i].SentAt = v.SentAt
				}
			}
		}
	}
	return out, nil
}

// NoticeWorker works breach.subject_notice.
type NoticeWorker struct {
	river.WorkerDefaults[NoticeArgs]
	Service *Service
}

func (w *NoticeWorker) Work(ctx context.Context, job *river.Job[NoticeArgs]) error {
	id, err := uuid.Parse(job.Args.NoticeID)
	if err != nil {
		return river.JobCancel(err)
	}
	more, err := w.Service.SendChunk(ctx, id)
	if err != nil || !more {
		return err
	}
	next := job.Args
	next.Chunk++
	_, err = jobs.Enqueue(ctx, w.Service.River, next, nil)
	return err
}

// TimerWorker works breach.sla_timer.
type TimerWorker struct {
	river.WorkerDefaults[TimerArgs]
	Service *Service
}

func (w *TimerWorker) Work(ctx context.Context, job *river.Job[TimerArgs]) error {
	id, err1 := uuid.Parse(job.Args.IncidentID)
	aware, err2 := time.Parse(time.RFC3339Nano, job.Args.AwareAt)
	if err := errors.Join(err1, err2); err != nil {
		return river.JobCancel(err)
	}
	return w.Service.FireTimer(ctx, id, job.Args.Hours, aware)
}
