package main

import (
	"net/http"
	"strings"
)

// maxBody caps request bodies: 1 MiB for JSON APIs, and the file limit plus room for the multipart
// envelope on the two upload routes (PLT-09's own admin upload, and DSAR-19's own public document
// submission, which carries the same kind of body — a handful of text fields plus one file part).
// Bodies over the cap fail to read, so nothing reads an unbounded body into memory or onto disk.
func maxBody(fileLimit int64) func(http.Handler) http.Handler {
	const jsonLimit = 1 << 20
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := int64(jsonLimit)
			if r.Method == http.MethodPost && strings.TrimSuffix(r.URL.Path, "/") == "/admin/v1/platform/files" {
				limit = fileLimit + 1<<20
			}
			if r.Method == http.MethodPost && strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/documents") &&
				strings.HasPrefix(r.URL.Path, "/public/v1/dsar-tracking/") {
				limit = fileLimit + 1<<20
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
