package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	notifystore "pdpa-platform/internal/platform/notify/store"
)

// Delivery is one row of the delivery log. The recipient is masked; message text and variables are
// never exposed here.
type Delivery struct {
	ID                uuid.UUID
	TemplateCode      string
	Channel           string
	RecipientUserID   *uuid.UUID
	RecipientMasked   string
	Status            string
	Attempts          int
	ProviderMessageID string
	SentAt            *time.Time
	Error             string
	EntityType        string
	EntityID          *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Get returns one notification's delivery status.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Delivery, error) {
	row, err := notifystore.New(pdb.MustTxFromContext(ctx)).GetNotification(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, ErrNotFound
	}
	if err != nil {
		return Delivery{}, err
	}
	return s.delivery(ctx, notifystore.ListNotificationsRow(row)), nil
}

// ListFilter narrows List. Cursor is the id of the last row of the previous page.
type ListFilter struct {
	Status  string
	Channel string
	Cursor  string
	Limit   int
}

// ErrInvalidCursor: the cursor did not come from a previous page.
var ErrInvalidCursor = errors.New("notify: invalid cursor")

// List returns one page of the delivery log, newest first, and the next page's cursor ("" at the end).
func (s *Service) List(ctx context.Context, f ListFilter) ([]Delivery, string, error) {
	q := notifystore.New(pdb.MustTxFromContext(ctx))
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	params := notifystore.ListNotificationsParams{
		BeforeCreatedAt: pgtype.Timestamptz{InfinityModifier: pgtype.Infinity, Valid: true},
		BeforeID:        uuid.Max,
		PageSize:        int32(limit),
	}
	if f.Status != "" {
		params.Status = &f.Status
	}
	if f.Channel != "" {
		params.Channel = &f.Channel
	}
	if f.Cursor != "" {
		id, err := uuid.Parse(f.Cursor)
		if err != nil {
			return nil, "", ErrInvalidCursor
		}
		last, err := q.GetNotification(ctx, id)
		if err != nil {
			return nil, "", ErrInvalidCursor
		}
		params.BeforeCreatedAt, params.BeforeID = last.CreatedAt, last.ID
	}
	rows, err := q.ListNotifications(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("notify: list: %w", err)
	}
	out := make([]Delivery, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.delivery(ctx, r))
	}
	next := ""
	if len(rows) == limit {
		next = rows[len(rows)-1].ID.String()
	}
	return out, next, nil
}

func (s *Service) delivery(ctx context.Context, r notifystore.ListNotificationsRow) Delivery {
	d := Delivery{
		ID: r.ID, Channel: r.Channel, Status: r.Status, Attempts: int(r.Attempts),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.TemplateCode != nil {
		d.TemplateCode = *r.TemplateCode
	}
	if r.RecipientUserID.Valid {
		id := uuid.UUID(r.RecipientUserID.Bytes)
		d.RecipientUserID = &id
	}
	if len(r.RecipientAddressEnc) > 0 {
		if addr, err := s.Keyring.Decrypt(ctx, cryptoClass, addressContext, r.RecipientAddressEnc); err == nil {
			d.RecipientMasked = Mask(string(addr))
		} else {
			d.RecipientMasked = "***"
		}
	}
	if r.ProviderMessageID != nil {
		d.ProviderMessageID = *r.ProviderMessageID
	}
	if r.SentAt.Valid {
		t := r.SentAt.Time
		d.SentAt = &t
	}
	if r.Error != nil {
		d.Error = *r.Error
	}
	if r.EntityType != nil {
		d.EntityType = *r.EntityType
	}
	if r.EntityID.Valid {
		id := uuid.UUID(r.EntityID.Bytes)
		d.EntityID = &id
	}
	return d
}

// Mask hides most of an address (CLAUDE.md rule 3: responses are masked by default):
// "somchai@example.co.th" → "so****@example.co.th", "+66812345678" → "+66******78".
func Mask(addr string) string {
	if at := strings.LastIndex(addr, "@"); at > 0 {
		local := []rune(addr[:at])
		keep := 2
		if len(local) <= 2 {
			keep = 1
		}
		return string(local[:keep]) + "****" + addr[at:]
	}
	r := []rune(addr)
	if len(r) <= 4 {
		return "****"
	}
	head := 3
	if len(r) < 8 {
		head = 1
	}
	return string(r[:head]) + strings.Repeat("*", len(r)-head-2) + string(r[len(r)-2:])
}

// InboxItem is an in-app message rendered for its recipient.
type InboxItem struct {
	ID        uuid.UUID
	Title     string
	Body      string
	Read      bool
	CreatedAt time.Time
}

// Inbox returns the signed-in user's recent in-app messages, rendered, and their unread count.
func (s *Service) Inbox(ctx context.Context, userID uuid.UUID, limit int) ([]InboxItem, int64, error) {
	q := notifystore.New(pdb.MustTxFromContext(ctx))
	if limit <= 0 {
		limit = 20
	}
	rows, err := q.ListInbox(ctx, notifystore.ListInboxParams{UserID: pgtype.UUID{Bytes: userID, Valid: true}, PageSize: int32(limit)})
	if err != nil {
		return nil, 0, err
	}
	items := make([]InboxItem, 0, len(rows))
	for _, r := range rows {
		item := InboxItem{ID: r.ID, Read: r.Status == "delivered", CreatedAt: r.CreatedAt.Time}
		if r.TemplateBody != nil {
			if _, vars, err := s.decryptVars(ctx, r.Payload); err == nil {
				if out, err := render(r.TemplateSubject, *r.TemplateBody, vars); err == nil {
					item.Title, item.Body = out.Subject, out.Body
				}
			}
		}
		items = append(items, item)
	}
	unread, err := s.Unread(ctx, userID)
	return items, unread, err
}

// Unread counts the user's unread in-app messages.
func (s *Service) Unread(ctx context.Context, userID uuid.UUID) (int64, error) {
	return notifystore.New(pdb.MustTxFromContext(ctx)).CountUnread(ctx, pgtype.UUID{Bytes: userID, Valid: true})
}

// MarkRead marks one of the user's own in-app messages read (status sent → delivered).
func (s *Service) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	n, err := notifystore.New(pdb.MustTxFromContext(ctx)).MarkRead(ctx, notifystore.MarkReadParams{ID: id, UserID: pgtype.UUID{Bytes: userID, Valid: true}})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// marshalVariables / parseVariables convert notification_templates.variables (a JSON array of names).
func parseVariables(raw []byte) []string {
	var out []string
	_ = json.Unmarshal(raw, &out)
	return out
}
