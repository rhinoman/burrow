package configure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jcadam/burrow/pkg/config"
)

// fakeProvider is a mock LLM provider for testing.
type fakeProvider struct {
	response string
	err      error
}

func (f *fakeProvider) Complete(_ context.Context, _, _ string) (string, error) {
	return f.response, f.err
}

func TestExtractYAMLBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic yaml block",
			input: "Here's your config:\n```yaml\nservices:\n  - name: test\n```\nDone!",
			want:  "services:\n  - name: test",
		},
		{
			name:  "yml marker",
			input: "```yml\nkey: value\n```",
			want:  "key: value",
		},
		{
			name:  "no yaml block",
			input: "Just some text without any code blocks",
			want:  "",
		},
		{
			name:  "unclosed block",
			input: "```yaml\nincomplete block",
			want:  "",
		},
		{
			name:  "empty block",
			input: "```yaml\n```",
			want:  "",
		},
		{
			name:  "indented yaml block",
			input: "Here's the config:\n   ```yaml\n   services:\n     - name: test\n   ```\n",
			want:  "services:\n     - name: test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractYAMLBlock(tt.input)
			if got != tt.want {
				t.Errorf("extractYAMLBlock() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractProfileYAMLBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "profile yaml block",
			input: "Here's your profile:\n```yaml profile\nname: Trivyn\ninterests:\n  - geo\n```\nDone!",
			want:  "name: Trivyn\ninterests:\n  - geo",
		},
		{
			name:  "regular yaml not matched",
			input: "```yaml\nservices:\n  - name: test\n```",
			want:  "",
		},
		{
			name:  "no block",
			input: "Just some text.",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractProfileYAMLBlock(tt.input)
			if got != tt.want {
				t.Errorf("extractProfileYAMLBlock() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractYAMLBlockDoesNotMatchProfile(t *testing.T) {
	// A ```yaml profile block should NOT be matched by extractYAMLBlock
	input := "```yaml profile\nname: Trivyn\n```"
	got := extractYAMLBlock(input)
	if got != "" {
		t.Errorf("extractYAMLBlock matched profile block: %q", got)
	}
}

func TestSessionProcessMessageNoYAML(t *testing.T) {
	provider := &fakeProvider{response: "Sure, I can help with that. What services do you want to add?"}
	cfg := &config.Config{}
	session := NewSession(t.TempDir(), cfg, provider)

	response, change, _, err := session.ProcessMessage(context.Background(), "Help me configure Burrow")
	if err != nil {
		t.Fatalf("ProcessMessage: %v", err)
	}
	if change != nil {
		t.Error("expected no change for non-YAML response")
	}
	if response == "" {
		t.Error("expected non-empty response")
	}
}

func TestSessionProcessMessageWithYAML(t *testing.T) {
	yamlResponse := `I'll add an Ollama provider for you.

` + "```yaml" + `
llm:
  providers:
    - name: local/llama3
      type: ollama
      endpoint: http://localhost:11434
      model: llama3:latest
      privacy: local
` + "```" + `

This configures a local Ollama provider.`

	provider := &fakeProvider{response: yamlResponse}
	cfg := &config.Config{}
	session := NewSession(t.TempDir(), cfg, provider)

	response, change, _, err := session.ProcessMessage(context.Background(), "Add Ollama as my LLM provider")
	if err != nil {
		t.Fatalf("ProcessMessage: %v", err)
	}
	if response == "" {
		t.Error("expected non-empty response")
	}
	if change == nil {
		t.Fatal("expected a change")
	}
	if change.Config == nil {
		t.Fatal("expected config in change")
	}
	if len(change.Config.LLM.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(change.Config.LLM.Providers))
	}
	if change.Config.LLM.Providers[0].Type != "ollama" {
		t.Errorf("expected ollama, got %q", change.Config.LLM.Providers[0].Type)
	}
}

func TestSessionApplyChange(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	session := NewSession(dir, cfg, nil)

	proposed := &config.Config{
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "local/test", Type: "ollama", Model: "test", Privacy: "local"},
			},
		},
	}

	change := &Change{Config: proposed, Description: "test change"}
	if err := session.ApplyChange(change); err != nil {
		t.Fatalf("ApplyChange: %v", err)
	}

	// Verify saved to disk
	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load after Apply: %v", err)
	}
	if len(loaded.LLM.Providers) != 1 {
		t.Errorf("expected 1 provider after apply, got %d", len(loaded.LLM.Providers))
	}
}

