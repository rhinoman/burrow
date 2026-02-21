package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/configure"
	"github.com/jcadam/burrow/pkg/pipeline"
	"github.com/jcadam/burrow/pkg/profile"
	"github.com/jcadam/burrow/pkg/synthesis"
	"github.com/spf13/cobra"
)

var quickstartForce bool

func init() {
	quickstartCmd.Flags().BoolVar(&quickstartForce, "force", false, "Overwrite existing configuration")
	rootCmd.AddCommand(quickstartCmd)
}

var quickstartCmd = &cobra.Command{
	Use:   "quickstart",
	Short: "Create a working demo with free public APIs (weather, earthquakes, news, AI papers)",
	Long:  "Sets up a multi-source pipeline using 5 free APIs (NWS weather, Open-Meteo, USGS earthquakes, Hacker News, ArXiv), creates a demo profile, runs the pipeline, and generates a synthesized daily brief — all in one command. No API keys required.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runQuickstart(cmd.Context())
	},
}

// buildQuickstartConfig returns a Config using 5 free public APIs.
func buildQuickstartConfig() *config.Config {
	return &config.Config{
		Services: []config.ServiceConfig{
			{
				Name:     "weather-gov",
				Type:     "rest",
				Endpoint: "https://api.weather.gov",
				Auth: config.AuthConfig{
					Method: "user_agent",
					Value:  "burrow/1.0 (quickstart-demo@example.com)",
				},
				Tools: []config.ToolConfig{
					{
						Name:        "forecast",
						Description: "7-day forecast for Anchorage, AK",
						Method:      "GET",
						Path:        "/gridpoints/AER/143,236/forecast",
					},
					{
						Name:        "alerts",
						Description: "Active weather alerts for Alaska",
						Method:      "GET",
						Path:        "/alerts/active?area=AK",
					},
				},
			},
			{
				Name:     "usgs-earthquakes",
				Type:     "rest",
				Endpoint: "https://earthquake.usgs.gov",
				Auth: config.AuthConfig{
					Method: "none",
				},
				Tools: []config.ToolConfig{
					{
						Name:        "recent",
						Description: "Recent earthquakes near Anchorage, AK (M2.5+, 500km radius)",
						Method:      "GET",
						Path:        "/fdsnws/event/1/query?format=geojson&minmagnitude=2.5&latitude=61.22&longitude=-149.90&maxradiuskm=500&orderby=time&limit=20",
					},
				},
			},
			{
				Name:     "open-meteo",
				Type:     "rest",
				Endpoint: "https://api.open-meteo.com",
				Auth: config.AuthConfig{
					Method: "none",
				},
				Tools: []config.ToolConfig{
					{
						Name:        "forecast",
						Description: "Detailed hourly/daily weather for Anchorage, AK",
						Method:      "GET",
						Path:        "/v1/forecast?latitude=61.22&longitude=-149.90&hourly=temperature_2m,precipitation_probability,windspeed_10m&daily=temperature_2m_max,temperature_2m_min,precipitation_sum,sunrise,sunset&temperature_unit=fahrenheit&windspeed_unit=mph&precipitation_unit=inch&timezone=America/Anchorage&forecast_days=3",
					},
				},
			},
			{
				Name:     "hackernews",
				Type:     "rss",
				Endpoint: "https://hnrss.org/newest?points=100",
				Auth: config.AuthConfig{
					Method: "none",
				},
				MaxItems: 15,
			},
			{
				Name:     "arxiv-ai",
				Type:     "rss",
				Endpoint: "https://rss.arxiv.org/rss/cs.AI",
				Auth: config.AuthConfig{
					Method: "none",
				},
				MaxItems: 10,
			},
		},
		Privacy: config.PrivacyConfig{
			StripReferrers:   true,
			MinimizeRequests: true,
		},
	}
}

