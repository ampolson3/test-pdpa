package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// PDFRenderer prints a self-contained HTML page to PDF.
type PDFRenderer interface {
	PDF(ctx context.Context, html []byte) ([]byte, error)
}

// ErrNoRenderer means neither Gotenberg nor a local Chromium is configured.
var ErrNoRenderer = errors.New("render: no PDF renderer configured (GOTENBERG_URL or CHROMIUM_PATH)")

// Gotenberg prints through a Gotenberg server's Chromium route (docs/architecture: the production renderer).
type Gotenberg struct {
	URL    string // e.g. http://gotenberg:3000
	Client *http.Client
}

func (g *Gotenberg) PDF(ctx context.Context, html []byte) ([]byte, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	f, err := w.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(html); err != nil {
		return nil, err
	}
	for k, v := range map[string]string{"paperWidth": "8.27", "paperHeight": "11.7", "marginTop": "0", "marginBottom": "0", "marginLeft": "0",
		"marginRight": "0", "preferCssPageSize": "true", "printBackground": "true"} {
		_ = w.WriteField(k, v)
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.URL+"/forms/chromium/convert/html", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	c := g.Client
	if c == nil {
		c = &http.Client{Timeout: 60 * time.Second}
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("render: gotenberg: %w", err)
	}
	defer res.Body.Close()
	out, err := io.ReadAll(io.LimitReader(res.Body, 100<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("render: gotenberg answered %d", res.StatusCode)
	}
	return out, nil
}

// Chromium prints with a local headless Chromium — for development and tests where no Gotenberg runs.
type Chromium struct {
	Path string
}

func (c *Chromium) PDF(ctx context.Context, html []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "pdpa-render-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in, out := filepath.Join(dir, "index.html"), filepath.Join(dir, "out.pdf")
	if err := os.WriteFile(in, html, 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Path, "--headless=new", "--no-sandbox", "--disable-gpu", "--no-pdf-header-footer",
		"--user-data-dir="+filepath.Join(dir, "profile"), "--print-to-pdf="+out, "file://"+in)
	if msg, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("render: chromium: %w: %s", err, lastLine(msg))
	}
	return os.ReadFile(out)
}

func lastLine(b []byte) string {
	b = bytes.TrimSpace(b)
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
		b = b[i+1:]
	}
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}

// FromEnv picks the renderer: GOTENBERG_URL, else CHROMIUM_PATH, else nil (rendering PDF then fails with
// ErrNoRenderer; DOCX and HTML still work).
func FromEnv() PDFRenderer {
	if u := os.Getenv("GOTENBERG_URL"); u != "" {
		return &Gotenberg{URL: u}
	}
	if p := os.Getenv("CHROMIUM_PATH"); p != "" {
		return &Chromium{Path: p}
	}
	return nil
}
