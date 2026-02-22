package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jcadam/burrow/pkg/services"
)

// MCPService wraps an MCP client as a services.Service.
type MCPService struct {
	name     string
	endpoint string
	client   *Client
	tools    map[string]ToolInfo
	initOnce sync.Once
	initErr  error
}

// NewMCPService creates an MCP service adapter. The httpClient should be built
// with NewHTTPClient to ensure per-service transport isolation and auth injection.
func NewMCPService(name string, endpoint string, httpClient *http.Client) *MCPService {
	return &MCPService{
		name:     name,
		endpoint: endpoint,
		client:   NewClient(endpoint, httpClient),
	}
}

func (m *MCPService) Name() string { return m.name }

// Execute calls a tool on the MCP server. On first call, initializes the
// connection and discovers available tools.
func (m *MCPService) Execute(ctx context.Context, tool string, params map[string]string) (*services.Result, error) {
	m.initOnce.Do(func() {
		m.initErr = m.init(ctx)
	})
	if m.initErr != nil {
		return nil, fmt.Errorf("MCP init for %q: %w", m.name, m.initErr)
	}

	if _, ok := m.tools[tool]; !ok {
		available := make([]string, 0, len(m.tools))
		for name := range m.tools {
			available = append(available, name)
		}
		return nil, fmt.Errorf("MCP service %q has no tool %q (available: %s)", m.name, tool, strings.Join(available, ", "))
	}

	// Convert map[string]string to map[string]any (MCP uses any-typed args).
	args := make(map[string]any, len(params))
	for k, v := range params {
		args[k] = v
	}

	result, err := m.client.CallTool(ctx, tool, args)
	if err != nil {
		return &services.Result{
			Service:   m.name,
			Tool:      tool,
			URL:       m.endpoint,
			Timestamp: time.Now().UTC(),
			Error:     err.Error(),
		}, nil
	}

	if result.IsError {
		errMsg := extractText(result)
		return &services.Result{
			Service:   m.name,
			Tool:      tool,
			URL:       m.endpoint,
			Timestamp: time.Now().UTC(),
			Error:     errMsg,
		}, nil
	}

	data := extractText(result)
	return &services.Result{
		Service:   m.name,
		Tool:      tool,
		Data:      []byte(data),
		URL:       m.endpoint,
		Timestamp: time.Now().UTC(),
	}, nil
}

func (m *MCPService) init(ctx context.Context) error {
	if _, err := m.client.Initialize(ctx); err != nil {
		return err
	}
	tools, err := m.client.ListTools(ctx)
	if err != nil {
		return err
	}
	m.tools = make(map[string]ToolInfo, len(tools))
	for _, t := range tools {
		m.tools[t.Name] = t
	}
	return nil
}

// extractText concatenates all text content blocks from a tool result.
func extractText(result *ToolResult) string {
	var parts []string
	for _, block := range result.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}
