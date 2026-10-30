package noticehttp

import (
	"bytes"
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"

	noticeservice "pdpa-platform/internal/notice/service"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/docs/render"
)

// NoticeGetCctvSign is PNG-12's own acceptance criterion: a printable CCTV signage sheet whose QR code
// opens the full notice — the same octet-stream-with-real-content-type pattern dataflow's own export.go
// and risk's own report.go already use for a rendered file built on demand.
func (h *Strict) NoticeGetCctvSign(ctx context.Context, req NoticeGetCctvSignRequestObject) (NoticeGetCctvSignResponseObject, error) {
	in := noticeservice.CCTVSignInput{}
	if req.Params.FullNoticeId != nil {
		in.FullNoticeID = req.Params.FullNoticeId
	}
	if req.Params.Lang != nil {
		in.Lang = string(*req.Params.Lang)
	}
	if req.Params.Format != nil {
		in.Format = string(*req.Params.Format)
	}
	if req.Params.Paper != nil {
		in.Paper = string(*req.Params.Paper)
	}
	b, name, err := h.svc.GenerateCCTVSign(ctx, req.Id, in)
	if err != nil {
		return nil, cctvSignProblem(err)
	}
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return cctvSignResponse{
		NoticeGetCctvSign200ApplicationoctetStreamResponse{Body: bytes.NewReader(b), ContentLength: int64(len(b)),
			Headers: NoticeGetCctvSign200ResponseHeaders{ContentDisposition: &cd}},
		cctvSignContentType(name),
	}, nil
}

func cctvSignProblem(err error) error {
	switch {
	case errors.Is(err, render.ErrNoRenderer), errors.Is(err, render.ErrNoImageRenderer):
		return httpx.Problem{Status: 503, Code: "notice.no_renderer", Title: "Rendering is not configured"}
	}
	return problem(err)
}

type cctvSignResponse struct {
	NoticeGetCctvSign200ApplicationoctetStreamResponse
	ctype string
}

func (r cctvSignResponse) VisitNoticeGetCctvSignResponse(w http.ResponseWriter) error {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	return r.NoticeGetCctvSign200ApplicationoctetStreamResponse.VisitNoticeGetCctvSignResponse(&cctvSignTypedWriter{ResponseWriter: w, ctype: r.ctype})
}

type cctvSignTypedWriter struct {
	http.ResponseWriter
	ctype string
}

func (t *cctvSignTypedWriter) WriteHeader(code int) {
	t.ResponseWriter.Header().Set("Content-Type", t.ctype)
	t.ResponseWriter.WriteHeader(code)
}

func cctvSignContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	}
	return "application/octet-stream"
}
