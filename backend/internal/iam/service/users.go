package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	iamstore "pdpa-platform/internal/iam/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// UserName is the part of a user other modules may show next to their content (no contact details).
type UserName struct {
	ID          uuid.UUID
	DisplayName string
}

// SearchUsers finds up to 10 active users of the tenant whose display name starts with prefix — the
// @mention picker (PLT-07). LIKE wildcards in the input match literally.
func SearchUsers(ctx context.Context, prefix string) ([]UserName, error) {
	p := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(prefix))
	rows, err := iamstore.New(pdb.MustTxFromContext(ctx)).SearchActiveUsers(ctx, &p)
	if err != nil {
		return nil, fmt.Errorf("iam: search users: %w", err)
	}
	out := make([]UserName, 0, len(rows))
	for _, r := range rows {
		out = append(out, UserName{ID: r.ID, DisplayName: r.DisplayName})
	}
	return out, nil
}

// Names returns the display names of those ids that are active users of the tenant.
func Names(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := iamstore.New(pdb.MustTxFromContext(ctx)).ActiveUserNames(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("iam: names: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r.DisplayName
	}
	return out, nil
}
