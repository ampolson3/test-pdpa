// Package jobshttp holds the admin job-status endpoint of PLT-10. The types and ServerInterface in
// jobs.gen.go are generated from api/openapi/openapi.yaml (operation platformListJobs) by
// oapi-codegen — see oapi-codegen.yaml and `make gen`. The handler only translates between the
// wire format and internal/platform/jobs.
package jobshttp

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/jobs"
)

const lastErrorMaxRunes = 500

// Strict implements StrictServerInterface.
type Strict struct {
	river *river.Client[pgx.Tx]
}

func NewStrict(client *river.Client[pgx.Tx]) *Strict {
	return &Strict{river: client}
}

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) PlatformListJobs(ctx context.Context, req PlatformListJobsRequestObject) (PlatformListJobsResponseObject, error) {
	f := jobs.ListFilter{}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		f.Cursor = *req.Params.Cursor
	}
	if req.Params.Kind != nil {
		f.Kind = *req.Params.Kind
	}
	if req.Params.State != nil {
		for _, s := range *req.Params.State {
			f.States = append(f.States, rivertype.JobState(s))
		}
	}

	page, err := jobs.ListForTenant(ctx, h.river, f)
	if errors.Is(err, jobs.ErrInvalidCursor) {
		// Returned as an error so cmd/api's ResponseErrorHandlerFunc writes it through
		// httpx.WriteProblem, which localizes the title and adds the request id.
		return nil, httpx.RequestInvalid("cursor is not from a previous page")
	}
	if err != nil {
		return nil, err
	}

	resp := PlatformListJobs200JSONResponse{Data: make([]Job, 0, len(page.Jobs))}
	for _, j := range page.Jobs {
		out := Job{
			Id:          j.ID,
			Kind:        j.Kind,
			Queue:       j.Queue,
			State:       JobState(j.State),
			Attempt:     j.Attempt,
			MaxAttempts: j.MaxAttempts,
			CreatedAt:   j.CreatedAt.UTC(),
			ScheduledAt: j.ScheduledAt.UTC(),
		}
		if j.AttemptedAt != nil {
			t := j.AttemptedAt.UTC()
			out.AttemptedAt = &t
		}
		if j.FinalizedAt != nil {
			t := j.FinalizedAt.UTC()
			out.FinalizedAt = &t
		}
		if msg := jobs.LastError(j, lastErrorMaxRunes); msg != "" {
			out.LastError = &msg
		}
		resp.Data = append(resp.Data, out)
	}
	if page.NextCursor != "" {
		resp.NextCursor = &page.NextCursor
	}
	return resp, nil
}
