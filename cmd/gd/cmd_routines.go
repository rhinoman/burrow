package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jcadam/burrow/pkg/cache"
	"github.com/jcadam/burrow/pkg/config"
	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/jcadam/burrow/pkg/debug"
	bhttp "github.com/jcadam/burrow/pkg/http"
	"github.com/jcadam/burrow/pkg/mcp"
	"github.com/jcadam/burrow/pkg/pipeline"
	"github.com/jcadam/burrow/pkg/privacy"
	"github.com/jcadam/burrow/pkg/profile"
	"github.com/jcadam/burrow/pkg/reports"
	brss "github.com/jcadam/burrow/pkg/rss"
	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/synthesis"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(routinesCmd)
	routinesCmd.AddCommand(routinesListCmd)
	routinesCmd.AddCommand(routinesRunCmd)
	routinesCmd.AddCommand(routinesHistoryCmd)
	routinesCmd.AddCommand(routinesTestCmd)

	routinesRunCmd.Flags().Bool("debug", false, "Print debug output (full requests, responses, timing)")
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
		routines, err := pipeline.LoadAllRoutines(routinesDir, os.Stderr)
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

		// Load user profile (optional) — needed before buildRegistry for
		// template expansion in tool paths.
		prof, _ := profile.Load(burrowDir)

		// Set up debug logging if requested.
		debugFlag, _ := cmd.Flags().GetBool("debug")
		var dbg *debug.Logger
		if debugFlag {
			dbg = debug.NewLogger(os.Stderr)
			dbg.Section("routine: " + routineName)
		}

		// Build service registry
		registry, err := buildRegistry(cfg, burrowDir, prof, dbg)
		if err != nil {
			return err
		}

		// Select synthesizer based on routine's LLM field
		synth, err := buildSynthesizer(routine, cfg)
		if err != nil {
			return fmt.Errorf("configuring synthesizer: %w", err)
		}

		// Wrap synthesizer with debug logging if enabled.
		if dbg != nil {
			synth = &debugSynthesizer{inner: synth, dbg: dbg}
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
		if prof != nil {
			executor.SetProfile(prof)
		}
		if dbg != nil {
			executor.SetDebug(dbg)
		}

		report, err := executor.Run(cmd.Context(), routine)
		if err != nil {
			return fmt.Errorf("running routine: %w", err)
		}

		fmt.Printf("Report generated: %s\n", report.Dir)
		return nil
	},
}

var routinesHistoryCmd = &cobra.Command{
	Use:   "history <name>",
	Short: "Show report history for a routine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		routineName := args[0]

		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}
		reportsDir := filepath.Join(burrowDir, "reports")

		all, err := reports.List(reportsDir)
		if err != nil {
			return fmt.Errorf("listing reports: %w", err)
		}

		var matching []*reports.Report
		for _, r := range all {
			if r.Routine == routineName {
				matching = append(matching, r)
			}
		}

		if len(matching) == 0 {
			fmt.Printf("No reports found for routine %q\n", routineName)
			return nil
		}

		fmt.Printf("Report history for %q (%d reports):\n\n", routineName, len(matching))
		for _, r := range matching {
			title := r.Title
			if title == "" {
				title = "(untitled)"
			}
			fmt.Printf("  %s  %s  (%d sources)\n", r.Date, title, len(r.Sources))
		}
		return nil
	},
}

var routinesTestCmd = &cobra.Command{
	Use:   "test <name>",
	Short: "Test a routine's source connectivity (dry run)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		routineName := args[0]

		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}

		cfg, err := config.Load(burrowDir)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		config.ResolveEnvVars(cfg)
		if err := config.Validate(cfg); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}

		// Load routine
		routinesDir := filepath.Join(burrowDir, "routines")
		routinePath := filepath.Join(routinesDir, routineName+".yaml")
		if _, err := os.Stat(routinePath); os.IsNotExist(err) {
			ymlPath := filepath.Join(routinesDir, routineName+".yml")
			if _, err := os.Stat(ymlPath); os.IsNotExist(err) {
				return fmt.Errorf("routine %q not found", routineName)
			}
			routinePath = ymlPath
		}
		routine, err := pipeline.LoadRoutine(routinePath)
		if err != nil {
			return fmt.Errorf("loading routine: %w", err)
		}

		// Load user profile (optional) — needed before buildRegistry for
		// template expansion in tool paths.
		prof, _ := profile.Load(burrowDir)

		registry, err := buildRegistry(cfg, burrowDir, prof, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Testing %d source(s) for routine %q...\n\n", len(routine.Sources), routineName)

		synth := synthesis.NewPassthroughSynthesizer()
		reportsDir := filepath.Join(burrowDir, "reports")
		executor := pipeline.NewExecutor(registry, synth, reportsDir)
		if prof != nil {
			executor.SetProfile(prof)
		}

		statuses := executor.TestSources(cmd.Context(), routine)

		allOK := true
		for _, s := range statuses {
			status := "OK"
			if !s.OK {
				status = "FAIL"
				allOK = false
			}
			fmt.Printf("  %-4s  %s / %s", status, s.Service, s.Tool)
			if s.OK {
				fmt.Printf("  (%s)", s.Latency.Round(time.Millisecond))
			} else {
				fmt.Printf("  — %s", s.Error)
			}
			fmt.Println()
			if s.URL != "" {
				fmt.Printf("        %s\n", s.URL)
			}
		}

		fmt.Println()
		if allOK {
			fmt.Println("All sources OK.")
		} else {
			fmt.Println("Some sources failed. Check configuration and credentials.")
		}
		return nil
	},
}

