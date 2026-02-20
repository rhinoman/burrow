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

func TestValidateEmptyConfig(t *testing.T) {
	// An empty config is valid — represents a fresh install before user configuration.
	// gd init produces this, then the wizard adds services and providers.
	cfg := &Config{}
	if err := Validate(cfg); err != nil {
		t.Fatalf("empty config should be valid (fresh install): %v", err)
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

func TestValidateEmptyAPIKey(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{
			{Name: "svc", Type: "rest", Endpoint: "http://a.com", Auth: AuthConfig{Method: "api_key", Key: ""}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty api_key")
	}
	if !strings.Contains(err.Error(), "requires a key") {
		t.Errorf("expected 'requires a key' error, got: %v", err)
	}
}

func TestValidateEmptyAPIKeyHeader(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{
			{Name: "svc", Type: "rest", Endpoint: "http://a.com", Auth: AuthConfig{Method: "api_key_header", Key: ""}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation error for empty api_key_header key")
	}
}

func TestValidateEmptyBearerToken(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{
			{Name: "svc", Type: "rest", Endpoint: "http://a.com", Auth: AuthConfig{Method: "bearer", Token: ""}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty bearer token")
	}
	if !strings.Contains(err.Error(), "requires a token") {
		t.Errorf("expected 'requires a token' error, got: %v", err)
	}
}

func TestValidateEmptyUserAgentValue(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{
			{Name: "svc", Type: "rest", Endpoint: "http://a.com", Auth: AuthConfig{Method: "user_agent", Value: ""}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation error for empty user_agent value")
	}
}

func TestValidateEnvVarKeyPassesValidation(t *testing.T) {
	// ${ENV_VAR} is non-empty and should pass validation (resolved later)
	cfg := &Config{
		Services: []ServiceConfig{
			{Name: "svc", Type: "rest", Endpoint: "http://a.com", Auth: AuthConfig{Method: "api_key", Key: "${MY_KEY}"}},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("env var reference should pass validation: %v", err)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := &Config{
		Services: []ServiceConfig{
			{
				Name:     "test-svc",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth:     AuthConfig{Method: "api_key", Key: "${TEST_KEY}"},
				Tools: []ToolConfig{
					{Name: "search", Method: "GET", Path: "/search"},
				},
			},
		},
		LLM: LLMConfig{
			Providers: []ProviderConfig{
				{Name: "local/test", Type: "ollama", Model: "test:latest", Privacy: "local"},
			},
		},
		Privacy: PrivacyConfig{
			StripAttributionForRemote: true,
			StripReferrers:            true,
		},
		Apps: AppsConfig{Email: "thunderbird"},
	}

	if err := Save(dir, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}

	if len(loaded.Services) != 1 || loaded.Services[0].Name != "test-svc" {
		t.Errorf("service round-trip failed: %+v", loaded.Services)
	}
	if loaded.Services[0].Auth.Key != "${TEST_KEY}" {
		t.Errorf("auth key round-trip failed: %q", loaded.Services[0].Auth.Key)
	}
	if len(loaded.LLM.Providers) != 1 || loaded.LLM.Providers[0].Name != "local/test" {
		t.Errorf("provider round-trip failed: %+v", loaded.LLM.Providers)
	}
	if !loaded.Privacy.StripAttributionForRemote {
		t.Error("privacy round-trip failed")
	}
	if loaded.Apps.Email != "thunderbird" {
		t.Errorf("apps round-trip failed: %q", loaded.Apps.Email)
	}
}

func TestSaveCreatesParentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "burrow")

	cfg := &Config{}
	if err := Save(dir, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); os.IsNotExist(err) {
		t.Error("config.yaml not created")
	}
}

func TestSaveHasHeader(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, &Config{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# Burrow configuration") {
		t.Error("expected header comment in saved config")
	}
}

func TestDeepCopy(t *testing.T) {
	original := &Config{
		Services: []ServiceConfig{
			{Name: "svc", Type: "rest", Endpoint: "http://example.com", Auth: AuthConfig{Method: "api_key", Key: "${SECRET}"}},
		},
		LLM: LLMConfig{
			Providers: []ProviderConfig{
				{Name: "local", Type: "ollama", Privacy: "local"},
			},
		},
	}

	copy := original.DeepCopy()

	// Modify the copy
	copy.Services[0].Auth.Key = "resolved-value"
	copy.LLM.Providers[0].Name = "changed"

	// Original must be unaffected
	if original.Services[0].Auth.Key != "${SECRET}" {
		t.Errorf("original mutated: %q", original.Services[0].Auth.Key)
	}
	if original.LLM.Providers[0].Name != "local" {
		t.Errorf("original provider mutated: %q", original.LLM.Providers[0].Name)
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

func TestValidateRetentionNegativeDays(t *testing.T) {
	cfg := &Config{
		Context: ContextConfig{
			Retention: RetentionConfig{RawResults: -1},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for negative raw_results")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Errorf("expected non-negative error, got: %v", err)
	}

	cfg2 := &Config{
		Context: ContextConfig{
			Retention: RetentionConfig{Sessions: -5},
		},
	}
	err = Validate(cfg2)
	if err == nil {
		t.Fatal("expected validation error for negative sessions")
	}
}

func TestValidateRetentionInvalidReports(t *testing.T) {
	cfg := &Config{
		Context: ContextConfig{
			Retention: RetentionConfig{Reports: "30"},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for non-'forever' reports string")
	}
	if !strings.Contains(err.Error(), "forever") {
		t.Errorf("expected 'forever' in error, got: %v", err)
	}

	// "forever" should be valid
	cfg2 := &Config{
		Context: ContextConfig{
			Retention: RetentionConfig{Reports: "forever"},
		},
	}
	if err := Validate(cfg2); err != nil {
		t.Fatalf("reports='forever' should be valid: %v", err)
	}

	// Empty string should also be valid
	cfg3 := &Config{
		Context: ContextConfig{
			Retention: RetentionConfig{Reports: ""},
		},
	}
	if err := Validate(cfg3); err != nil {
		t.Fatalf("reports='' should be valid: %v", err)
	}
}