func TestApplyChangeRestoresRedactedCredentials(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "svc1",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth: config.AuthConfig{
					Method: "api_key",
					Key:    "${MY_API_KEY}",
				},
			},
			{
				Name:     "svc2",
				Type:     "rest",
				Endpoint: "https://other.example.com",
				Auth: config.AuthConfig{
					Method: "bearer",
					Token:  "real-bearer-token",
				},
			},
		},
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "cloud/gpt", Type: "openrouter", APIKey: "${OPENROUTER_KEY}", Privacy: "remote"},
			},
		},
	}
	session := NewSession(dir, cfg, nil)

	// Simulate what the LLM produces: credentials are mangled in various ways.
	// svc1: LLM echoes back ${REDACTED}
	// svc2: LLM omits the credential field entirely (empty string)
	// provider: LLM omits the api_key
	proposed := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "svc1",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth: config.AuthConfig{
					Method: "api_key",
					Key:    "${REDACTED}",
				},
			},
			{
				Name:     "svc2",
				Type:     "rest",
				Endpoint: "https://other.example.com",
				Auth: config.AuthConfig{
					Method: "bearer",
					// Token omitted — LLM just left it out
				},
			},
			{
				Name:     "svc3",
				Type:     "rest",
				Endpoint: "https://new.example.com",
				Auth:     config.AuthConfig{Method: "none"},
			},
		},
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				// APIKey omitted — LLM left it out
				{Name: "cloud/gpt", Type: "openrouter", Privacy: "remote"},
			},
		},
	}

	change := &Change{Config: proposed, Description: "add svc3"}
	if err := session.ApplyChange(change); err != nil {
		t.Fatalf("ApplyChange: %v", err)
	}

	// Verify credentials were restored
	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Services[0].Auth.Key != "${MY_API_KEY}" {
		t.Errorf("svc1 api_key not restored: got %q", loaded.Services[0].Auth.Key)
	}
	if loaded.Services[1].Auth.Token != "real-bearer-token" {
		t.Errorf("svc2 bearer token not restored: got %q", loaded.Services[1].Auth.Token)
	}
	if loaded.LLM.Providers[0].APIKey != "${OPENROUTER_KEY}" {
		t.Errorf("provider api_key not restored: got %q", loaded.LLM.Providers[0].APIKey)
	}
	// New service should have no credentials
	if loaded.Services[2].Auth.Key != "" || loaded.Services[2].Auth.Token != "" {
		t.Error("new service should have no credentials")
	}
}

func TestSessionApplyChangeInvalid(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	session := NewSession(dir, cfg, nil)

	proposed := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "", Type: "rest"}, // Missing name — invalid
		},
	}

	change := &Change{Config: proposed}
	if err := session.ApplyChange(change); err == nil {
		t.Error("expected validation error for invalid config")
	}
}

