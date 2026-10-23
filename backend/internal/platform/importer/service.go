package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/files"
	importerstore "pdpa-platform/internal/platform/importer/store"
	"pdpa-platform/internal/platform/jobs"
)

// EntityType is how import jobs appear to other platform services (their file, their error report, audit).
const EntityType = "import_job"

var (
	ErrUnknownType     = errors.New("importer: import type not registered")
	ErrForbidden       = errors.New("importer: not allowed")
	ErrNotFound        = errors.New("importer: not found")
	ErrInvalidMapping  = errors.New("importer: invalid column mapping")
	ErrInvalidState    = errors.New("importer: not possible in the import's current status")
	ErrVersionMismatch = errors.New("importer: version mismatch")
	ErrFileNotUsable   = errors.New("importer: file not found, not yours, or already used")
)

// mappingDoc is platform.import_jobs.mapping: what the framework learned about the file and what the user chose.
type mappingDoc struct {
	Headers   []string          `json:"headers,omitempty"`
	Suggested map[string]string `json:"suggested,omitempty"`
	Columns   map[string]string `json:"columns,omitempty"` // column key → header text
	Failure   string            `json:"failure,omitempty"` // stable reason when status is failed
}

// Job is an import as the API shows it.
type Job struct {
	ID             uuid.UUID
	Type           string
	FileID         uuid.UUID
	Status         string
	Headers        []string
	Suggested      map[string]string
	Mapping        map[string]string
	Columns        []Column
	TotalRows      *int32
	ValidRows      *int32
	ErrorRows      *int32
	HasErrorReport bool
	Failure        string
	RowVersion     int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Service runs imports for the tenant of the transaction in ctx.
type Service struct {
	Types Registry
	Files *files.Service
	River *river.Client[pgx.Tx]
	Audit *audit.Service
}

func (s *Service) authorize(ctx context.Context, importType string) (Type, authz.Grants, error) {
	t, ok := s.Types[importType]
	if !ok {
		return t, authz.Grants{}, ErrUnknownType
	}
	g, _ := authz.FromContext(ctx)
	if !g.Has(t.Permission) {
		return t, g, ErrForbidden
	}
	return t, g, nil
}

// Create starts an import of a file the caller uploaded (POST /admin/v1/platform/files) and queues its
// preparation: once the file has passed the virus scan, its header row is read and a mapping suggested.
func (s *Service) Create(ctx context.Context, importType string, fileID uuid.UUID) (Job, error) {
	t, g, err := s.authorize(ctx, importType)
	if err != nil {
		return Job{}, err
	}
	f, err := s.Files.Get(ctx, fileID) // only the uploader sees an unattached file
	if err != nil || f.EntityType != "" {
		return Job{}, ErrFileNotUsable
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Job{}, err
	}
	q := importerstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertImportJob(ctx, importerstore.InsertImportJobParams{ID: id, ImportType: importType, FileID: fileID})
	if err != nil {
		return Job{}, fmt.Errorf("importer: create: %w", err)
	}
	// The file now belongs to the import (and is kept past the orphan deadline).
	if err := s.Files.AttachSystem(ctx, fileID, EntityType, id); err != nil {
		return Job{}, err
	}
	if _, err := jobs.Enqueue(ctx, s.River, PrepareArgs{TenantArgs: jobs.TenantArgs{TenantID: g.TenantID}, ImportID: id.String()}, nil); err != nil {
		return Job{}, err
	}
	if err := s.audit(ctx, g, "platform.import.create", id, nil, map[string]any{"import_type": importType, "file_name": f.FileName}); err != nil {
		return Job{}, err
	}
	return toJob(importerstore.GetImportJobRow(row), t), nil
}

// Get returns an import the caller may see.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Job, error) {
	row, err := importerstore.New(pdb.MustTxFromContext(ctx)).GetImportJob(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	t, _, err := s.authorize(ctx, row.ImportType)
	if err != nil {
		return Job{}, ErrNotFound
	}
	return toJob(row, t), nil
}

// List returns recent imports of the types the caller may run (optionally one type).
func (s *Service) List(ctx context.Context, importType string) ([]Job, error) {
	g, _ := authz.FromContext(ctx)
	var types []string
	for name, t := range s.Types {
		if (importType == "" || importType == name) && g.Has(t.Permission) {
			types = append(types, name)
		}
	}
	sort.Strings(types)
	rows, err := importerstore.New(pdb.MustTxFromContext(ctx)).ListImportJobs(ctx, types)
	if err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(rows))
	for _, r := range rows {
		out = append(out, toJob(importerstore.GetImportJobRow(r), s.Types[r.ImportType]))
	}
	return out, nil
}

