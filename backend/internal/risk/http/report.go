package riskhttp

import (
	"bytes"
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"

	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/docs/render"
	riskservice "pdpa-platform/internal/risk/service"
)

// RiskGetExecutiveReport is RRA-14: "รายงานสร้างได้ในคลิกเดียวและตรงกับทะเบียน" — the same pattern
// internal/dpia/http's own report.go and internal/dataflow/http's own export.go already use for a
// pdf/docx/html document built on demand.
func (h *Strict) RiskGetExecutiveReport(ctx context.Context, req RiskGetExecutiveReportRequestObject) (RiskGetExecutiveReportResponseObject, error) {
	b, name, err := h.svc.ExecutiveReport(ctx, string(req.Params.Language), string(req.Params.Format))
	if err != nil {
		return nil, reportProblem(err)
	}
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return reportResponse{RiskGetExecutiveReport200ApplicationoctetStreamResponse{Body: bytes.NewReader(b), ContentLength: int64(len(b)),
		Headers: RiskGetExecutiveReport200ResponseHeaders{ContentDisposition: &cd}}, reportContentType(name)}, nil
}

// reportProblem maps a format/language validation error and the renderer-not-configured sentinel to the
// same problem+json shapes dataflow's own exportProblem already establishes for a report endpoint.
func reportProblem(err error) error {
	switch {
	case errors.Is(err, render.ErrNoRenderer):
		return httpx.Problem{Status: 503, Code: "risk.no_renderer", Title: "PDF rendering is not configured"}
	case errors.Is(err, riskservice.ErrInvalid):
		return httpx.UnprocessableEntity("risk.invalid_input", err.Error())
	}
	return err
}

// reportResponse sets the file's real media type (the contract says octet-stream so one response covers
// pdf/docx/html).
type reportResponse struct {
	RiskGetExecutiveReport200ApplicationoctetStreamResponse
	ctype string
}

func (r reportResponse) VisitRiskGetExecutiveReportResponse(w http.ResponseWriter) error {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	return r.RiskGetExecutiveReport200ApplicationoctetStreamResponse.VisitRiskGetExecutiveReportResponse(&reportTypedWriter{ResponseWriter: w, ctype: r.ctype})
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
