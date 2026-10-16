package files

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Config holds the tunable upload rules. None of these values is decided in docs/decisions.md; the
// defaults below are conservative placeholders (see DefaultConfig) and every one is configuration.
type Config struct {
	MaxBytes int64
	// Allowed maps a file extension (lower case, with dot) to the content types its sniffed bytes may
	// have. A file is accepted only if its extension is listed and its content matches.
	Allowed map[string][]string
	// DownloadTTL is how long a pre-signed download URL stays valid (SEQ-07: 5 minutes).
	DownloadTTL time.Duration
	// OrphanTTL is how long an upload may stay unattached to any record before it is deleted.
	OrphanTTL time.Duration
}

// DefaultConfig: 25 MB; PDF, images, Office documents, CSV and plain text; 5-minute links (SEQ-07);
// orphans removed after 24 hours.
func DefaultConfig() Config {
	zip := []string{"application/zip"}
	text := []string{"text/plain; charset=utf-8", "text/plain; charset=utf-16be", "text/plain; charset=utf-16le"}
	return Config{
		MaxBytes: 25 << 20,
		Allowed: map[string][]string{
			".pdf":  {"application/pdf"},
			".png":  {"image/png"},
			".jpg":  {"image/jpeg"},
			".jpeg": {"image/jpeg"},
			".docx": zip,
			".xlsx": zip,
			".pptx": zip,
			".csv":  text,
			".txt":  text,
		},
		DownloadTTL: 5 * time.Minute,
		OrphanTTL:   24 * time.Hour,
	}
}

// contentTypeFor returns the stored content type for a file whose first bytes are head, or "" if the
// combination of name and content isn't allowed. The declared (client) content type is never trusted.
func (c Config) contentTypeFor(fileName string, head []byte) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	allowed, ok := c.Allowed[ext]
	if !ok {
		return ""
	}
	sniffed := http.DetectContentType(head)
	for _, a := range allowed {
		if sniffed == a {
			return storedType(ext, sniffed)
		}
	}
	return ""
}

func storedType(ext, sniffed string) string {
	switch ext {
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".csv":
		return "text/csv; charset=utf-8"
	}
	return sniffed
}

// cleanFileName keeps the base name only (no directories), drops control characters and caps the
// length; Thai names are kept as they are.
func cleanFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "." || out == "/" {
		out = ""
	}
	if utf8.RuneCountInString(out) > 200 {
		out = string([]rune(out)[:200])
	}
	return out
}
