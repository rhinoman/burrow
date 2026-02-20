// Package config handles loading, validating, and resolving Burrow configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level Burrow configuration loaded from config.yaml.
type Config struct {
	Services  []ServiceConfig  `yaml:"services"`
	LLM       LLMConfig        `yaml:"llm"`
	Privacy   PrivacyConfig    `yaml:"privacy"`
	Apps      AppsConfig       `yaml:"apps"`
	Rendering RenderingConfig  `yaml:"rendering"`
	Context   ContextConfig    `yaml:"context"`
}

// ServiceConfig defines an external service endpoint.
type ServiceConfig struct {
	Name     string       `yaml:"name"`
	Type     string       `yaml:"type"` // rest | mcp
	Endpoint string       `yaml:"endpoint"`
	Auth     AuthConfig   `yaml:"auth"`
	Spec     string       `yaml:"spec,omitempty"` // Reserved: OpenAPI/Swagger spec URL for auto-discovery (Phase 4)
	Tools    []ToolConfig `yaml:"tools,omitempty"`
	CacheTTL int          `yaml:"cache_ttl,omitempty"`
}

// AuthConfig defines how to authenticate with a service.
type AuthConfig struct {
	Method   string `yaml:"method"` // api_key | api_key_header | bearer | user_agent | none
	Key      string `yaml:"key,omitempty"`
	KeyParam string `yaml:"key_param,omitempty"` // query param name for api_key auth (default: "api_key")
	Token    string `yaml:"token,omitempty"`
	Value    string `yaml:"value,omitempty"`
}

// ToolConfig defines a named operation on a REST service.
type ToolConfig struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Method      string        `yaml:"method"`
	Path        string        `yaml:"path"`
	Body        string        `yaml:"body,omitempty"` // param name whose value becomes the POST body
	Params      []ParamConfig `yaml:"params,omitempty"`
}

// ParamConfig maps user-facing parameter names to API parameter names.
type ParamConfig struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"`
	MapsTo string `yaml:"maps_to"`
}

// LLMConfig defines available LLM providers.
type LLMConfig struct {
	Providers []ProviderConfig `yaml:"providers"`
}

// ProviderConfig defines a single LLM provider.
type ProviderConfig struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"` // ollama | llamacpp | openrouter | passthrough
	Endpoint string `yaml:"endpoint,omitempty"`
	APIKey   string `yaml:"api_key,omitempty"`
	Model    string `yaml:"model,omitempty"`
	Privacy  string `yaml:"privacy"`          // local | remote
	Timeout  int    `yaml:"timeout,omitempty"` // Seconds; 0 means default (Ollama: 300, OpenRouter: 120)
}

// PrivacyConfig defines privacy-related settings.
type PrivacyConfig struct {
	StripAttributionForRemote bool            `yaml:"strip_attribution_for_remote"`
	DefaultProxy              string          `yaml:"default_proxy,omitempty"`
	Routes                    []RouteConfig   `yaml:"routes,omitempty"`
	MinimizeRequests          bool            `yaml:"minimize_requests"`
	StripReferrers            bool            `yaml:"strip_referrers"`
	RandomizeUserAgent        bool            `yaml:"randomize_user_agent"`
}

// RouteConfig defines per-service proxy routing.
type RouteConfig struct {
	Service string `yaml:"service"`
	Proxy   string `yaml:"proxy"`
}

// AppsConfig defines system app handoff targets.
type AppsConfig struct {
	Email   string `yaml:"email,omitempty"`
	Browser string `yaml:"browser,omitempty"`
	Editor  string `yaml:"editor,omitempty"`
	Media   string `yaml:"media,omitempty"`
}

// RenderingConfig defines terminal rendering behavior.
type RenderingConfig struct {
	Images string `yaml:"images,omitempty"` // auto | inline | external | text
}

// ContextConfig defines context ledger retention.
type ContextConfig struct {
	Retention RetentionConfig `yaml:"retention,omitempty"`
}

// RetentionConfig defines how long to keep different types of data.
type RetentionConfig struct {
	Reports    string `yaml:"reports,omitempty"`
	RawResults int    `yaml:"raw_results,omitempty"`
	Sessions   int    `yaml:"sessions,omitempty"`
}

// DeepCopy returns a deep copy of the config by round-tripping through YAML.
func (c *Config) DeepCopy() *Config {
	data, err := yaml.Marshal(c)
	if err != nil {
		// Config was already valid YAML — this should never happen.
		panic(fmt.Sprintf("config marshal during DeepCopy: %v", err))
	}
	var copy Config
	if err := yaml.Unmarshal(data, &copy); err != nil {
		panic(fmt.Sprintf("config unmarshal during DeepCopy: %v", err))
	}
	return &copy
}