// buildQuickstartRoutine returns a Routine for the multi-source daily brief.
// When llmName is not "none", a synthesis system prompt is included.
// The synthesis prompt uses {{profile.X}} template variables for demonstration.
func buildQuickstartRoutine(llmName string) *pipeline.Routine {
	routine := &pipeline.Routine{
		Name: "daily-brief",
		LLM:  llmName,
		Report: pipeline.ReportConfig{
			Title: "Daily Brief — {{profile.name}}",
		},
		Sources: []pipeline.SourceConfig{
			{
				Service:      "weather-gov",
				Tool:         "forecast",
				ContextLabel: "NWS 7-Day Forecast — Anchorage",
			},
			{
				Service:      "weather-gov",
				Tool:         "alerts",
				ContextLabel: "NWS Active Alerts — Alaska",
			},
			{
				Service:      "open-meteo",
				Tool:         "forecast",
				ContextLabel: "Detailed Weather — Anchorage",
			},
			{
				Service:      "usgs-earthquakes",
				Tool:         "recent",
				ContextLabel: "Recent Earthquakes — Alaska Region",
			},
			{
				Service:      "hackernews",
				Tool:         "feed",
				ContextLabel: "Hacker News — Top Stories",
			},
			{
				Service:      "arxiv-ai",
				Tool:         "feed",
				ContextLabel: "ArXiv — Latest AI Papers",
			},
		},
	}

	if llmName != "none" {
		routine.Synthesis = pipeline.SynthesisConfig{
			System: "You are a personal research analyst for {{profile.name}},\n" +
				"{{profile.description}}\n\n" +
				"Produce a daily intelligence brief covering:\n" +
				"1. Weather outlook for {{profile.location}} — combine NWS forecast with Open-Meteo data\n" +
				"2. Recent seismic activity — highlight any notable earthquakes\n" +
				"3. Tech news — summarize the most interesting stories from Hacker News\n" +
				"4. AI research — highlight notable new papers from ArXiv\n\n" +
				"Prioritize topics related to: {{profile.interests}}\n\n" +
				"Format as a structured, scannable report with clear section headers.\n" +
				"Include suggested actions where relevant (e.g., [Draft] follow-up, [Open] links).",
		}
	}

	return routine
}

// buildQuickstartProfile returns a demo Profile for the quickstart.
// Typed fields (Name, Description, Interests) are synced into Raw by
// profile.Save. Only ad-hoc fields need to be set in Raw directly.
func buildQuickstartProfile() *profile.Profile {
	return &profile.Profile{
		Name: "Demo User",
		Description: "Tech professional based in Anchorage, Alaska.\n" +
			"Interested in AI research, seismic activity monitoring,\n" +
			"and Arctic weather patterns.",
		Interests: []string{
			"artificial intelligence",
			"machine learning",
			"earthquake monitoring",
			"Arctic weather",
			"technology trends",
		},
		Raw: map[string]interface{}{
			// Ad-hoc fields — these demonstrate that profile.yaml is open-ended.
			// Use {{profile.field_name}} in routines to reference any field.
			"location":        "Anchorage, AK",
			"focus_topics":    []interface{}{"transformer architectures", "seismic early warning"},
			"alert_threshold": "M3.0+",
		},
	}
}

