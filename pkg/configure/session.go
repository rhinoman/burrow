package configure

import (
	"context"
	"fmt"
	"strings"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/synthesis"
	"gopkg.in/yaml.v3"
)

// Message represents a conversation turn.
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// Change represents a proposed configuration change.
type Change struct {
	Description string
	Config      *config.Config
	Raw         string // The YAML block from LLM output
}

// Session provides LLM-driven conversational configuration.
type Session struct {
	burrowDir string
	cfg       *config.Config
	provider  synthesis.Provider
	history   []Message
	specCache map[string]*FetchedSpec // keyed by service name
}

// NewSession creates a new conversational configuration session.
func NewSession(burrowDir string, cfg *config.Config, provider synthesis.Provider) *Session {
	return &Session{
		burrowDir: burrowDir,
		cfg:       cfg,
		provider:  provider,
		specCache: make(map[string]*FetchedSpec),
	}
}

const configSystemPrompt = `You are Burrow's configuration assistant. Help the user configure their Burrow installation.

Current configuration (YAML):
%s

Rules:
- When the user wants to change config, output the COMPLETE updated config in a YAML code block (` + "```yaml" + ` ... ` + "```" + `)
- Use ${ENV_VAR} syntax for credentials — never store raw secrets
- Valid service types: rest, mcp
- Valid auth methods: api_key, api_key_header, bearer, user_agent, none
- Valid LLM types: ollama, openrouter, llamacpp, passthrough
- Valid privacy values: local, remote
- All tool paths must start with /
- Explain what you're changing before showing the YAML
- If the user's request is unclear, ask for clarification
- Never remove config the user didn't ask to change

API Spec Discovery:
- When adding a REST service, ask if the API has published documentation (OpenAPI, Swagger, docs page)
- If the user provides a spec URL, include it as the "spec" field on the service config
- When API spec content is provided below, use it to generate tool mappings:
  1. Present available API endpoints/capabilities to the user
  2. Let the user choose which endpoints to map as tools
  3. Generate tool entries with name, description, method, path, and params
  4. Each param needs name (user-facing), type, and maps_to (actual API parameter name)
- The user can always modify or override generated tool mappings`

const specContextTemplate = `

## API Specification for service %q

Source: %s (format: %s)

%s

Use this specification to generate tool mappings when the user asks about this service.
Present available endpoints and let the user choose which ones to map as tools.
Each tool needs: name, description, method, path, and params (with name, type, maps_to).`

// ProcessMessage sends a user message and returns the assistant's response
// along with any proposed config change (if YAML was found in the response).
func (s *Session) ProcessMessage(ctx context.Context, userMsg string) (string, *Change, error) {
	s.history = append(s.history, Message{Role: "user", Content: userMsg})

	// Build the full conversation as a user prompt
	var conversationBuilder strings.Builder
	for _, m := range s.history {
		conversationBuilder.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, m.Content))
	}

	// Fetch specs for any services with spec URLs (best-effort, cached).
	s.fetchServiceSpecs(ctx)

	systemPrompt := s.buildSystemPrompt()
	response, err := s.provider.Complete(ctx, systemPrompt, conversationBuilder.String())
	if err != nil {
		return "", nil, fmt.Errorf("LLM error: %w", err)
	}

	s.history = append(s.history, Message{Role: "assistant", Content: response})

	// Check for YAML block in response
	yamlBlock := extractYAMLBlock(response)
	if yamlBlock == "" {
		return response, nil, nil
	}

	// Try to parse the YAML as a config
	var proposed config.Config
	if err := yaml.Unmarshal([]byte(yamlBlock), &proposed); err != nil {
		return response, nil, nil // Invalid YAML — just return the text
	}

	change := &Change{
		Description: extractDescription(response, yamlBlock),
		Config:      &proposed,
		Raw:         yamlBlock,
	}

	return response, change, nil
}

// fetchServiceSpecs fetches specs for any configured services with a spec URL
// not already in the cache. Results (including errors) are cached to prevent retries.
// Prunes cache entries for services no longer in the config.
func (s *Session) fetchServiceSpecs(ctx context.Context) {
	// Build set of current service names with spec URLs.
	active := make(map[string]bool, len(s.cfg.Services))
	for _, svc := range s.cfg.Services {
		if svc.Spec != "" {
			active[svc.Name] = true
		}
	}

	// Prune cache entries for removed services.
	for name := range s.specCache {
		if !active[name] {
			delete(s.specCache, name)
		}
	}

	// Fetch specs for new services.
	for _, svc := range s.cfg.Services {
		if svc.Spec == "" {
			continue
		}
		if _, cached := s.specCache[svc.Name]; cached {
			continue
		}
		spec, err := FetchSpec(ctx, svc.Spec)
		if err != nil {
			// Cache the error to prevent retry on subsequent messages.
			s.specCache[svc.Name] = &FetchedSpec{
				URL:   svc.Spec,
				Error: err.Error(),
			}
			continue
		}
		s.specCache[svc.Name] = spec
	}
}

// buildSystemPrompt constructs the system prompt with current config and any fetched specs.
func (s *Session) buildSystemPrompt() string {
	redacted := redactConfig(s.cfg)
	cfgYAML, _ := yaml.Marshal(redacted)
	prompt := fmt.Sprintf(configSystemPrompt, string(cfgYAML))

	// Append spec context for successfully fetched specs.
	for svcName, spec := range s.specCache {
		if spec.Error != "" || spec.Content == "" {
			continue
		}
		prompt += fmt.Sprintf(specContextTemplate, svcName, spec.URL, spec.Format, spec.Content)
	}
	return prompt
}

// ApplyChange validates and saves a proposed configuration change.
func (s *Session) ApplyChange(change *Change) error {
	if err := config.Validate(change.Config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	if err := config.Save(s.burrowDir, change.Config); err != nil {
		return fmt.Errorf("saving configuration: %w", err)
	}
	s.cfg = change.Config
	return nil
}

// redactConfig returns a deep copy of the config with credential fields replaced
// by placeholder text. This prevents leaking secrets to the LLM.
func redactConfig(cfg *config.Config) *config.Config {
	c := cfg.DeepCopy()
	for i := range c.Services {
		if c.Services[i].Auth.Key != "" {
			c.Services[i].Auth.Key = "${REDACTED}"
		}
		if c.Services[i].Auth.Token != "" {
			c.Services[i].Auth.Token = "${REDACTED}"
		}
		// Auth.Value (user-agent) is not a secret — leave it visible.
	}
	for i := range c.LLM.Providers {
		if c.LLM.Providers[i].APIKey != "" {
			c.LLM.Providers[i].APIKey = "${REDACTED}"
		}
	}
	return c
}

// extractYAMLBlock finds the first ```yaml ... ``` block in text.
// Handles indented code blocks by searching line-by-line for the markers.
func extractYAMLBlock(text string) string {
	lines := strings.Split(text, "\n")
	inBlock := false
	var content strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if trimmed == "```yaml" || trimmed == "```yml" {
				inBlock = true
				continue
			}
		} else {
			if trimmed == "```" {
				return strings.TrimSpace(content.String())
			}
			content.WriteString(line)
			content.WriteByte('\n')
		}
	}
	return ""
}

// extractDescription gets the text before the YAML block as a change description.
func extractDescription(response, yamlBlock string) string {
	idx := strings.Index(response, "```")
	if idx <= 0 {
		return "Configuration update"
	}
	desc := strings.TrimSpace(response[:idx])
	// Take only the last paragraph before the code block
	parts := strings.Split(desc, "\n\n")
	return parts[len(parts)-1]
}
