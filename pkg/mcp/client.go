// Package mcp provides a JSON-RPC 2.0 client for the Model Context Protocol.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/privacy"
)

const protocolVersion = "2025-03-26"

// Client communicates with an MCP server over HTTP using JSON-RPC 2.0.
type Client struct {
	endpoint   string
	httpClient *http.Client
	mu         sync.Mutex // protects sessionID
	sessionID  string
	nextID     atomic.Int64
}

// NewClient creates a new MCP client for the given endpoint.
func NewClient(endpoint string, httpClient *http.Client) *Client {
	return &Client{
		endpoint:   endpoint,
		httpClient: httpClient,
	}
}

// ToolInfo describes a tool offered by the MCP server.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ToolResult holds the response from a tool call.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

// ContentBlock is one piece of content returned by a tool.
type ContentBlock struct {
	Type string `json:"type"` // "text" or "image"
	Text string `json:"text,omitempty"`
}

// InitializeResult holds the server's response to the initialize handshake.
type InitializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    any        `json:"capabilities"`
	ServerInfo      ServerInfo `json:"serverInfo"`
}

// ServerInfo identifies the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Initialize performs the MCP handshake. Must be called before other methods.
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "burrow",
			"version": "0.1",
		},
	}

	raw, err := c.call(ctx, "initialize", params)
	if err != nil {
		return nil, fmt.Errorf("MCP initialize: %w", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parsing initialize result: %w", err)
	}
	return &result, nil
}

// ListTools discovers available tools from the server.
func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	raw, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("MCP tools/list: %w", err)
	}

	var result struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parsing tools/list result: %w", err)
	}
	return result.Tools, nil
}

// CallTool invokes a named tool with arguments.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}

	raw, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("MCP tools/call %q: %w", name, err)
	}

	var result ToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parsing tools/call result: %w", err)
	}
	return &result, nil
}

// jsonRPCRequest is the JSON-RPC 2.0 request envelope.
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCResponse is the JSON-RPC 2.0 response envelope.
type jsonRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      int64            `json:"id"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *jsonRPCError    `json:"error,omitempty"`
}

// jsonRPCError is a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// call sends a JSON-RPC request and returns the result field from the response.
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send session ID if we have one from a previous response.
	c.mu.Lock()
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	c.mu.Unlock()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Store session ID from server.
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.mu.Lock()
		c.sessionID = sid
		c.mu.Unlock()
	}

	// Limit response body to 10MB.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("parsing JSON-RPC response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("JSON-RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

// NewHTTPClient builds an *http.Client suitable for MCP requests, with per-service
// transport isolation, auth injection, and optional privacy wrapping.
func NewHTTPClient(auth config.AuthConfig, privacyCfg *privacy.Config) *http.Client {
	var transport http.RoundTripper = &http.Transport{}
	if privacyCfg != nil {
		transport = privacy.NewTransport(&http.Transport{}, *privacyCfg)
	}
	transport = &authTransport{base: transport, auth: auth}
	return &http.Client{Timeout: 30 * time.Second, Transport: transport}
}

// authTransport injects auth headers into every request.
type authTransport struct {
	base http.RoundTripper
	auth config.AuthConfig
}

func (a *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	switch a.auth.Method {
	case "api_key_header":
		headerName := a.auth.KeyParam
		if headerName == "" {
			headerName = "X-API-Key"
		}
		r.Header.Set(headerName, a.auth.Key)
	case "bearer":
		r.Header.Set("Authorization", "Bearer "+a.auth.Token)
	case "api_key":
		paramName := a.auth.KeyParam
		if paramName == "" {
			paramName = "api_key"
		}
		q := r.URL.Query()
		q.Set(paramName, a.auth.Key)
		r.URL.RawQuery = q.Encode()
	}
	return a.base.RoundTrip(r)
}
