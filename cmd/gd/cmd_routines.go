package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jcadam/burrow/pkg/config"
	bhttp "github.com/jcadam/burrow/pkg/http"
	"github.com/jcadam/burrow/pkg/pipeline"
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

		// Build service registry
		registry := services.NewRegistry()
		for _, svcCfg := range cfg.Services {
			if svcCfg.Type != "rest" {
				continue // MCP not implemented in Phase 1
			}
			svc := bhttp.NewRESTService(svcCfg)
			if err := registry.Register(svc); err != nil {
				return fmt.Errorf("registering service: %w", err)
			}
		}

		// Create synthesizer (passthrough for Phase 1)
		synth := synthesis.NewPassthroughSynthesizer()

		// Run pipeline
		reportsDir := filepath.Join(burrowDir, "reports")
		executor := pipeline.NewExecutor(registry, synth, reportsDir)

		report, err := executor.Run(cmd.Context(), routine)
		if err != nil {
			return fmt.Errorf("running routine: %w", err)
		}

		fmt.Printf("Report generated: %s\n", report.Dir)
		return nil
	},
}
