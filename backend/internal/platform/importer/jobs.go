package importer

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/files"
	importerstore "pdpa-platform/internal/platform/importer/store"
	"pdpa-platform/internal/platform/jobs"
)

// Big files take a while; River's default one-minute job timeout would cut them off.
const jobTimeout = 30 * time.Minute

// PrepareArgs is import.prepare: wait for the virus scan, then read the header row.
type PrepareArgs struct {
	jobs.TenantArgs
	ImportID string `json:"import_id"`
}

func (PrepareArgs) Kind() string { return "import.prepare" }

// ValidateArgs is import.validate: the dry run over every row.
type ValidateArgs struct {
	jobs.TenantArgs
	ImportID string `json:"import_id"`
}

func (ValidateArgs) Kind() string { return "import.validate" }

// ApplyArgs is import.apply: import the valid rows, all or nothing.
type ApplyArgs struct {
	jobs.TenantArgs
	ImportID string `json:"import_id"`
}

func (ApplyArgs) Kind() string { return "import.apply" }

// Preparer works import.prepare.
type Preparer struct {
	river.WorkerDefaults[PrepareArgs]
	Service *Service
}

func (w *Preparer) Timeout(*river.Job[PrepareArgs]) time.Duration { return jobTimeout }

func (w *Preparer) Work(ctx context.Context, job *river.Job[PrepareArgs]) error {
	row, err := lock(ctx, job.Args.ImportID)
	if err != nil || row.Status != "queued" || len(parseMapping(row.Mapping).Headers) > 0 {
		return err
	}
	switch status, err := w.Service.Files.Status(ctx, row.FileID); {
	case err != nil:
		return err
	case status == "pending":
		return river.JobSnooze(3 * time.Second) // the scan hasn't finished
	case status != "clean":
		return w.fail(ctx, row, "file_rejected")
	}
	rr, err := w.open(ctx, row)
	if err != nil {
		return w.fail(ctx, row, "file_unreadable")
	}
	defer rr.Close()
	header, err := rr.Next()
	if err != nil || len(nonEmpty(header)) == 0 {
		return w.fail(ctx, row, "no_header_row")
	}
	headers := cleanHeaders(header)
	doc := parseMapping(row.Mapping)
	doc.Headers = headers
	doc.Suggested = w.Service.Types[row.ImportType].suggestMapping(headers)
	_, err = w.Service.update(ctx, row, "queued", doc, true, nil, nil, nil, pgtype.UUID{})
	return err
}

func (w *Preparer) open(ctx context.Context, row importerstore.GetImportJobRow) (rowReader, error) {
	return openFile(ctx, w.Service.Files, row.FileID)
}

func (w *Preparer) fail(ctx context.Context, row importerstore.GetImportJobRow, reason string) error {
	return failJob(ctx, w.Service, row, reason)
}

// Validator works import.validate: every row is checked (required columns, then the type's Validate),
// nothing is written; the counts and a CSV error report (line, column, reason — never the cell value)
// are stored on the import, which becomes ready.
type Validator struct {
	river.WorkerDefaults[ValidateArgs]
	Service *Service
}

func (w *Validator) Timeout(*river.Job[ValidateArgs]) time.Duration { return jobTimeout }

func (w *Validator) Work(ctx context.Context, job *river.Job[ValidateArgs]) error {
	row, err := lock(ctx, job.Args.ImportID)
	if err != nil || row.Status != "validating" {
		return err
	}
	t := w.Service.Types[row.ImportType]
	doc := parseMapping(row.Mapping)

	var report bytes.Buffer
	report.Write([]byte{0xEF, 0xBB, 0xBF}) // so Excel opens the Thai text as UTF-8
	rep := csv.NewWriter(&report)
	_ = rep.Write([]string{"line", "column", "error"})
	var total, valid, invalid int32
	err = eachRow(ctx, w.Service.Files, row.FileID, doc.Columns, func(line int, r Row) error {
		total++
		errs := checkRow(ctx, t, line, r)
		if len(errs) == 0 {
			valid++
			return nil
		}
		invalid++
		for _, e := range errs {
			_ = rep.Write([]string{strconv.Itoa(line), e.Column, e.Message})
		}
		return nil
	})
	if errors.Is(err, ErrUnreadable) {
		return failJob(ctx, w.Service, row, "file_unreadable")
	}
	if err != nil {
		return err
	}
	rep.Flush()

	var errorFile pgtype.UUID
	if invalid > 0 {
		f, err := w.Service.Files.SaveGenerated(ctx, fmt.Sprintf("import-errors-%s.csv", row.ID.String()[:8]), &report, EntityType, row.ID)
		if err != nil {
			return err
		}
		errorFile = pgtype.UUID{Bytes: f.ID, Valid: true}
	}
	_, err = w.Service.update(ctx, row, "ready", doc, true, &total, &valid, &invalid, errorFile)
	return err
}

