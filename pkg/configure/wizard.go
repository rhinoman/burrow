package configure

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/profile"
)

// Wizard provides a structured (non-LLM) configuration interface.
type Wizard struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewWizard creates a wizard with the given IO.
func NewWizard(r io.Reader, w io.Writer) *Wizard {
	return &Wizard{
		reader: bufio.NewReader(r),
		writer: w,
	}
}

// RunInit walks through initial Burrow configuration.
func (w *Wizard) RunInit() (*config.Config, error) {
	cfg := &config.Config{}

	w.print("\n  Burrow — Personal Research Assistant\n")
	w.print("  ────────────────────────────────────\n\n")
	w.print("  Let's set up your configuration.\n\n")

	// Step 1: LLM Provider
	if err := w.configureLLM(cfg); err != nil {
		return nil, err
	}

	// Step 2: First service (optional)
	if err := w.configureFirstService(cfg); err != nil {
		return nil, err
	}

	// Step 3: Privacy defaults
	w.configurePrivacy(cfg)

	// Step 4: App defaults
	w.configureApps(cfg)

	return cfg, nil
}

// ConfigureProfile asks the user for essential profile fields.
// All fields are optional — user can press Enter to skip each.
// Returns nil if user skipped everything.
func (w *Wizard) ConfigureProfile() *profile.Profile {
	w.print("  User Profile (optional)\n")
	w.print("  ───────────────────────\n")
	w.print("  Your profile flows into routines, reports, and search context.\n")
	w.print("  Press Enter to skip any field.\n\n")

	name := strings.TrimSpace(w.prompt("  Name or organization: "))
	desc := strings.TrimSpace(w.prompt("  Describe what you do (1-2 sentences): "))
	interestsRaw := strings.TrimSpace(w.prompt("  Topics you track (comma-separated): "))

	// If user skipped everything, return nil
	if name == "" && desc == "" && interestsRaw == "" {
		w.print("  Profile skipped. Create one later with: gd profile edit\n\n")
		return nil
	}

	p := &profile.Profile{
		Name:        name,
		Description: desc,
	}

	if interestsRaw != "" {
		for _, item := range strings.Split(interestsRaw, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				p.Interests = append(p.Interests, item)
			}
		}
	}

	w.print("\n")
	return p
}

// RunModify presents the current config and applies user-requested changes.
func (w *Wizard) RunModify(cfg *config.Config) error {
	w.print("\n  Current Configuration\n")
	w.print("  ────────────────────\n\n")
	w.printConfigSummary(cfg)

	w.print("\n  What would you like to change?\n")
	w.print("  1) LLM providers\n")
	w.print("  2) Services\n")
	w.print("  3) Privacy settings\n")
	w.print("  4) App settings\n")
	w.print("  5) Done\n\n")

	for {
		choice := w.prompt("  Choice [1-5]: ")
		switch strings.TrimSpace(choice) {
		case "1":
			if err := w.configureLLM(cfg); err != nil {
				return err
			}
		case "2":
			if err := w.configureFirstService(cfg); err != nil {
				return err
			}
		case "3":
			w.configurePrivacy(cfg)
		case "4":
			w.configureApps(cfg)
		case "5", "":
			return nil
		default:
			w.print("  Invalid choice.\n")
		}

		w.print("\n  Continue editing? (y/n): ")
		ans := w.prompt("")
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			return nil
		}
	}
}

