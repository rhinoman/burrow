package debug

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestTransportLogsRequestAndResponse(t *testing.T) {
	var buf bytes.Buffer
	dbg := NewLogger(&buf)

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})

	tr := NewTransport(base, dbg)

	req, _ := http.NewRequest("GET", "https://example.com/api?q=test", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()

	// Request logged
	if !strings.Contains(out, "→ GET https://example.com/api?q=test") {
		t.Errorf("expected request URL in log, got:\n%s", out)
	}
	// Response status logged
	if !strings.Contains(out, "← 200") {
		t.Errorf("expected 200 status in log, got:\n%s", out)
	}
	// Response body logged
	if !strings.Contains(out, `"ok"`) {
		t.Errorf("expected JSON body in log, got:\n%s", out)
	}

	// Response body still readable by caller
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"ok":true}` {
		t.Errorf("response body corrupted: got %q", body)
	}
}

func TestTransportLogsPostBody(t *testing.T) {
	var buf bytes.Buffer
	dbg := NewLogger(&buf)

	var receivedBody string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		receivedBody = string(b)
		return &http.Response{
			StatusCode: 201,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	tr := NewTransport(base, dbg)

	body := `{"name":"test"}`
	req, _ := http.NewRequest("POST", "https://example.com/api", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	_, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()

	// Request body logged
	if !strings.Contains(out, `"name"`) {
		t.Errorf("expected request body in log, got:\n%s", out)
	}
	// Base transport still received the body
	if receivedBody != body {
		t.Errorf("base transport got %q, want %q", receivedBody, body)
	}
}

func TestPrettyJSON(t *testing.T) {
	input := `{"a":1,"b":[2,3]}`
	got := prettyJSON([]byte(input))
	if !strings.Contains(got, "\n") {
		t.Errorf("expected indented output, got: %s", got)
	}
	if !strings.Contains(got, `"a": 1`) {
		t.Errorf("expected formatted JSON, got: %s", got)
	}
}

func TestPrettyJSONNonJSON(t *testing.T) {
	input := "this is not json"
	got := prettyJSON([]byte(input))
	if got != input {
		t.Errorf("expected raw passthrough, got: %s", got)
	}
}

func TestTransportTruncatesLargeBody(t *testing.T) {
	var buf bytes.Buffer
	dbg := NewLogger(&buf)

	// 20KB response
	largeBody := strings.Repeat("x", 20*1024)
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(largeBody)),
		}, nil
	})

	tr := NewTransport(base, dbg)
	req, _ := http.NewRequest("GET", "https://example.com/big", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation notice in log, got:\n%s", out)
	}

	// Full body still readable by caller
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 20*1024 {
		t.Errorf("caller body length = %d, want %d", len(body), 20*1024)
	}
}

func TestNilLoggerNoOp(t *testing.T) {
	var l *Logger
	// Must not panic
	l.Printf("test %d", 42)
	l.Section("test")
}

func TestNilLoggerTransportPassthrough(t *testing.T) {
	called := false
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	tr := NewTransport(base, nil)
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	_, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("base transport was not called")
	}
}

func TestTransportRedactsAuth(t *testing.T) {
	var buf bytes.Buffer
	dbg := NewLogger(&buf)

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	tr := NewTransport(base, dbg)
	req, _ := http.NewRequest("GET", "https://example.com/api", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	tr.RoundTrip(req)

	out := buf.String()
	if strings.Contains(out, "secret-token-123") {
		t.Errorf("auth token leaked in debug output:\n%s", out)
	}
	if !strings.Contains(out, "Bearer <redacted>") {
		t.Errorf("expected redacted auth header, got:\n%s", out)
	}
}
