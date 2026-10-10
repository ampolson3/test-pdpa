package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	pdb "pdpa-platform/internal/pkg/db"
)

// Enqueue inserts a job inside the request's (or job's) own transaction, taken from ctx — never a
// separate one — so the job exists if and only if the business change that caused it commits
// (docs/architecture/code-structure.md: "enqueue River job ด้วย InsertTx"). client may be an
// insert-only River client (no queues or workers configured), which is what cmd/api uses.
//
// A tenant-scoped job must be for the transaction's own tenant: a job for another tenant would
// later run under that tenant's RLS context, bypassing the isolation the request itself was under.
func Enqueue(ctx context.Context, client *river.Client[pgx.Tx], args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	tx := pdb.MustTxFromContext(ctx)

	if tj, ok := args.(TenantJob); ok {
		var current string
		if err := tx.QueryRow(ctx, `SELECT coalesce(current_setting('app.tenant_id', true), '')`).Scan(&current); err != nil {
			return nil, fmt.Errorf("jobs: read tenant context: %w", err)
		}
		if tj.JobTenantID() != current {
			return nil, fmt.Errorf("jobs: %s args tenant_id does not match the transaction's tenant", args.Kind())
		}
	}

	res, err := client.InsertTx(ctx, tx, args, opts)
	if err != nil {
		return nil, fmt.Errorf("jobs: enqueue %s: %w", args.Kind(), err)
	}
	return res, nil
}
