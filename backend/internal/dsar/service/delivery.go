package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	dsarstore "pdpa-platform/internal/dsar/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/publickeys"
)

// DSAR-14 (ม.37(1), ส่งข้อมูลให้เจ้าของข้อมูลอย่างปลอดภัย): wraps an already-generated deliverable file —
// the response letter PLT-16 composes (DSAR-13), DSAR-15's own portability export, or any file already
// attached to the request — in a time-limited, logged download. The link itself is a platform.public_keys
// key (EntityDsarPackage) naming the package, resolved before the tenant transaction opens the same way
// every other /public/v1 surface already works; a separately-held access code — the module doc's own
// "รหัสผ่าน" — gates the actual download when the package is password_protected, hashed into
// dsar.packages.token_hash (never stored in the clear, rule 3) and shown to the caller exactly once, for
// them to share with the data subject through a different channel than the link itself.

// packageFormats mirrors dsar.packages.format's own CHECK constraint.
var packageFormats = []string{"pdf", "csv", "json", "xml", "zip"}

// DefaultPackageTTL is how long a link lives when the caller doesn't set one — a tunable default
// (docs/decisions.md has no entry for it: a configurable value, not a legally-mandated one), not the
// 30-day DSAR response deadline itself.
const DefaultPackageTTL = 7 * 24 * time.Hour

// Package is one dsar.packages row — a secure delivery of a file already attached to the request.
type Package struct {
	ID                uuid.UUID
	RequestID         uuid.UUID
	FileID            uuid.UUID
	Format            string
	PasswordProtected bool
	ExpiresAt         time.Time
	DownloadCount     int32
	LastDownloadedAt  *time.Time
	PurgedAt          *time.Time
	RowVersion        int32
	CreatedAt         time.Time
	PublicKey         string // only set by CreatePackage, on the response to the request that issued it
}

func toPackage(r dsarstore.GetPackageRow) Package {
	p := Package{ID: r.ID, RequestID: r.RequestID, FileID: r.FileID, Format: r.Format, PasswordProtected: r.PasswordProtected,
		ExpiresAt: r.ExpiresAt.Time.UTC(), DownloadCount: r.DownloadCount, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time.UTC()}
	if r.LastDownloadedAt.Valid {
		t := r.LastDownloadedAt.Time.UTC()
		p.LastDownloadedAt = &t
	}
	if r.PurgedAt.Valid {
		t := r.PurgedAt.Time.UTC()
		p.PurgedAt = &t
	}
	return p
}

func randomAccessCode() (string, error) {
	// 10 characters from an unambiguous alphabet (no 0/O/1/I/L), meant to be read aloud or typed by hand —
	// a "password" shared with the data subject over a different channel than the link itself.
	const alphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 10)
	for i, v := range b {
		out[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(out), nil
}

func hashAccessCode(code string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return hex.EncodeToString(sum[:])
}

// CreatePackageInput is what staff give to package a file for delivery — their own clean, still-unattached
// upload (attached to the request here), or one already attached to it.
type CreatePackageInput struct {
	FileID            uuid.UUID
	Format            string
	PasswordProtected bool
	ExpiresAt         *time.Time // optional; defaults to now()+DefaultPackageTTL
}

// CreatePackage is the acceptance criterion's own starting point: the file is either the caller's own
// clean, still-unattached PLT-09 upload — attached to the request here, the same "record + attach in one
// call" shape DSAR-04's own RecordAgent already uses — or a file already attached to this request (DSAR-13's
// generated letter, or anything DSAR-17's own collaboration panel attached). Never an arbitrary file id.
// Returns the access code in the clear exactly once — the caller must relay it, this call never stores it.
func (s *Service) CreatePackage(ctx context.Context, requestID uuid.UUID, in CreatePackageInput) (Package, string, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("dsar.package.create") {
		return Package{}, "", ErrForbidden
	}
	if !slices.Contains(packageFormats, in.Format) {
		return Package{}, "", fmt.Errorf("%w: format", ErrInvalid)
	}
	if s.Files == nil {
		return Package{}, "", fmt.Errorf("dsar: file storage not wired")
	}
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return Package{}, "", err
	}
	f, err := s.Files.Get(ctx, in.FileID)
	if err != nil || f.AVStatus != "clean" {
		return Package{}, "", fmt.Errorf("%w: file_id", ErrInvalid)
	}
	switch {
	case f.EntityType == "":
		if err := s.Files.Attach(ctx, in.FileID, "dsar_request", requestID); err != nil {
			return Package{}, "", fmt.Errorf("%w: file_id", ErrInvalid)
		}
	case f.EntityType == "dsar_request" && f.EntityID != nil && *f.EntityID == requestID:
		// already attached to this request — nothing to do
	default:
		return Package{}, "", fmt.Errorf("%w: file_id", ErrInvalid)
	}
	return s.createPackage(ctx, requestID, in.FileID, in.Format, in.PasswordProtected, in.ExpiresAt)
}

