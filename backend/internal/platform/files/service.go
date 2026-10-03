package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	filesstore "pdpa-platform/internal/platform/files/store"
	"pdpa-platform/internal/platform/jobs"
)

// Upload / download errors; the HTTP layer maps each to a problem code.
var (
	ErrInvalidName     = errors.New("files: invalid file name")
	ErrEmpty           = errors.New("files: empty file")
	ErrTooLarge        = errors.New("files: file too large")
	ErrTypeNotAllowed  = errors.New("files: file type not allowed")
	ErrNotFound        = errors.New("files: not found")
	ErrScanPending     = errors.New("files: virus scan not finished")
	ErrInfected        = errors.New("files: file is infected")
	ErrScanFailed      = errors.New("files: virus scan failed")
	ErrUnknownEntity   = errors.New("files: entity type not registered")
	ErrAlreadyAttached = errors.New("files: file already attached or not visible")
)

// File is the metadata of a stored file.
type File struct {
	ID         uuid.UUID
	FileName   string
	MimeType   string
	SizeBytes  int64
	SHA256     string
	AVStatus   string
	EntityType string
	EntityID   *uuid.UUID
	CreatedAt  time.Time
}

// Service is the file store of one process. EntityPermissions maps an entity type a module attaches
// files to (e.g. "dsar_package") to the permission a user needs to download them; a module registers
// its entry when it starts attaching files. An attached file whose type isn't registered is never
// downloadable (deny by default).
type Service struct {
	Store             ObjectStore
	River             *river.Client[pgx.Tx]
	Config            Config
	EntityPermissions map[string]string
	Now               func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Upload validates and stores one file for the tenant of the transaction in ctx and queues its virus
// scan and its orphan expiry. The object key carries no part of the file name (names can be personal
// data); the name is kept only in platform.files.
func (s *Service) Upload(ctx context.Context, fileName string, r io.Reader) (File, error) {
	name := cleanFileName(fileName)
	if name == "" {
		return File{}, ErrInvalidName
	}

	// Spool to a temp file: size and type are checked before anything reaches object storage, and the
	// upload then has a known length.
	tmp, err := os.CreateTemp("", "pdpa-upload-*")
	if err != nil {
		return File{}, err
	}
	defer func() { tmp.Close(); os.Remove(tmp.Name()) }()
	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(r, s.Config.MaxBytes+1))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return File{}, ErrTooLarge // the request body cap (cmd/api) tripped before our own limit
	}
	if err != nil {
		return File{}, fmt.Errorf("files: read upload: %w", err)
	}
	switch {
	case n == 0:
		return File{}, ErrEmpty
	case n > s.Config.MaxBytes:
		return File{}, ErrTooLarge
	}
	head := make([]byte, 512)
	m, _ := tmp.ReadAt(head, 0)
	contentType := s.Config.contentTypeFor(name, head[:m])
	if contentType == "" {
		return File{}, ErrTypeNotAllowed
	}

	tx := pdb.MustTxFromContext(ctx)
	var tenant string
	if err := tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&tenant); err != nil {
		return File{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return File{}, err
	}
	now := s.now().UTC()
	key := fmt.Sprintf("%s/%s/%s", tenant, now.Format("2006/01"), id)

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return File{}, err
	}
	if err := s.Store.Put(ctx, key, tmp, n, contentType); err != nil {
		return File{}, fmt.Errorf("files: store object: %w", err)
	}

	var encKey *string
	if st, ok := s.Store.(*S3Store); ok && st.cfg.SSE {
		v := "sse-s3"
		encKey = &v
	}
	expires := now.Add(s.Config.OrphanTTL)
	sum := hex.EncodeToString(hasher.Sum(nil))
	created, err := filesstore.New(tx).InsertFile(ctx, filesstore.InsertFileParams{
		ID: id, Bucket: s.Store.Bucket(), ObjectKey: key, FileName: name, MimeType: contentType,
		SizeBytes: n, Sha256: sum, EncryptionKeyID: encKey,
		RetentionUntil: pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err != nil {
		_ = s.Store.Delete(context.WithoutCancel(ctx), key)
		return File{}, fmt.Errorf("files: record file: %w", err)
	}

	tenantArgs := jobs.TenantArgs{TenantID: tenant}
	if _, err := jobs.Enqueue(ctx, s.River, ScanArgs{TenantArgs: tenantArgs, FileID: id.String()}, nil); err != nil {
		return File{}, err
	}
	if _, err := jobs.Enqueue(ctx, s.River, ExpireArgs{TenantArgs: tenantArgs, FileID: id.String()},
		&river.InsertOpts{ScheduledAt: expires}); err != nil {
		return File{}, err
	}

	return File{ID: id, FileName: name, MimeType: contentType, SizeBytes: n, SHA256: sum, AVStatus: "pending", CreatedAt: created.Time}, nil
}

// Get returns a file's metadata if the caller may see it (see DownloadURL).
func (s *Service) Get(ctx context.Context, id uuid.UUID) (File, error) {
	row, err := s.visible(ctx, id)
	if err != nil {
		return File{}, err
	}
	return toFile(row), nil
}

// DownloadURL returns a pre-signed URL valid for Config.DownloadTTL, for a clean file the caller may
// see: its uploader while it is unattached; once attached, whoever holds the permission registered
// for its entity type. Anything else is ErrNotFound, so ids of other people's files reveal nothing.
func (s *Service) DownloadURL(ctx context.Context, id uuid.UUID) (string, error) {
	row, err := s.visible(ctx, id)
	if err != nil {
		return "", err
	}
	switch row.AvStatus {
	case "clean":
	case "pending":
		return "", ErrScanPending
	case "infected":
		return "", ErrInfected
	default:
		return "", ErrScanFailed
	}
	u, err := s.Store.PresignGet(ctx, row.ObjectKey, row.FileName, s.Config.DownloadTTL)
	if err != nil {
		return "", fmt.Errorf("files: presign: %w", err)
	}
	return u, nil
}

// Attach links an uploaded file to a record of entityType (registered in EntityPermissions). Called by
// the owning module's service after its own authorization, in the same transaction as the record.
// Attaching clears the orphan deadline; retention of attached files follows the record's.
func (s *Service) Attach(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error {
	if _, ok := s.EntityPermissions[entityType]; !ok {
		return ErrUnknownEntity
	}
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

func (s *Service) visible(ctx context.Context, id uuid.UUID) (filesstore.GetFileRow, error) {
	row, err := filesstore.New(pdb.MustTxFromContext(ctx)).GetFile(ctx, id) // RLS: other tenants → no rows
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("files: read: %w", err)
	}
	grants, _ := authz.FromContext(ctx)
	if row.EntityType == nil {
		if !row.CreatedBy.Valid || uuid.UUID(row.CreatedBy.Bytes).String() != grants.UserID {
			return row, ErrNotFound
		}
		return row, nil
	}
	perm, ok := s.EntityPermissions[*row.EntityType]
	if !ok || !grants.Has(perm) {
		return row, ErrNotFound
	}
	return row, nil
}

func toFile(row filesstore.GetFileRow) File {
	f := File{
		ID: row.ID, FileName: row.FileName, MimeType: row.MimeType, SizeBytes: row.SizeBytes,
		SHA256: row.Sha256, AVStatus: row.AvStatus, CreatedAt: row.CreatedAt.Time,
	}
	if row.EntityType != nil {
		f.EntityType = *row.EntityType
	}
	if row.EntityID.Valid {
		id := uuid.UUID(row.EntityID.Bytes)
		f.EntityID = &id
	}
	return f
}
