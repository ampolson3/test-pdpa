package dataflowhttp

import (
	"bytes"
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"

	dataflowservice "pdpa-platform/internal/dataflow/service"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/docs/render"
)

func (h *Strict) DataflowExportGraph(ctx context.Context, req DataflowExportGraphRequestObject) (DataflowExportGraphResponseObject, error) {
	paper := dataflowservice.PaperA3
	if req.Params.Paper != nil {
		paper = dataflowservice.Paper(*req.Params.Paper)
	}
	b, name, err := h.svc.ExportDiagram(ctx, req.Params.LegalEntityId, dataflowservice.ExportFormat(req.Params.Format), paper)
	if err != nil {
		return nil, exportProblem(err)
	}
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return exportResponse{DataflowExportGraph200ApplicationoctetStreamResponse{Body: bytes.NewReader(b), ContentLength: int64(len(b)),
		Headers: DataflowExportGraph200ResponseHeaders{ContentDisposition: &cd}}, exportContentType(name)}, nil
}

func (h *Strict) DataflowGetReport(ctx context.Context, req DataflowGetReportRequestObject) (DataflowGetReportResponseObject, error) {
	b, name, err := h.svc.Report(ctx, req.Params.LegalEntityId, string(req.Params.Language), string(req.Params.Format))
	if err != nil {
		return nil, exportProblem(err)
	}
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return reportResponse{DataflowGetReport200ApplicationoctetStreamResponse{Body: bytes.NewReader(b), ContentLength: int64(len(b)),
		Headers: DataflowGetReport200ResponseHeaders{ContentDisposition: &cd}}, exportContentType(name)}, nil
}

// exportProblem maps a DFG-08 format/paper validation error and the renderer-not-configured sentinels to the
// same problem+json shapes DPIA-15's own report endpoint already established, so a client sees one consistent
// 422/503 shape across modules.
func exportProblem(err error) error {
	switch {
	case errors.Is(err, render.ErrNoRenderer):
		return httpx.Problem{Status: 503, Code: "dataflow.no_renderer", Title: "PDF rendering is not configured"}
	case errors.Is(err, render.ErrNoImageRenderer):
		return httpx.Problem{Status: 503, Code: "dataflow.no_image_renderer", Title: "PNG rendering is not configured"}
	case errors.Is(err, dataflowservice.ErrInvalid):
		return httpx.UnprocessableEntity("dataflow.invalid_input", err.Error())
	}
	return err
}

// exportResponse/reportResponse set the file's real media type (the contract says octet-stream so one
// response covers svg/png/pdf and pdf/docx/html) — the same pattern internal/dpia/http's own report.go and
// internal/platform/docs/http's own export handler already use.
type exportResponse struct {
	DataflowExportGraph200ApplicationoctetStreamResponse
	ctype string
}

func (r exportResponse) VisitDataflowExportGraphResponse(w http.ResponseWriter) error {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	return r.DataflowExportGraph200ApplicationoctetStreamResponse.VisitDataflowExportGraphResponse(&exportTypedWriter{ResponseWriter: w, ctype: r.ctype})
}

type reportResponse struct {
	DataflowGetReport200ApplicationoctetStreamResponse
	ctype string
}

func (r reportResponse) VisitDataflowGetReportResponse(w http.ResponseWriter) error {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	return r.DataflowGetReport200ApplicationoctetStreamResponse.VisitDataflowGetReportResponse(&exportTypedWriter{ResponseWriter: w, ctype: r.ctype})
}

type exportTypedWriter struct {
	http.ResponseWriter
	ctype string
}

func (t *exportTypedWriter) WriteHeader(code int) {
	t.ResponseWriter.Header().Set("Content-Type", t.ctype)
	t.ResponseWriter.WriteHeader(code)
}

func exportContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(name, ".docx"):
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	}
	return "application/octet-stream"
}
