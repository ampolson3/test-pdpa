// Package fileshttp holds the file endpoints of PLT-09 (upload, metadata, signed download). Types in
// files.gen.go are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package fileshttp

import (
	"context"
	"errors"
	"io"
	"net/http"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/files"
)

type Strict struct {
	svc *files.Service
}

func NewStrict(svc *files.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) PlatformUploadFile(ctx context.Context, req PlatformUploadFileRequestObject) (PlatformUploadFileResponseObject, error) {
	for {
		part, err := req.Body.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, httpx.RequestInvalid("multipart field \"file\" is required")
		}
		if err != nil {
			return nil, httpx.RequestInvalid("malformed multipart body")
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		f, err := h.svc.Upload(ctx, part.FileName(), part)
		part.Close()
		if err != nil {
			return nil, problem(err)
		}
		return PlatformUploadFile201JSONResponse(toWire(f)), nil
	}
}

func (h *Strict) PlatformGetFile(ctx context.Context, req PlatformGetFileRequestObject) (PlatformGetFileResponseObject, error) {
	f, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformGetFile200JSONResponse(toWire(f)), nil
}

func (h *Strict) PlatformDownloadFile(ctx context.Context, req PlatformDownloadFileRequestObject) (PlatformDownloadFileResponseObject, error) {
	u, err := h.svc.DownloadURL(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformDownloadFile302Response{Headers: PlatformDownloadFile302ResponseHeaders{Location: u}}, nil
}

func toWire(f files.File) StoredFile {
	out := StoredFile{
		Id: f.ID, FileName: f.FileName, MimeType: f.MimeType, SizeBytes: f.SizeBytes, Sha256: f.SHA256,
		AvStatus: StoredFileAvStatus(f.AVStatus), CreatedAt: f.CreatedAt.UTC(), EntityId: f.EntityID,
	}
	if f.EntityType != "" {
		out.EntityType = &f.EntityType
	}
	return out
}

// problem maps service errors to the problem codes in the contract; cmd/api writes (and localizes)
// any httpx.Problem a handler returns.
func problem(err error) error {
	p := func(status int, code, title string) httpx.Problem {
		return httpx.Problem{Status: status, Code: code, Title: title}
	}
	switch {
	case errors.Is(err, files.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, files.ErrTooLarge):
		return p(http.StatusRequestEntityTooLarge, "files.too_large", "File too large")
	case errors.Is(err, files.ErrTypeNotAllowed):
		return p(http.StatusUnsupportedMediaType, "files.type_not_allowed", "File type not allowed")
	case errors.Is(err, files.ErrEmpty):
		return p(http.StatusBadRequest, "files.empty", "File is empty")
	case errors.Is(err, files.ErrInvalidName):
		return p(http.StatusBadRequest, "files.invalid_name", "Invalid file name")
	case errors.Is(err, files.ErrScanPending):
		return p(http.StatusConflict, "files.scan_pending", "Virus scan not finished")
	case errors.Is(err, files.ErrInfected):
		return p(http.StatusConflict, "files.infected", "File rejected: malware found")
	case errors.Is(err, files.ErrScanFailed):
		return p(http.StatusConflict, "files.scan_failed", "Virus scan failed")
	}
	return err
}
