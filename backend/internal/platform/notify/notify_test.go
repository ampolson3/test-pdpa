package notify_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
)

// ---- unit ----

func TestQuietHours_Release(t *testing.T) {
	q := notify.DefaultQuietHours()
	bkk := q.Location
	at := func(h, m int) time.Time { return time.Date(2026, 9, 25, h, m, 0, 0, bkk) }
	cases := []struct {
		name    string
		now     time.Time
		channel string
		urgent  bool
		want    time.Time
	}{
		{"daytime sms goes now", at(14, 0), notify.ChannelSMS, false, at(14, 0)},
		{"late-evening sms waits for 08:00 next day", at(23, 30), notify.ChannelSMS, false, time.Date(2026, 9, 26, 8, 0, 0, 0, bkk)},
		{"early-morning line waits for 08:00 same day", at(6, 15), notify.ChannelLine, false, at(8, 0)},
		{"21:00 sharp is quiet", at(21, 0), notify.ChannelSMS, false, time.Date(2026, 9, 26, 8, 0, 0, 0, bkk)},
		{"08:00 sharp is not", at(8, 0), notify.ChannelSMS, false, at(8, 0)},
		{"urgent sms goes now", at(23, 30), notify.ChannelSMS, true, at(23, 30)},
		{"email is never held", at(23, 30), notify.ChannelEmail, false, at(23, 30)},
	}
	for _, tc := range cases {
		if got := q.Release(tc.now, tc.channel, tc.urgent); !got.Equal(tc.want) {
			t.Errorf("%s: %s, want %s", tc.name, got.In(bkk), tc.want)
		}
	}
}

func TestMask(t *testing.T) {
	for in, want := range map[string]string{
		"somchai@example.co.th": "so****@example.co.th",
		"a@example.com":         "a****@example.com",
		"+66812345678":          "+66*******78",
		"U1234":                 "U**34",
	} {
		if got := notify.Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPreview_FillsDeclaredVariables(t *testing.T) {
	r, err := notify.Preview(notify.TemplateInput{Subject: "คำขอ {{.request_no}}", Body: "เรียน {{.name}} คำขอ {{.request_no}} ครบกำหนด {{.due}}", Variables: []string{"name", "request_no", "due"}},
		map[string]any{"name": "สมชาย"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Subject != "คำขอ {request_no}" || r.Body != "เรียน สมชาย คำขอ {request_no} ครบกำหนด {due}" {
		t.Errorf("preview = %+v", r)
	}
	if _, err := notify.Preview(notify.TemplateInput{Body: "Hi {{.undeclared}}"}, nil); !errors.Is(err, notify.ErrTemplateInvalid) {
		t.Errorf("undeclared variable: %v, want ErrTemplateInvalid", err)
	}
	if _, err := notify.Preview(notify.TemplateInput{Body: "Hi {{.name"}, nil); !errors.Is(err, notify.ErrTemplateInvalid) {
		t.Errorf("unparsable template: %v, want ErrTemplateInvalid", err)
	}
}

// ---- integration ----

type fixture struct {
	app, owner *pgxpool.Pool
	a, b       dbtest.Tenant
	svc        *notify.Service
	sms        *notify.MockSender
	deliverer  *notify.Deliverer
	now        time.Time
	logs       *bytes.Buffer
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	f := &fixture{app: dbtest.Pool(t), owner: dbtest.OwnerPool(t), logs: &bytes.Buffer{}}
	platform := dbtest.PlatformPool(t)
	f.a = dbtest.SeedTenant(t, ctx, f.app, platform, "notify-a")
	f.b = dbtest.SeedTenant(t, ctx, f.app, platform, "notify-b")
	client, err := jobs.NewInsertClient(f.app)
	if err != nil {
		t.Fatal(err)
	}
	f.now = time.Date(2026, 9, 25, 7, 0, 0, 0, time.UTC) // 14:00 in Bangkok
	f.svc = &notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client, Quiet: notify.DefaultQuietHours(), Now: func() time.Time { return f.now }}
	f.sms = &notify.MockSender{Channel: notify.ChannelSMS}
	f.deliverer = &notify.Deliverer{Service: f.svc, Senders: map[string]notify.Sender{notify.ChannelSMS: f.sms, notify.ChannelLine: &notify.MockSender{}},
		Audit: audit.New(), Logger: slog.New(slog.NewJSONHandler(f.logs, nil))}

	t.Cleanup(func() {
		for _, tenant := range []dbtest.Tenant{f.a, f.b} {
			_ = pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				_, _ = tx.Exec(ctx, `DELETE FROM platform.notifications`)
				_, err := tx.Exec(ctx, `DELETE FROM platform.notification_templates WHERE tenant_id IS NOT NULL`)
				return err
			})
			_ = pdb.WithTenantTx(context.Background(), f.owner, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				_, _ = tx.Exec(ctx, `DELETE FROM platform.audit_log`)
				_, err := tx.Exec(ctx, `DELETE FROM platform.tenant_keys`)
				return err
			})
			_, _ = f.app.Exec(context.Background(), `DELETE FROM river_job WHERE kind = 'notify.deliver' AND args->>'tenant_id' = $1`, tenant.ID.String())
		}
	})
	return f
}

