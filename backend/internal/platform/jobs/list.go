package jobs

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	pdb "pdpa-platform/internal/pkg/db"
)

// ListFilter narrows ListForTenant. Zero values mean "no filter"; Limit defaults to 50.
type ListFilter struct {
	States []rivertype.JobState
	Kind   string
	Cursor string
	Limit  int
}

// Page is one page of a tenant's jobs, newest first.
type Page struct {
	Jobs       []*rivertype.JobRow
	NextCursor string // "" when there are no more pages
}

// ErrInvalidCursor means the cursor did not come from a previous page.
var ErrInvalidCursor = errors.New("jobs: invalid cursor")

// ListForTenant lists the jobs of the request transaction's tenant, for the admin job-status page.
//
// river_job has no RLS (River owns it, and the worker must see every tenant's jobs), so the tenant
// filter is applied here: it compares the job's args.tenant_id with app.tenant_id read from the
// same transaction WithTenantTx scoped — never with a value passed in by the caller — so a request
// can only ever see its own tenant's jobs. Platform-wide jobs (no tenant_id) never match.
func ListForTenant(ctx context.Context, client *river.Client[pgx.Tx], f ListFilter) (Page, error) {
	tx := pdb.MustTxFromContext(ctx)

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	params := river.NewJobListParams().
		Where("args->>'tenant_id' = current_setting('app.tenant_id', true)").
		OrderBy(river.JobListOrderByID, river.SortOrderDesc).
		First(limit)
	if len(f.States) > 0 {
		params = params.States(f.States...)
	}
	if f.Kind != "" {
		params = params.Kinds(f.Kind)
	}
	if f.Cursor != "" {
		var cursor river.JobListCursor
		if err := cursor.UnmarshalText([]byte(f.Cursor)); err != nil {
			return Page{}, ErrInvalidCursor
		}
		params = params.After(&cursor)
	}

	res, err := client.JobListTx(ctx, tx, params)
	if err != nil {
		return Page{}, fmt.Errorf("jobs: list: %w", err)
	}

	page := Page{Jobs: res.Jobs}
	if len(res.Jobs) == limit && res.LastCursor != nil {
		text, err := res.LastCursor.MarshalText()
		if err != nil {
			return Page{}, fmt.Errorf("jobs: encode cursor: %w", err)
		}
		page.NextCursor = string(text)
	}
	return page, nil
}

// LastError returns the most recent attempt's error message, cut to maxRunes, or "" if none.
func LastError(job *rivertype.JobRow, maxRunes int) string {
	if len(job.Errors) == 0 {
		return ""
	}
	msg := job.Errors[len(job.Errors)-1].Error
	if utf8.RuneCountInString(msg) <= maxRunes {
		return msg
	}
	return string([]rune(msg)[:maxRunes])
}
