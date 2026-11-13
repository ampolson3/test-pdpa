package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"net/textproto"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Message is one rendered message for one recipient.
type Message struct {
	NotificationID uuid.UUID
	Channel        string
	To             string // e-mail address, E.164 phone or LINE user id
	Subject        string
	Body           string
}

// Sender delivers messages on one channel and returns the provider's message id.
type Sender interface {
	Send(ctx context.Context, m Message) (providerMessageID string, err error)
}

// PermanentError marks a failure retrying cannot fix (bad address, rejected content); the message
// fails at once instead of using up its retries.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// SMTPSender sends plain-text UTF-8 e-mail through an SMTP relay (STARTTLS when the server offers
// it). Credentials come from the environment / OpenBao (CLAUDE.md rule 14).
type SMTPSender struct {
	Addr     string // host:port
	From     string
	Username string
	Password string
}

func (s *SMTPSender) Send(ctx context.Context, m Message) (string, error) {
	id := fmt.Sprintf("<%s@pdpa-platform>", m.NotificationID)
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", s.From)
	fmt.Fprintf(&msg, "To: %s\r\n", m.To)
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.BEncoding.Encode("UTF-8", m.Subject))
	fmt.Fprintf(&msg, "Message-ID: %s\r\n", id)
	fmt.Fprintf(&msg, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	msg.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(m.Body))
	for len(enc) > 76 {
		msg.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	msg.WriteString(enc + "\r\n")

	if err := s.deliver(ctx, m.To, msg.Bytes()); err != nil {
		var perr *textproto.Error
		if errors.As(err, &perr) && perr.Code >= 500 {
			return "", &PermanentError{Err: err} // 5xx: rejected recipient / content
		}
		return "", err
	}
	return id, nil
}

// deliver is smtp.SendMail with a context and a deadline: a relay that stops answering can't hold a
// worker slot forever.
func (s *SMTPSender) deliver(ctx context.Context, to string, msg []byte) error {
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return err
	}
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(60 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
			return err
		}
	}
	if err := c.Mail(s.From); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// MockSender stands in for a channel whose provider isn't chosen yet (decisions.md Q-03: SMS, LINE
// OA). It records messages in memory (tests read Sent) and logs — without the recipient or text —
// that nothing actually left the system. FailWith makes every send fail, for retry tests.
type MockSender struct {
	Channel  string
	Logger   *slog.Logger
	FailWith error

	mu   sync.Mutex
	Sent []Message
}

func (s *MockSender) Send(ctx context.Context, m Message) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.FailWith != nil {
		return "", s.FailWith
	}
	s.Sent = append(s.Sent, m)
	if s.Logger != nil {
		s.Logger.WarnContext(ctx, "mock sender: message recorded, not delivered (provider not chosen, decisions.md Q-03)",
			"channel", s.Channel, "notification_id", m.NotificationID.String())
	}
	return "mock-" + m.NotificationID.String(), nil
}

// Messages returns a copy of what the mock recorded.
func (s *MockSender) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.Sent...)
}

var (
	emailRE  = regexp.MustCompile(`[^\s<>@"']+@[^\s<>@"']+`)
	digitsRE = regexp.MustCompile(`\+?\d[\d\s-]{5,}\d`)
)

// redact strips e-mail addresses and phone-like numbers from a provider error before it is stored in
// platform.notifications.error or logged (CLAUDE.md rule 3), and caps its length.
func redact(msg string) string {
	msg = emailRE.ReplaceAllString(msg, "[address]")
	msg = digitsRE.ReplaceAllString(msg, "[number]")
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return strings.TrimSpace(msg)
}
