package configure

import (
	"context"
	"fmt"
	"strings"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/profile"
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
	Description      string
	Config           *config.Config
	Raw              string // The YAML block from LLM output
	RemoteLLMWarning bool   // Set by ApplyChange when a new remote provider is added
}

// ProfileChange represents a proposed profile change.
type ProfileChange struct {
	Description string
	Profile     *profile.Profile
	Raw         string // The YAML block from LLM output
}

// Session provides LLM-driven conversational configuration.
type Session struct {
	burrowDir  string
	cfg        *config.Config
	profileCfg *profile.Profile
	provider   synthesis.Provider
	history    []Message
	specCache  map[string]*FetchedSpec // keyed by service name
}

// NewSession creates a new conversational configuration session.
func NewSession(burrowDir string, cfg *config.Config, provider synthesis.Provider) *Session {
	// Load existing profile (best-effort).
	prof, _ := profile.Load(burrowDir)

	return &Session{
		burrowDir:  burrowDir,
		cfg:        cfg,
		profileCfg: prof,
		provider:   provider,
		specCache:  make(map[string]*FetchedSpec),
	}
}

const configSystemPrompt = `You are Burrow's configuration assistant. Help the user configure their Burrow installation.

Current configuration (YAML):
%s

%s

Rules:
- When the user wants to change config, output the COMPLETE updated config in a YAML code block (` + "```yaml" + ` ... ` + "```" + `)
- When the user describes themselves, their interests, competitors, or industry, propose profile.yaml changes in a ` + "```yaml profile" + ` block (distinct from the config ` + "```yaml" + ` block)
- profile.yaml stores: name, description, interests (list), and any user-defined fields (competitors, naics_codes, focus_agencies, etc.)
- Profile fields are referenced in routines via {{profile.field_name}} template syntax
- Use ${ENV_VAR} syntax for credentials — never store raw secrets
- Valid service types: rest, mcp, rss
- RSS services use type: rss with the feed URL as endpoint. No tools config needed — they auto-provide a 'feed' tool. Optional: max_items (default 20)
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
// along with any proposed config change and/or profile change.
func (s *Session) ProcessMessage(ctx context.Context, userMsg string) (string, *Change, *ProfileChange, error) {
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
		return "", nil, nil, fmt.Errorf("LLM error: %w", err)
	}

	s.history = append(s.history, Message{Role: "assistant", Content: response})

	// Check for profile YAML block first (```yaml profile ... ```)
	var profChange *ProfileChange
	if profileBlock := extractProfileYAMLBlock(response); profileBlock != "" {
		var p profile.Profile
		if err := yaml.Unmarshal([]byte(profileBlock), &p); err == nil {
			// Also unmarshal into raw map for ad-hoc fields
			var raw map[string]interface{}
			if err := yaml.Unmarshal([]byte(profileBlock), &raw); err == nil {
				p.Raw = raw
			}
			profChange = &ProfileChange{
				Description: extractDescription(response, profileBlock),
				Profile:     &p,
				Raw:         profileBlock,
			}
		}
	}

	// Check for config YAML block (```yaml ... ```)
	var change *Change
	if yamlBlock := extractYAMLBlock(response); yamlBlock != "" {
		var proposed config.Config
		if err := yaml.Unmarshal([]byte(yamlBlock), &proposed); err == nil {
			change = &Change{
				Description: extractDescription(response, yamlBlock),
				Config:      &proposed,
				Raw:         yamlBlock,
			}
		}
	}

	return response, change, profChange, nil
}

// ApplyProfileChange saves a proposed profile change.
func (s *Session) ApplyProfileChange(change *ProfileChange) error {
	if err := profile.Save(s.burrowDir, change.Profile); err != nil {
		return fmt.Errorf("saving profile: %w", err)
	}
	s.profileCfg = change.Profile
	return nil
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

// buildSystemPrompt constructs the system prompt with current config, profile, and any fetched specs.
func (s *Session) buildSystemPrompt() string {
	redacted := redactConfig(s.cfg)
	cfgYAML, _ := yaml.Marshal(redacted)

	var profileContext string
	if s.profileCfg != nil && s.profileCfg.Raw != nil {
		profYAML, err := yaml.Marshal(s.profileCfg.Raw)
		if err == nil {
			profileContext = "Current profile (profile.yaml):\n" + string(profYAML)
		}
	}
	if profileContext == "" {
		profileContext = "No profile configured yet. The user can create one by describing themselves."
	}

	prompt := fmt.Sprintf(configSystemPrompt, string(cfgYAML), profileContext)

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

	// Spec §4.2: warn when a remote LLM provider is being added.
	if hasNewRemoteProvider(s.cfg, change.Config) {
		change.RemoteLLMWarning = true
	}

	if err := config.Save(s.burrowDir, change.Config); err != nil {
		return fmt.Errorf("saving configuration: %w", err)
	}
	s.cfg = change.Config
	return nil
}

// hasNewRemoteProvider checks if the proposed config introduces a remote LLM
// provider that wasn't in the current config.
func hasNewRemoteProvider(current, proposed *config.Config) bool {
	existing := make(map[string]bool)
	if current != nil {
		for _, p := range current.LLM.Providers {
			if p.Privacy == "remote" {
				existing[p.Name] = true
			}
		}
	}
	for _, p := range proposed.LLM.Providers {
		if p.Privacy == "remote" && !existing[p.Name] {
			return true
		}
	}
	return false
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

// extractYAMLBlock finds the first ```yaml ... ``` block in text (not ```yaml profile).
// Uses exact string equality on trimmed lines, so variant whitespace like
// "```yaml  profile" would not match either extractor and be silently dropped.
func extractYAMLBlock(text string) string {
	lines := strings.Split(text, "\n")
	inBlock := false
	var content strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			// Match ```yaml or ```yml but NOT ```yaml profile
			if (trimmed == "```yaml" || trimmed == "```yml") {
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

// extractProfileYAMLBlock finds the first ```yaml profile ... ``` block in text.
func extractProfileYAMLBlock(text string) string {
	lines := strings.Split(text, "\n")
	inBlock := false
	var content strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if trimmed == "```yaml profile" || trimmed == "```yml profile" {
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
