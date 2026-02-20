package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jcadam/burrow/pkg/actions"
	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/render"
	"github.com/jcadam/burrow/pkg/reports"
	"github.com/spf13/cobra"
)

var exportFormat string

func init() {
	rootCmd.AddCommand(reportsCmd)
	reportsCmd.AddCommand(reportsViewCmd)
	reportsCmd.AddCommand(reportsSearchCmd)
	reportsCmd.AddCommand(reportsExportCmd)
	reportsCmd.AddCommand(reportsCompareCmd)

	reportsExportCmd.Flags().StringVar(&exportFormat, "format", "md", "export format: md or html")
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
			report, err = resolveReport(reportsDir, args[0])
			if err != nil {
				return err
			}
		} else {
			all, err := reports.List(reportsDir)
			if err != nil {
				return fmt.Errorf("listing reports: %w", err)
			}
			if len(all) == 0 {
				return fmt.Errorf("no reports found")
			}
			report = all[0]
		}

		title := report.Title
		if title == "" {
			title = report.Routine + " — " + report.Date
		}

		cfg, _ := loadConfigQuiet(burrowDir)
		opts := viewerOptions(cfg)
		return render.RunViewer(title, report.Markdown, opts...)
	},
}

var reportsSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search reports by content",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")

		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}
		reportsDir := filepath.Join(burrowDir, "reports")

		results, err := reports.Search(reportsDir, query)
		if err != nil {
			return fmt.Errorf("searching reports: %w", err)
		}

		if len(results) == 0 {
			fmt.Printf("No reports matching %q\n", query)
			return nil
		}

		fmt.Printf("Found %d report(s) matching %q:\n\n", len(results), query)
		for _, r := range results {
			title := r.Title
			if title == "" {
				title = r.Routine
			}
			// Show a snippet around the match
			snippet := extractSnippet(r.Markdown, query, 80)
			fmt.Printf("  %s  %s\n", r.Date, title)
			if snippet != "" {
				fmt.Printf("    ...%s...\n", snippet)
			}
		}
		return nil
	},
}

var reportsExportCmd = &cobra.Command{
	Use:   "export <routine|date>",
	Short: "Export a report to a file (md or html)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}
		reportsDir := filepath.Join(burrowDir, "reports")

		report, err := resolveReport(reportsDir, args[0])
		if err != nil {
			return err
		}

		title := report.Title
		if title == "" {
			title = report.Routine + " — " + report.Date
		}

		switch exportFormat {
		case "md":
			outName := report.Routine + "-" + report.Date + ".md"
			if err := os.WriteFile(outName, []byte(report.Markdown), 0o644); err != nil {
				return fmt.Errorf("writing markdown: %w", err)
			}
			fmt.Printf("Exported: %s\n", outName)

		case "html":
			html, err := reports.ExportHTML(report.Markdown, title)
			if err != nil {
				return fmt.Errorf("exporting HTML: %w", err)
			}
			outName := report.Routine + "-" + report.Date + ".html"
			if err := os.WriteFile(outName, []byte(html), 0o644); err != nil {
				return fmt.Errorf("writing HTML: %w", err)
			}
			fmt.Printf("Exported: %s\n", outName)

		default:
			return fmt.Errorf("unsupported format %q (use md or html)", exportFormat)
		}

		return nil
	},
}

// resolveReport tries exact match, then fuzzy match, then date prefix scan.
func resolveReport(reportsDir, ref string) (*reports.Report, error) {
	// Try exact routine name match
	report, err := reports.FindLatest(reportsDir, ref)
	if err != nil {
		return nil, fmt.Errorf("finding report: %w", err)
	}
	if report != nil {
		return report, nil
	}

	// Try fuzzy match
	report, err = reports.FindLatestFuzzy(reportsDir, ref)
	if err != nil {
		return nil, fmt.Errorf("finding report: %w", err)
	}
	if report != nil {
		return report, nil
	}

	// Try date prefix scan — look for directories starting with the ref
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		return nil, fmt.Errorf("no reports found for %q", ref)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.IsDir() && strings.HasPrefix(e.Name(), ref) {
			return reports.Load(filepath.Join(reportsDir, e.Name()))
		}
	}

	return nil, fmt.Errorf("no reports found for %q", ref)
}