func TestRedactConfig(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "svc1",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth: config.AuthConfig{
					Method: "api_key",
					Key:    "super-secret-key",
				},
			},
			{
				Name:     "svc2",
				Type:     "rest",
				Endpoint: "https://other.example.com",
				Auth: config.AuthConfig{
					Method:  "bearer",
					Token:   "bearer-secret",
				},
			},
			{
				Name:     "svc3",
				Type:     "rest",
				Endpoint: "https://public.example.com",
				Auth: config.AuthConfig{
					Method: "user_agent",
					Value:  "burrow/1.0 contact@example.com",
				},
			},
		},
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "cloud", Type: "openrouter", APIKey: "or-secret-key", Privacy: "remote"},
			},
		},
	}

	redacted := redactConfig(cfg)

	// Credentials must be redacted
	if redacted.Services[0].Auth.Key != "${REDACTED}" {
		t.Errorf("api_key not redacted: %q", redacted.Services[0].Auth.Key)
	}
	if redacted.Services[1].Auth.Token != "${REDACTED}" {
		t.Errorf("bearer token not redacted: %q", redacted.Services[1].Auth.Token)
	}
	if redacted.LLM.Providers[0].APIKey != "${REDACTED}" {
		t.Errorf("provider api_key not redacted: %q", redacted.LLM.Providers[0].APIKey)
	}

	// User-agent value is not a secret — should remain
	if redacted.Services[2].Auth.Value != "burrow/1.0 contact@example.com" {
		t.Errorf("user_agent value should not be redacted: %q", redacted.Services[2].Auth.Value)
	}

	// Original must be unchanged
	if cfg.Services[0].Auth.Key != "super-secret-key" {
		t.Errorf("original was mutated: %q", cfg.Services[0].Auth.Key)
	}
	if cfg.LLM.Providers[0].APIKey != "or-secret-key" {
		t.Errorf("original provider was mutated: %q", cfg.LLM.Providers[0].APIKey)
	}

	// Structural fields must be preserved
	if redacted.Services[0].Name != "svc1" {
		t.Errorf("service name lost: %q", redacted.Services[0].Name)
	}
	if redacted.Services[0].Endpoint != "https://api.example.com" {
		t.Errorf("endpoint lost: %q", redacted.Services[0].Endpoint)
	}
}

func TestSessionHistory(t *testing.T) {
	provider := &fakeProvider{response: "Got it."}
	cfg := &config.Config{}
	session := NewSession(t.TempDir(), cfg, provider)

	session.ProcessMessage(context.Background(), "first message")   //nolint:errcheck
	session.ProcessMessage(context.Background(), "second message") //nolint:errcheck

	if len(session.history) != 4 { // 2 user + 2 assistant
		t.Errorf("expected 4 history entries, got %d", len(session.history))
	}
}

// capturingProvider records the system and user prompts sent to the LLM.
type capturingProvider struct {
	response     string
	systemPrompt string
	userPrompt   string
}

func (c *capturingProvider) Complete(_ context.Context, system, user string) (string, error) {
	c.systemPrompt = system
	c.userPrompt = user
	return c.response, nil
}

func TestSessionFetchesSpecOnFirstMessage(t *testing.T) {
	specBody := `{"openapi": "3.0.0", "info": {"title": "Pet Store"}, "paths": {"/pets": {"get": {}}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(specBody))
	}))
	defer srv.Close()

	provider := &capturingProvider{response: "I see the Pet Store API."}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "petstore",
				Type:     "rest",
				Endpoint: "https://petstore.example.com",
				Auth:     config.AuthConfig{Method: "none"},
				Spec:     srv.URL,
			},
		},
	}
	session := NewSession(t.TempDir(), cfg, provider)

	_, _, _, err := session.ProcessMessage(context.Background(), "Show me the petstore endpoints")
	if err != nil {
		t.Fatalf("ProcessMessage: %v", err)
	}

	if !strings.Contains(provider.systemPrompt, "Pet Store") {
		t.Error("expected spec content in system prompt")
	}
	if !strings.Contains(provider.systemPrompt, "API Specification for service") {
		t.Error("expected spec context header in system prompt")
	}
}

func TestSessionSpecCachedAcrossMessages(t *testing.T) {
	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"openapi": "3.0.0"}`))
	}))
	defer srv.Close()

	provider := &capturingProvider{response: "OK."}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "cached-api",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth:     config.AuthConfig{Method: "none"},
				Spec:     srv.URL,
			},
		},
	}
	session := NewSession(t.TempDir(), cfg, provider)

	session.ProcessMessage(context.Background(), "first")  //nolint:errcheck
	session.ProcessMessage(context.Background(), "second") //nolint:errcheck

	if count := requestCount.Load(); count != 1 {
		t.Errorf("expected 1 spec fetch, got %d", count)
	}
}