// setupQuickstartLLM guides the user through LLM provider configuration.
// It auto-detects Ollama, offers manual choices, and verifies the provider.
// Returns the provider name to use in the routine ("none" if skipped).
func setupQuickstartLLM(ctx context.Context, cfg *config.Config) (string, error) {
	fmt.Println()
	fmt.Println("  LLM Provider Setup")
	fmt.Println("  ──────────────────")

	// Auto-detect Ollama
	if provider := configure.DetectOllama(); provider != nil {
		if op, ok := provider.(*synthesis.OllamaProvider); ok {
			model := op.Model()
			fmt.Printf("  Ollama detected — using model %s\n", model)
			fmt.Print("  Verifying...")
			if configure.VerifyProvider(ctx, provider) {
				name := "local/" + model
				cfg.LLM.Providers = append(cfg.LLM.Providers, config.ProviderConfig{
					Name:     name,
					Type:     "ollama",
					Endpoint: "http://localhost:11434",
					Model:    model,
					Privacy:  "local",
				})
				fmt.Println(" OK")
				return name, nil
			}
			fmt.Println(" failed")
			fmt.Println("  Ollama is running but model didn't respond.")
			fmt.Println()
		}
	}

	// Manual choice
	fmt.Println("  Burrow needs an LLM to turn raw data into readable reports.")
	fmt.Println()
	fmt.Println("  1) Ollama (local — install from https://ollama.com)")
	fmt.Println("  2) OpenRouter (remote — needs API key from https://openrouter.ai)")
	fmt.Println("  3) Skip (reports will contain raw JSON)")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	prompt := func(msg string) string {
		fmt.Print(msg)
		if scanner.Scan() {
			return strings.TrimSpace(scanner.Text())
		}
		return ""
	}

	choice := prompt("  Choice [1-3]: ")
	switch choice {
	case "1":
		endpoint := prompt("  Ollama endpoint [http://localhost:11434]: ")
		if endpoint == "" {
			endpoint = "http://localhost:11434"
		}
		model := prompt("  Model name [llama3:latest]: ")
		if model == "" {
			model = "llama3:latest"
		}
		name := "local/" + model

		provCfg := config.ProviderConfig{
			Name:     name,
			Type:     "ollama",
			Endpoint: endpoint,
			Model:    model,
			Privacy:  "local",
		}

		// Verify connectivity
		provider, err := synthesis.NewProvider(provCfg)
		if err != nil {
			return "", fmt.Errorf("creating Ollama provider: %w", err)
		}
		fmt.Print("  Verifying...")
		if configure.VerifyProvider(ctx, provider) {
			fmt.Println(" OK")
		} else {
			fmt.Println(" failed")
			fmt.Println("  Could not reach Ollama. Config saved — verify later with: gd routines test daily-brief")
		}

		cfg.LLM.Providers = append(cfg.LLM.Providers, provCfg)
		return name, nil

	case "2":
		apiKey := prompt("  OpenRouter API key (or $ENV_VAR): ")
		if apiKey == "" {
			fmt.Println("  No API key provided. Skipping.")
			fmt.Println("  Reports will contain raw JSON. Add an LLM later with: gd configure")
			return "none", nil
		}
		model := prompt("  Model [openai/gpt-4o-mini]: ")
		if model == "" {
			model = "openai/gpt-4o-mini"
		}
		name := "openrouter/" + model

		// Privacy warning (spec §4.2)
		fmt.Printf("\n  Warning: Provider '%s' sends synthesis data to openrouter.ai.\n", name)
		fmt.Println("  Collected results will leave your machine during synthesis.")
		fmt.Println("  For maximum privacy, use a local LLM provider.")
		fmt.Println()
		ack := prompt("  Acknowledge and continue? [y/N]: ")
		if strings.ToLower(ack) != "y" {
			fmt.Println("  Remote provider not added.")
			fmt.Println("  Reports will contain raw JSON. Add an LLM later with: gd configure")
			return "none", nil
		}

		provCfg := config.ProviderConfig{
			Name:    name,
			Type:    "openrouter",
			APIKey:  apiKey,
			Model:   model,
			Privacy: "remote",
		}

		// Verify with resolved key (env var expanded for the test call only)
		verifyProvCfg := provCfg
		verifyProvCfg.APIKey = resolveEnvRef(apiKey)
		if verifyProvCfg.APIKey != "" {
			provider, err := synthesis.NewProvider(verifyProvCfg)
			if err == nil && provider != nil {
				fmt.Print("  Verifying...")
				if configure.VerifyProvider(ctx, provider) {
					fmt.Println(" OK")
				} else {
					fmt.Println(" failed")
					fmt.Println("  Provider didn't respond. Config saved — verify later with: gd routines test daily-brief")
				}
			}
		}

		cfg.LLM.Providers = append(cfg.LLM.Providers, provCfg)
		cfg.Privacy.StripAttributionForRemote = true
		return name, nil

	default:
		// Choice 3 or anything else — skip
		fmt.Println("  Reports will contain raw JSON. Add an LLM later with: gd configure")
		return "none", nil
	}
}

// resolveEnvRef expands a single $VAR or ${VAR} reference for verification.
// The original reference is preserved in config — only the resolved value is used for testing.
func resolveEnvRef(s string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		varName := s[2 : len(s)-1]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
	} else if strings.HasPrefix(s, "$") {
		varName := s[1:]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
	}
	return s
}