func (f *fixture) in(t *testing.T, tenant dbtest.Tenant, fn func(ctx context.Context) error) error {
	t.Helper()
	return pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), tenant.UserID.String(), fn)
}

func (f *fixture) template(t *testing.T, tenant dbtest.Tenant, in notify.TemplateInput) notify.Template {
	t.Helper()
	var out notify.Template
	if err := f.in(t, tenant, func(ctx context.Context) error {
		var err error
		out, err = f.svc.CreateTemplate(ctx, in)
		return err
	}); err != nil {
		t.Fatalf("create template: %v", err)
	}
	return out
}

func (f *fixture) send(t *testing.T, req notify.Request) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.in(t, f.a, func(ctx context.Context) error {
		var err error
		id, err = f.svc.Send(ctx, req)
		return err
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	return id
}

func (f *fixture) deliver(t *testing.T, id uuid.UUID, attempt int) {
	t.Helper()
	job := &river.Job[notify.DeliverArgs]{Args: notify.DeliverArgs{TenantArgs: jobs.TenantArgs{TenantID: f.a.ID.String()}, NotificationID: id.String(), Attempt: attempt}}
	if err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return f.deliverer.Work(ctx, job)
	}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
}

func (f *fixture) status(t *testing.T, id uuid.UUID) notify.Delivery {
	t.Helper()
	var d notify.Delivery
	if err := f.in(t, f.a, func(ctx context.Context) error {
		var err error
		d, err = f.svc.Get(ctx, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return d
}

func (f *fixture) pendingRetries(t *testing.T, id uuid.UUID) []int {
	t.Helper()
	rows, err := f.app.Query(context.Background(),
		`SELECT (args->>'attempt')::int FROM river_job WHERE kind = 'notify.deliver' AND args->>'notification_id' = $1 ORDER BY 1`, id.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var a int
		_ = rows.Scan(&a)
		out = append(out, a)
	}
	return out
}

var smsTemplate = notify.TemplateInput{Code: "dsar.otp", Channel: "sms", Language: "th", Body: "รหัส OTP ของคุณคือ {{.otp}}", Variables: []string{"otp"}}

// Acceptance criterion: "ส่งล้มเหลวมี retry อัตโนมัติ และตรวจสถานะรายข้อความได้" — every failed attempt is
// recorded on the message and the next one is scheduled on the backoff; when they run out the message
// fails, and each outcome is visible per message.
func TestDelivery_RetriesAutomaticallyAndRecordsEachAttempt(t *testing.T) {
	f := setup(t)
	f.template(t, f.a, smsTemplate)
	f.sms.FailWith = errors.New("gateway timeout calling +66812345678")
	id := f.send(t, notify.Request{TemplateCode: "dsar.otp", Channel: "sms", RecipientAddress: "+66812345678", Vars: map[string]any{"otp": "482913"}})

	if d := f.status(t, id); d.Status != "queued" || d.Attempts != 0 {
		t.Fatalf("after send: %+v", d)
	}
	for attempt := 0; attempt <= len(notify.DefaultBackoff); attempt++ {
		f.deliver(t, id, attempt)
		d := f.status(t, id)
		if d.Attempts != attempt+1 {
			t.Fatalf("attempt %d: attempts = %d", attempt, d.Attempts)
		}
		if strings.Contains(d.Error, "812345678") || !strings.Contains(d.Error, "gateway timeout") {
			t.Errorf("attempt %d: stored error %q — want the cause without the phone number", attempt, d.Error)
		}
		last := attempt == len(notify.DefaultBackoff)
		switch {
		case !last && d.Status != "queued":
			t.Fatalf("attempt %d: status %s, want queued (retrying)", attempt, d.Status)
		case !last && !contains(f.pendingRetries(t, id), attempt+1):
			t.Fatalf("attempt %d: no retry job with attempt %d scheduled", attempt, attempt+1)
		case last && d.Status != "failed":
			t.Fatalf("after the last retry: status %s, want failed", d.Status)
		}
	}
	if !strings.Contains(f.logs.String(), `"alert":"notification_failed"`) {
		t.Error("no notification_failed alert logged")
	}
	if strings.Contains(f.logs.String(), "812345678") {
		t.Error("the phone number reached the logs")
	}
}

func contains(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// The whole loop run by a real worker: two failures, then success — the message ends sent after three
// attempts without anyone touching it.
func TestDelivery_WorkerRetriesUntilSent(t *testing.T) {
	f := setup(t)
	f.template(t, f.a, smsTemplate)
	flaky := &flakySender{failures: 2}
	workers := river.NewWorkers()
	f.deliverer.Senders[notify.ChannelSMS] = flaky
	f.deliverer.Backoff = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond}
	f.svc.Now = time.Now
	river.AddWorker(workers, f.deliverer)
	client, err := jobs.NewWorkerClient(f.app, jobs.WorkerOptions{Workers: workers})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); <-client.Stopped() })

	// Urgent, as an OTP is: with the wall clock in use, a non-urgent SMS sent during quiet hours (21:00–08:00
	// Bangkok) would wait for the morning and the test would fail at night.
	id := f.send(t, notify.Request{TemplateCode: "dsar.otp", Channel: "sms", RecipientAddress: "0812345678", Vars: map[string]any{"otp": "1"}, Urgent: true})
	deadline := time.Now().Add(30 * time.Second)
	for {
		d := f.status(t, id)
		if d.Status == "sent" {
			if d.Attempts != 3 || !strings.HasPrefix(d.ProviderMessageID, "flaky-") {
				t.Errorf("sent after %d attempts, provider id %q; want 3", d.Attempts, d.ProviderMessageID)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("not sent after 30s: %+v", d)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type flakySender struct {
	mu       sync.Mutex
	failures int
}

func (s *flakySender) Send(_ context.Context, m notify.Message) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failures > 0 {
		s.failures--
		return "", errors.New("temporarily unavailable")
	}
	return "flaky-" + m.NotificationID.String(), nil
}

// Real SMTP against an in-process server: Thai subject and body arrive intact; a 5xx rejection fails
// the message at once instead of retrying.
func TestSMTPSender(t *testing.T) {
	f := setup(t)
	srv := startSMTP(t)
	f.deliverer.Senders[notify.ChannelEmail] = &notify.SMTPSender{Addr: srv.addr, From: "noreply@pdpa.example"}
	f.template(t, f.a, notify.TemplateInput{Code: "dsar.received", Channel: "email", Language: "th",
		Subject: "ได้รับคำขอ {{.request_no}}", Body: "เรียน {{.name}}\nเราได้รับคำขอ {{.request_no}} แล้ว", Variables: []string{"name", "request_no"}})

	id := f.send(t, notify.Request{TemplateCode: "dsar.received", Channel: "email", RecipientAddress: "somchai@example.co.th",
		Vars: map[string]any{"name": "สมชาย", "request_no": "DSAR-2026-0001"}})
	f.deliver(t, id, 0)
	if d := f.status(t, id); d.Status != "sent" || d.RecipientMasked != "so****@example.co.th" {
		t.Fatalf("status = %+v", d)
	}
	msgs := srv.messages()
	if len(msgs) != 1 {
		t.Fatalf("server got %d messages", len(msgs))
	}
	m, err := mail.ReadMessage(strings.NewReader(msgs[0].data))
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	raw, _ := io.ReadAll(m.Body)
	body, _ := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(raw), "\r\n", ""))
	if msgs[0].rcpt != "somchai@example.co.th" || subject != "ได้รับคำขอ DSAR-2026-0001" || !strings.Contains(string(body), "เรียน สมชาย") {
		t.Errorf("rcpt %q subject %q body %q", msgs[0].rcpt, subject, body)
	}

	rejected := f.send(t, notify.Request{TemplateCode: "dsar.received", Channel: "email", RecipientAddress: "reject@example.co.th",
		Vars: map[string]any{"name": "x", "request_no": "y"}})
	f.deliver(t, rejected, 0)
	if d := f.status(t, rejected); d.Status != "failed" || d.Attempts != 1 {
		t.Errorf("5xx rejection: %+v, want failed after one attempt", d)
	}
	if retries := f.pendingRetries(t, rejected); contains(retries, 1) {
		t.Errorf("permanent failure scheduled a retry: %v", retries)
	}
}

// Non-urgent SMS queued at night waits until quiet hours end; urgent goes now.
func TestSend_HoldsNonUrgentSMSInQuietHours(t *testing.T) {
	f := setup(t)
	f.template(t, f.a, smsTemplate)
	f.now = time.Date(2026, 9, 25, 16, 30, 0, 0, time.UTC) // 23:30 Bangkok
	held := f.send(t, notify.Request{TemplateCode: "dsar.otp", Channel: "sms", RecipientAddress: "0812345678", Vars: map[string]any{"otp": "1"}})
	urgent := f.send(t, notify.Request{TemplateCode: "dsar.otp", Channel: "sms", RecipientAddress: "0812345678", Vars: map[string]any{"otp": "2"}, Urgent: true})

	scheduled := func(id uuid.UUID) time.Time {
		var at time.Time
		if err := f.app.QueryRow(context.Background(), `SELECT scheduled_at FROM river_job WHERE kind = 'notify.deliver' AND args->>'notification_id' = $1`, id.String()).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	want := time.Date(2026, 9, 26, 1, 0, 0, 0, time.UTC) // 08:00 Bangkok
	if got := scheduled(held); !got.Equal(want) {
		t.Errorf("non-urgent scheduled at %s, want %s", got, want)
	}
	if got := scheduled(urgent); got.After(time.Now().Add(time.Minute)) {
		t.Errorf("urgent scheduled at %s, want immediately", got)
	}
}

// Recipient addresses and variables are encrypted at rest (PLT-13): the row holds neither in clear.
func TestSend_EncryptsRecipientAndVariables(t *testing.T) {
	f := setup(t)
	f.template(t, f.a, smsTemplate)
	id := f.send(t, notify.Request{TemplateCode: "dsar.otp", Channel: "sms", RecipientAddress: "+66812345678", Vars: map[string]any{"otp": "482913"}})
	var addr, payload []byte
	_ = f.in(t, f.a, func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT recipient_address_enc, payload::text::bytea FROM platform.notifications WHERE id = $1`, id).Scan(&addr, &payload)
	})
	for _, secret := range []string{"812345678", "482913"} {
		if bytes.Contains(addr, []byte(secret)) || bytes.Contains(payload, []byte(secret)) {
			t.Errorf("%s stored in clear", secret)
		}
	}
	f.deliver(t, id, 0)
	if msgs := f.sms.Messages(); len(msgs) != 1 || msgs[0].To != "+66812345678" || msgs[0].Body != "รหัส OTP ของคุณคือ 482913" {
		t.Errorf("sent = %+v", msgs)
	}
}

func TestTemplates_ResolutionAndValidation(t *testing.T) {
	f := setup(t)
	// A global (platform) template, which only the owner can write.
	if err := pdb.WithTenantTx(context.Background(), f.owner, f.a.ID.String(), "", func(ctx context.Context) error {
		// Global rows are written the way migrations seed them: RLS force lifted inside the transaction.
		tx := pdb.MustTxFromContext(ctx)
		for _, stmt := range []string{
			`ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY`,
			`INSERT INTO platform.notification_templates (tenant_id, code, channel, language, body, variables)
			 VALUES (NULL, 'test.global', 'sms', 'th', 'global {{.x}}', '["x"]')`,
			`ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY`,
		} {
			if _, err := tx.Exec(ctx, stmt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), f.owner, f.a.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			_, _ = tx.Exec(ctx, `ALTER TABLE platform.notification_templates NO FORCE ROW LEVEL SECURITY`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.notifications WHERE template_id IN (SELECT id FROM platform.notification_templates WHERE code = 'test.global')`)
			_, _ = tx.Exec(ctx, `DELETE FROM platform.notification_templates WHERE code = 'test.global'`)
			_, err := tx.Exec(ctx, `ALTER TABLE platform.notification_templates FORCE ROW LEVEL SECURITY`)
			return err
		})
	})

	// English requested, only Thai exists → Thai; tenant B sees the global one.
	id := f.send(t, notify.Request{TemplateCode: "test.global", Channel: "sms", Language: "en", RecipientAddress: "0812345678", Vars: map[string]any{"x": "1"}})
	f.deliver(t, id, 0)
	// Tenant A overrides it; its own now wins.
	f.template(t, f.a, notify.TemplateInput{Code: "test.global", Channel: "sms", Language: "th", Body: "tenant {{.x}}", Variables: []string{"x"}})
	id2 := f.send(t, notify.Request{TemplateCode: "test.global", Channel: "sms", RecipientAddress: "0812345678", Vars: map[string]any{"x": "2"}})
	f.deliver(t, id2, 0)
	msgs := f.sms.Messages()
	if len(msgs) != 2 || msgs[0].Body != "global 1" || msgs[1].Body != "tenant 2" {
		t.Errorf("bodies = %+v", msgs)
	}

	err := f.in(t, f.a, func(ctx context.Context) error {
		_, err := f.svc.Send(ctx, notify.Request{TemplateCode: "test.global", Channel: "sms", RecipientAddress: "0812345678"})
		return err
	})
	if !errors.Is(err, notify.ErrTemplateInvalid) {
		t.Errorf("missing variable: %v, want ErrTemplateInvalid", err)
	}
	err = f.in(t, f.a, func(ctx context.Context) error {
		_, err := f.svc.Send(ctx, notify.Request{TemplateCode: "nope", Channel: "sms", RecipientAddress: "0812345678"})
		return err
	})
	if !errors.Is(err, notify.ErrTemplateNotFound) {
		t.Errorf("unknown template: %v, want ErrTemplateNotFound", err)
	}
}

