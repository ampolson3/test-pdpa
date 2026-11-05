// Package importerhttp holds the PLT-14 import endpoints. Types in importer.gen.go are generated from
// api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package importerhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/importer"
)

type Strict struct {
	svc *importer.Service
}

func NewStrict(svc *importer.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) PlatformListImports(ctx context.Context, req PlatformListImportsRequestObject) (PlatformListImportsResponseObject, error) {
	t := ""
	if req.Params.ImportType != nil {
		t = *req.Params.ImportType
	}
	list, err := h.svc.List(ctx, t)
	if err != nil {
		return nil, err
	}
	resp := PlatformListImports200JSONResponse{Data: make([]ImportJob, 0, len(list))}
	for _, j := range list {
		resp.Data = append(resp.Data, toWire(j))
	}
	return resp, nil
}

func (h *Strict) PlatformCreateImport(ctx context.Context, req PlatformCreateImportRequestObject) (PlatformCreateImportResponseObject, error) {
	j, err := h.svc.Create(ctx, req.Body.ImportType, req.Body.FileId)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformCreateImport201JSONResponse{Body: toWire(j), Headers: PlatformCreateImport201ResponseHeaders{ETag: etag(j.RowVersion)}}, nil
}

func (h *Strict) PlatformGetImport(ctx context.Context, req PlatformGetImportRequestObject) (PlatformGetImportResponseObject, error) {
	j, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformGetImport200JSONResponse{Body: toWire(j), Headers: PlatformGetImport200ResponseHeaders{ETag: etag(j.RowVersion)}}, nil
}

func (h *Strict) PlatformSetImportMapping(ctx context.Context, req PlatformSetImportMappingRequestObject) (PlatformSetImportMappingResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	j, err := h.svc.SetMapping(ctx, req.Id, v, req.Body.Columns)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformSetImportMapping200JSONResponse{Body: toWire(j), Headers: PlatformSetImportMapping200ResponseHeaders{ETag: etag(j.RowVersion)}}, nil
}

func (h *Strict) PlatformConfirmImport(ctx context.Context, req PlatformConfirmImportRequestObject) (PlatformConfirmImportResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	j, err := h.svc.Confirm(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformConfirmImport200JSONResponse{Body: toWire(j), Headers: PlatformConfirmImport200ResponseHeaders{ETag: etag(j.RowVersion)}}, nil
}

func (h *Strict) PlatformDownloadImportErrors(ctx context.Context, req PlatformDownloadImportErrorsRequestObject) (PlatformDownloadImportErrorsResponseObject, error) {
	u, err := h.svc.ErrorReportURL(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return PlatformDownloadImportErrors302Response{Headers: PlatformDownloadImportErrors302ResponseHeaders{Location: u}}, nil
}

func toWire(j importer.Job) ImportJob {
	out := ImportJob{
		Id: j.ID, ImportType: j.Type, Status: ImportJobStatus(j.Status),
		Headers: nonNil(j.Headers), Suggested: nonNilMap(j.Suggested), Mapping: nonNilMap(j.Mapping),
		HasErrorReport: j.HasErrorReport, RowVersion: int(j.RowVersion), CreatedAt: j.CreatedAt.UTC(), UpdatedAt: j.UpdatedAt.UTC(),
		TotalRows: intPtr(j.TotalRows), ValidRows: intPtr(j.ValidRows), ErrorRows: intPtr(j.ErrorRows),
	}
	if j.Failure != "" {
		out.Failure = &j.Failure
	}
	for _, c := range j.Columns {
		out.Columns = append(out.Columns, struct {
			Key      string            `json:"key"`
			Label    map[string]string `json:"label"`
			Required bool              `json:"required"`
		}{Key: c.Key, Label: nonNilMap(c.Label), Required: c.Required})
	}
	if out.Columns == nil {
		out.Columns = []struct {
			Key      string            `json:"key"`
			Label    map[string]string `json:"label"`
			Required bool              `json:"required"`
		}{}
	}
	return out
}

func intPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	i := int(*v)
	return &i
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func etag(v int32) *string {
	s := fmt.Sprintf("%q", strconv.Itoa(int(v)))
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func problem(err error) error {
	switch {
	case errors.Is(err, importer.ErrNotFound), errors.Is(err, importer.ErrUnknownType):
		return httpx.NotFound()
	case errors.Is(err, importer.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, importer.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, importer.ErrInvalidState):
		return httpx.Problem{Status: http.StatusConflict, Code: "import.invalid_transition", Title: "Invalid state transition"}
	case errors.Is(err, importer.ErrInvalidMapping):
		return httpx.UnprocessableEntity("import.invalid_mapping", err.Error())
	case errors.Is(err, importer.ErrFileNotUsable):
		return httpx.UnprocessableEntity("import.file_not_usable", err.Error())
	case errors.Is(err, files.ErrScanPending), errors.Is(err, files.ErrNotClean):
		return httpx.Problem{Status: http.StatusConflict, Code: "files.scan_pending", Title: "Virus scan not finished"}
	}
	return err
}
