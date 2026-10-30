package dpiahttp

import (
	"bytes"
	"context"
	"mime"
	"net/http"
	"strings"
)

func (h *Strict) DpiaGetReport(ctx context.Context, req DpiaGetReportRequestObject) (DpiaGetReportResponseObject, error) {
	b, name, err := h.svc.Report(ctx, req.Id, string(req.Params.Language), string(req.Params.Format))
	if err != nil {
		return nil, problem(err)
	}
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return reportResponse{DpiaGetReport200ApplicationoctetStreamResponse{Body: bytes.NewReader(b), ContentLength: int64(len(b)),
		Headers: DpiaGetReport200ResponseHeaders{ContentDisposition: &cd}}, reportContentType(name)}, nil
}

// reportResponse sets the file's real media type (the contract says octet-stream so one response covers
// pdf/docx) — the same pattern internal/platform/docs/http's own export handler already uses.
type reportResponse struct {
	DpiaGetReport200ApplicationoctetStreamResponse
	ctype string
}

func (r reportResponse) VisitDpiaGetReportResponse(w http.ResponseWriter) error {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	return r.DpiaGetReport200ApplicationoctetStreamResponse.VisitDpiaGetReportResponse(&reportTypedWriter{ResponseWriter: w, ctype: r.ctype})
}

type reportTypedWriter struct {
	http.ResponseWriter
	ctype string
}

func (t *reportTypedWriter) WriteHeader(code int) {
	t.ResponseWriter.Header().Set("Content-Type", t.ctype)
	t.ResponseWriter.WriteHeader(code)
}

func reportContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(name, ".docx"):
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	}
	return "application/octet-stream"
}
