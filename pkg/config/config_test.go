package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testConfig = `
services:
  - name: sam-gov
    type: rest
    endpoint: https://api.sam.gov
    auth:
      method: api_key
      key: ${SAM_API_KEY}
    tools:
      - name: search_opportunities
        description: "Search active contract opportunities"
        method: GET
        path: /opportunities/v2/search
        params:
          - name: naics
            type: string
            maps_to: api.ncode

  - name: edgar
    type: rest
    endpoint: https://efts.sec.gov
    auth:
      method: user_agent
      value: "burrow/1.0 contact@example.com"

llm:
  providers:
    - name: local/qwen-14b
      type: ollama
      endpoint: http://localhost:11434
      model: qwen2.5:14b
      privacy: local
    - name: none
      type: passthrough
      privacy: local

privacy:
  strip_attribution_for_remote: true
  minimize_requests: true
  strip_referrers: true

rendering:
  images: auto

apps:
  email: default
  browser: default
`

func writeTestConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, testConfig)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(cfg.Services))
	}
	if cfg.Services[0].Name != "sam-gov" {
		t.Errorf("expected service name sam-gov, got %q", cfg.Services[0].Name)
	}
	if cfg.Services[0].Auth.Method != "api_key" {
		t.Errorf("expected auth method api_key, got %q", cfg.Services[0].Auth.Method)
	}
	if len(cfg.Services[0].Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(cfg.Services[0].Tools))
	}
	if cfg.Services[0].Tools[0].Name != "search_opportunities" {
		t.Errorf("expected tool name search_opportunities, got %q", cfg.Services[0].Tools[0].Name)
	}
	if len(cfg.LLM.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(cfg.LLM.Providers))
	}
	if !cfg.Privacy.StripAttributionForRemote {
		t.Error("expected strip_attribution_for_remote to be true")
	}
	if cfg.Rendering.Images != "auto" {
		t.Errorf("expected rendering.images auto, got %q", cfg.Rendering.Images)
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error loading missing config")
	}
}

func TestResolveEnvVars(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, testConfig)

	t.Setenv("SAM_API_KEY", "secret-key-123")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Before resolution, key should have the env var reference
	if cfg.Services[0].Auth.Key != "${SAM_API_KEY}" {
		t.Errorf("expected unresolved ${SAM_API_KEY}, got %q", cfg.Services[0].Auth.Key)
	}

	ResolveEnvVars(cfg)

	if cfg.Services[0].Auth.Key != "secret-key-123" {
		t.Errorf("expected resolved key secret-key-123, got %q", cfg.Services[0].Auth.Key)
	}

	// Non-env values should be unchanged
	if cfg.Services[1].Auth.Value != "burrow/1.0 contact@example.com" {
		t.Errorf("expected unchanged user_agent value, got %q", cfg.Services[1].Auth.Value)
	}
}

func TestResolveEnvVarsUnset(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, testConfig)

	// t.Setenv restores the original value when the test finishes
	t.Setenv("SAM_API_KEY", "")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	ResolveEnvVars(cfg)

	// Env var is set but empty — should resolve to empty string
	if cfg.Services[0].Auth.Key != "" {
		t.Errorf("expected empty resolved key, got %q", cfg.Services[0].Auth.Key)
	}
}

func TestValidate(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, testConfig)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateMissingName(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{{Type: "rest", Endpoint: "http://example.com"}},
	}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation error for missing name")
	}
}

func TestValidateDuplicateName(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{
			{Name: "svc", Type: "rest", Endpoint: "http://a.com"},
			{Name: "svc", Type: "rest", Endpoint: "http://b.com"},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation error for duplicate name")
	}
}

func TestValidateBadType(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{
			{Name: "svc", Type: "graphql", Endpoint: "http://a.com"},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation error for unknown type")
	}
}

func TestValidateBadRenderingImages(t *testing.T) {
	cfg := &Config{
		Rendering: RenderingConfig{Images: "hologram"},
	}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation error for bad rendering.images")
	}
}

func TestValidateRelativeToolPath(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{
			{
				Name:     "svc",
				Type:     "rest",
				Endpoint: "http://example.com",
				Tools: []ToolConfig{
					{Name: "search", Method: "GET", Path: "search"},
				},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for relative tool path")
	}
	if !strings.Contains(err.Error(), "relative path") {
		t.Errorf("expected relative path error, got: %v", err)
	}
}

func TestValidateLLMProviderMissingName(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			Providers: []ProviderConfig{{Type: "ollama"}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing provider name")
	}
	if !strings.Contains(err.Error(), "missing name") {
		t.Errorf("expected missing name error, got: %v", err)
	}
}

func TestValidateLLMProviderDuplicateName(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			Providers: []ProviderConfig{
				{Name: "llm-a", Type: "ollama"},
				{Name: "llm-a", Type: "ollama"},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for duplicate provider name")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate error, got: %v", err)
	}
}

func TestValidateLLMProviderUnknownType(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			Providers: []ProviderConfig{
				{Name: "llm-a", Type: "chatgpt"},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for unknown provider type")
	}
	if !strings.Contains(err.Error(), "unknown type") {
		t.Errorf("expected unknown type error, got: %v", err)
	}
}

func TestValidateLLMProviderUnknownPrivacy(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			Providers: []ProviderConfig{
				{Name: "llm-a", Type: "ollama", Privacy: "hybrid"},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for unknown privacy")
	}
	if !strings.Contains(err.Error(), "unknown privacy") {
		t.Errorf("expected unknown privacy error, got: %v", err)
	}
}

func TestBurrowDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BURROW_DIR", filepath.Join(dir, "custom-burrow"))

	got, err := BurrowDir()
	if err != nil {
		t.Fatalf("BurrowDir: %v", err)
	}
	if got != filepath.Join(dir, "custom-burrow") {
		t.Errorf("expected custom dir, got %q", got)
	}
	// Should have been created
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}