// SetMapping records which header feeds each column and (re)validates the whole file as a dry run.
// Allowed once the headers are known (queued) or after a validation (ready).
func (s *Service) SetMapping(ctx context.Context, id uuid.UUID, version int32, columns map[string]string) (Job, error) {
	row, t, g, err := s.load(ctx, id, version)
	if err != nil {
		return Job{}, err
	}
	doc := parseMapping(row.Mapping)
	if !((row.Status == "queued" && len(doc.Headers) > 0) || row.Status == "ready") {
		return Job{}, ErrInvalidState
	}
	if err := checkMapping(t, doc.Headers, columns); err != nil {
		return Job{}, err
	}
	doc.Columns = columns
	doc.Failure = ""
	updated, err := s.update(ctx, row, "validating", doc, true, nil, nil, nil, pgtype.UUID{})
	if err != nil {
		return Job{}, err
	}
	if _, err := jobs.Enqueue(ctx, s.River, ValidateArgs{TenantArgs: jobs.TenantArgs{TenantID: g.TenantID}, ImportID: id.String()}, nil); err != nil {
		return Job{}, err
	}
	if err := s.audit(ctx, g, "platform.import.map", id, map[string]any{"status": row.Status}, map[string]any{"status": "validating"}); err != nil {
		return Job{}, err
	}
	return toJob(importerstore.GetImportJobRow(updated), t), nil
}

// Confirm imports the rows that passed validation, all or nothing.
func (s *Service) Confirm(ctx context.Context, id uuid.UUID, version int32) (Job, error) {
	row, t, g, err := s.load(ctx, id, version)
	if err != nil {
		return Job{}, err
	}
	if row.Status != "ready" || row.SuccessRows == nil || *row.SuccessRows == 0 {
		return Job{}, ErrInvalidState
	}
	updated, err := s.update(ctx, row, "importing", parseMapping(row.Mapping), false, row.TotalRows, row.SuccessRows, row.ErrorRows, row.ErrorFileID)
	if err != nil {
		return Job{}, err
	}
	if _, err := jobs.Enqueue(ctx, s.River, ApplyArgs{TenantArgs: jobs.TenantArgs{TenantID: g.TenantID}, ImportID: id.String()}, nil); err != nil {
		return Job{}, err
	}
	if err := s.audit(ctx, g, "platform.import.confirm", id, map[string]any{"status": "ready"}, map[string]any{"status": "importing", "rows": *row.SuccessRows}); err != nil {
		return Job{}, err
	}
	return toJob(importerstore.GetImportJobRow(updated), t), nil
}

// ErrorReportURL presigns the error report of an import the caller may see.
func (s *Service) ErrorReportURL(ctx context.Context, id uuid.UUID) (string, error) {
	j, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	row, err := importerstore.New(pdb.MustTxFromContext(ctx)).GetImportJob(ctx, j.ID)
	if err != nil {
		return "", err
	}
	if !row.ErrorFileID.Valid {
		return "", ErrNotFound
	}
	return s.Files.SignedURL(ctx, uuid.UUID(row.ErrorFileID.Bytes))
}

