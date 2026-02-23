package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/privacy"
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
	}, nil, "")

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
	}, nil, "")

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
	}, nil, "")

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
	}, nil, "")

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
	}, nil, "")

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecuteUserAgentAuthWithPrivacy(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != "burrow/1.0 test@example.com" {
			t.Errorf("privacy should preserve auth UA, got %q", ua)
		}
		// Sentinel must not leak
		if r.Header.Get("X-Burrow-Preserve-UA") != "" {
			t.Error("sentinel header leaked to server")
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	privCfg := &privacy.Config{RandomizeUserAgent: true, StripReferrers: true}
	svc := NewRESTService(config.ServiceConfig{
		Name:     "ua-priv-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "user_agent", Value: "burrow/1.0 test@example.com"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/data"},
		},
	}, privCfg, "")

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
	}, nil, "")

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result.Error, "HTTP 404") {
		t.Errorf("expected HTTP 404 in error, got %q", result.Error)
	}
	if !strings.Contains(result.Error, "not found") {
		t.Errorf("expected error body in error message, got %q", result.Error)
	}
}

func TestExecuteAbsoluteToolPath(t *testing.T) {
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
	}, nil, "")

	result, err := svc.Execute(context.Background(), "search", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestBuildURLPreservesExistingQueryParams(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// The tool path has ?type=active, and the mapped param adds naics
		if got := r.URL.Query().Get("type"); got != "active" {
			t.Errorf("expected existing param type=active, got %q", got)
		}
		if got := r.URL.Query().Get("api.ncode"); got != "541370" {
			t.Errorf("expected mapped param api.ncode=541370, got %q", got)
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "query-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "search",
				Method: "GET",
				Path:   "/search?type=active",
				Params: []config.ParamConfig{
					{Name: "naics", Type: "string", MapsTo: "api.ncode"},
				},
			},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "search", map[string]string{
		"naics": "541370",
	})
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
	}, nil, "")

	_, err := svc.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestExecutePOSTWithBody(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"term": "test"}` {
			t.Errorf("expected body, got %q", string(body))
		}
		// Body param should not appear in query string
		if r.URL.Query().Get("query") != "" {
			t.Error("body param should not appear in query string")
		}
		w.Write([]byte(`{"ok": true}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "post-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "search",
				Method: "POST",
				Path:   "/v1/search",
				Body:   "query",
				Params: []config.ParamConfig{
					{Name: "query", Type: "string", MapsTo: "query"},
				},
			},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "search", map[string]string{
		"query": `{"term": "test"}`,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecutePOSTWithoutBody(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// No body param configured — body should be nil/empty
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Errorf("expected empty body, got %q", string(body))
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "post-no-body",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{Name: "action", Method: "POST", Path: "/v1/action"},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "action", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecutePOSTBodyParamMissing(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// Body param configured but not in params map — should be nil body
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Errorf("expected empty body when param missing, got %q", string(body))
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "post-missing",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{Name: "search", Method: "POST", Path: "/v1/search", Body: "query"},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "search", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestTransportIsolation(t *testing.T) {
	svcA := NewRESTService(config.ServiceConfig{
		Name:     "svc-a",
		Endpoint: "http://localhost",
		Auth:     config.AuthConfig{Method: "none"},
	}, nil, "")
	svcB := NewRESTService(config.ServiceConfig{
		Name:     "svc-b",
		Endpoint: "http://localhost",
		Auth:     config.AuthConfig{Method: "none"},
	}, nil, "")

	tA := svcA.client.Transport
	tB := svcB.client.Transport
	if tA == nil || tB == nil {
		t.Fatal("expected non-nil transports")
	}
	if tA == tB {
		t.Error("services must have distinct transports for compartmentalization")
	}
}

func TestTransportIsolationWithPrivacy(t *testing.T) {
	privCfg := &privacy.Config{RandomizeUserAgent: true}
	svcA := NewRESTService(config.ServiceConfig{
		Name:     "svc-a",
		Endpoint: "http://localhost",
		Auth:     config.AuthConfig{Method: "none"},
	}, privCfg, "")
	svcB := NewRESTService(config.ServiceConfig{
		Name:     "svc-b",
		Endpoint: "http://localhost",
		Auth:     config.AuthConfig{Method: "none"},
	}, privCfg, "")

	if svcA.client.Transport == svcB.client.Transport {
		t.Error("services must have distinct transports even with privacy config")
	}
}

func TestExecuteAPIKeyHeaderAuth(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "secret456" {
			t.Errorf("expected X-API-Key header secret456, got %q", got)
		}
		// Key must NOT appear in the URL
		if r.URL.Query().Get("X-API-Key") != "" {
			t.Error("api_key_header key should not appear in query string")
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "header-auth-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "api_key_header", Key: "secret456"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/data"},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecuteAPIKeyHeaderCustomName(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Custom-Auth"); got != "key789" {
			t.Errorf("expected X-Custom-Auth header key789, got %q", got)
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "custom-header-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "api_key_header", Key: "key789", KeyParam: "X-Custom-Auth"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/data"},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestPrivacyTransportApplied(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if strings.Contains(ua, "Go-http-client") {
			t.Errorf("expected rotated UA, got Go default: %q", ua)
		}
		if r.Header.Get("Referer") != "" {
			t.Error("expected Referer stripped")
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	privCfg := &privacy.Config{
		RandomizeUserAgent: true,
		StripReferrers:     true,
		MinimizeRequests:   true,
	}
	svc := NewRESTService(config.ServiceConfig{
		Name:     "priv-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/data"},
		},
	}, privCfg, "")

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecuteWithExpandFunc(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// Verify the template ref was expanded in the path
		if got := r.URL.Query().Get("latitude"); got != "61.22" {
			t.Errorf("expected latitude=61.22, got %q", got)
		}
		if got := r.URL.Query().Get("longitude"); got != "-149.90" {
			t.Errorf("expected longitude=-149.90, got %q", got)
		}
		w.Write([]byte(`{"ok": true}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "expand-test",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "forecast",
				Method: "GET",
				Path:   "/v1/forecast?latitude=LATITUDE&longitude=LONGITUDE",
			},
		},
	}, nil, "")

	// Set an expand func that simulates profile template expansion
	svc.SetExpandFunc(func(s string) (string, error) {
		s = strings.ReplaceAll(s, "LATITUDE", "61.22")
		s = strings.ReplaceAll(s, "LONGITUDE", "-149.90")
		return s, nil
	})

	result, err := svc.Execute(context.Background(), "forecast", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecutePathParam(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/42" {
			t.Errorf("expected path /users/42, got %q", r.URL.Path)
		}
		w.Write([]byte(`{"id": 42}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "path-param-test",
		Type:     "rest",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "get_user",
				Method: "GET",
				Path:   "/users/{id}",
				Params: []config.ParamConfig{
					{Name: "user_id", Type: "string", MapsTo: "id", In: "path"},
				},
			},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "get_user", map[string]string{
		"user_id": "42",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecutePathParamMixed(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/42/posts" {
			t.Errorf("expected path /users/42/posts, got %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "10" {
			t.Errorf("expected limit=10, got %q", got)
		}
		w.Write([]byte(`[]`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "mixed-test",
		Type:     "rest",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "get_user_posts",
				Method: "GET",
				Path:   "/users/{id}/posts",
				Params: []config.ParamConfig{
					{Name: "user_id", Type: "string", MapsTo: "id", In: "path"},
					{Name: "limit", Type: "string", MapsTo: "limit"},
				},
			},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "get_user_posts", map[string]string{
		"user_id": "42",
		"limit":   "10",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecuteMultiplePathParams(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orgs/acme/repos/burrow/issues" {
			t.Errorf("expected path /orgs/acme/repos/burrow/issues, got %q", r.URL.Path)
		}
		w.Write([]byte(`[]`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "multi-path-test",
		Type:     "rest",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "list_issues",
				Method: "GET",
				Path:   "/orgs/{org}/repos/{repo}/issues",
				Params: []config.ParamConfig{
					{Name: "organization", Type: "string", MapsTo: "org", In: "path"},
					{Name: "repository", Type: "string", MapsTo: "repo", In: "path"},
				},
			},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "list_issues", map[string]string{
		"organization": "acme",
		"repository":   "burrow",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecutePathParamMissing(t *testing.T) {
	svc := NewRESTService(config.ServiceConfig{
		Name:     "missing-path-test",
		Type:     "rest",
		Endpoint: "http://localhost",
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "get_user",
				Method: "GET",
				Path:   "/users/{id}",
				Params: []config.ParamConfig{
					{Name: "user_id", Type: "string", MapsTo: "id", In: "path"},
				},
			},
		},
	}, nil, "")

	_, err := svc.Execute(context.Background(), "get_user", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing path param")
	}
	if !strings.Contains(err.Error(), "missing required path parameter") {
		t.Errorf("expected missing path param error, got: %v", err)
	}
}

func TestExecutePathParamURLEncoding(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// url.PathEscape encodes spaces as %20 and slashes as %2F
		if r.URL.RawPath != "/users/hello%20world%2Ffoo" && r.URL.Path != "/users/hello world/foo" {
			t.Errorf("expected encoded path, got Path=%q RawPath=%q", r.URL.Path, r.URL.RawPath)
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "encode-test",
		Type:     "rest",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "get_user",
				Method: "GET",
				Path:   "/users/{id}",
				Params: []config.ParamConfig{
					{Name: "user_id", Type: "string", MapsTo: "id", In: "path"},
				},
			},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "get_user", map[string]string{
		"user_id": "hello world/foo",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecutePathParamBackwardCompat(t *testing.T) {
	// No In field — all params should be query params as before.
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("id"); got != "42" {
			t.Errorf("expected query id=42, got %q", got)
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "compat-test",
		Type:     "rest",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "get_user",
				Method: "GET",
				Path:   "/users",
				Params: []config.ParamConfig{
					{Name: "user_id", Type: "string", MapsTo: "id"},
				},
			},
		},
	}, nil, "")

	result, err := svc.Execute(context.Background(), "get_user", map[string]string{
		"user_id": "42",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestExecutePathParamWithExpandFunc(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// expandFunc resolves {{profile "org"}} to "acme", then path param replaces {id}
		if r.URL.Path != "/orgs/acme/users/42" {
			t.Errorf("expected path /orgs/acme/users/42, got %q", r.URL.Path)
		}
		w.Write([]byte(`{}`))
	})
	defer srv.Close()

	svc := NewRESTService(config.ServiceConfig{
		Name:     "expand-path-test",
		Type:     "rest",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{
				Name:   "get_user",
				Method: "GET",
				Path:   `/orgs/ORGNAME/users/{id}`,
				Params: []config.ParamConfig{
					{Name: "user_id", Type: "string", MapsTo: "id", In: "path"},
				},
			},
		},
	}, nil, "")

	svc.SetExpandFunc(func(s string) (string, error) {
		return strings.ReplaceAll(s, "ORGNAME", "acme"), nil
	})

	result, err := svc.Execute(context.Background(), "get_user", map[string]string{
		"user_id": "42",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}
}

func TestProxyURLSetOnTransport(t *testing.T) {
	svc := NewRESTService(config.ServiceConfig{
		Name:     "proxy-test",
		Endpoint: "http://localhost",
		Auth:     config.AuthConfig{Method: "none"},
	}, nil, "socks5h://127.0.0.1:9050")

	transport := svc.client.Transport.(*http.Transport)
	if transport.Proxy == nil {
		t.Fatal("expected Proxy function to be set on transport")
	}

	// Verify the proxy function returns the correct URL.
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy function error: %v", err)
	}
	if proxyURL == nil {
		t.Fatal("expected non-nil proxy URL")
	}
	if got := proxyURL.String(); got != "socks5h://127.0.0.1:9050" {
		t.Errorf("expected socks5h://127.0.0.1:9050, got %q", got)
	}
}
