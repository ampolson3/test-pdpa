package files

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	filesstore "pdpa-platform/internal/platform/files/store"
	"pdpa-platform/internal/platform/jobs"
)

// ScanArgs is files.scan for one uploaded file.
type ScanArgs struct {
	jobs.TenantArgs
	FileID string `json:"file_id"`
}

func (ScanArgs) Kind() string { return "files.scan" }

// ScanMaxAttempts: a file whose scan keeps failing (clamd down, object unreadable) ends in av_status
// error after this many tries instead of retrying for weeks.
const ScanMaxAttempts = 10

func (ScanArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: ScanMaxAttempts} }

// Scanner works files.scan inside the tenant transaction: pending → clean | infected (state machine
// PLT-09 in docs/states/state-machines.yaml), writing the status and an audit entry together.
type Scanner struct {
	river.WorkerDefaults[ScanArgs]

	Store  ObjectStore
	AV     AVScanner
	Audit  *audit.Service
	Logger *slog.Logger
}

func (w *Scanner) Work(ctx context.Context, job *river.Job[ScanArgs]) error {
	id, err := uuid.Parse(job.Args.FileID)
	if err != nil {
		return river.JobCancel(err)
	}
	q := filesstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.LockFileForScan(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // expired and deleted before its scan ran
	}
	if err != nil {
		return err
	}
	if row.AvStatus != "pending" {
		return nil // already decided; a duplicate or retried job
	}

	verdict, scanErr := w.scan(ctx, row.ObjectKey)
	if scanErr != nil {
		if job.Attempt < job.MaxAttempts {
			return scanErr // retried by River; AlertHandler logs it
		}
		w.logger().ErrorContext(ctx, "file scan failed permanently", "alert", "file_scan_failed",
			"file_id", id.String(), "tenant_id", job.Args.TenantID, "error", scanErr.Error())
		return w.transition(ctx, q, job, id, "error", nil)
	}
	if !verdict.Infected {
		return w.transition(ctx, q, job, id, "clean", nil)
	}

	// Infected: the bytes go, the row stays as the record that something was rejected.
	if err := w.Store.Delete(ctx, row.ObjectKey); err != nil {
		return fmt.Errorf("files.scan: delete infected object: %w", err)
	}
	w.logger().WarnContext(ctx, "infected upload rejected", "alert", "file_infected",
		"file_id", id.String(), "tenant_id", job.Args.TenantID, "signature", verdict.Signature)
	return w.transition(ctx, q, job, id, "infected", map[string]any{"signature": verdict.Signature})
}

func (w *Scanner) scan(ctx context.Context, key string) (Verdict, error) {
	obj, err := w.Store.Get(ctx, key)
	if err != nil {
		return Verdict{}, fmt.Errorf("files.scan: open object: %w", err)
	}
	defer obj.Close()
	return w.AV.Scan(ctx, obj)
}

func (w *Scanner) transition(ctx context.Context, q *filesstore.Queries, job *river.Job[ScanArgs], id uuid.UUID, to string, extra map[string]any) error {
	if err := q.SetFileAVStatus(ctx, filesstore.SetFileAVStatusParams{ID: id, AvStatus: to}); err != nil {
		return err
	}
	after := map[string]any{"av_status": to}
	for k, v := range extra {
		after[k] = v
	}
	tenantID, _ := uuid.Parse(job.Args.TenantID)
	return w.Audit.Write(ctx, audit.Entry{
		TenantID: tenantID, ActorType: "system", Action: "platform.file.scan",
		EntityType: "file", EntityID: &id,
		Before: map[string]any{"av_status": "pending"}, After: after,
	})
}

func (w *Scanner) logger() *slog.Logger {
	if w.Logger != nil {
		return w.Logger
	}
	return slog.Default()
}

// ExpireArgs is files.expire, scheduled at upload for the end of the orphan TTL.
type ExpireArgs struct {
	jobs.TenantArgs
	FileID string `json:"file_id"`
}

func (ExpireArgs) Kind() string { return "files.expire" }

// Expirer deletes an upload that was never attached to a record ("อายุไฟล์"): the object first, then
// the row. Attached files are left alone (their retention follows the record's).
type Expirer struct {
	river.WorkerDefaults[ExpireArgs]

	Store ObjectStore
}

func (w *Expirer) Work(ctx context.Context, job *river.Job[ExpireArgs]) error {
	id, err := uuid.Parse(job.Args.FileID)
	if err != nil {
		return river.JobCancel(err)
	}
	q := filesstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.LockOrphanFile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // attached, already gone, or not yet due
	}
	if err != nil {
		return err
	}
	if err := w.Store.Delete(ctx, row.ObjectKey); err != nil {
		return fmt.Errorf("files.expire: delete object: %w", err)
	}
	return q.DeleteFile(ctx, id)
}
