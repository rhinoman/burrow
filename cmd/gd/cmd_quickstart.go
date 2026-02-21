package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/pipeline"
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
func buildQuickstartRoutine() *pipeline.Routine {
	return &pipeline.Routine{
		Name: "weather",
		LLM:  "none",
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

	// Build and validate config
	cfg := buildQuickstartConfig()
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("internal error: quickstart config invalid: %w", err)
	}

	// Save config
	if err := config.Save(burrowDir, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("  Created %s\n", configPath)

	// Create standard directories
	for _, sub := range []string{"routines", "reports", "context", "contacts"} {
		if err := os.MkdirAll(filepath.Join(burrowDir, sub), 0o755); err != nil {
			return fmt.Errorf("creating %s directory: %w", sub, err)
		}
	}

	// Build and save routine
	routine := buildQuickstartRoutine()
	routinesDir := filepath.Join(burrowDir, "routines")
	if err := pipeline.SaveRoutine(routinesDir, routine); err != nil {
		return fmt.Errorf("saving routine: %w", err)
	}
	fmt.Printf("  Created %s\n", filepath.Join(routinesDir, "weather.yaml"))

	// Build registry and test connectivity
	registry, err := buildRegistry(cfg, burrowDir)
	if err != nil {
		return fmt.Errorf("building registry: %w", err)
	}

	synth := synthesis.NewPassthroughSynthesizer()
	reportsDir := filepath.Join(burrowDir, "reports")
	executor := pipeline.NewExecutor(registry, synth, reportsDir)

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
