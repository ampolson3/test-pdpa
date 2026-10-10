package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	iamstore "pdpa-platform/internal/iam/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// Contact is how to reach a user: iam's exported view for other modules (CLAUDE.md rule 9), used by
// the notification service to resolve a recipient_user_id.
type Contact struct {
	Email  string
	Phone  string
	Locale string
}

// ErrNoContact: the user doesn't exist in this tenant or isn't active.
var ErrNoContact = errors.New("iam: user not found or not active")

// ContactOf reads the user's contact details inside the transaction in ctx (RLS scopes it to the tenant).
func ContactOf(ctx context.Context, userID uuid.UUID) (Contact, error) {
	row, err := iamstore.New(pdb.MustTxFromContext(ctx)).GetUserContact(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Contact{}, ErrNoContact
	}
	if err != nil {
		return Contact{}, fmt.Errorf("iam: contact: %w", err)
	}
	c := Contact{Email: row.Email, Locale: row.Locale}
	if row.Phone != nil {
		c.Phone = *row.Phone
	}
	return c, nil
}
