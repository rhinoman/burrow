package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jcadam/burrow/pkg/config"
)

func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func TestExecuteGET(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if got := r.URL.Query().Get("api.ncode"); got != "541370" {
			t.Errorf("expected naics param 541370, got %q", got)
		}
		w.Write([]byte(`{"results": [{"title": "test"}]}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "test-api",
		Type:     "rest",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "search",
				Method: "GET",
				Path:   "/search",
				Params: []config.ParamConfig{
					{Name: "naics", Type: "string", MapsTo: "api.ncode"},
				},
			},
		},
	})

	result, err := svc.Execute(context.Background(), "search", map[string]string{
		"naics": "541370",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
	if result.Service != "test-api" {
		t.Errorf("expected service test-api, got %q", result.Service)
	}
	if len(result.Data) == 0 {
		t.Error("expected non-empty data")
	}
}

func TestExecuteAPIKeyAuth(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("api_key"); got != "secret123" {
			t.Errorf("expected api_key secret123, got %q", got)
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "auth-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "api_key", Key: "secret123"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/data"},
		},
	})

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecuteAPIKeyCustomParam(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("subscription-key"); got != "sub123" {
			t.Errorf("expected subscription-key sub123, got %q", got)
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "custom-key-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "api_key", Key: "sub123", KeyParam: "subscription-key"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/data"},
		},
	})

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecuteBearerAuth(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer tok-456" {
			t.Errorf("expected Bearer tok-456, got %q", auth)
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "bearer-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "bearer", Token: "tok-456"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/data"},
		},
	})

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecuteUserAgentAuth(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != "burrow/1.0 test@example.com" {
			t.Errorf("expected custom user-agent, got %q", ua)
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "ua-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "user_agent", Value: "burrow/1.0 test@example.com"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/data"},
		},
	})

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecuteHTTPError(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not found"}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "error-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/missing"},
		},
	})

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "HTTP 404" {
		t.Errorf("expected HTTP 404 error, got %q", result.Error)
	}
}

func TestExecuteAbsoluteToolPath(t *testing.T) {
	// Tool paths are absolute from the host root. An endpoint with a path
	// component (e.g., /v2) is informational — the tool path determines
	// the actual request path.
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/search" {
			t.Errorf("expected path /v2/search, got %q", r.URL.Path)
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "path-test",
		Endpoint: srv.URL + "/v2",
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{Name: "search", Method: "GET", Path: "/v2/search"},
		},
	})

	result, err := svc.Execute(context.Background(), "search", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecuteUnknownTool(t *testing.T) {
	svc := NewRESTService(config.ServiceConfig{
		Name:     "test",
		Endpoint: "http://localhost",
		Auth:     config.AuthConfig{Method: "none"},
	})

	_, err := svc.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}
