package notify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/jobs"
	notifystore "pdpa-platform/internal/platform/notify/store"
)

const (
	cryptoClass    = "notification"
	addressContext = "platform.notifications.recipient_address_enc"
	varsContext    = "platform.notifications.payload.vars_enc"
	defaultLang    = "th" // decisions.md Q-17
)

var (
	ErrTemplateNotFound = errors.New("notify: template not found")
	ErrInvalidRequest   = errors.New("notify: invalid request")
	ErrNotFound         = errors.New("notify: not found")
	ErrVersionMismatch  = errors.New("notify: version mismatch")
)

// Request asks for one message to one recipient.
type Request struct {
	TemplateCode string
	Channel      string
	Language     string // "th" / "en"; empty = the recipient user's locale, else th
	// Exactly one of RecipientUserID (a user of the tenant; required for in_app) or RecipientAddress
	// (an e-mail address, phone number or LINE user id of someone who isn't a user, e.g. a data subject).
	RecipientUserID  *uuid.UUID
	RecipientAddress string
	Vars             map[string]any
	EntityType       string
	EntityID         *uuid.UUID
	// Urgent skips quiet hours (security alerts, OTP, breach deadlines).
	Urgent bool
}

// storedPayload is platform.notifications.payload: variables are encrypted — they routinely carry
// names and other personal data.
type storedPayload struct {
	Language string `json:"language"`
	VarsEnc  string `json:"vars_enc"`
}

// Service queues and reads notifications for the tenant of the transaction in ctx.
type Service struct {
	Keyring *crypto.Keyring
	River   *river.Client[pgx.Tx]
	Quiet   QuietHours
	Now     func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Send validates the request, renders it once to catch missing variables, stores it and — except for
// in-app messages, which are available to their recipient at once — queues notify.deliver, held until
// quiet hours end for non-urgent SMS and LINE. It runs in the caller's transaction, so a message is
// queued only if the business change that caused it commits.
func (s *Service) Send(ctx context.Context, req Request) (uuid.UUID, error) {
	if !validChannel(req.Channel) || req.TemplateCode == "" {
		return uuid.Nil, fmt.Errorf("%w: channel %q / template %q", ErrInvalidRequest, req.Channel, req.TemplateCode)
	}
	if (req.RecipientUserID == nil) == (req.RecipientAddress == "") {
		return uuid.Nil, fmt.Errorf("%w: give exactly one of recipient user or address", ErrInvalidRequest)
	}
	if req.Channel == ChannelInApp && req.RecipientUserID == nil {
		return uuid.Nil, fmt.Errorf("%w: in-app messages go to users", ErrInvalidRequest)
	}

	lang := req.Language
	if lang == "" && req.RecipientUserID != nil {
		if c, err := iamservice.ContactOf(ctx, *req.RecipientUserID); err == nil {
			lang = c.Locale
		} else if errors.Is(err, iamservice.ErrNoContact) {
			return uuid.Nil, fmt.Errorf("%w: recipient user not found or inactive", ErrInvalidRequest)
		} else {
			return uuid.Nil, err
		}
	}
	tx := pdb.MustTxFromContext(ctx)
	q := notifystore.New(tx)
	tpl, err := resolveTemplate(ctx, q, req.TemplateCode, req.Channel, lang)
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := render(tpl.Subject, tpl.Body, req.Vars); err != nil {
		return uuid.Nil, err
	}

	varsJSON, err := json.Marshal(req.Vars)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: vars: %v", ErrInvalidRequest, err)
	}
	varsEnc, err := s.Keyring.Encrypt(ctx, cryptoClass, varsContext, varsJSON)
	if err != nil {
		return uuid.Nil, err
	}
	payload, _ := json.Marshal(storedPayload{Language: tpl.Language, VarsEnc: base64.StdEncoding.EncodeToString(varsEnc)})

	var addressEnc []byte
	if req.RecipientAddress != "" {
		addr, err := normalizeAddress(req.Channel, req.RecipientAddress)
		if err != nil {
			return uuid.Nil, err
		}
		if addressEnc, err = s.Keyring.Encrypt(ctx, cryptoClass, addressContext, []byte(addr)); err != nil {
			return uuid.Nil, err
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	status := "queued"
	if req.Channel == ChannelInApp {
		status = "sent"
	}
	params := notifystore.InsertNotificationParams{
		ID: id, TemplateID: pgtype.UUID{Bytes: tpl.ID, Valid: true}, Channel: req.Channel,
		RecipientAddressEnc: addressEnc, Payload: payload, Status: status,
	}
	if req.RecipientUserID != nil {
		params.RecipientUserID = pgtype.UUID{Bytes: *req.RecipientUserID, Valid: true}
	}
	if req.EntityType != "" {
		params.EntityType = &req.EntityType
	}
	if req.EntityID != nil {
		params.EntityID = pgtype.UUID{Bytes: *req.EntityID, Valid: true}
	}
	if _, err := q.InsertNotification(ctx, params); err != nil {
		return uuid.Nil, fmt.Errorf("notify: store: %w", err)
	}
	if req.Channel == ChannelInApp {
		return id, nil
	}

	var tenant string
	if err := tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&tenant); err != nil {
		return uuid.Nil, err
	}
	release := s.Quiet.Release(s.now(), req.Channel, req.Urgent)
	opts := &river.InsertOpts{}
	if release.After(s.now()) {
		opts.ScheduledAt = release
	}
	if _, err := jobs.Enqueue(ctx, s.River, DeliverArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant}, NotificationID: id.String()}, opts); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// normalizeAddress puts an address in the form its channel sends to: lower-case e-mail, E.164 phone
// (Thai numbers without a country code become +66…), LINE user ids as given.
func normalizeAddress(channel, addr string) (string, error) {
	var kind crypto.IdentifierKind
	switch channel {
	case ChannelEmail:
		kind = crypto.KindEmail
	case ChannelSMS:
		kind = crypto.KindPhone
	default:
		kind = crypto.KindLineUID
	}
	out, err := crypto.Normalize(kind, addr)
	if err != nil {
		return "", fmt.Errorf("%w: recipient address is not a valid %s address", ErrInvalidRequest, channel)
	}
	return out, nil
}

// resolveTemplate finds code/channel in lang, falling back to Thai; a tenant's own template wins over
// the global one.
func resolveTemplate(ctx context.Context, q *notifystore.Queries, code, channel, lang string) (notifystore.ResolveTemplateRow, error) {
	if lang == "" {
		lang = defaultLang
	}
	for _, l := range []string{lang, defaultLang} {
		row, err := q.ResolveTemplate(ctx, notifystore.ResolveTemplateParams{Code: code, Channel: channel, Language: l})
		if err == nil {
			return row, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return row, fmt.Errorf("notify: template: %w", err)
		}
	}
	return notifystore.ResolveTemplateRow{}, fmt.Errorf("%w: %s/%s/%s", ErrTemplateNotFound, code, channel, lang)
}

func (s *Service) decryptVars(ctx context.Context, raw []byte) (string, map[string]any, error) {
	var p storedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", nil, err
	}
	enc, err := base64.StdEncoding.DecodeString(p.VarsEnc)
	if err != nil {
		return "", nil, err
	}
	plain, err := s.Keyring.Decrypt(ctx, cryptoClass, varsContext, enc)
	if err != nil {
		return "", nil, err
	}
	var vars map[string]any
	if err := json.Unmarshal(plain, &vars); err != nil {
		return "", nil, err
	}
	return p.Language, vars, nil
}