func TestSessionSpecFetchErrorDoesNotBlock(t *testing.T) {
	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	provider := &capturingProvider{response: "I can still help."}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "broken-api",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth:     config.AuthConfig{Method: "none"},
				Spec:     srv.URL,
			},
		},
	}
	session := NewSession(t.TempDir(), cfg, provider)

	resp, _, _, err := session.ProcessMessage(context.Background(), "help")
	if err != nil {
		t.Fatalf("ProcessMessage should succeed despite spec error: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty response")
	}

	// Call again — error should be cached, not retried.
	session.ProcessMessage(context.Background(), "again") //nolint:errcheck
	if count := requestCount.Load(); count != 1 {
		t.Errorf("expected 1 spec fetch attempt (error cached), got %d", count)
	}

	// Spec error should not appear in system prompt.
	if strings.Contains(provider.systemPrompt, "API Specification") {
		t.Error("failed spec should not appear in system prompt")
	}
}

func TestSessionSpecAfterConfigChange(t *testing.T) {
	specBody := `{"openapi": "3.0.0", "info": {"title": "New API"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(specBody))
	}))
	defer srv.Close()

	provider := &capturingProvider{response: "OK."}
	cfg := &config.Config{} // No services initially.
	session := NewSession(t.TempDir(), cfg, provider)

	// First message — no specs.
	session.ProcessMessage(context.Background(), "hello") //nolint:errcheck
	if strings.Contains(provider.systemPrompt, "API Specification") {
		t.Error("no spec expected before service added")
	}

	// Apply a config change that adds a service with a spec URL.
	newCfg := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "new-api",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth:     config.AuthConfig{Method: "none"},
				Spec:     srv.URL,
			},
		},
	}
	session.ApplyChange(&Change{Config: newCfg, Description: "add service"})

	// Second message — spec should be fetched and included.
	session.ProcessMessage(context.Background(), "show me the new API") //nolint:errcheck
	if !strings.Contains(provider.systemPrompt, "New API") {
		t.Error("expected spec content in system prompt after config change")
	}
}

func TestApplyChangeDetectsNoChanges(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "svc1",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth: config.AuthConfig{
					Method: "api_key",
					Key:    "${MY_API_KEY}",
				},
			},
		},
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "local/llama", Type: "ollama", Model: "llama3", Privacy: "local"},
			},
		},
	}
	session := NewSession(dir, cfg, nil)

	// Simulate the LLM echoing back the same config with redacted credentials.
	// After restoreCredentials, this should be identical to the original.
	proposed := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "svc1",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth: config.AuthConfig{
					Method: "api_key",
					Key:    "${REDACTED}",
				},
			},
		},
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "local/llama", Type: "ollama", Model: "llama3", Privacy: "local"},
			},
		},
	}

	change := &Change{Config: proposed, Description: "echoed config"}
	err := session.ApplyChange(change)
	if err == nil {
		t.Fatal("expected error for unchanged config, got nil")
	}
	if !strings.Contains(err.Error(), "no changes detected") {
		t.Errorf("expected 'no changes detected' error, got: %v", err)
	}
}

func TestSessionSpecPrunedAfterServiceRemoval(t *testing.T) {
	specBody := `{"openapi": "3.0.0", "info": {"title": "Removable API"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(specBody))
	}))
	defer srv.Close()

	provider := &capturingProvider{response: "OK."}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "temp-api",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth:     config.AuthConfig{Method: "none"},
				Spec:     srv.URL,
			},
		},
	}
	session := NewSession(t.TempDir(), cfg, provider)

	// First message — spec fetched and included.
	session.ProcessMessage(context.Background(), "hello") //nolint:errcheck
	if !strings.Contains(provider.systemPrompt, "Removable API") {
		t.Fatal("expected spec in system prompt")
	}

	// Apply config change that removes the service.
	session.ApplyChange(&Change{Config: &config.Config{}, Description: "remove service"})

	// Next message — stale spec should be pruned.
	session.ProcessMessage(context.Background(), "what now") //nolint:errcheck
	if strings.Contains(provider.systemPrompt, "Removable API") {
		t.Error("stale spec should be pruned after service removal")
	}
	if strings.Contains(provider.systemPrompt, "API Specification") {
		t.Error("no spec context expected after service removal")
	}
}

