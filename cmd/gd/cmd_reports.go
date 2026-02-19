package main

import (
	"fmt"
	"path/filepath"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/render"
	"github.com/jcadam/burrow/pkg/reports"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(reportsCmd)
	reportsCmd.AddCommand(reportsViewCmd)
}

var reportsCmd = &cobra.Command{
	Use:   "reports",
	Short: "List and view generated reports",
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}
		reportsDir := filepath.Join(burrowDir, "reports")
		all, err := reports.List(reportsDir)
		if err != nil {
			return fmt.Errorf("listing reports: %w", err)
		}
		if len(all) == 0 {
			fmt.Println("No reports found. Run a routine first: gd routines run <name>")
			return nil
		}
		for _, r := range all {
			title := r.Title
			if title == "" {
				title = r.Routine
			}
			fmt.Printf("  %s  %s  (%d sources)\n", r.Date, title, len(r.Sources))
		}
		return nil
	},
}

var reportsViewCmd = &cobra.Command{
	Use:   "view [routine]",
	Short: "View the latest report (optionally for a specific routine)",
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}
		reportsDir := filepath.Join(burrowDir, "reports")

		var report *reports.Report

		if len(args) > 0 {
			report, err = reports.FindLatest(reportsDir, args[0])
			if err != nil {
				return fmt.Errorf("finding report: %w", err)
			}
			if report == nil {
				return fmt.Errorf("no reports found for routine %q", args[0])
			}
		} else {
			all, err := reports.List(reportsDir)
			if err != nil {
				return fmt.Errorf("listing reports: %w", err)
			}
			if len(all) == 0 {
				return fmt.Errorf("no reports found")
			}
			report = all[0] // most recent
		}

		title := report.Title
		if title == "" {
			title = report.Routine + " — " + report.Date
		}

		return render.RunViewer(title, report.Markdown)
	},
}
