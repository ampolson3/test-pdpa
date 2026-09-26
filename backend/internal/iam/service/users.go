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

// GroupIDsOf lists the groups a user belongs to (work assigned to a group is theirs to claim, PLT-05).
func GroupIDsOf(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := iamstore.New(pdb.MustTxFromContext(ctx)).GroupIDsOfUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("iam: groups of user: %w", err)
	}
	return ids, nil
}

// GroupMembers lists the active members of a group.
func GroupMembers(ctx context.Context, groupID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := iamstore.New(pdb.MustTxFromContext(ctx)).ActiveGroupMembers(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("iam: group members: %w", err)
	}
	return ids, nil
}

// GroupNames returns the names of those ids that are groups of the tenant.
func GroupNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := iamstore.New(pdb.MustTxFromContext(ctx)).GroupNames(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("iam: group names: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}

// SearchGroups finds up to 10 groups whose name starts with prefix (assignee pickers).
func SearchGroups(ctx context.Context, prefix string) ([]UserName, error) {
	p := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(prefix))
	rows, err := iamstore.New(pdb.MustTxFromContext(ctx)).SearchGroups(ctx, &p)
	if err != nil {
		return nil, fmt.Errorf("iam: search groups: %w", err)
	}
	out := make([]UserName, 0, len(rows))
	for _, r := range rows {
		out = append(out, UserName{ID: r.ID, DisplayName: r.Name})
	}
	return out, nil
}

// AllNames returns display names of those ids that are users of the tenant, active or not (for records
// such as the audit trail that name people who may have left since).
func AllNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := iamstore.New(pdb.MustTxFromContext(ctx)).UserNamesAnyStatus(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("iam: names: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r.DisplayName
	}
	return out, nil
}

// UsersWithRole returns the active users holding a role (by code, e.g. "DPO") in the tenant, directly or
// through a group.
func UsersWithRole(ctx context.Context, roleCode string) ([]uuid.UUID, error) {
	ids, err := iamstore.New(pdb.MustTxFromContext(ctx)).ActiveUsersWithRole(ctx, roleCode)
	if err != nil {
		return nil, fmt.Errorf("iam: users with role: %w", err)
	}
	return ids, nil
}
