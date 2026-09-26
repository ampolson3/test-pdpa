// Package files stores uploaded and generated files in S3-compatible object storage and gates every
// download behind a ClamAV scan (PLT-09, SEQ-07):
//
//   - Service.Upload checks size and type (sniffed content, not just the name), writes the object with
//     server-side encryption, records platform.files (av_status pending) and enqueues files.scan and
//     files.expire in the request transaction.
//   - Scanner (job files.scan) streams the object through clamd: clean → downloadable; infected → the
//     object is deleted, the row kept as evidence, an audit entry and alert written.
//   - Service.DownloadURL returns a short-lived pre-signed GET URL, only for clean files the caller
//     may see (the uploader until the file is attached; then the permission its entity type requires).
//   - Expirer (job files.expire) deletes an upload nobody attached to a record within the orphan TTL.
package files

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/encrypt"
)

// ObjectStore is the part of the S3 API the platform uses.
type ObjectStore interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	// PresignGet returns a URL that downloads key as fileName until ttl has passed.
	PresignGet(ctx context.Context, key, fileName string, ttl time.Duration) (string, error)
	Bucket() string
}

// S3Config configures S3Store (S3, MinIO, SeaweedFS). Credentials come from the environment /
// OpenBao (CLAUDE.md rule 14).
type S3Config struct {
	Endpoint  string // host:port
	AccessKey string
	SecretKey string
	Bucket    string
	UseTLS    bool
	Region    string
	// SSE requests server-side encryption (SSE-S3) on every object. Leave on in every environment
	// whose storage has a KMS configured; test gateways without one set it false.
	SSE bool
}

// S3Store is ObjectStore over minio-go.
type S3Store struct {
	client *minio.Client
	cfg    S3Config
}

func NewS3Store(cfg S3Config) (*S3Store, error) {
	c, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseTLS,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("files: s3 client: %w", err)
	}
	return &S3Store{client: c, cfg: cfg}, nil
}

func (s *S3Store) Bucket() string { return s.cfg.Bucket }

// EnsureBucket creates the bucket if it doesn't exist (dev / test convenience; production buckets are
// provisioned with versioning and object lock by infrastructure code).
func (s *S3Store) EnsureBucket(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.cfg.Bucket)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return s.client.MakeBucket(ctx, s.cfg.Bucket, minio.MakeBucketOptions{Region: s.cfg.Region})
}

func (s *S3Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	opts := minio.PutObjectOptions{ContentType: contentType}
	if s.cfg.SSE {
		opts.ServerSideEncryption = encrypt.NewSSE()
	}
	_, err := s.client.PutObject(ctx, s.cfg.Bucket, key, r, size, opts)
	return err
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, s.cfg.Bucket, key, minio.GetObjectOptions{})
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.cfg.Bucket, key, minio.RemoveObjectOptions{})
}

func (s *S3Store) PresignGet(ctx context.Context, key, fileName string, ttl time.Duration) (string, error) {
	params := url.Values{}
	params.Set("response-content-disposition", contentDisposition(fileName))
	u, err := s.client.PresignedGetObject(ctx, s.cfg.Bucket, key, ttl, params)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// contentDisposition forces a download (never inline rendering of user content) and carries the
// original, possibly Thai, file name per RFC 6266 / 5987.
func contentDisposition(fileName string) string {
	return "attachment; filename*=UTF-8''" + url.PathEscape(fileName)
}
