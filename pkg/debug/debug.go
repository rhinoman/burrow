// Package debug provides logging utilities for troubleshooting Burrow's
// pipeline execution. All types are nil-safe: a nil *Logger is a no-op,
// and a Transport with a nil logger passes through without overhead.
package debug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxBodyDisplay is the maximum number of bytes shown for a response body.
const maxBodyDisplay = 10 * 1024 // 10KB

// Logger writes debug output to a writer. A nil *Logger is safe to use;
// all methods are no-ops.
type Logger struct {
	w io.Writer
}

// NewLogger creates a Logger that writes to w.
func NewLogger(w io.Writer) *Logger {
	return &Logger{w: w}
}

// Printf writes a formatted debug line. No-op on nil receiver.
func (l *Logger) Printf(format string, args ...any) {
	if l == nil {
		return
	}
	fmt.Fprintf(l.w, "[debug] "+format+"\n", args...)
}

// Section writes a visual separator. No-op on nil receiver.
func (l *Logger) Section(label string) {
	if l == nil {
		return
	}
	fmt.Fprintf(l.w, "[debug] ─── %s ───\n", label)
}

// Transport is an http.RoundTripper that logs request and response details
// before delegating to a base transport. A nil Logger disables all logging
// and adds zero overhead.
type Transport struct {
	Base http.RoundTripper
	Log  *Logger
}

// NewTransport wraps base with debug logging. If dbg is nil, RoundTrip
// delegates directly to base with no additional work.
func NewTransport(base http.RoundTripper, dbg *Logger) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Transport{Base: base, Log: dbg}
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Log == nil {
		return t.Base.RoundTrip(req)
	}

	t.logRequest(req)

	start := time.Now()
	resp, err := t.Base.RoundTrip(req)
	elapsed := time.Since(start)

	if err != nil {
		t.Log.Printf("← error (%s): %v", elapsed.Round(time.Millisecond), err)
		return resp, err
	}

	t.logResponse(resp, elapsed)
	return resp, nil
}

func (t *Transport) logRequest(req *http.Request) {
	t.Log.Printf("→ %s %s", req.Method, req.URL.String())

	if ct := req.Header.Get("Content-Type"); ct != "" {
		t.Log.Printf("  Content-Type: %s", ct)
	}
	if auth := req.Header.Get("Authorization"); auth != "" {
		// Show scheme but redact the actual token.
		if idx := strings.Index(auth, " "); idx > 0 {
			t.Log.Printf("  Authorization: %s <redacted>", auth[:idx])
		} else {
			t.Log.Printf("  Authorization: <redacted>")
		}
	}

	// Log POST/PUT/PATCH bodies.
	if req.Body != nil && req.Body != http.NoBody {
		body, err := io.ReadAll(req.Body)
		if err == nil && len(body) > 0 {
			// Re-wrap so the base transport can still read the body.
			req.Body = io.NopCloser(bytes.NewReader(body))
			t.Log.Printf("  Request body (%d bytes):", len(body))
			t.logBody(body)
		}
	}
}

func (t *Transport) logResponse(resp *http.Response, elapsed time.Duration) {
	t.Log.Printf("← %d %s (%s)", resp.StatusCode, http.StatusText(resp.StatusCode), elapsed.Round(time.Millisecond))

	if resp.Body == nil {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Log.Printf("  (error reading response body: %v)", err)
		resp.Body = io.NopCloser(bytes.NewReader(nil))
		return
	}

	// Re-wrap so the caller can still read the body.
	resp.Body = io.NopCloser(bytes.NewReader(body))

	if len(body) == 0 {
		return
	}

	display := body
	truncated := false
	if len(display) > maxBodyDisplay {
		display = display[:maxBodyDisplay]
		truncated = true
	}

	t.Log.Printf("  Response (%d bytes):", len(body))
	t.logBody(display)
	if truncated {
		t.Log.Printf("  ... truncated at %d bytes", maxBodyDisplay)
	}
}

func (t *Transport) logBody(data []byte) {
	pretty := prettyJSON(data)
	for _, line := range strings.Split(pretty, "\n") {
		t.Log.Printf("  %s", line)
	}
}

// prettyJSON attempts to indent JSON. Falls back to the raw string for
// non-JSON content.
func prettyJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return string(data)
	}
	return buf.String()
}