// extractSnippet returns a substring of text around the first case-insensitive
// match of query, trimmed to maxLen runes. Uses rune-aware slicing to avoid
// splitting multi-byte UTF-8 characters.
func extractSnippet(text, query string, maxLen int) string {
	lower := strings.ToLower(text)
	byteIdx := strings.Index(lower, strings.ToLower(query))
	if byteIdx < 0 {
		return ""
	}

	runes := []rune(text)
	// Convert byte offset to rune offset
	runeIdx := len([]rune(text[:byteIdx]))

	start := runeIdx - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(runes) {
		end = len(runes)
	}
	snippet := string(runes[start:end])
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	return strings.TrimSpace(snippet)
}

// loadConfigQuiet loads config without erroring if missing.
func loadConfigQuiet(burrowDir string) (*config.Config, error) {
	cfg, err := config.Load(burrowDir)
	if err != nil {
		return nil, err
	}
	config.ResolveEnvVars(cfg)
	return cfg, nil
}

var reportsCompareCmd = &cobra.Command{
	Use:   "compare <ref1> <ref2>",
	Short: "Compare two reports using a local LLM",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}
		reportsDir := filepath.Join(burrowDir, "reports")

		r1, err := resolveReport(reportsDir, args[0])
		if err != nil {
			return fmt.Errorf("resolving first report: %w", err)
		}
		r2, err := resolveReport(reportsDir, args[1])
		if err != nil {
			return fmt.Errorf("resolving second report: %w", err)
		}

		cfg, _ := loadConfigQuiet(burrowDir)
		provider := findLocalProvider(cfg)
		if provider == nil {
			return fmt.Errorf("report comparison requires a local LLM provider.\nConfigure one with: gd configure")
		}

		systemPrompt := `You are a research analyst comparing two reports. Identify:
1. New items in the second report that weren't in the first
2. Items that changed between reports
3. Items that were removed
4. Trends or patterns across both reports

Format your response as structured markdown with clear sections.`

		// Truncate large reports to avoid exceeding LLM context windows.
		const maxCompareBytes = 50_000
		md1 := r1.Markdown
		md2 := r2.Markdown
		combinedSize := len(md1) + len(md2)
		if combinedSize > maxCompareBytes {
			half := maxCompareBytes / 2
			if len(md1) > half {
				md1 = md1[:half] + "\n\n[... truncated ...]\n"
			}
			if len(md2) > half {
				md2 = md2[:half] + "\n\n[... truncated ...]\n"
			}
			fmt.Fprintf(os.Stderr, "Note: reports truncated from %d to ~%d bytes for comparison.\n",
				combinedSize, maxCompareBytes)
		}

		userPrompt := fmt.Sprintf("## Report 1 (%s — %s)\n\n%s\n\n## Report 2 (%s — %s)\n\n%s",
			r1.Routine, r1.Date, md1,
			r2.Routine, r2.Date, md2)

		fmt.Fprintln(os.Stderr, "Comparing reports...")
		response, err := provider.Complete(cmd.Context(), systemPrompt, userPrompt)
		if err != nil {
			return fmt.Errorf("LLM comparison: %w", err)
		}

		rendered, err := render.RenderMarkdown(response, 80)
		if err != nil {
			fmt.Println(response)
			return nil
		}
		fmt.Print(rendered)
		return nil
	},
}

// viewerOptions builds viewer options from config for the enhanced viewer.
func viewerOptions(cfg *config.Config) []render.ViewerOption {
	if cfg == nil {
		return nil
	}

	var opts []render.ViewerOption
	opts = append(opts, render.WithHandoff(actions.NewHandoff(cfg.Apps)))

	if p := findLocalProvider(cfg); p != nil {
		opts = append(opts, render.WithProvider(p))
	}

	return opts
}