func TestTemplates_CRUDWithOptimisticLocking(t *testing.T) {
	f := setup(t)
	tpl := f.template(t, f.a, smsTemplate)
	err := f.in(t, f.a, func(ctx context.Context) error {
		if _, err := f.svc.CreateTemplate(ctx, smsTemplate); !errors.Is(err, notify.ErrDuplicateTemplate) {
			t.Errorf("duplicate: %v", err)
		}
		in := smsTemplate
		in.Body = "OTP: {{.otp}}"
		updated, err := f.svc.UpdateTemplate(ctx, tpl.ID, tpl.RowVersion, in)
		if err != nil {
			return err
		}
		if updated.RowVersion != tpl.RowVersion+1 || updated.Body != "OTP: {{.otp}}" {
			t.Errorf("updated = %+v", updated)
		}
		if _, err := f.svc.UpdateTemplate(ctx, tpl.ID, tpl.RowVersion, in); !errors.Is(err, notify.ErrVersionMismatch) {
			t.Errorf("stale If-Match: %v, want ErrVersionMismatch", err)
		}
		bad := in
		bad.Body = "OTP: {{.code}}"
		if _, err := f.svc.UpdateTemplate(ctx, tpl.ID, updated.RowVersion, bad); !errors.Is(err, notify.ErrTemplateInvalid) {
			t.Errorf("undeclared variable: %v, want ErrTemplateInvalid", err)
		}
		return f.svc.DeleteTemplate(ctx, tpl.ID, updated.RowVersion)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInApp_InboxAndRead(t *testing.T) {
	f := setup(t)
	f.template(t, f.a, notify.TemplateInput{Code: "dsar.assigned", Channel: "in_app", Language: "th", Subject: "งานใหม่", Body: "คุณได้รับมอบหมายคำขอ {{.request_no}}", Variables: []string{"request_no"}})
	user := f.a.UserID
	if err := f.in(t, f.a, func(ctx context.Context) error { // seeded users start invited; notify active ones
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE iam.users SET status = 'active' WHERE id = $1`, user)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	id := f.send(t, notify.Request{TemplateCode: "dsar.assigned", Channel: "in_app", RecipientUserID: &user, Vars: map[string]any{"request_no": "D-9"}})

	err := f.in(t, f.a, func(ctx context.Context) error {
		items, unread, err := f.svc.Inbox(ctx, user, 10)
		if err != nil {
			return err
		}
		if unread != 1 || len(items) != 1 || items[0].Title != "งานใหม่" || items[0].Body != "คุณได้รับมอบหมายคำขอ D-9" {
			t.Errorf("inbox = %+v unread %d", items, unread)
		}
		if err := f.svc.MarkRead(ctx, uuid.New(), id); !errors.Is(err, notify.ErrNotFound) {
			t.Errorf("someone else marking it read: %v, want ErrNotFound", err)
		}
		if err := f.svc.MarkRead(ctx, user, id); err != nil {
			return err
		}
		n, err := f.svc.Unread(ctx, user)
		if n != 0 {
			t.Errorf("unread after read = %d", n)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if d := f.status(t, id); d.Status != "delivered" {
		t.Errorf("status after read = %s, want delivered", d.Status)
	}
}

// Two-tenant isolation (CLAUDE.md rule 1): tenant B sees none of tenant A's messages or templates.
func TestTenantsAreIsolated(t *testing.T) {
	f := setup(t)
	tpl := f.template(t, f.a, smsTemplate)
	id := f.send(t, notify.Request{TemplateCode: "dsar.otp", Channel: "sms", RecipientAddress: "0812345678", Vars: map[string]any{"otp": "1"}})
	err := f.in(t, f.b, func(ctx context.Context) error {
		if _, err := f.svc.Get(ctx, id); !errors.Is(err, notify.ErrNotFound) {
			t.Errorf("B reading A's message: %v", err)
		}
		list, _, err := f.svc.List(ctx, notify.ListFilter{Limit: 200})
		if err != nil {
			return err
		}
		for _, d := range list {
			if d.ID == id {
				t.Error("A's message in B's delivery log")
			}
		}
		if _, err := f.svc.GetTemplate(ctx, tpl.ID); !errors.Is(err, notify.ErrNotFound) {
			t.Errorf("B reading A's template: %v", err)
		}
		if _, err := f.svc.Send(ctx, notify.Request{TemplateCode: "dsar.otp", Channel: "sms", RecipientAddress: "0812345678", Vars: map[string]any{"otp": "1"}}); !errors.Is(err, notify.ErrTemplateNotFound) {
			t.Errorf("B using A's template: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestList_PagesNewestFirst(t *testing.T) {
	f := setup(t)
	f.template(t, f.a, smsTemplate)
	var ids []uuid.UUID
	for i := 0; i < 5; i++ {
		ids = append(ids, f.send(t, notify.Request{TemplateCode: "dsar.otp", Channel: "sms", RecipientAddress: "0812345678", Vars: map[string]any{"otp": "1"}}))
	}
	var seen []uuid.UUID
	cursor := ""
	for pages := 0; pages < 5; pages++ {
		var page []notify.Delivery
		var next string
		_ = f.in(t, f.a, func(ctx context.Context) error {
			var err error
			page, next, err = f.svc.List(ctx, notify.ListFilter{Status: "queued", Limit: 2, Cursor: cursor})
			return err
		})
		for _, d := range page {
			seen = append(seen, d.ID)
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != 5 || seen[0] != ids[4] || seen[4] != ids[0] {
		t.Errorf("paged %v, want the 5 newest first", seen)
	}
}

// ---- a minimal SMTP server ----

type smtpMsg struct{ rcpt, data string }

type smtpServer struct {
	addr string
	mu   sync.Mutex
	msgs []smtpMsg
}

func (s *smtpServer) messages() []smtpMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]smtpMsg(nil), s.msgs...)
}

// startSMTP accepts mail for anyone except reject@…, which gets a 550.
func startSMTP(t *testing.T) *smtpServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &smtpServer{addr: ln.Addr().String()}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *smtpServer) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := func(line string) { conn.Write([]byte(line + "\r\n")) }
	w("220 test ESMTP")
	var rcpt string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			w("250 test")
		case strings.HasPrefix(cmd, "MAIL FROM"):
			w("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO"):
			rcpt = strings.Trim(strings.TrimSpace(line)[len("RCPT TO:"):], "<>")
			if strings.HasPrefix(rcpt, "reject@") {
				w("550 5.1.1 <" + rcpt + ">: Recipient address rejected")
				continue
			}
			w("250 ok")
		case cmd == "DATA":
			w("354 go ahead")
			var data strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				data.WriteString(l)
			}
			s.mu.Lock()
			s.msgs = append(s.msgs, smtpMsg{rcpt: rcpt, data: data.String()})
			s.mu.Unlock()
			w("250 queued")
		case cmd == "QUIT":
			w("221 bye")
			return
		default:
			w("250 ok")
		}
	}
}
