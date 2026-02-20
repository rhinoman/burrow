package main

import (
	"fmt"
	"path/filepath"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/render"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gd [routine]",
	Short: "Burrow — personal research assistant",
	Long:  "Burrow queries services on a schedule, synthesizes results, and produces actionable reports. It never acts on your behalf.",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return runInteractive(cmd.Context())
		}
		return viewRoutineShortcut(args[0])
	},
}

// viewRoutineShortcut opens the latest report for a routine by name.
// Tries exact match first, then fuzzy match via resolveReport.
func viewRoutineShortcut(name string) error {
	burrowDir, err := config.BurrowDir()
	if err != nil {
		return err
	}
	reportsDir := filepath.Join(burrowDir, "reports")

	report, err := resolveReport(reportsDir, name)
	if err != nil {
		return fmt.Errorf("no report found for %q: %w", name, err)
	}

	title := report.Title
	if title == "" {
		title = report.Routine + " — " + report.Date
	}

	cfg, _ := loadConfigQuiet(burrowDir)
	opts := viewerOptions(cfg)
	return render.RunViewer(title, report.Markdown, opts...)
}