func (w *Wizard) configureLLM(cfg *config.Config) error {
	w.print("  LLM Provider Setup\n")
	w.print("  ──────────────────\n")
	w.print("  1) Ollama (local)\n")
	w.print("  2) OpenRouter (remote)\n")
	w.print("  3) None (passthrough only)\n\n")

	choice := w.prompt("  Choice [1-3]: ")
	switch strings.TrimSpace(choice) {
	case "1":
		endpoint := w.prompt("  Ollama endpoint [http://localhost:11434]: ")
		if strings.TrimSpace(endpoint) == "" {
			endpoint = "http://localhost:11434"
		}
		model := w.prompt("  Model name (e.g., qwen2.5:14b): ")
		model = strings.TrimSpace(model)
		if model == "" {
			model = "llama3:latest"
		}
		name := "local/" + model
		cfg.LLM.Providers = append(cfg.LLM.Providers, config.ProviderConfig{
			Name:     name,
			Type:     "ollama",
			Endpoint: strings.TrimSpace(endpoint),
			Model:    model,
			Privacy:  "local",
		})

	case "2":
		apiKey := w.prompt("  OpenRouter API key (or $ENV_VAR): ")
		model := w.prompt("  Model (e.g., openai/gpt-4): ")
		model = strings.TrimSpace(model)
		if model == "" {
			model = "openai/gpt-4"
		}
		name := "openrouter/" + model

		// Spec §4.2: warn users when first configuring a remote LLM provider.
		w.print(fmt.Sprintf("\n  ⚠ LLM provider '%s' sends synthesis data\n", name))
		w.print("    to openrouter.ai. Collected results will leave your\n")
		w.print("    machine during synthesis.\n\n")
		w.print("    For maximum privacy, use a local LLM provider.\n\n")
		ack := w.prompt("  Acknowledge and continue? [y/N]: ")
		if strings.ToLower(strings.TrimSpace(ack)) != "y" {
			w.print("  Remote provider not added.\n")
			break
		}

		cfg.LLM.Providers = append(cfg.LLM.Providers, config.ProviderConfig{
			Name:    name,
			Type:    "openrouter",
			APIKey:  strings.TrimSpace(apiKey),
			Model:   model,
			Privacy: "remote",
		})
		cfg.Privacy.StripAttributionForRemote = true

	case "3", "":
		cfg.LLM.Providers = append(cfg.LLM.Providers, config.ProviderConfig{
			Name:    "none",
			Type:    "passthrough",
			Privacy: "local",
		})
	}

	w.print("\n")
	return nil
}

func (w *Wizard) configureFirstService(cfg *config.Config) error {
	w.print("  Service Configuration\n")
	w.print("  ─────────────────────\n")
	w.print("  Add a REST API service? (y/n): ")
	ans := w.prompt("")
	if strings.ToLower(strings.TrimSpace(ans)) != "y" {
		w.print("\n")
		return nil
	}

	name := w.prompt("  Service name (e.g., sam-gov): ")
	endpoint := w.prompt("  API base URL: ")
	w.print("  Auth method:\n")
	w.print("  1) API key (query param)\n")
	w.print("  2) API key (header)\n")
	w.print("  3) Bearer token\n")
	w.print("  4) User-Agent\n")
	w.print("  5) None\n")
	authChoice := w.prompt("  Choice [1-5]: ")

	svc := config.ServiceConfig{
		Name:     strings.TrimSpace(name),
		Type:     "rest",
		Endpoint: strings.TrimSpace(endpoint),
	}

	switch strings.TrimSpace(authChoice) {
	case "1":
		svc.Auth.Method = "api_key"
		svc.Auth.Key = strings.TrimSpace(w.prompt("  API key (or $ENV_VAR): "))
		param := w.prompt("  Query param name [api_key]: ")
		if strings.TrimSpace(param) != "" {
			svc.Auth.KeyParam = strings.TrimSpace(param)
		}
	case "2":
		svc.Auth.Method = "api_key_header"
		svc.Auth.Key = strings.TrimSpace(w.prompt("  API key (or $ENV_VAR): "))
		param := w.prompt("  Header name [X-API-Key]: ")
		if strings.TrimSpace(param) != "" {
			svc.Auth.KeyParam = strings.TrimSpace(param)
		}
	case "3":
		svc.Auth.Method = "bearer"
		svc.Auth.Token = strings.TrimSpace(w.prompt("  Bearer token (or $ENV_VAR): "))
	case "4":
		svc.Auth.Method = "user_agent"
		svc.Auth.Value = strings.TrimSpace(w.prompt("  User-Agent value: "))
	case "5", "":
		svc.Auth.Method = "none"
	}

	// API spec URL (optional — used by conversational config to auto-generate tools)
	specURL := w.prompt("  API spec URL (OpenAPI/Swagger, or Enter to skip): ")
	if strings.TrimSpace(specURL) != "" {
		svc.Spec = strings.TrimSpace(specURL)
		w.print("  Spec URL saved. Run 'gd configure' with an LLM to auto-generate tools.\n")
	}

	// Tools
	w.print("\n  Add a tool for this service? (y/n): ")
	if strings.ToLower(strings.TrimSpace(w.prompt(""))) == "y" {
		toolName := w.prompt("  Tool name (e.g., search): ")
		method := w.prompt("  HTTP method [GET]: ")
		if strings.TrimSpace(method) == "" {
			method = "GET"
		}
		path := w.prompt("  Path (e.g., /v2/search): ")

		tool := config.ToolConfig{
			Name:   strings.TrimSpace(toolName),
			Method: strings.ToUpper(strings.TrimSpace(method)),
			Path:   strings.TrimSpace(path),
		}

		// Optional params
		w.print("  How many query parameters? [0]: ")
		numStr := w.prompt("")
		numParams, _ := strconv.Atoi(strings.TrimSpace(numStr))
		for i := 0; i < numParams; i++ {
			pName := w.prompt(fmt.Sprintf("  Param %d name: ", i+1))
			pMapsTo := w.prompt(fmt.Sprintf("  Param %d maps to (API param): ", i+1))
			tool.Params = append(tool.Params, config.ParamConfig{
				Name:   strings.TrimSpace(pName),
				Type:   "string",
				MapsTo: strings.TrimSpace(pMapsTo),
			})
		}

		svc.Tools = append(svc.Tools, tool)
	}

	cfg.Services = append(cfg.Services, svc)
	w.print("\n")
	return nil
}

