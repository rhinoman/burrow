package configure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchSpecOpenAPIJSON(t *testing.T) {
	body := `{"openapi": "3.0.0", "info": {"title": "Test API"}, "paths": {}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	spec, err := FetchSpec(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchSpec: %v", err)
	}
	if spec.Format != SpecFormatOpenAPIJSON {
		t.Errorf("expected format %q, got %q", SpecFormatOpenAPIJSON, spec.Format)
	}
	if spec.Content == "" {
		t.Error("expected non-empty content")
	}
	if spec.Error != "" {
		t.Errorf("unexpected error: %s", spec.Error)
	}
}

func TestFetchSpecOpenAPIYAML(t *testing.T) {
	body := "openapi: '3.0.0'\ninfo:\n  title: Test API\npaths: {}\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	spec, err := FetchSpec(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchSpec: %v", err)
	}
	if spec.Format != SpecFormatOpenAPIYAML {
		t.Errorf("expected format %q, got %q", SpecFormatOpenAPIYAML, spec.Format)
	}
}

func TestFetchSpecHTML(t *testing.T) {
	body := "<html><head><title>API Docs</title></head><body><h1>API</h1></body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	spec, err := FetchSpec(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchSpec: %v", err)
	}
	if spec.Format != SpecFormatHTML {
		t.Errorf("expected format %q, got %q", SpecFormatHTML, spec.Format)
	}
}

func TestFetchSpecLargeSpec(t *testing.T) {
	// Generate content larger than maxSpecBytes.
	large := `{"openapi": "3.0.0", "data": "` + strings.Repeat("x", maxSpecBytes+1000) + `"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(large))
	}))
	defer srv.Close()

	spec, err := FetchSpec(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchSpec: %v", err)
	}
	if !strings.Contains(spec.Content, "[Spec truncated") {
		t.Error("expected truncation note in content")
	}
	// Verify truncated content is smaller than original.
	if len([]rune(spec.Content)) > maxSpecBytes+200 { // allow for truncation note
		t.Errorf("content too large after truncation: %d runes", len([]rune(spec.Content)))
	}
}

func TestFetchSpecNetworkError(t *testing.T) {
	_, err := FetchSpec(context.Background(), "http://127.0.0.1:1/nonexistent")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestFetchSpecTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.Write([]byte("too late"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := FetchSpec(ctx, srv.URL)
	if err == nil {
		t.Error("expected error for timed-out request")
	}
}

func TestFetchSpecHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := FetchSpec(context.Background(), srv.URL)
	if err == nil {
		t.Error("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("expected HTTP 500 in error, got: %v", err)
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        SpecFormat
	}{
		{
			name:        "json content-type with openapi keyword",
			contentType: "application/json",
			body:        `{"openapi": "3.0.0"}`,
			want:        SpecFormatOpenAPIJSON,
		},
		{
			name:        "json content-type with swagger keyword",
			contentType: "application/json; charset=utf-8",
			body:        `{"swagger": "2.0"}`,
			want:        SpecFormatOpenAPIJSON,
		},
		{
			name:        "yaml content-type with openapi prefix",
			contentType: "text/yaml",
			body:        "openapi: '3.0.0'\ninfo:\n  title: Test",
			want:        SpecFormatOpenAPIYAML,
		},
		{
			name:        "application/yaml content-type",
			contentType: "application/yaml",
			body:        "swagger: '2.0'\ninfo:\n  title: Test",
			want:        SpecFormatOpenAPIYAML,
		},
		{
			name:        "html content-type",
			contentType: "text/html; charset=utf-8",
			body:        "<html><body>API Docs</body></html>",
			want:        SpecFormatHTML,
		},
		{
			name:        "no content-type json body detection",
			contentType: "",
			body:        `{"openapi": "3.0.0", "paths": {}}`,
			want:        SpecFormatOpenAPIJSON,
		},
		{
			name:        "no content-type yaml body detection",
			contentType: "",
			body:        "openapi: '3.0.0'\npaths: {}",
			want:        SpecFormatOpenAPIYAML,
		},
		{
			name:        "no content-type html body detection",
			contentType: "",
			body:        "<html><body>docs</body></html>",
			want:        SpecFormatHTML,
		},
		{
			name:        "unknown format",
			contentType: "text/plain",
			body:        "just some random text",
			want:        SpecFormatUnknown,
		},
		{
			name:        "empty body",
			contentType: "",
			body:        "",
			want:        SpecFormatUnknown,
		},
		{
			name:        "json content-type but not openapi",
			contentType: "application/json",
			body:        `{"name": "not an api spec"}`,
			want:        SpecFormatUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectFormat(tt.contentType, []byte(tt.body))
			if got != tt.want {
				t.Errorf("detectFormat(%q, ...) = %q, want %q", tt.contentType, got, tt.want)
			}
		})
	}
}
