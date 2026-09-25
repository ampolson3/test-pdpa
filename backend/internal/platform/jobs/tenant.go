package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	pdb "pdpa-platform/internal/pkg/db"
)

// TenantArgs is embedded in every tenant-scoped job's args. The job's JSON therefore always carries
// "tenant_id", which TenantTxMiddleware reads to open the job's one transaction, and which makes
// River's UniqueOpts{ByArgs: true} unique per tenant without further work.
//
// Job args are stored unencrypted in river_job (which has no RLS), so they carry identifiers only —
// never personal data, tokens or secrets (CLAUDE.md rule 3). Workers load everything else inside
// the tenant transaction.
type TenantArgs struct {
	TenantID string `json:"tenant_id"`
}

// JobTenantID lets Enqueue check that a job is enqueued for the tenant of the current transaction.
func (a TenantArgs) JobTenantID() string { return a.TenantID }

// TenantJob is implemented by any args type that embeds TenantArgs.
type TenantJob interface {
	JobTenantID() string
}

// ErrMissingTenant is the cancel reason for a job that is neither tenant-scoped nor registered as
// global. Retrying cannot fix it, so the job is cancelled instead of retried.
var ErrMissingTenant = errors.New("jobs: job args carry no valid tenant_id and the kind is not registered as global")

// TenantTxMiddleware enforces docs/architecture/code-structure.md's job wrapper rule and CLAUDE.md
// rule 1: every job runs inside exactly one db.WithTenantTx, opened from the tenant_id in its own
// args (the acting user is "" — jobs are system work). Workers read the transaction with
// db.MustTxFromContext and never begin one themselves. The transaction commits when the worker
// returns nil and rolls back on error, so a failed attempt leaves no partial writes behind.
//
// Kinds listed in GlobalKinds (e.g. partition.maintain) touch no tenant data and run without a
// transaction; any other kind without a valid tenant_id is cancelled rather than run unscoped.
type TenantTxMiddleware struct {
	river.MiddlewareDefaults

	Pool        *pgxpool.Pool
	GlobalKinds map[string]bool
}

func (m *TenantTxMiddleware) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	if m.GlobalKinds[job.Kind] {
		return doInner(ctx)
	}

	var args TenantArgs
	if err := json.Unmarshal(job.EncodedArgs, &args); err != nil {
		return river.JobCancel(fmt.Errorf("%w: %v", ErrMissingTenant, err))
	}
	if _, err := uuid.Parse(args.TenantID); err != nil {
		return river.JobCancel(ErrMissingTenant)
	}

	return pdb.WithTenantTx(ctx, m.Pool, args.TenantID, "", doInner)
}
