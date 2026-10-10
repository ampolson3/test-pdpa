package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// AlertHandler is the worker's river.ErrorHandler — PLT-10's "job ที่ล้มเหลวมี … แจ้งเตือน".
// Every failed attempt is logged and counted; the attempt that exhausts MaxAttempts (or a
// cancellation) is logged at ERROR with alert=job_discarded and counted in pdpa.jobs.discarded,
// which is what the alert rules in docs/architecture/deployment.md fire on. It never changes the
// retry decision: River's retry policy and MaxAttempts stay in charge.
//
// Only the job's kind, id, queue, attempt and tenant go into logs and metrics — never its args or
// user data (CLAUDE.md rule 3). Workers must not put personal data into returned errors either,
// since River also persists them in river_job.errors.
type AlertHandler struct {
	Logger *slog.Logger

	failed    metric.Int64Counter
	discarded metric.Int64Counter
}

// NewAlertHandler registers its counters with the global OTel meter provider (a no-op until one is
// installed).
func NewAlertHandler(logger *slog.Logger) (*AlertHandler, error) {
	meter := otel.Meter("pdpa-platform/jobs")
	failed, err := meter.Int64Counter("pdpa.jobs.failed", metric.WithDescription("Failed job attempts, including ones that will be retried"))
	if err != nil {
		return nil, err
	}
	discarded, err := meter.Int64Counter("pdpa.jobs.discarded", metric.WithDescription("Jobs that will not be retried again (attempts exhausted or cancelled)"))
	if err != nil {
		return nil, err
	}
	return &AlertHandler{Logger: logger, failed: failed, discarded: discarded}, nil
}

// Final reports whether this failure ends the job: no attempts left, or the worker cancelled it.
func Final(job *rivertype.JobRow, err error) bool {
	var cancel *river.JobCancelError
	return job.Attempt >= job.MaxAttempts || errors.As(err, &cancel)
}

func (h *AlertHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	h.record(ctx, job, Final(job, err), slog.String("error", err.Error()))
	return nil
}

func (h *AlertHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, _ string) *river.ErrorHandlerResult {
	// The stack trace can include argument values, so it is not logged; the panic value's type is.
	h.record(ctx, job, job.Attempt >= job.MaxAttempts, slog.String("panic_type", fmt.Sprintf("%T", panicVal)))
	return nil
}

func (h *AlertHandler) record(ctx context.Context, job *rivertype.JobRow, final bool, detail slog.Attr) {
	attrs := metric.WithAttributes(attribute.String("kind", job.Kind), attribute.String("queue", job.Queue))
	h.failed.Add(ctx, 1, attrs)

	logAttrs := []any{
		slog.String("kind", job.Kind), slog.Int64("job_id", job.ID), slog.String("queue", job.Queue),
		slog.Int("attempt", job.Attempt), slog.Int("max_attempts", job.MaxAttempts),
		slog.String("tenant_id", tenantOf(job)), detail,
	}
	if final {
		h.discarded.Add(ctx, 1, attrs)
		h.Logger.ErrorContext(ctx, "job discarded", append(logAttrs, slog.String("alert", "job_discarded"))...)
		return
	}
	h.Logger.WarnContext(ctx, "job attempt failed; will retry", logAttrs...)
}

func tenantOf(job *rivertype.JobRow) string {
	var args TenantArgs
	_ = json.Unmarshal(job.EncodedArgs, &args)
	return args.TenantID
}
