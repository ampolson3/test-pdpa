package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	store "pdpa-platform/internal/iam/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
)

// NewLoader returns the authz.Loader the AuthZ middleware (#9) uses on a cache miss, per
// docs/architecture/code-structure.md: "โหลดผ่าน db.WithTenantTx แบบอ่านอย่างเดียวของตัวเอง" — its own
// read-only transaction, independent of the request's own Tx (#11), since AuthZ runs before Tx.
func NewLoader(pool *pgxpool.Pool) authz.Loader {
	return func(ctx context.Context, tenantID, userID string) (authz.Grants, error) {
		uid, err := uuid.Parse(userID)
		if err != nil {
			return authz.Grants{}, fmt.Errorf("iam: parse user id: %w", err)
		}

		var grants authz.Grants
		err = pdb.WithTenantTx(ctx, pool, tenantID, userID, func(ctx context.Context) error {
			q := store.New(pdb.MustTxFromContext(ctx))

			assignments, err := q.ListEffectiveRoleAssignments(ctx, pgtype.UUID{Bytes: uid, Valid: true})
			if err != nil {
				return fmt.Errorf("load role assignments: %w", err)
			}

			seenRole := make(map[uuid.UUID]bool, len(assignments))
			roleIDs := make([]uuid.UUID, 0, len(assignments))
			roles := make([]string, 0, len(assignments))
			scopes := make([]authz.DataScope, 0, len(assignments))
			for _, a := range assignments {
				if !seenRole[a.RoleID] {
					seenRole[a.RoleID] = true
					roleIDs = append(roleIDs, a.RoleID)
					roles = append(roles, a.RoleCode)
				}
				scopes = append(scopes, authz.DataScope{
					ScopeType:          a.ScopeType,
					LegalEntityID:      uuidPtr(a.LegalEntityID),
					OrgUnitID:          uuidPtr(a.OrgUnitID),
					IncludeDescendants: a.IncludeDescendants,
				})
			}

			var permissions []string
			if len(roleIDs) > 0 {
				permissions, err = q.ListPermissionsForRoles(ctx, roleIDs)
				if err != nil {
					return fmt.Errorf("load permissions: %w", err)
				}
			}

			grants = authz.Grants{
				UserID:      userID,
				TenantID:    tenantID,
				Roles:       roles,
				Permissions: permissions,
				Scopes:      scopes,
			}
			return nil
		})
		if err != nil {
			return authz.Grants{}, fmt.Errorf("iam: load grants: %w", err)
		}
		return grants, nil
	}
}