// createPackage is the shared core of CreatePackage (a caller's own file, checked clean above) and
// DSAR-15's GeneratePortabilityExport (a file this package itself just generated, trusted by construction —
// never checked for a clean AV status, the same distinction PLT-09's own trusted system operations already
// draw against the ordinary upload path).
func (s *Service) createPackage(ctx context.Context, requestID, fileID uuid.UUID, format string, passwordProtected bool, expiresAt *time.Time) (Package, string, error) {
	expires := s.now().Add(DefaultPackageTTL)
	if expiresAt != nil {
		expires = expiresAt.UTC()
	}
	if !expires.After(s.now()) {
		return Package{}, "", fmt.Errorf("%w: expires_at", ErrInvalid)
	}

	code, err := randomAccessCode()
	if err != nil {
		return Package{}, "", err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Package{}, "", err
	}
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertPackage(ctx, dsarstore.InsertPackageParams{ID: id, RequestID: requestID, FileID: fileID, Format: format,
		TokenHash: hashAccessCode(code), PasswordProtected: passwordProtected, ExpiresAt: pgTimestamptz(&expires)})
	if err != nil {
		return Package{}, "", err
	}
	key, err := publickeys.Issue(ctx, publickeys.EntityDsarPackage, id, nil)
	if err != nil {
		return Package{}, "", err
	}
	if err := s.schedulePurge(ctx, id, expires); err != nil {
		return Package{}, "", err
	}
	pkg := toPackage(dsarstore.GetPackageRow(row))
	pkg.PublicKey = key
	if err := s.audit(ctx, "dsar.package.create", requestID, nil, map[string]any{
		"package_id": id, "file_id": fileID, "format": format, "password_protected": passwordProtected, "expires_at": expires}); err != nil {
		return Package{}, "", err
	}
	return pkg, code, nil
}

// ListPackages returns a request's delivery packages, newest first.
func (s *Service) ListPackages(ctx context.Context, requestID uuid.UUID) ([]Package, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("dsar.package.read") {
		return nil, ErrForbidden
	}
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return nil, err
	}
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListPackagesForRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	out := make([]Package, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPackage(dsarstore.GetPackageRow(r)))
	}
	return out, nil
}

func liveOrNotFound(p Package, now time.Time) error {
	if p.PurgedAt != nil || !p.ExpiresAt.After(now) {
		return ErrNotFound
	}
	return nil
}

// PublicPackage is what the portal's own download page needs before any code is checked: whether one is
// required at all. A link past its expiry (the acceptance criterion's own "ลิงก์หมดอายุตามที่ตั้ง") or
// already purged reads exactly like an unknown one — no information is leaked either way.
func (s *Service) PublicPackage(ctx context.Context, id uuid.UUID) (Package, error) {
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).GetPackage(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Package{}, ErrNotFound
	}
	if err != nil {
		return Package{}, err
	}
	p := toPackage(row)
	if err := liveOrNotFound(p, s.now()); err != nil {
		return Package{}, err
	}
	return p, nil
}

// PublicDownload is the acceptance criterion itself: checks the access code when one is required, hands
// back a short-lived signed URL for the underlying file, and records the download — download_count,
// last_downloaded_at, and an audit log entry (the module doc's own "บันทึกการดาวน์โหลด").
func (s *Service) PublicDownload(ctx context.Context, id uuid.UUID, code string) (string, error) {
	if s.Files == nil {
		return "", fmt.Errorf("dsar: file storage not wired")
	}
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.LockPackage(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	p := toPackage(dsarstore.GetPackageRow(row))
	if err := liveOrNotFound(p, s.now()); err != nil {
		return "", err
	}
	if p.PasswordProtected {
		if subtle.ConstantTimeCompare([]byte(hashAccessCode(code)), []byte(row.TokenHash)) != 1 {
			return "", fmt.Errorf("%w: code", ErrInvalid)
		}
	}
	url, err := s.Files.SignedURL(ctx, p.FileID)
	if err != nil {
		return "", err
	}
	if err := q.RecordPackageDownload(ctx, id); err != nil {
		return "", err
	}
	if err := s.audit(ctx, "dsar.package.download", p.RequestID, nil, map[string]any{"package_id": id}); err != nil {
		return "", err
	}
	return url, nil
}

// PackagePurgeArgs is dsar.package_purge: the one checkpoint of a package's own expiry, scheduled exactly
// at its expires_at when the package is created.
type PackagePurgeArgs struct {
	jobs.TenantArgs
	PackageID string `json:"package_id"`
}

func (PackagePurgeArgs) Kind() string { return "dsar.package_purge" }

func (s *Service) schedulePurge(ctx context.Context, packageID uuid.UUID, at time.Time) error {
	if s.River == nil {
		return nil
	}
	tenant, err := tenantID(ctx)
	if err != nil {
		return err
	}
	_, err = jobs.Enqueue(ctx, s.River, PackagePurgeArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, PackageID: packageID.String()},
		&river.InsertOpts{ScheduledAt: at, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}

// PurgeExpiredPackage runs one scheduled purge: deletes the underlying file's bytes while keeping both the
// platform.files row and the package's own audit trail (the same "object deleted, row + audit kept"
// pattern PLT-09's own infected-file handling already uses) — the module doc's "ลบไฟล์อัตโนมัติเมื่อหมดอายุ".
// A harmless no-op if the package was already purged.
func (s *Service) PurgeExpiredPackage(ctx context.Context, packageID uuid.UUID) error {
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.LockPackage(ctx, packageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.PurgedAt.Valid {
		return nil
	}
	if s.Files != nil {
		if err := s.Files.DeleteObject(ctx, row.FileID); err != nil && !errors.Is(err, files.ErrNotFound) {
			return err
		}
	}
	if err := q.MarkPackagePurged(ctx, packageID); err != nil {
		return err
	}
	return s.audit(ctx, "dsar.package.purge", row.RequestID, nil, map[string]any{"package_id": packageID})
}

// PackagePurgeWorker works dsar.package_purge.
type PackagePurgeWorker struct {
	river.WorkerDefaults[PackagePurgeArgs]
	Service *Service
}

func (w *PackagePurgeWorker) Work(ctx context.Context, job *river.Job[PackagePurgeArgs]) error {
	id, err := uuid.Parse(job.Args.PackageID)
	if err != nil {
		return river.JobCancel(err)
	}
	return w.Service.PurgeExpiredPackage(ctx, id)
}
