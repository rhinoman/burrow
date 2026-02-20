package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jcadam/burrow/pkg/config"
)

// newTestMCPServer creates an httptest server that speaks MCP JSON-RPC.
func newTestMCPServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "test-session-42")

		switch req.Method {
		case "initialize":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": protocolVersion,
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "test-server", "version": "1.0"},
				},
			})

		case "tools/list":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "search", "description": "Search for items"},
						{"name": "fetch", "description": "Fetch a resource"},
					},
				},
			})

		case "tools/call":
			params, _ := req.Params.(map[string]any)
			toolName, _ := params["name"].(string)

			if toolName == "error-tool" {
				json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result": map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": "something went wrong"},
						},
						"isError": true,
					},
				})
				return
			}

			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": `{"results": [{"title": "Item A"}]}`},
					},
					"isError": false,
				},
			})

		default:
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
}

func TestClientInitialize(t *testing.T) {
	srv := newTestMCPServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
	result, err := client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if result.ProtocolVersion != protocolVersion {
		t.Errorf("expected protocol version %q, got %q", protocolVersion, result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "test-server" {
		t.Errorf("expected server name test-server, got %q", result.ServerInfo.Name)
	}
}

func TestClientSessionID(t *testing.T) {
	var receivedSessionID string
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		receivedSessionID = r.Header.Get("Mcp-Session-Id")

		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "session-abc")
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "test", "version": "1"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, &http.Client{Timeout: 5 * time.Second})

	// First call should not have a session ID.
	client.Initialize(context.Background())
	if receivedSessionID != "" {
		t.Errorf("first call should have no session ID, got %q", receivedSessionID)
	}

	// Second call should send the session ID from the first response.
	client.Initialize(context.Background())
	if receivedSessionID != "session-abc" {
		t.Errorf("expected session ID session-abc, got %q", receivedSessionID)
	}
}

func TestClientListTools(t *testing.T) {
	srv := newTestMCPServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "search" || tools[1].Name != "fetch" {
		t.Errorf("unexpected tools: %+v", tools)
	}
}

func TestClientCallTool(t *testing.T) {
	srv := newTestMCPServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
	result, err := client.CallTool(context.Background(), "search", map[string]any{"q": "test"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatal("expected non-error result")
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("unexpected content: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, "Item A") {
		t.Errorf("expected Item A in result text, got %q", result.Content[0].Text)
	}
}

func TestClientCallToolError(t *testing.T) {
	srv := newTestMCPServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
	result, err := client.CallTool(context.Background(), "error-tool", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if result.Content[0].Text != "something went wrong" {
		t.Errorf("expected error text, got %q", result.Content[0].Text)
	}
}

func TestClientJSONRPCError(t *testing.T) {
	srv := newTestMCPServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
	_, err := client.call(context.Background(), "nonexistent/method", nil)
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
	if !strings.Contains(err.Error(), "method not found") {
		t.Errorf("expected method not found error, got: %v", err)
	}
}

func TestClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, &http.Client{Timeout: 50 * time.Millisecond})
	_, err := client.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestClientHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
	_, err := client.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("expected HTTP 500 error, got: %v", err)
	}
}

func TestNewHTTPClientBearerAuth(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "test", "version": "1"},
			},
		})
	}))
	defer srv.Close()

	httpClient := NewHTTPClient(
		config.AuthConfig{Method: "bearer", Token: "my-secret-token"},
		nil, "",
	)

	client := NewClient(srv.URL, httpClient)
	_, err := client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if receivedAuth != "Bearer my-secret-token" {
		t.Errorf("expected Bearer auth header, got %q", receivedAuth)
	}
}

func TestNewHTTPClientAPIKeyHeader(t *testing.T) {
	var receivedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "test", "version": "1"},
			},
		})
	}))
	defer srv.Close()

	httpClient := NewHTTPClient(
		config.AuthConfig{Method: "api_key_header", Key: "key-123"},
		nil, "",
	)

	client := NewClient(srv.URL, httpClient)
	_, err := client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if receivedKey != "key-123" {
		t.Errorf("expected X-API-Key header key-123, got %q", receivedKey)
	}
}
