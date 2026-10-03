// Package tenants reads the tenant record (platform.tenants) for other modules (CLAUDE.md rule 9).
package tenants

import (
	"context"

	pdb "pdpa-platform/internal/pkg/db"
)

// Region is where the transaction's tenant keeps its data (platform.tenants.data_region, an ISO country code).
func Region(ctx context.Context) (string, error) {
	var region string
	err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT data_region FROM platform.tenants WHERE id = current_setting('app.tenant_id')::uuid`).Scan(&region)
	return region, err
}