func (w *Wizard) configurePrivacy(cfg *config.Config) {
	w.print("  Privacy Settings\n")
	w.print("  ────────────────\n")
	w.print("  Enable privacy hardening (recommended)? (y/n) [y]: ")
	ans := w.prompt("")
	if strings.ToLower(strings.TrimSpace(ans)) == "n" {
		return
	}

	cfg.Privacy.StripAttributionForRemote = true
	cfg.Privacy.MinimizeRequests = true
	cfg.Privacy.StripReferrers = true
	cfg.Privacy.RandomizeUserAgent = true
	w.print("  Privacy hardening enabled.\n\n")
}

func (w *Wizard) configureApps(cfg *config.Config) {
	w.print("  App Settings (press Enter for system default)\n")
	w.print("  ─────────────────────────────────────────────\n")

	email := w.prompt("  Email app [default]: ")
	if strings.TrimSpace(email) != "" {
		cfg.Apps.Email = strings.TrimSpace(email)
	}
	browser := w.prompt("  Browser [default]: ")
	if strings.TrimSpace(browser) != "" {
		cfg.Apps.Browser = strings.TrimSpace(browser)
	}
	w.print("\n")
}

func (w *Wizard) printConfigSummary(cfg *config.Config) {
	w.print(fmt.Sprintf("  Services: %d\n", len(cfg.Services)))
	for _, s := range cfg.Services {
		w.print(fmt.Sprintf("    - %s (%s)\n", s.Name, s.Endpoint))
	}
	w.print(fmt.Sprintf("  LLM Providers: %d\n", len(cfg.LLM.Providers)))
	for _, p := range cfg.LLM.Providers {
		w.print(fmt.Sprintf("    - %s (%s)\n", p.Name, p.Type))
	}
	w.print(fmt.Sprintf("  Privacy: strip_attribution=%v minimize=%v\n",
		cfg.Privacy.StripAttributionForRemote, cfg.Privacy.MinimizeRequests))
}

func (w *Wizard) prompt(msg string) string {
	if msg != "" {
		fmt.Fprint(w.writer, msg)
	}
	line, err := w.reader.ReadString('\n')
	if err != nil && line == "" {
		// EOF or read error with no data — return empty to let callers use defaults.
		return ""
	}
	return strings.TrimRight(line, "\n\r")
}

func (w *Wizard) print(msg string) {
	fmt.Fprint(w.writer, msg)
}
