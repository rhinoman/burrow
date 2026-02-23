// Package http provides a REST service adapter for Burrow's service interface.
package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/privacy"
	"github.com/jcadam/burrow/pkg/services"
)

// RESTService implements services.Service for REST API endpoints.
type RESTService struct {
	name       string
	endpoint   string
	auth       config.AuthConfig
	tools      map[string]config.ToolConfig
	client     *http.Client
	expandFunc func(string) (string, error) // optional template expansion
}

// SetExpandFunc sets a function for expanding template references in tool paths
// and other string fields before URL construction.
func (r *RESTService) SetExpandFunc(fn func(string) (string, error)) {
	r.expandFunc = fn
}

// NewRESTService creates a REST service from config. Each service gets its own
// http.Client to support per-service proxy routing. If privacyCfg is non-nil,
// a privacy transport is applied for referrer stripping, UA rotation, and
// request minimization. proxyURL sets the proxy on the underlying transport
// (empty string means direct connection).
func NewRESTService(cfg config.ServiceConfig, privacyCfg *privacy.Config, proxyURL string) *RESTService {
	tools := make(map[string]config.ToolConfig, len(cfg.Tools))
	for _, tool := range cfg.Tools {
		tools[tool.Name] = tool
	}

	// Each service gets its own transport to prevent connection pool sharing.
	// Shared pools break compartmentalization (spec §2.2).
	baseTransport := &http.Transport{}
	if proxyURL != "" {
		if parsed, err := url.Parse(proxyURL); err == nil && parsed != nil {
			baseTransport.Proxy = http.ProxyURL(parsed)
		}
	}
	var transport http.RoundTripper = baseTransport
	if privacyCfg != nil {
		transport = privacy.NewTransport(baseTransport, *privacyCfg)
	}

	return &RESTService{
		name:     cfg.Name,
		endpoint: cfg.Endpoint,
		auth:     cfg.Auth,
		tools:    tools,
		client:   &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

func (r *RESTService) Name() string { return r.name }

// Execute runs a named tool against the REST endpoint.
func (r *RESTService) Execute(ctx context.Context, tool string, params map[string]string) (*services.Result, error) {
	tc, ok := r.tools[tool]
	if !ok {
		return nil, fmt.Errorf("service %q has no tool %q", r.name, tool)
	}

	// Expand template references in the tool path before URL construction.
	path := tc.Path
	if r.expandFunc != nil {
		expanded, err := r.expandFunc(path)
		if err != nil {
			// Log warning, use partially-expanded path.
			fmt.Fprintf(os.Stderr, "warning: expanding path template: %v\n", err)
		}
		path = expanded
	}

	reqURL, err := r.buildURL(path, tc, params)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}

	var reqBody io.Reader
	if tc.Body != "" {
		if val, ok := params[tc.Body]; ok {
			reqBody = strings.NewReader(val)
		}
	}

	req, err := http.NewRequestWithContext(ctx, tc.Method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	r.applyAuth(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return &services.Result{
			Service:   r.name,
			Tool:      tool,
			URL:       reqURL,
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
			URL:       reqURL,
			Timestamp: time.Now().UTC(),
			Error:     fmt.Sprintf("reading response: %v", err),
		}, nil
	}

	if resp.StatusCode >= 400 {
		errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if len(body) > 0 {
			snippet := body
			if len(snippet) > 512 {
				snippet = snippet[:512]
			}
			errMsg += ": " + string(snippet)
		}
		return &services.Result{
			Service:   r.name,
			Tool:      tool,
			Data:      body,
			URL:       reqURL,
			Timestamp: time.Now().UTC(),
			Error:     errMsg,
		}, nil
	}

	return &services.Result{
		Service:   r.name,
		Tool:      tool,
		Data:      body,
		URL:       reqURL,
		Timestamp: time.Now().UTC(),
	}, nil
}

// unreplacedPlaceholder matches {name} placeholders remaining after substitution,
// excluding Go template expressions {{...}} which are handled by expandFunc.
var unreplacedPlaceholder = regexp.MustCompile(`\{([^{}]+)\}`)

func (r *RESTService) buildURL(path string, tc config.ToolConfig, params map[string]string) (string, error) {
	// Phase 1: Substitute path parameters.
	for _, pc := range tc.Params {
		if pc.In != "path" {
			continue
		}
		val, ok := params[pc.Name]
		if !ok {
			return "", fmt.Errorf("missing required path parameter %q", pc.Name)
		}
		placeholder := "{" + pc.MapsTo + "}"
		path = strings.ReplaceAll(path, placeholder, url.PathEscape(val))
	}

	// Phase 1b: Check for unreplaced path placeholders.
	if m := unreplacedPlaceholder.FindString(path); m != "" {
		return "", fmt.Errorf("unreplaced path placeholder %s in %q", m, path)
	}

	// Phase 2: Build URL with query parameters.
	base, err := url.Parse(r.endpoint)
	if err != nil {
		return "", err
	}

	// Tool paths are absolute from the host root (e.g., /v2/search), not relative
	// to the endpoint path. ResolveReference handles this correctly.
	ref, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(ref)

	// Merge mapped params with any existing query params from the tool path
	// (e.g., /search?type=active). Mapped params take precedence on collision.
	query := resolved.Query()
	for _, pc := range tc.Params {
		// Skip path params — already substituted above
		if pc.In == "path" {
			continue
		}
		// Skip the body param — it's sent as the request body, not a query param
		if tc.Body != "" && pc.Name == tc.Body {
			continue
		}
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
	case "api_key_header":
		headerName := r.auth.KeyParam
		if headerName == "" {
			headerName = "X-API-Key"
		}
		req.Header.Set(headerName, r.auth.Key)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+r.auth.Token)
	case "user_agent":
		req.Header.Set("User-Agent", r.auth.Value)
		// Signal the privacy transport to preserve this auth-required UA.
		req.Header.Set("X-Burrow-Preserve-UA", "true")
	}
}
