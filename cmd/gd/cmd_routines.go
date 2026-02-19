package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jcadam/burrow/pkg/config"
	bcontext "github.com/jcadam/burrow/pkg/context"
	bhttp "github.com/jcadam/burrow/pkg/http"
	"github.com/jcadam/burrow/pkg/pipeline"
	"github.com/jcadam/burrow/pkg/privacy"
	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/synthesis"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(routinesCmd)
	routinesCmd.AddCommand(routinesListCmd)
	routinesCmd.AddCommand(routinesRunCmd)
}

var routinesCmd = &cobra.Command{
	Use:   "routines",
	Short: "Manage and run data collection routines",
}

var routinesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available routines",
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}
		routinesDir := filepath.Join(burrowDir, "routines")
		routines, err := pipeline.LoadAllRoutines(routinesDir)
		if err != nil {
			return fmt.Errorf("loading routines: %w", err)
		}
		if len(routines) == 0 {
			fmt.Println("No routines found. Add .yaml files to ~/.burrow/routines/")
			return nil
		}
		for _, r := range routines {
			fmt.Printf("  %s — %s\n", r.Name, r.Report.Title)
			fmt.Printf("    Sources: %d", len(r.Sources))
			if r.Schedule != "" {
				fmt.Printf(" | Schedule: %s", r.Schedule)
			}
			fmt.Println()
		}
		return nil
	},
}

var routinesRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Run a routine and generate a report",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		routineName := args[0]

		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}

		// Load config
		cfg, err := config.Load(burrowDir)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		config.ResolveEnvVars(cfg)

		if err := config.Validate(cfg); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}

		// Load routine — try .yaml first, then .yml
		routinesDir := filepath.Join(burrowDir, "routines")
		routinePath := filepath.Join(routinesDir, routineName+".yaml")
		if _, err := os.Stat(routinePath); os.IsNotExist(err) {
			ymlPath := filepath.Join(routinesDir, routineName+".yml")
			if _, err := os.Stat(ymlPath); os.IsNotExist(err) {
				return fmt.Errorf("routine %q not found (looked for %s.yaml and %s.yml in %s)",
					routineName, routineName, routineName, routinesDir)
			}
			routinePath = ymlPath
		}
		routine, err := pipeline.LoadRoutine(routinePath)
		if err != nil {
			return fmt.Errorf("loading routine: %w", err)
		}

		// Build privacy config
		var privCfg *privacy.Config
		if cfg.Privacy.StripReferrers || cfg.Privacy.RandomizeUserAgent || cfg.Privacy.MinimizeRequests {
			privCfg = &privacy.Config{
				StripReferrers:     cfg.Privacy.StripReferrers,
				RandomizeUserAgent: cfg.Privacy.RandomizeUserAgent,
				MinimizeRequests:   cfg.Privacy.MinimizeRequests,
			}
		}

		// Build service registry
		registry := services.NewRegistry()
		for _, svcCfg := range cfg.Services {
			if svcCfg.Type != "rest" {
				continue // MCP not implemented yet
			}
			svc := bhttp.NewRESTService(svcCfg, privCfg)
			if err := registry.Register(svc); err != nil {
				return fmt.Errorf("registering service: %w", err)
			}
		}

		// Select synthesizer based on routine's LLM field
		synth, err := buildSynthesizer(routine, cfg)
		if err != nil {
			return fmt.Errorf("configuring synthesizer: %w", err)
		}

		// Create context ledger
		contextDir := filepath.Join(burrowDir, "context")
		ledger, err := bcontext.NewLedger(contextDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not initialize context ledger: %v\n", err)
		}

		// Run pipeline
		reportsDir := filepath.Join(burrowDir, "reports")
		executor := pipeline.NewExecutor(registry, synth, reportsDir)
		if ledger != nil {
			executor.SetLedger(ledger)
		}

		report, err := executor.Run(cmd.Context(), routine)
		if err != nil {
			return fmt.Errorf("running routine: %w", err)
		}

		fmt.Printf("Report generated: %s\n", report.Dir)
		return nil
	},
}

// buildSynthesizer creates the appropriate synthesizer based on the routine's
// LLM config and the global provider configuration.
func buildSynthesizer(routine *pipeline.Routine, cfg *config.Config) (synthesis.Synthesizer, error) {
	llmName := routine.LLM
	if llmName == "" || llmName == "none" || llmName == "passthrough" {
		return synthesis.NewPassthroughSynthesizer(), nil
	}

	// Find matching provider in config
	var provCfg *config.ProviderConfig
	for i := range cfg.LLM.Providers {
		if cfg.LLM.Providers[i].Name == llmName {
			provCfg = &cfg.LLM.Providers[i]
			break
		}
	}
	if provCfg == nil {
		return nil, fmt.Errorf("LLM provider %q not found in config", llmName)
	}

	provider, err := synthesis.NewProvider(*provCfg)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return synthesis.NewPassthroughSynthesizer(), nil
	}

	// Strip attribution for remote providers when configured
	stripAttribution := provCfg.Privacy == "remote" && cfg.Privacy.StripAttributionForRemote

	return synthesis.NewLLMSynthesizer(provider, stripAttribution), nil
}
