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
}

// NewSession creates a new conversational configuration session.
func NewSession(burrowDir string, cfg *config.Config, provider synthesis.Provider) *Session {
	return &Session{
		burrowDir: burrowDir,
		cfg:       cfg,
		provider:  provider,
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
- Valid LLM types: ollama, openrouter, passthrough
- Valid privacy values: local, remote
- All tool paths must start with /
- Explain what you're changing before showing the YAML
- If the user's request is unclear, ask for clarification
- Never remove config the user didn't ask to change`

// ProcessMessage sends a user message and returns the assistant's response
// along with any proposed config change (if YAML was found in the response).
func (s *Session) ProcessMessage(ctx context.Context, userMsg string) (string, *Change, error) {
	s.history = append(s.history, Message{Role: "user", Content: userMsg})

	// Build the full conversation as a user prompt
	var conversationBuilder strings.Builder
	for _, m := range s.history {
		conversationBuilder.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, m.Content))
	}

	// Marshal a redacted copy of the config for the system prompt.
	// Credentials must never be sent to the LLM.
	redacted := redactConfig(s.cfg)
	cfgYAML, err := yaml.Marshal(redacted)
	if err != nil {
		return "", nil, fmt.Errorf("marshaling current config: %w", err)
	}

	systemPrompt := fmt.Sprintf(configSystemPrompt, string(cfgYAML))
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
