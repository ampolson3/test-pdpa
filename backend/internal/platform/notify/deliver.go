package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/jobs"
	notifystore "pdpa-platform/internal/platform/notify/store"
)

// DeliverArgs is notify.deliver: one attempt to send one notification. Attempt counts from 0.
type DeliverArgs struct {
	jobs.TenantArgs
	NotificationID string `json:"notification_id"`
	Attempt        int    `json:"attempt"`
}

func (DeliverArgs) Kind() string { return "notify.deliver" }

// DefaultBackoff is the wait before each retry (like webhook delivery, ST-07): 1 m, 5 m, 30 m, 2 h, 6 h
// — six attempts over about eight and a half hours, then the message is failed.
var DefaultBackoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}

// Deliverer works notify.deliver inside the tenant transaction. A failed attempt is recorded on the
// notification (attempts, redacted error) and committed, and the next attempt is scheduled as a new
// job — so the delivery log shows every retry as it happens (PLT-04: "ตรวจสถานะรายข้อความได้").
// Status: queued → sent | failed (state machine PLT-04), each change audited.
type Deliverer struct {
	river.WorkerDefaults[DeliverArgs]

	Service *Service
	Senders map[string]Sender // by channel
	Audit   *audit.Service
	Backoff []time.Duration // nil = DefaultBackoff
	Logger  *slog.Logger
}

func (d *Deliverer) Work(ctx context.Context, job *river.Job[DeliverArgs]) error {
	id, err := uuid.Parse(job.Args.NotificationID)
	if err != nil {
		return river.JobCancel(err)
	}
	q := notifystore.New(pdb.MustTxFromContext(ctx))
	n, err := q.LockNotification(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if n.Status != "queued" {
		return nil // sent, failed or cancelled already
	}

	msg, err := d.prepare(ctx, q, n)
	var providerID string
	if err == nil {
		sender, ok := d.Senders[n.Channel]
		if !ok {
			err = &PermanentError{Err: fmt.Errorf("no sender configured for channel %s", n.Channel)}
		} else {
			providerID, err = sender.Send(ctx, msg)
		}
	}
	if err == nil {
		if err := q.MarkNotificationSent(ctx, notifystore.MarkNotificationSentParams{ID: id, ProviderMessageID: &providerID}); err != nil {
			return err
		}
		return d.audit(ctx, job, id, "sent")
	}
	return d.fail(ctx, q, job, id, err)
}

// prepare decrypts the recipient and variables and renders the message.
func (d *Deliverer) prepare(ctx context.Context, q *notifystore.Queries, n notifystore.LockNotificationRow) (Message, error) {
	msg := Message{NotificationID: n.ID, Channel: n.Channel}
	switch {
	case len(n.RecipientAddressEnc) > 0:
		addr, err := d.Service.Keyring.Decrypt(ctx, cryptoClass, addressContext, n.RecipientAddressEnc)
		if err != nil {
			return msg, &PermanentError{Err: fmt.Errorf("recipient address unreadable: %w", err)}
		}
		msg.To = string(addr)
	case n.RecipientUserID.Valid:
		c, err := iamservice.ContactOf(ctx, uuid.UUID(n.RecipientUserID.Bytes))
		if errors.Is(err, iamservice.ErrNoContact) {
			return msg, &PermanentError{Err: err}
		}
		if err != nil {
			return msg, err
		}
		switch n.Channel {
		case ChannelEmail:
			msg.To = c.Email
		case ChannelSMS:
			msg.To = c.Phone
		}
		if msg.To == "" {
			return msg, &PermanentError{Err: fmt.Errorf("user has no address for channel %s", n.Channel)}
		}
	}
	if !n.TemplateID.Valid {
		return msg, &PermanentError{Err: errors.New("template missing")}
	}
	tpl, err := q.GetTemplate(ctx, uuid.UUID(n.TemplateID.Bytes))
	if err != nil {
		return msg, &PermanentError{Err: fmt.Errorf("template: %w", err)}
	}
	_, vars, err := d.Service.decryptVars(ctx, n.Payload)
	if err != nil {
		return msg, &PermanentError{Err: fmt.Errorf("variables unreadable: %w", err)}
	}
	r, err := render(tpl.Subject, tpl.Body, vars)
	if err != nil {
		return msg, &PermanentError{Err: err}
	}
	msg.Subject, msg.Body = r.Subject, r.Body
	return msg, nil
}

func (d *Deliverer) fail(ctx context.Context, q *notifystore.Queries, job *river.Job[DeliverArgs], id uuid.UUID, cause error) error {
	backoff := d.Backoff
	if backoff == nil {
		backoff = DefaultBackoff
	}
	var permanent *PermanentError
	final := errors.As(cause, &permanent) || job.Args.Attempt >= len(backoff)
	reason := redact(cause.Error())
	if _, err := q.MarkNotificationAttemptFailed(ctx, notifystore.MarkNotificationAttemptFailedParams{ID: id, Error: &reason, Final: final}); err != nil {
		return err
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	attrs := []any{"notification_id", id.String(), "tenant_id", job.Args.TenantID, "attempt", job.Args.Attempt + 1, "error", reason}
	if final {
		logger.ErrorContext(ctx, "notification failed", append(attrs, "alert", "notification_failed")...)
		return d.audit(ctx, job, id, "failed")
	}
	logger.WarnContext(ctx, "notification attempt failed; retry scheduled", attrs...)
	next := job.Args
	next.Attempt++
	_, err := jobs.Enqueue(ctx, d.Service.River, next, &river.InsertOpts{ScheduledAt: d.Service.now().Add(backoff[job.Args.Attempt])})
	return err
}

func (d *Deliverer) audit(ctx context.Context, job *river.Job[DeliverArgs], id uuid.UUID, to string) error {
	if d.Audit == nil {
		return nil
	}
	tenantID, _ := uuid.Parse(job.Args.TenantID)
	return d.Audit.Write(ctx, audit.Entry{
		TenantID: tenantID, ActorType: "system", Action: "platform.notification.deliver",
		EntityType: "notification", EntityID: &id,
		Before: map[string]any{"status": "queued"}, After: map[string]any{"status": to, "attempt": job.Args.Attempt + 1},
	})
}