func (s *Service) load(ctx context.Context, id uuid.UUID, version int32) (importerstore.GetImportJobRow, Type, authz.Grants, error) {
	row, err := importerstore.New(pdb.MustTxFromContext(ctx)).LockImportJob(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return importerstore.GetImportJobRow{}, Type{}, authz.Grants{}, ErrNotFound
	}
	if err != nil {
		return importerstore.GetImportJobRow{}, Type{}, authz.Grants{}, err
	}
	t, g, err := s.authorize(ctx, row.ImportType)
	if err != nil {
		return importerstore.GetImportJobRow{}, t, g, ErrNotFound
	}
	if row.RowVersion != version {
		return importerstore.GetImportJobRow{}, t, g, ErrVersionMismatch
	}
	return importerstore.GetImportJobRow(row), t, g, nil
}

func (s *Service) update(ctx context.Context, row importerstore.GetImportJobRow, status string, doc mappingDoc, dryRun bool,
	total, valid, errs *int32, errorFile pgtype.UUID) (importerstore.UpdateImportJobRow, error) {
	raw, _ := json.Marshal(doc)
	updated, err := importerstore.New(pdb.MustTxFromContext(ctx)).UpdateImportJob(ctx, importerstore.UpdateImportJobParams{
		ID: row.ID, RowVersion: row.RowVersion, Status: status, Mapping: raw, DryRun: dryRun,
		TotalRows: total, SuccessRows: valid, ErrorRows: errs, ErrorFileID: errorFile,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return updated, ErrVersionMismatch
	}
	return updated, err
}

func (s *Service) audit(ctx context.Context, g authz.Grants, action string, id uuid.UUID, before, after map[string]any) error {
	if s.Audit == nil {
		return nil
	}
	tenant, _ := uuid.Parse(g.TenantID)
	e := audit.Entry{TenantID: tenant, ActorType: "user", Action: action, EntityType: EntityType, EntityID: &id}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorID = &actor
	} else {
		e.ActorType = "system"
	}
	if before != nil {
		e.Before = before
	}
	if after != nil {
		e.After = after
	}
	return s.Audit.Write(ctx, e)
}

// checkMapping: every key is a column of the type, every header exists in the file, every required
// column is mapped, and no header feeds two columns.
func checkMapping(t Type, headers []string, columns map[string]string) error {
	have := map[string]bool{}
	for _, h := range headers {
		have[h] = true
	}
	known := map[string]Column{}
	for _, c := range t.Columns {
		known[c.Key] = c
	}
	used := map[string]string{}
	for key, header := range columns {
		if _, ok := known[key]; !ok {
			return fmt.Errorf("%w: unknown column %q", ErrInvalidMapping, key)
		}
		if header == "" {
			continue
		}
		if !have[header] {
			return fmt.Errorf("%w: no header %q in the file", ErrInvalidMapping, header)
		}
		if other, dup := used[header]; dup {
			return fmt.Errorf("%w: header %q mapped to both %s and %s", ErrInvalidMapping, header, other, key)
		}
		used[header] = key
	}
	for _, c := range t.Columns {
		if c.Required && columns[c.Key] == "" {
			return fmt.Errorf("%w: required column %s is not mapped", ErrInvalidMapping, c.Key)
		}
	}
	return nil
}

func parseMapping(raw []byte) mappingDoc {
	var d mappingDoc
	_ = json.Unmarshal(raw, &d)
	return d
}

func toJob(r importerstore.GetImportJobRow, t Type) Job {
	d := parseMapping(r.Mapping)
	return Job{
		ID: r.ID, Type: r.ImportType, FileID: r.FileID, Status: r.Status,
		Headers: d.Headers, Suggested: d.Suggested, Mapping: d.Columns, Columns: t.Columns,
		TotalRows: r.TotalRows, ValidRows: r.SuccessRows, ErrorRows: r.ErrorRows, HasErrorReport: r.ErrorFileID.Valid,
		Failure: d.Failure, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}
