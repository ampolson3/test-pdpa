// Package notify is the platform notification service (PLT-04): templates with variables per channel
// and language, queued delivery by e-mail (SMTP), SMS and LINE through River with automatic retry and a
// per-message delivery log, and in-app messages with an unread inbox and an SSE stream.
//
// SMS, e-mail provider and LINE OA are open (decisions.md Q-03, default "interface + adapter แบบ mock"):
// Sender is the interface, SMTPSender speaks plain SMTP, MockSender stands in for SMS and LINE.
// Recipient addresses and template variables are stored encrypted (crypto.Keyring, PLT-13).
package notify

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"
)

// Channels (platform.notifications.channel / notification_templates.channel).
const (
	ChannelEmail = "email"
	ChannelSMS   = "sms"
	ChannelLine  = "line"
	ChannelInApp = "in_app"
)

func validChannel(c string) bool {
	switch c {
	case ChannelEmail, ChannelSMS, ChannelLine, ChannelInApp:
		return true
	}
	return false
}

// ErrTemplateInvalid means a subject/body doesn't parse, or rendering needs a variable not supplied.
var ErrTemplateInvalid = errors.New("notify: template invalid")

// Rendered is a template filled with variables.
type Rendered struct {
	Subject string
	Body    string
}

// render fills subject and body with vars using text/template syntax ({{.name}}). A variable the
// template uses but vars lacks is an error, never an empty string in a message someone receives.
// Messages are plain text on every channel, so there is no markup to inject into.
func render(subject *string, body string, vars map[string]any) (Rendered, error) {
	var out Rendered
	if subject != nil && *subject != "" {
		s, err := execute("subject", *subject, vars)
		if err != nil {
			return Rendered{}, err
		}
		out.Subject = s
	}
	b, err := execute("body", body, vars)
	if err != nil {
		return Rendered{}, err
	}
	out.Body = b
	return out, nil
}

func execute(name, text string, vars map[string]any) (string, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrTemplateInvalid, name, err)
	}
	if vars == nil {
		vars = map[string]any{}
	}
	var b bytes.Buffer
	if err := t.Execute(&b, vars); err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrTemplateInvalid, name, err)
	}
	return strings.TrimSpace(b.String()), nil
}

// sampleVars fills every declared variable with a placeholder, for validating and previewing a
// template without real data.
func sampleVars(declared []string) map[string]any {
	vars := map[string]any{}
	for _, v := range declared {
		vars[v] = "{" + v + "}"
	}
	return vars
}
