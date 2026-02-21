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
	Short: "Create a working demo with the free NWS weather API",
	Long:  "Sets up a complete pipeline using weather.gov (no API key needed), runs it, and generates a real report — all in one command.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runQuickstart(cmd.Context())
	},
}

// buildQuickstartConfig returns a Config using the free NWS weather API.
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
						Description: "7-day forecast for Denver/Boulder, CO",
						Method:      "GET",
						Path:        "/gridpoints/BOU/62,60/forecast",
					},
					{
						Name:        "alerts",
						Description: "Active weather alerts for Colorado",
						Method:      "GET",
						Path:        "/alerts/active?area=CO",
					},
				},
			},
		},
		Privacy: config.PrivacyConfig{
			StripReferrers:   true,
			MinimizeRequests: true,
		},
	}
}

// buildQuickstartRoutine returns a Routine for the weather demo.
// When llmName is not "none", a synthesis system prompt is included.
func buildQuickstartRoutine(llmName string) *pipeline.Routine {
	routine := &pipeline.Routine{
		Name: "weather",
		LLM:  llmName,
		Report: pipeline.ReportConfig{
			Title: "Weather Report — Denver/Boulder, CO",
		},
		Sources: []pipeline.SourceConfig{
			{
				Service:      "weather-gov",
				Tool:         "forecast",
				ContextLabel: "7-Day Forecast",
			},
			{
				Service:      "weather-gov",
				Tool:         "alerts",
				ContextLabel: "Active Weather Alerts",
			},
		},
	}

	if llmName != "none" {
		routine.Synthesis = pipeline.SynthesisConfig{
			System: "You are a weather analyst. Produce a clear, readable weather report from the\n" +
				"raw forecast and alert data. Include:\n" +
				"- Today's conditions and notable weather\n" +
				"- Multi-day outlook highlighting significant changes\n" +
				"- Any active alerts with recommended actions\n" +
				"- A brief summary suitable for planning the day\n\n" +
				"Use plain language. Skip technical grid metadata.",
		}
	}

	return routine
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
			fmt.Println("  Could not reach Ollama. Config saved — verify later with: gd routines test weather")
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
					fmt.Println("  Provider didn't respond. Config saved — verify later with: gd routines test weather")
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

	// Build and save routine
	routine := buildQuickstartRoutine(llmName)
	routinesDir := filepath.Join(burrowDir, "routines")
	if err := pipeline.SaveRoutine(routinesDir, routine); err != nil {
		return fmt.Errorf("saving routine: %w", err)
	}
	fmt.Printf("  Created %s\n", filepath.Join(routinesDir, "weather.yaml"))

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

	// Load user profile (optional)
	prof, _ := profile.Load(burrowDir)

	reportsDir := filepath.Join(burrowDir, "reports")
	executor := pipeline.NewExecutor(registry, synth, reportsDir)
	if prof != nil {
		executor.SetProfile(prof)
	}

	fmt.Println()
	fmt.Println("  Testing weather.gov connectivity...")

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
		fmt.Println("    gd routines run weather")
		return nil
	}

	// Run the pipeline
	fmt.Println()
	fmt.Println("  Generating report...")

	report, err := executor.Run(ctx, routine)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Report generation failed: %v\n", err)
		fmt.Println("  Config files were created. Retry with: gd routines run weather")
		return nil
	}

	fmt.Printf("  Report saved: %s\n", report.Dir)

	// Print next steps
	fmt.Println()
	fmt.Println("  View the report:")
	fmt.Println("    gd weather")
	fmt.Println("    gd reports view weather")
	fmt.Println()
	fmt.Println("  Customize the location:")
	fmt.Println("    Edit ~/.burrow/config.yaml to change the NWS grid point")
	fmt.Println("    Find your grid point: https://api.weather.gov/points/{lat},{lon}")
	fmt.Println()
	fmt.Println("  Ready for real services?")
	fmt.Println("    gd init")

	return nil
}
