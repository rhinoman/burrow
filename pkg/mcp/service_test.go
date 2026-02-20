package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMCPServiceExecute(t *testing.T) {
	srv := newTestMCPServer(t)
	defer srv.Close()

	svc := NewMCPService("test-mcp", srv.URL, &http.Client{Timeout: 5 * time.Second})

	result, err := svc.Execute(context.Background(), "search", map[string]string{"q": "test"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Service != "test-mcp" {
		t.Errorf("expected service test-mcp, got %q", result.Service)
	}
	if result.Tool != "search" {
		t.Errorf("expected tool search, got %q", result.Tool)
	}
	if !strings.Contains(string(result.Data), "Item A") {
		t.Errorf("expected Item A in data, got %q", string(result.Data))
	}
}

func TestMCPServiceToolDiscovery(t *testing.T) {
	srv := newTestMCPServer(t)
	defer srv.Close()

	svc := NewMCPService("test-mcp", srv.URL, &http.Client{Timeout: 5 * time.Second})

	// First call triggers init + discovery.
	_, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute fetch: %v", err)
	}

	// Try a tool that doesn't exist.
	_, err = svc.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
	if !strings.Contains(err.Error(), "no tool \"nonexistent\"") {
		t.Errorf("expected no tool error, got: %v", err)
	}
}

func TestMCPServiceErrorResult(t *testing.T) {
	srv := newTestMCPServer(t)
	defer srv.Close()

	// Add "error-tool" to the mock server's tools list — we need to customize.
	customSrv := newMCPServerWithTools(t, []ToolInfo{
		{Name: "error-tool", Description: "tool that errors"},
	})
	defer customSrv.Close()

	svc := NewMCPService("test-mcp", customSrv.URL, &http.Client{Timeout: 5 * time.Second})

	result, err := svc.Execute(context.Background(), "error-tool", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error in result")
	}
	if !strings.Contains(result.Error, "something went wrong") {
		t.Errorf("expected error text, got: %s", result.Error)
	}
}

func TestMCPServiceInitFailureMemoized(t *testing.T) {
	callCount := 0
	srv := newHTTPTestServer(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		http.Error(w, "server down", http.StatusInternalServerError)
	})
	defer srv.Close()

	svc := NewMCPService("fail-mcp", srv.URL, &http.Client{Timeout: 5 * time.Second})

	// First call should fail.
	_, err1 := svc.Execute(context.Background(), "search", nil)
	if err1 == nil {
		t.Fatal("expected init failure")
	}

	// Second call should fail with same error without hitting server again.
	firstCount := callCount
	_, err2 := svc.Execute(context.Background(), "search", nil)
	if err2 == nil {
		t.Fatal("expected init failure on second call")
	}
	if callCount != firstCount {
		t.Errorf("expected init failure memoized (no additional server calls), but got %d after first %d", callCount, firstCount)
	}
}

func TestMCPServiceName(t *testing.T) {
	svc := NewMCPService("my-service", "http://localhost:9999", &http.Client{})
	if svc.Name() != "my-service" {
		t.Errorf("expected name my-service, got %q", svc.Name())
	}
}

// newMCPServerWithTools creates a mock MCP server with a specific tool list.
func newMCPServerWithTools(t *testing.T, tools []ToolInfo) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")

		switch req.Method {
		case "initialize":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": protocolVersion,
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]any{"name": "test", "version": "1"},
				},
			})
		case "tools/list":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"tools": tools},
			})
		case "tools/call":
			params, _ := req.Params.(map[string]any)
			toolName, _ := params["name"].(string)
			if toolName == "error-tool" {
				json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result": map[string]any{
						"content": []map[string]any{{"type": "text", "text": "something went wrong"}},
						"isError": true,
					},
				})
			} else {
				json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result": map[string]any{
						"content": []map[string]any{{"type": "text", "text": `{"ok": true}`}},
						"isError": false,
					},
				})
			}
		}
	}))
}

// newHTTPTestServer is a helper to create httptest servers without the t.Helper() coupling.
func newHTTPTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}