// BurrowDir returns the path to the Burrow data directory (~/.burrow/),
// creating it if it doesn't exist. Override with BURROW_DIR env var.
func BurrowDir() (string, error) {
	dir := os.Getenv("BURROW_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("determining home directory: %w", err)
		}
		dir = filepath.Join(home, ".burrow")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating burrow directory: %w", err)
	}
	return dir, nil
}

// Load reads and parses the config.yaml from the Burrow directory.
func Load(burrowDir string) (*Config, error) {
	path := filepath.Join(burrowDir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// ResolveEnvVars expands ${VAR} references in credential fields from the environment.
// Only auth-related fields are resolved — credentials are never stored expanded.
func ResolveEnvVars(cfg *Config) {
	for i := range cfg.Services {
		cfg.Services[i].Auth.Key = expandEnv(cfg.Services[i].Auth.Key)
		cfg.Services[i].Auth.Token = expandEnv(cfg.Services[i].Auth.Token)
		cfg.Services[i].Auth.Value = expandEnv(cfg.Services[i].Auth.Value)
	}
	for i := range cfg.LLM.Providers {
		cfg.LLM.Providers[i].APIKey = expandEnv(cfg.LLM.Providers[i].APIKey)
	}
}

func expandEnv(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match // leave unresolved if env var not set
	})
}

// Save marshals the config to YAML and writes it to config.yaml in the Burrow directory.
// Creates the parent directory if it doesn't exist.
func Save(burrowDir string, cfg *Config) error {
	if err := os.MkdirAll(burrowDir, 0o755); err != nil {
		return fmt.Errorf("creating burrow directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	header := "# Burrow configuration — https://github.com/jcadam/burrow\n# Edit this file directly or use: gd configure\n\n"
	path := filepath.Join(burrowDir, "config.yaml")
	return os.WriteFile(path, []byte(header+string(data)), 0o644)
}

// Validate checks internal consistency of the config.
func Validate(cfg *Config) error {
	names := make(map[string]bool)
	for _, svc := range cfg.Services {
		if svc.Name == "" {
			return fmt.Errorf("service missing name")
		}
		if names[svc.Name] {
			return fmt.Errorf("duplicate service name: %q", svc.Name)
		}
		names[svc.Name] = true

		switch svc.Type {
		case "rest", "mcp":
			// valid
		case "":
			return fmt.Errorf("service %q missing type", svc.Name)
		default:
			return fmt.Errorf("service %q has unknown type %q", svc.Name, svc.Type)
		}

		if svc.Endpoint == "" {
			return fmt.Errorf("service %q missing endpoint", svc.Name)
		}

		switch svc.Auth.Method {
		case "api_key", "api_key_header":
			if svc.Auth.Key == "" {
				return fmt.Errorf("service %q auth method %q requires a key", svc.Name, svc.Auth.Method)
			}
		case "bearer":
			if svc.Auth.Token == "" {
				return fmt.Errorf("service %q auth method \"bearer\" requires a token", svc.Name)
			}
		case "user_agent":
			if svc.Auth.Value == "" {
				return fmt.Errorf("service %q auth method \"user_agent\" requires a value", svc.Name)
			}
		case "none", "":
			// valid — no credentials needed
		default:
			return fmt.Errorf("service %q has unknown auth method %q", svc.Name, svc.Auth.Method)
		}
	}

	// Validate tool paths
	for _, svc := range cfg.Services {
		for _, tool := range svc.Tools {
			if tool.Path != "" && !strings.HasPrefix(tool.Path, "/") {
				return fmt.Errorf("service %q tool %q has relative path %q (must start with /)", svc.Name, tool.Name, tool.Path)
			}
		}
	}

	// Validate LLM providers
	provNames := make(map[string]bool)
	for _, prov := range cfg.LLM.Providers {
		if prov.Name == "" {
			return fmt.Errorf("LLM provider missing name")
		}
		if provNames[prov.Name] {
			return fmt.Errorf("duplicate LLM provider name: %q", prov.Name)
		}
		provNames[prov.Name] = true

		switch prov.Type {
		case "ollama", "openrouter", "passthrough", "":
			// valid
		default:
			return fmt.Errorf("LLM provider %q has unknown type %q", prov.Name, prov.Type)
		}

		switch prov.Privacy {
		case "local", "remote", "":
			// valid
		default:
			return fmt.Errorf("LLM provider %q has unknown privacy %q", prov.Name, prov.Privacy)
		}
	}

	if cfg.Rendering.Images != "" {
		switch strings.ToLower(cfg.Rendering.Images) {
		case "auto", "inline", "external", "text":
			// valid
		default:
			return fmt.Errorf("invalid rendering.images value %q", cfg.Rendering.Images)
		}
	}

	return nil
}