func runQuickstart(ctx context.Context) error {
	burrowDir, err := config.BurrowDir()
	if err != nil {
		return err
	}

	// Check for existing config
	configPath := filepath.Join(burrowDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil && !quickstartForce {
		fmt.Println("Configuration already exists at", configPath)
		fmt.Println("Use --force to overwrite, or 'gd init' to configure for real services.")
		return nil
	}

	// Build config and guide user through LLM setup
	cfg := buildQuickstartConfig()

	llmName, err := setupQuickstartLLM(ctx, cfg)
	if err != nil {
		return err
	}

	// Validate and save config (now includes LLM provider if user chose one)
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("internal error: quickstart config invalid: %w", err)
	}
	if err := config.Save(burrowDir, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("\n  Created %s\n", configPath)

	// Create standard directories
	for _, sub := range []string{"routines", "reports", "context", "contacts"} {
		if err := os.MkdirAll(filepath.Join(burrowDir, sub), 0o755); err != nil {
			return fmt.Errorf("creating %s directory: %w", sub, err)
		}
	}

	// Build and save profile
	prof := buildQuickstartProfile()
	if err := profile.Save(burrowDir, prof); err != nil {
		return fmt.Errorf("saving profile: %w", err)
	}
	fmt.Printf("  Created %s\n", filepath.Join(burrowDir, "profile.yaml"))

	// Build and save routine
	routine := buildQuickstartRoutine(llmName)
	routinesDir := filepath.Join(burrowDir, "routines")
	if err := pipeline.SaveRoutine(routinesDir, routine); err != nil {
		return fmt.Errorf("saving routine: %w", err)
	}
	fmt.Printf("  Created %s\n", filepath.Join(routinesDir, "daily-brief.yaml"))

	// Resolve env vars on a copy for runtime use (saved config keeps ${VAR} references).
	runtimeCfg := cfg.DeepCopy()
	config.ResolveEnvVars(runtimeCfg)

	// Build registry and test connectivity
	registry, err := buildRegistry(runtimeCfg, burrowDir)
	if err != nil {
		return fmt.Errorf("building registry: %w", err)
	}

	// Select synthesizer based on LLM choice
	synth, err := buildSynthesizer(routine, runtimeCfg)
	if err != nil {
		return fmt.Errorf("configuring synthesizer: %w", err)
	}

	reportsDir := filepath.Join(burrowDir, "reports")
	executor := pipeline.NewExecutor(registry, synth, reportsDir)
	executor.SetProfile(prof)

	fmt.Println()
	fmt.Println("  Testing connectivity (5 services, 6 sources)...")

	statuses := executor.TestSources(ctx, routine)
	allOK := true
	for _, s := range statuses {
		if s.OK {
			fmt.Printf("    OK    %s/%s  (%s)\n", s.Service, s.Tool, s.Latency.Round(time.Millisecond))
		} else {
			fmt.Printf("    FAIL  %s/%s  — %s\n", s.Service, s.Tool, s.Error)
			allOK = false
		}
	}

	if !allOK {
		fmt.Println()
		fmt.Println("  Some sources failed. Config files were created — retry with:")
		fmt.Println("    gd routines run daily-brief")
		return nil
	}

	// Run the pipeline
	fmt.Println()
	fmt.Println("  Generating report...")

	report, err := executor.Run(ctx, routine)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Report generation failed: %v\n", err)
		fmt.Println("  Config files were created. Retry with: gd routines run daily-brief")
		return nil
	}

	fmt.Printf("  Report saved: %s\n", report.Dir)

	// Print next steps
	fmt.Println()
	fmt.Println("  View the report:")
	fmt.Println("    gd daily-brief")
	fmt.Println("    gd reports view daily-brief")
	fmt.Println()
	fmt.Println("  What just happened:")
	fmt.Println("    5 services queried independently (compartmentalized)")
	fmt.Println("    Profile template expanded ({{profile.name}}, {{profile.location}}, ...)")
	fmt.Println("    Results synthesized into a single daily brief")
	fmt.Println()
	fmt.Println("  Explore:")
	fmt.Println("    gd profile                  — view your demo profile")
	fmt.Println("    ~/.burrow/config.yaml        — service configuration")
	fmt.Println("    ~/.burrow/routines/          — routine definitions")
	fmt.Println("    ~/.burrow/profile.yaml       — profile (referenced via {{profile.X}})")
	fmt.Println()
	fmt.Println("  Ready for real services?")
	fmt.Println("    gd init")

	return nil
}
