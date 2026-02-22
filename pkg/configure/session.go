package configure

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/pipeline"
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

// RoutineChange represents a proposed routine creation or update.
type RoutineChange struct {
	Description string
	Routine     *pipeline.Routine
	Raw         string // The YAML block from LLM output
	IsNew       bool   // True if this is a new routine, false if updating existing
}

// Session provides LLM-driven conversational configuration.
type Session struct {
	burrowDir  string
	cfg        *config.Config
	profileCfg *profile.Profile
	routines   []*pipeline.Routine
	provider   synthesis.Provider
	history    []Message
	specCache  map[string]*FetchedSpec // keyed by service name
}

// NewSession creates a new conversational configuration session.
func NewSession(burrowDir string, cfg *config.Config, provider synthesis.Provider) *Session {
	// Load existing profile (best-effort).
	prof, _ := profile.Load(burrowDir)

	// Load existing routines (best-effort).
	routines, _ := pipeline.LoadAllRoutines(filepath.Join(burrowDir, "routines"))

	return &Session{
		burrowDir:  burrowDir,
		cfg:        cfg,
		profileCfg: prof,
		routines:   routines,
		provider:   provider,
		specCache:  make(map[string]*FetchedSpec),
	}
}

const configSystemPrompt = `You are Burrow's configuration assistant. Help the user configure their Burrow installation.

Current configuration (YAML):
%s

%s

%s

Rules:
- When the user wants to change config, output the COMPLETE updated config in a YAML code block (` + "```yaml" + ` ... ` + "```" + `)
- When the user describes themselves, their interests, competitors, or industry, propose profile.yaml changes in a ` + "```yaml profile" + ` block (distinct from the config ` + "```yaml" + ` block)
- When the user wants to create or update a routine, use a ` + "```yaml routine <name>" + ` block (e.g. ` + "```yaml routine morning-intel" + `)
- All YAML values must be plain strings, string lists, or string maps — never use JSON-style inline arrays like ["a", "b"] or nested objects where a simple string is expected
- profile.yaml stores: name, description, interests (list), and any user-defined fields (competitors, naics_codes, focus_agencies, etc.)
- Templates use Go text/template syntax with these built-in functions:
  - {{profile "field_name"}} — profile field lookup
  - {{profile "parent.child"}} — nested profile field
  - {{today}}, {{yesterday}} — date strings (YYYY-MM-DD)
  - {{now}} — RFC3339 timestamp
  - {{year}}, {{month}}, {{day}} — date components
  - {{yesterday | date "01/02/2006"}} — reformat date (Go reference time layout)
  - {{split}}, {{join}}, {{lower}}, {{upper}} — string helpers
  - {{index (split (profile "coordinates") ",") 0}} — expressions
- Legacy syntax {{profile.field_name}} is also supported (auto-converted)
- Structure profile data so each value is directly referenceable
- Routines are separate YAML files in ~/.burrow/routines/<name>.yaml — they are NOT part of config.yaml
- Routine YAML must include report.title and at least one source with service and tool fields
- Do NOT include a name field in routine YAML — the name comes from the block tag / filename
- Sources reference services from config.yaml by name
- When updating an existing routine, output the COMPLETE routine with ALL fields — omitted fields will be reset to defaults
- Never remove routine fields the user didn't ask to change
- Available routine fields: schedule (cron), timezone, jitter (seconds), llm (provider name), report (title, style, generate_charts, max_length, compare_with), synthesis.system (system prompt for LLM), sources (list of service, tool, params, context_label)
- Source params must be key-value string pairs, e.g. params: {q: "search term", limit: "10"} — values are always plain strings, never arrays or nested objects
- Example routine source:
    sources:
      - service: my-api
        tool: search
        params:
          q: "{{profile \"interests\" | join \", \"}}"
          limit: "10"
        context_label: search results
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
// along with any proposed config change, profile change, routine change,
// parse warnings, and/or error. Warnings are non-fatal issues (e.g. YAML
// parse failures) that should be surfaced to the user.
func (s *Session) ProcessMessage(ctx context.Context, userMsg string) (string, *Change, *ProfileChange, *RoutineChange, []string, error) {
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
		return "", nil, nil, nil, nil, fmt.Errorf("LLM error: %w", err)
	}

	s.history = append(s.history, Message{Role: "assistant", Content: response})

	var warnings []string

	// Check for profile YAML block first (```yaml profile ... ```)
	var profChange *ProfileChange
	if profileBlock := extractProfileYAMLBlock(response); profileBlock != "" {
		var p profile.Profile
		if err := yaml.Unmarshal([]byte(profileBlock), &p); err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to parse profile YAML: %v", err))
		} else {
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

	// Check for routine YAML block (```yaml routine <name> ... ```)
	var routineChange *RoutineChange
	if routineBlock, routineName := extractRoutineYAMLBlock(response); routineBlock != "" {
		var r pipeline.Routine
		if err := yaml.Unmarshal([]byte(routineBlock), &r); err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to parse routine YAML: %v", err))
		} else {
			r.Name = routineName

			// Determine if this is a new routine or an update.
			isNew := true
			for _, existing := range s.routines {
				if existing.Name == routineName {
					isNew = false
					break
				}
			}

			routineChange = &RoutineChange{
				Description: extractDescription(response, routineBlock),
				Routine:     &r,
				Raw:         routineBlock,
				IsNew:       isNew,
			}
		}
	}

	// Check for config YAML block (```yaml ... ```)
	var change *Change
	if yamlBlock := extractYAMLBlock(response); yamlBlock != "" {
		// Unmarshal onto a copy of the current config so that fields the
		// LLM omits retain their current values instead of being zeroed.
		proposed := s.cfg.DeepCopy()
		if err := yaml.Unmarshal([]byte(yamlBlock), proposed); err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to parse config YAML: %v", err))
		} else {
			change = &Change{
				Description: extractDescription(response, yamlBlock),
				Config:      proposed,
				Raw:         yamlBlock,
			}
		}
	}

	return response, change, profChange, routineChange, warnings, nil
}

// ApplyProfileChange saves a proposed profile change.
func (s *Session) ApplyProfileChange(change *ProfileChange) error {
	if err := profile.Save(s.burrowDir, change.Profile); err != nil {
		return fmt.Errorf("saving profile: %w", err)
	}
	s.profileCfg = change.Profile
	return nil
}

// ApplyRoutineChange validates and saves a proposed routine change.
func (s *Session) ApplyRoutineChange(change *RoutineChange) error {
	if err := pipeline.ValidateRoutine(change.Routine); err != nil {
		return fmt.Errorf("invalid routine: %w", err)
	}

	routinesDir := filepath.Join(s.burrowDir, "routines")
	if err := pipeline.SaveRoutine(routinesDir, change.Routine); err != nil {
		return fmt.Errorf("saving routine: %w", err)
	}

	// Update the in-memory routines list.
	found := false
	for i, r := range s.routines {
		if r.Name == change.Routine.Name {
			s.routines[i] = change.Routine
			found = true
			break
		}
	}
	if !found {
		s.routines = append(s.routines, change.Routine)
	}

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

	var routineContext string
	if len(s.routines) > 0 {
		var sb strings.Builder
		sb.WriteString("Current routines:\n")
		for _, r := range s.routines {
			sb.WriteString(fmt.Sprintf("\n--- %s.yaml ---\n", r.Name))
			rYAML, err := yaml.Marshal(r)
			if err == nil {
				sb.Write(rYAML)
			}
		}
		routineContext = sb.String()
	} else {
		routineContext = "No routines configured yet."
	}

	prompt := fmt.Sprintf(configSystemPrompt, string(cfgYAML), profileContext, routineContext)

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
	// The LLM never sees real credentials — restore originals before
	// validation so auth checks pass with the actual values.
	restoreCredentials(s.cfg, change.Config)

	// Detect when the LLM echoed back the current config without changes.
	currentYAML, _ := yaml.Marshal(s.cfg)
	proposedYAML, _ := yaml.Marshal(change.Config)
	if string(currentYAML) == string(proposedYAML) {
		return fmt.Errorf("no changes detected — the LLM echoed back the current config unchanged")
	}

	if err := config.Validate(change.Config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Spec §4.2: warn when a remote LLM provider is being added.
	if hasNewRemoteProvider(s.cfg, change.Config) {
		change.RemoteLLMWarning = true
	}

	// Best-effort backup before overwriting.
	if current, err := yaml.Marshal(s.cfg); err == nil {
		backupPath := filepath.Join(s.burrowDir, "config.yaml.bak")
		os.WriteFile(backupPath, current, 0o644) //nolint:errcheck
	}

	if err := config.Save(s.burrowDir, change.Config); err != nil {
		return fmt.Errorf("saving configuration: %w", err)
	}
	s.cfg = change.Config
	return nil
}

// restoreCredentials copies credential values from src into dst for matching
// services and providers. The LLM never sees real credentials (they're
// redacted before being sent), so it can never produce them — it may echo
// back "${REDACTED}", omit the field, or invent a placeholder. For any
// service/provider that existed in the original config, we unconditionally
// restore the original credential.
func restoreCredentials(src, dst *config.Config) {
	if src == nil {
		return
	}

	// Index source services by name for O(1) lookup.
	srcSvc := make(map[string]*config.ServiceConfig, len(src.Services))
	for i := range src.Services {
		srcSvc[src.Services[i].Name] = &src.Services[i]
	}
	for i := range dst.Services {
		s, ok := srcSvc[dst.Services[i].Name]
		if !ok {
			continue
		}
		if s.Auth.Key != "" {
			dst.Services[i].Auth.Key = s.Auth.Key
		}
		if s.Auth.Token != "" {
			dst.Services[i].Auth.Token = s.Auth.Token
		}
	}

	// Index source providers by name.
	srcProv := make(map[string]*config.ProviderConfig, len(src.LLM.Providers))
	for i := range src.LLM.Providers {
		srcProv[src.LLM.Providers[i].Name] = &src.LLM.Providers[i]
	}
	for i := range dst.LLM.Providers {
		s, ok := srcProv[dst.LLM.Providers[i].Name]
		if !ok {
			continue
		}
		if s.APIKey != "" {
			dst.LLM.Providers[i].APIKey = s.APIKey
		}
	}
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
// Matching is case-insensitive so ```YAML, ```Yaml, etc. all work.
func extractYAMLBlock(text string) string {
	lines := strings.Split(text, "\n")
	inBlock := false
	var content strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			// Match ```yaml or ```yml but NOT ```yaml profile / ```yaml routine
			lower := strings.ToLower(trimmed)
			if lower == "```yaml" || lower == "```yml" {
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
// Matching is case-insensitive so ```YAML profile, ```Yaml Profile, etc. all work.
func extractProfileYAMLBlock(text string) string {
	lines := strings.Split(text, "\n")
	inBlock := false
	var content strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			lower := strings.ToLower(trimmed)
			if lower == "```yaml profile" || lower == "```yml profile" {
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

// extractRoutineYAMLBlock finds the first ```yaml routine <name> ... ``` block in text.
// Returns the block content and the routine name. Requires a non-empty name after "routine".
// Matching is case-insensitive so ```YAML routine, ```Yaml Routine, etc. all work.
func extractRoutineYAMLBlock(text string) (string, string) {
	lines := strings.Split(text, "\n")
	inBlock := false
	var content strings.Builder
	var name string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			lower := strings.ToLower(trimmed)
			for _, prefix := range []string{"```yaml routine ", "```yml routine "} {
				if strings.HasPrefix(lower, prefix) {
					// Use original trimmed string for the name to preserve case.
					n := strings.TrimSpace(trimmed[len(prefix):])
					if n != "" {
						inBlock = true
						name = n
						break
					}
				}
			}
		} else {
			if trimmed == "```" {
				return strings.TrimSpace(content.String()), name
			}
			content.WriteString(line)
			content.WriteByte('\n')
		}
	}
	return "", ""
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