// Applier works import.apply: the rows that pass validation (re-checked now) are applied in one savepoint;
// if any fails, the savepoint is rolled back — nothing of the import remains — and the import fails.
type Applier struct {
	river.WorkerDefaults[ApplyArgs]
	Service *Service
}

func (w *Applier) Timeout(*river.Job[ApplyArgs]) time.Duration { return jobTimeout }

func (w *Applier) Work(ctx context.Context, job *river.Job[ApplyArgs]) error {
	row, err := lock(ctx, job.Args.ImportID)
	if err != nil || row.Status != "importing" {
		return err
	}
	t := w.Service.Types[row.ImportType]
	doc := parseMapping(row.Mapping)
	var applied int32
	failedLine := 0
	applyErr := pdb.Savepoint(ctx, func(ctx context.Context) error {
		return eachRow(ctx, w.Service.Files, row.FileID, doc.Columns, func(line int, r Row) error {
			if len(checkRow(ctx, t, line, r)) > 0 {
				return nil // reported during validation; not imported
			}
			if err := t.Apply(ctx, line, r); err != nil {
				failedLine = line
				return err
			}
			applied++
			return nil
		})
	})
	g := authz.Grants{TenantID: job.Args.TenantID}
	if applyErr != nil {
		reason := "apply_failed"
		if failedLine > 0 {
			reason = fmt.Sprintf("apply_failed_line_%d", failedLine)
		}
		if err := failJob(ctx, w.Service, row, reason); err != nil {
			return err
		}
		return w.Service.audit(ctx, g, "platform.import.apply", row.ID, map[string]any{"status": "importing"}, map[string]any{"status": "failed", "reason": reason})
	}
	if _, err := w.Service.update(ctx, row, "done", doc, false, row.TotalRows, &applied, row.ErrorRows, row.ErrorFileID); err != nil {
		return err
	}
	return w.Service.audit(ctx, g, "platform.import.apply", row.ID, map[string]any{"status": "importing"}, map[string]any{"status": "done", "rows": applied})
}

func lock(ctx context.Context, id string) (importerstore.GetImportJobRow, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return importerstore.GetImportJobRow{}, river.JobCancel(err)
	}
	row, err := importerstore.New(pdb.MustTxFromContext(ctx)).LockImportJob(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return importerstore.GetImportJobRow{}, river.JobCancel(ErrNotFound)
	}
	return importerstore.GetImportJobRow(row), err
}

func failJob(ctx context.Context, s *Service, row importerstore.GetImportJobRow, reason string) error {
	doc := parseMapping(row.Mapping)
	doc.Failure = reason
	_, err := s.update(ctx, row, "failed", doc, row.DryRun, row.TotalRows, row.SuccessRows, row.ErrorRows, row.ErrorFileID)
	return err
}

func openFile(ctx context.Context, fs *files.Service, fileID uuid.UUID) (rowReader, error) {
	r, f, err := fs.Open(ctx, fileID)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return openRows(r, f.FileName)
}

// eachRow reads the file and calls fn for every non-empty data row, mapped to column keys.
func eachRow(ctx context.Context, fs *files.Service, fileID uuid.UUID, mapping map[string]string, fn func(line int, r Row) error) error {
	rr, err := openFile(ctx, fs, fileID)
	if err != nil {
		if errors.Is(err, files.ErrNotClean) || errors.Is(err, files.ErrNotFound) {
			return ErrUnreadable
		}
		return err
	}
	defer rr.Close()
	header, err := rr.Next()
	if err != nil {
		return ErrUnreadable
	}
	index := map[string]int{}
	for i, h := range cleanHeaders(header) {
		index[h] = i
	}
	for line := 2; ; line++ {
		rec, err := rr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if len(nonEmpty(rec)) == 0 {
			continue
		}
		r := Row{}
		for key, h := range mapping {
			if i, ok := index[h]; ok && i < len(rec) {
				r[key] = strings.TrimSpace(rec[i])
			} else {
				r[key] = ""
			}
		}
		if err := fn(line, r); err != nil {
			return err
		}
	}
}

func checkRow(ctx context.Context, t Type, line int, r Row) []FieldError {
	var errs []FieldError
	for _, c := range t.Columns {
		if c.Required && r[c.Key] == "" {
			errs = append(errs, FieldError{Column: c.Key, Message: "required"})
		}
	}
	if len(errs) > 0 {
		return errs
	}
	if t.Validate != nil {
		errs = t.Validate(ctx, line, r)
	}
	return errs
}

func cleanHeaders(h []string) []string {
	out := make([]string, len(h))
	for i, v := range h {
		out[i] = strings.TrimSpace(strings.TrimPrefix(v, "\uFEFF"))
	}
	return out
}

func nonEmpty(rec []string) []string {
	var out []string
	for _, v := range rec {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}
