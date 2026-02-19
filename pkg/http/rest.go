// Package http provides a REST service adapter for Burrow's service interface.
package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/services"
)

// RESTService implements services.Service for REST API endpoints.
type RESTService struct {
	name     string
	endpoint string
	auth     config.AuthConfig
	tools    map[string]config.ToolConfig
	client   *http.Client
}

// NewRESTService creates a REST service from config. Each service gets its own
// http.Client to support per-service proxy routing in the future.
func NewRESTService(cfg config.ServiceConfig) *RESTService {
	tools := make(map[string]config.ToolConfig, len(cfg.Tools))
	for _, tool := range cfg.Tools {
		tools[tool.Name] = tool
	}
	return &RESTService{
		name:     cfg.Name,
		endpoint: cfg.Endpoint,
		auth:     cfg.Auth,
		tools:    tools,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (r *RESTService) Name() string { return r.name }

// Execute runs a named tool against the REST endpoint.
func (r *RESTService) Execute(ctx context.Context, tool string, params map[string]string) (*services.Result, error) {
	tc, ok := r.tools[tool]
	if !ok {
		return nil, fmt.Errorf("service %q has no tool %q", r.name, tool)
	}

	reqURL, err := r.buildURL(tc, params)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, tc.Method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	r.applyAuth(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return &services.Result{
			Service:   r.name,
			Tool:      tool,
			Timestamp: time.Now().UTC(),
			Error:     err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	// Limit response body to 10MB to prevent OOM from misbehaving APIs
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return &services.Result{
			Service:   r.name,
			Tool:      tool,
			Timestamp: time.Now().UTC(),
			Error:     fmt.Sprintf("reading response: %v", err),
		}, nil
	}

	if resp.StatusCode >= 400 {
		return &services.Result{
			Service:   r.name,
			Tool:      tool,
			Data:      body,
			Timestamp: time.Now().UTC(),
			Error:     fmt.Sprintf("HTTP %d", resp.StatusCode),
		}, nil
	}

	return &services.Result{
		Service:   r.name,
		Tool:      tool,
		Data:      body,
		Timestamp: time.Now().UTC(),
	}, nil
}

func (r *RESTService) buildURL(tc config.ToolConfig, params map[string]string) (string, error) {
	base, err := url.Parse(r.endpoint)
	if err != nil {
		return "", err
	}

	// Tool paths are absolute from the host root (e.g., /v2/search), not relative
	// to the endpoint path. ResolveReference handles this correctly.
	ref, err := url.Parse(tc.Path)
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(ref)

	query := url.Values{}
	for _, pc := range tc.Params {
		if val, ok := params[pc.Name]; ok {
			query.Set(pc.MapsTo, val)
		}
	}
	resolved.RawQuery = query.Encode()
	return resolved.String(), nil
}

func (r *RESTService) applyAuth(req *http.Request) {
	switch r.auth.Method {
	case "api_key":
		paramName := r.auth.KeyParam
		if paramName == "" {
			paramName = "api_key"
		}
		q := req.URL.Query()
		q.Set(paramName, r.auth.Key)
		req.URL.RawQuery = q.Encode()
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+r.auth.Token)
	case "user_agent":
		req.Header.Set("User-Agent", r.auth.Value)
	}
}
