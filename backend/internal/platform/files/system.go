package files

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	filesstore "pdpa-platform/internal/platform/files/store"
)

// The functions below are for trusted platform code (jobs, services that did their own authorization),
// not for request handlers: they skip the uploader / entity-permission visibility rule of Get and
// DownloadURL. Row-level security still scopes them to the transaction's tenant, and none of them hands
// out bytes that haven't passed the virus scan.

// ErrNotClean: the file hasn't passed the scan (yet).
var ErrNotClean = errors.New("files: file has not passed the virus scan")

// Status returns a file's scan status ("" if it doesn't exist in the tenant).
func (s *Service) Status(ctx context.Context, id uuid.UUID) (string, error) {
	row, err := filesstore.New(pdb.MustTxFromContext(ctx)).GetFile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return row.AvStatus, nil
}

// Open streams a clean file's content (e.g. an import file read by its job).
func (s *Service) Open(ctx context.Context, id uuid.UUID) (io.ReadCloser, File, error) {
	row, err := filesstore.New(pdb.MustTxFromContext(ctx)).GetFile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, File{}, ErrNotFound
	}
	if err != nil {
		return nil, File{}, err
	}
	if row.AvStatus != "clean" {
		return nil, File{}, ErrNotClean
	}
	r, err := s.Store.Get(ctx, row.ObjectKey)
	if err != nil {
		return nil, File{}, fmt.Errorf("files: open: %w", err)
	}
	return r, toFile(row), nil
}

// SignedURL presigns a download of a clean file for a caller that has already been authorized for it.
func (s *Service) SignedURL(ctx context.Context, id uuid.UUID) (string, error) {
	row, err := filesstore.New(pdb.MustTxFromContext(ctx)).GetFile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	switch row.AvStatus {
	case "clean":
	case "pending":
		return "", ErrScanPending
	default:
		return "", ErrNotClean
	}
	return s.Store.PresignGet(ctx, row.ObjectKey, row.FileName, s.Config.DownloadTTL)
}

// SaveGenerated stores a file the platform produced (an import error report, an export) and attaches it
// to its record at once, so it is never an orphan. It is scanned like any upload.
func (s *Service) SaveGenerated(ctx context.Context, fileName string, r io.Reader, entityType string, entityID uuid.UUID) (File, error) {
	f, err := s.Upload(ctx, fileName, r)
	if err != nil {
		return File{}, err
	}
	if err := s.AttachSystem(ctx, f.ID, entityType, entityID); err != nil {
		return File{}, err
	}
	f.EntityType, f.EntityID = entityType, &entityID
	return f, nil
}

// DeleteObject removes a clean file's bytes from storage while keeping its platform.files row and
// metadata (the SHA-256, the entity it was attached to) as the record that something was deleted and
// when — the same "object deleted, row + audit kept" pattern the scanner already uses for an infected
// upload. The caller owns deciding when deletion is appropriate (e.g. DSAR-14's own expiry sweep) and
// writing its own audit entry; this only touches storage.
func (s *Service) DeleteObject(ctx context.Context, id uuid.UUID) error {
	row, err := filesstore.New(pdb.MustTxFromContext(ctx)).GetFile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := s.Store.Delete(ctx, row.ObjectKey); err != nil {
		return fmt.Errorf("files: delete object: %w", err)
	}
	return nil
}

// AttachSystem attaches an unattached file to a platform record (e.g. an import job) without the
// entity-type registry used for module records. The caller has authorized the user for the file.
func (s *Service) AttachSystem(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error {
	n, err := filesstore.New(pdb.MustTxFromContext(ctx)).AttachFile(ctx, filesstore.AttachFileParams{
		ID: id, EntityType: &entityType, EntityID: pgtype.UUID{Bytes: entityID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("files: attach: %w", err)
	}
	if n == 0 {
		return ErrAlreadyAttached
	}
	return nil
}
