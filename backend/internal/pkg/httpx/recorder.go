package httpx

import (
	"bytes"
	"net/http"
)

// Recorder buffers a handler's response instead of writing it straight to the client. The Tx
// middleware (#11) uses it so a handler's response is only ever flushed to the real
// ResponseWriter after its transaction commits — if the transaction rolls back (handler 5xx or the
// audit write itself failing), the buffered response is discarded and a 500 is written instead,
// so the client never sees a 200 for work that didn't actually persist.
type Recorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func NewRecorder() *Recorder {
	return &Recorder{header: make(http.Header), status: http.StatusOK}
}

func (r *Recorder) Header() http.Header { return r.header }

func (r *Recorder) Write(b []byte) (int, error) { return r.body.Write(b) }

func (r *Recorder) WriteHeader(status int) { r.status = status }

func (r *Recorder) Status() int { return r.status }

func (r *Recorder) BodyString() string { return r.body.String() }

// Flush copies the buffered response to w. Call it only after the transaction committed.
func (r *Recorder) Flush(w http.ResponseWriter) {
	dst := w.Header()
	for k, v := range r.header {
		dst[k] = v
	}
	w.WriteHeader(r.status)
	_, _ = w.Write(r.body.Bytes())
}