// buildRegistry creates a service registry from config, wiring privacy transport,
// MCP clients, and result caching. burrowDir is used for cache storage.
// prof is optional — when non-nil, REST services get a template expand function
// for resolving {{profile.X}} references in tool paths.
// dbg is optional — when non-nil, a debug transport is injected into each service's
// HTTP client for request/response logging.
func buildRegistry(cfg *config.Config, burrowDir string, prof *profile.Profile, dbg *debug.Logger) (*services.Registry, error) {
	var privCfg *privacy.Config
	if cfg.Privacy.StripReferrers || cfg.Privacy.RandomizeUserAgent || cfg.Privacy.MinimizeRequests {
		privCfg = &privacy.Config{
			StripReferrers:     cfg.Privacy.StripReferrers,
			RandomizeUserAgent: cfg.Privacy.RandomizeUserAgent,
			MinimizeRequests:   cfg.Privacy.MinimizeRequests,
		}
	}

	// Build route entries for per-service proxy resolution.
	routes := make([]privacy.RouteEntry, len(cfg.Privacy.Routes))
	for i, r := range cfg.Privacy.Routes {
		routes[i] = privacy.RouteEntry{Service: r.Service, Proxy: r.Proxy}
	}

	cacheDir := filepath.Join(burrowDir, "cache")

	registry := services.NewRegistry()
	for _, svcCfg := range cfg.Services {
		var svc services.Service
		proxyURL := privacy.ResolveProxy(svcCfg.Name, cfg.Privacy.DefaultProxy, routes)

		switch svcCfg.Type {
		case "rest":
			restSvc := bhttp.NewRESTService(svcCfg, privCfg, proxyURL)
			if prof != nil {
				p := prof // capture for closure
				restSvc.SetExpandFunc(func(s string) (string, error) {
					return profile.Expand(s, p)
				})
			}
			if dbg != nil {
				restSvc.WrapTransport(func(rt http.RoundTripper) http.RoundTripper {
					return debug.NewTransport(rt, dbg)
				})
			}
			svc = restSvc
		case "mcp":
			httpClient := mcp.NewHTTPClient(svcCfg.Auth, privCfg, proxyURL)
			if dbg != nil {
				httpClient.Transport = debug.NewTransport(httpClient.Transport, dbg)
			}
			svc = mcp.NewMCPService(svcCfg.Name, svcCfg.Endpoint, httpClient)
		case "rss":
			rssSvc := brss.NewRSSService(svcCfg, privCfg, proxyURL)
			if dbg != nil {
				rssSvc.WrapTransport(func(rt http.RoundTripper) http.RoundTripper {
					return debug.NewTransport(rt, dbg)
				})
			}
			svc = rssSvc
		default:
			fmt.Fprintf(os.Stderr, "warning: unknown service type %q for %q, skipping\n", svcCfg.Type, svcCfg.Name)
			continue
		}

		// Wrap with cache if TTL > 0.
		if svcCfg.CacheTTL > 0 {
			svc = cache.NewCachedService(svc, cacheDir, svcCfg.CacheTTL)
		}

		if err := registry.Register(svc); err != nil {
			return nil, fmt.Errorf("registering service: %w", err)
		}
	}
	return registry, nil
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

	// Resolve context window: explicit config > privacy-based default.
	contextWindow := provCfg.ContextWindow
	if contextWindow == 0 {
		switch provCfg.Privacy {
		case "local":
			contextWindow = 8192
		default: // "remote" or unset
			contextWindow = 131072
		}
	}

	synth := synthesis.NewLLMSynthesizer(provider, stripAttribution)
	synth.SetLocalModel(provCfg.Privacy == "local")

	// Resolve preprocessing: explicit config wins, nil = auto (local models).
	preprocess := provCfg.Privacy == "local" // auto default
	if routine.Synthesis.Preprocess != nil {
		preprocess = *routine.Synthesis.Preprocess
	}
	synth.SetPreprocess(preprocess)

	synth.SetMultiStage(synthesis.MultiStageConfig{
		Strategy:        routine.Synthesis.Strategy,
		SummaryMaxWords: routine.Synthesis.SummaryMaxWords,
		MaxSourceWords:  routine.Synthesis.MaxSourceWords,
		Concurrency:     routine.Synthesis.Concurrency,
		ContextWindow:   contextWindow,
	})
	return synth, nil
}

// debugSynthesizer wraps a Synthesizer to log timing and sizes when --debug is active.
type debugSynthesizer struct {
	inner synthesis.Synthesizer
	dbg   *debug.Logger
}

func (d *debugSynthesizer) Synthesize(ctx context.Context, title string, systemPrompt string, results []*services.Result) (string, error) {
	d.dbg.Section("Synthesizer")
	d.dbg.Printf("title: %s", title)
	d.dbg.Printf("system prompt: %d chars", len(systemPrompt))
	totalData := 0
	for _, r := range results {
		totalData += len(r.Data)
	}
	d.dbg.Printf("input: %d sources, %d bytes total data", len(results), totalData)

	start := time.Now()
	md, err := d.inner.Synthesize(ctx, title, systemPrompt, results)
	elapsed := time.Since(start)

	if err != nil {
		d.dbg.Printf("synthesis error (%s): %v", elapsed.Round(time.Millisecond), err)
		return md, err
	}
	d.dbg.Printf("synthesis complete (%s): %d chars markdown", elapsed.Round(time.Millisecond), len(md))
	return md, nil
}