func TestApplyChangePartialYAMLPreservesConfig(t *testing.T) {
	dir := t.TempDir()

	// Full config with services, LLM providers, privacy, and rendering.
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "existing-svc",
				Type:     "rest",
				Endpoint: "https://api.example.com",
				Auth:     config.AuthConfig{Method: "api_key", Key: "${MY_KEY}"},
			},
		},
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "local/llama", Type: "ollama", Model: "llama3", Privacy: "local", Endpoint: "http://localhost:11434"},
			},
		},
		Privacy: config.PrivacyConfig{
			StripAttributionForRemote: true,
			MinimizeRequests:          true,
		},
		Rendering: config.RenderingConfig{Images: "auto"},
	}

	// LLM returns YAML with only a services change (adds a new service).
	partialYAML := `I'll add the new service for you.

` + "```yaml" + `
services:
  - name: existing-svc
    type: rest
    endpoint: https://api.example.com
    auth:
      method: api_key
      key: ${REDACTED}
  - name: new-svc
    type: rest
    endpoint: https://new.example.com
    auth:
      method: none
` + "```" + `

Done!`

	provider := &fakeProvider{response: partialYAML}
	session := NewSession(dir, cfg, provider)

	_, change, _, err := session.ProcessMessage(context.Background(), "Add a new service")
	if err != nil {
		t.Fatalf("ProcessMessage: %v", err)
	}
	if change == nil {
		t.Fatal("expected a change")
	}

	// Apply the change.
	if err := session.ApplyChange(change); err != nil {
		t.Fatalf("ApplyChange: %v", err)
	}

	// Load and verify the saved config preserves non-service sections.
	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Services should be updated.
	if len(loaded.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(loaded.Services))
	}
	if loaded.Services[1].Name != "new-svc" {
		t.Errorf("expected new-svc, got %q", loaded.Services[1].Name)
	}

	// LLM providers must be preserved (the partial YAML didn't mention them).
	if len(loaded.LLM.Providers) != 1 {
		t.Fatalf("expected 1 LLM provider preserved, got %d", len(loaded.LLM.Providers))
	}
	if loaded.LLM.Providers[0].Name != "local/llama" {
		t.Errorf("LLM provider name lost: got %q", loaded.LLM.Providers[0].Name)
	}

	// Privacy settings must be preserved.
	if !loaded.Privacy.StripAttributionForRemote {
		t.Error("privacy.strip_attribution_for_remote was lost")
	}
	if !loaded.Privacy.MinimizeRequests {
		t.Error("privacy.minimize_requests was lost")
	}

	// Rendering must be preserved.
	if loaded.Rendering.Images != "auto" {
		t.Errorf("rendering.images lost: got %q", loaded.Rendering.Images)
	}

	// Credential must be restored.
	if loaded.Services[0].Auth.Key != "${MY_KEY}" {
		t.Errorf("credential not restored: got %q", loaded.Services[0].Auth.Key)
	}
}

func TestApplyChangeCreatesBackup(t *testing.T) {
	dir := t.TempDir()

	cfg := &config.Config{
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "local/llama", Type: "ollama", Model: "llama3", Privacy: "local"},
			},
		},
	}
	session := NewSession(dir, cfg, nil)

	proposed := cfg.DeepCopy()
	proposed.Services = []config.ServiceConfig{
		{Name: "new-svc", Type: "rest", Endpoint: "https://example.com", Auth: config.AuthConfig{Method: "none"}},
	}

	change := &Change{Config: proposed, Description: "add service"}
	if err := session.ApplyChange(change); err != nil {
		t.Fatalf("ApplyChange: %v", err)
	}

	backupPath := filepath.Join(dir, "config.yaml.bak")
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
	if len(data) == 0 {
		t.Error("backup file is empty")
	}
	// Backup should contain the original config's LLM provider.
	if !strings.Contains(string(data), "local/llama") {
		t.Error("backup does not contain original config data")
	}
}
