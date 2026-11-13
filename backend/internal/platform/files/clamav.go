package files

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// Verdict is the result of scanning one file.
type Verdict struct {
	Infected  bool
	Signature string // e.g. "Eicar-Test-Signature", empty when clean
}

// AVScanner scans a stream for malware.
type AVScanner interface {
	Scan(ctx context.Context, r io.Reader) (Verdict, error)
}

// Clamd talks to clamd over TCP with the INSTREAM command (zINSTREAM, 4-byte big-endian chunk lengths,
// zero-length terminator). clamd must allow a StreamMaxLength at least Config.MaxBytes.
type Clamd struct {
	Addr    string // host:3310
	Timeout time.Duration
}

func (c *Clamd) Scan(ctx context.Context, r io.Reader) (Verdict, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return Verdict{}, fmt.Errorf("files: clamd dial: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return Verdict{}, fmt.Errorf("files: clamd: %w", err)
	}
	buf := make([]byte, 64*1024)
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			var size [4]byte
			binary.BigEndian.PutUint32(size[:], uint32(n))
			if _, err := conn.Write(size[:]); err != nil {
				return Verdict{}, fmt.Errorf("files: clamd: %w", err)
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return Verdict{}, fmt.Errorf("files: clamd: %w", err)
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return Verdict{}, fmt.Errorf("files: read object: %w", rerr)
		}
	}
	if _, err := conn.Write([]byte{0, 0, 0, 0}); err != nil {
		return Verdict{}, fmt.Errorf("files: clamd: %w", err)
	}

	reply, err := bufio.NewReader(conn).ReadString(0)
	if err != nil && !errors.Is(err, io.EOF) {
		return Verdict{}, fmt.Errorf("files: clamd reply: %w", err)
	}
	return parseClamdReply(strings.TrimRight(reply, "\x00\n"))
}

// parseClamdReply reads "stream: OK", "stream: <signature> FOUND" or "... ERROR".
func parseClamdReply(reply string) (Verdict, error) {
	body := strings.TrimSpace(strings.TrimPrefix(reply, "stream:"))
	switch {
	case body == "OK":
		return Verdict{}, nil
	case strings.HasSuffix(body, " FOUND"):
		return Verdict{Infected: true, Signature: strings.TrimSuffix(body, " FOUND")}, nil
	default:
		// e.g. "INSTREAM size limit exceeded. ERROR" — not a verdict; the scan is retried.
		return Verdict{}, fmt.Errorf("files: clamd: %s", body)
	}
}
