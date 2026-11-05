// PNG-12 (ประกาศแบบหลายชั้นและป้าย CCTV): the ม.23 layered short/full pairing needs no persisted
// relation — a "short" notice is just an ordinary notice with notice_type "layered_short" or "cctv"
// (already in noticeTypes since PNG-01); the caller picks which already-published notice is "the full
// version" at sign-generation time via full_notice_id, since nothing else in this feature needs that
// link to be stored. The real new work is the signage sheet itself.
package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"strings"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"

	"pdpa-platform/internal/platform/docs/render"
)

// CCTVSignInput groups PNG-12's own generation parameters.
type CCTVSignInput struct {
	// FullNoticeID is the already-published notice the QR code should open — "the full version" of this
	// sign's own short/CCTV notice. Nil means this notice is itself the full version (a standalone sign).
	FullNoticeID *uuid.UUID
	Lang         string // th|en, default th
	Format       string // pdf|png, default pdf
	Paper        string // a4|a3, default a4
}

func cctvPaperPixels(paper string) (int, int, error) {
	switch paper {
	case "a3":
		return 1754, 2481, nil // A3 portrait @150dpi
	case "a4", "":
		return 1240, 1754, nil // A4 portrait @150dpi
	}
	return 0, 0, fmt.Errorf("%w: paper", ErrInvalid)
}

func cctvPaperCSSName(paper string) (string, error) {
	switch paper {
	case "a3":
		return "A3", nil
	case "a4", "":
		return "A4", nil
	}
	return "", fmt.Errorf("%w: paper", ErrInvalid)
}

// cctvSignURL resolves the QR code's own target URL — the full notice's public page — without any
// rendering; split out so it's directly unit-testable with no renderer configured.
func (s *Service) cctvSignURL(ctx context.Context, n Notice, fullNoticeID *uuid.UUID, lang string) (string, error) {
	full := n
	if fullNoticeID != nil {
		f, err := s.GetNotice(ctx, *fullNoticeID)
		if err != nil {
			return "", fmt.Errorf("%w: full_notice_id", ErrInvalid)
		}
		full = f
	}
	if full.PublicKey == nil {
		return "", fmt.Errorf("%w: the linked notice is not published yet", ErrInvalid)
	}
	return strings.TrimRight(s.PortalBaseURL, "/") + "/" + lang + "/n/" + *full.PublicKey, nil
}

// GenerateCCTVSign is PNG-12's own acceptance criterion: a printable CCTV signage sheet (A4/A3, PDF or PNG)
// whose QR code opens the full notice's public page.
func (s *Service) GenerateCCTVSign(ctx context.Context, noticeID uuid.UUID, in CCTVSignInput) ([]byte, string, error) {
	n, err := s.GetNotice(ctx, noticeID)
	if err != nil {
		return nil, "", err
	}
	lang := in.Lang
	if lang == "" {
		lang = "th"
	}
	if lang != "th" && lang != "en" {
		return nil, "", fmt.Errorf("%w: lang", ErrInvalid)
	}
	url, err := s.cctvSignURL(ctx, n, in.FullNoticeID, lang)
	if err != nil {
		return nil, "", err
	}

	qrPNG, err := qrcode.Encode(url, qrcode.Medium, 512)
	if err != nil {
		return nil, "", err
	}
	body := cctvSignBody(n.Title, lang, qrPNG)

	format := in.Format
	if format == "" {
		format = "pdf"
	}
	switch format {
	case "png":
		w, h, err := cctvPaperPixels(in.Paper)
		if err != nil {
			return nil, "", err
		}
		if s.Image == nil {
			return nil, "", render.ErrNoImageRenderer
		}
		b, err := s.Image.PNG(ctx, cctvScreenshotHTML(body, w, h), w, h)
		return b, "cctv-sign.png", err
	case "pdf":
		pageSize, err := cctvPaperCSSName(in.Paper)
		if err != nil {
			return nil, "", err
		}
		if s.PDF == nil {
			return nil, "", render.ErrNoRenderer
		}
		b, err := s.PDF.PDF(ctx, cctvPrintHTML(body, pageSize))
		return b, "cctv-sign.pdf", err
	}
	return nil, "", fmt.Errorf("%w: format", ErrInvalid)
}

func cctvLabels(lang string) (heading, body, hint string) {
	if lang == "en" {
		return "Notice: CCTV in operation",
			"This area is monitored by closed-circuit television (CCTV) for security purposes, under the Personal Data Protection Act.",
			"Scan to view the full notice"
	}
	return "แจ้งเตือน: พื้นที่นี้มีการบันทึกภาพด้วยกล้องวงจรปิด",
		"พื้นที่นี้มีการบันทึกภาพด้วยกล้องวงจรปิด (CCTV) เพื่อการรักษาความปลอดภัย ตามพระราชบัญญัติคุ้มครองข้อมูลส่วนบุคคล",
		"สแกนเพื่อดูประกาศฉบับเต็ม"
}

const cctvSignCSS = `.sign{font-family:'Sarabun','Noto Sans Thai',sans-serif;text-align:center;box-sizing:border-box;padding:15mm}
.sign h1{font-size:28pt;margin:0 0 10mm}
.sign .body{font-size:16pt;line-height:1.5;margin:0 0 10mm}
.sign .qr{width:60mm;height:60mm}
.sign .hint{font-size:14pt;margin:6mm 0 2mm}
.sign .sub{font-size:10pt;color:#555}`

func cctvSignBody(title, lang string, qrPNG []byte) string {
	heading, body, hint := cctvLabels(lang)
	b64 := base64.StdEncoding.EncodeToString(qrPNG)
	return fmt.Sprintf(`<div class="sign"><h1>%s</h1><p class="body">%s</p><img class="qr" src="data:image/png;base64,%s" alt="QR"><p class="hint">%s</p><p class="sub">%s</p></div>`,
		html.EscapeString(heading), html.EscapeString(body), b64, html.EscapeString(hint), html.EscapeString(title))
}

func cctvScreenshotHTML(body string, pxW, pxH int) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><style>%s`, render.FontFaces())
	fmt.Fprintf(&b, `html,body{margin:0;width:%dpx;height:%dpx;background:#fff;display:flex;align-items:center;justify-content:center}%s</style></head><body>%s</body></html>`,
		pxW, pxH, cctvSignCSS, body)
	return []byte(b.String())
}

func cctvPrintHTML(body, pageSize string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><style>%s`, render.FontFaces())
	fmt.Fprintf(&b, `@page{size:%s;margin:0}body{margin:0}%s</style></head><body>%s</body></html>`, pageSize, cctvSignCSS, body)
	return []byte(b.String())
}
