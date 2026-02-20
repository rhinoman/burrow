package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jcadam/burrow/pkg/config"
	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(contextCmd)
	contextCmd.AddCommand(contextSearchCmd)
	contextCmd.AddCommand(contextShowCmd)
	contextCmd.AddCommand(contextStatsCmd)
	contextCmd.AddCommand(contextClearCmd)

	contextShowCmd.Flags().IntVarP(&contextShowLimit, "limit", "n", 20, "number of entries to show")
	contextShowCmd.Flags().StringVar(&contextShowType, "type", "", "filter by type: report, result, or session")
}

var (
	contextShowLimit int
	contextShowType  string
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage the local context ledger",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to "show"
		return contextShowCmd.RunE(cmd, args)
	},
}

var contextSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search context entries",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")

		ledger, err := openLedger()
		if err != nil {
			return err
		}

		entries, err := ledger.Search(query)
		if err != nil {
			return fmt.Errorf("searching context: %w", err)
		}

		if len(entries) == 0 {
			fmt.Printf("No results for %q\n", query)
			return nil
		}

		fmt.Printf("Found %d result(s) for %q:\n\n", len(entries), query)
		for _, e := range entries {
			ts := e.Timestamp.Format("2006-01-02 15:04")
			fmt.Printf("  %s  [%s]  %s\n", ts, e.Type, e.Label)
		}
		return nil
	},
}

var contextShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show recent context entries (all types by default)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ledger, err := openLedger()
		if err != nil {
			return err
		}

		types := []string{bcontext.TypeReport, bcontext.TypeResult, bcontext.TypeSession, bcontext.TypeContact}
		if contextShowType != "" {
			switch contextShowType {
			case bcontext.TypeReport, bcontext.TypeResult, bcontext.TypeSession, bcontext.TypeContact:
				types = []string{contextShowType}
			default:
				return fmt.Errorf("unknown type %q (use report, result, session, or contact)", contextShowType)
			}
		}

		var all []bcontext.Entry
		for _, t := range types {
			entries, err := ledger.List(t, 0)
			if err != nil {
				return fmt.Errorf("listing %s entries: %w", t, err)
			}
			all = append(all, entries...)
		}

		// Sort newest first
		sort.Slice(all, func(i, j int) bool {
			return all[i].Timestamp.After(all[j].Timestamp)
		})

		if contextShowLimit > 0 && len(all) > contextShowLimit {
			all = all[:contextShowLimit]
		}

		if len(all) == 0 {
			fmt.Println("No context entries found.")
			return nil
		}

		fmt.Printf("Recent entries (showing %d):\n\n", len(all))
		for _, e := range all {
			ts := e.Timestamp.Format("2006-01-02 15:04")
			fmt.Printf("  %s  [%-7s]  %s\n", ts, e.Type, e.Label)
		}
		return nil
	},
}

var contextStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show context ledger statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		ledger, err := openLedger()
		if err != nil {
			return err
		}

		stats, err := ledger.Stats()
		if err != nil {
			return fmt.Errorf("gathering stats: %w", err)
		}

		if len(stats) == 0 {
			fmt.Println("Context ledger is empty.")
			return nil
		}

		fmt.Println("Context ledger statistics:")
		fmt.Println()
		fmt.Printf("  %-10s  %5s  %10s  %-12s  %-12s\n", "Type", "Count", "Size", "Earliest", "Latest")
		fmt.Printf("  %-10s  %5s  %10s  %-12s  %-12s\n", "----", "-----", "----", "--------", "------")

		for _, entryType := range []string{bcontext.TypeReport, bcontext.TypeResult, bcontext.TypeSession, bcontext.TypeContact} {
			ts, ok := stats[entryType]
			if !ok {
				continue
			}
			earliest := ts.Earliest.Format("2006-01-02")
			latest := ts.Latest.Format("2006-01-02")
			fmt.Printf("  %-10s  %5d  %10s  %-12s  %-12s\n",
				entryType, ts.Count, formatBytes(ts.Bytes), earliest, latest)
		}
		return nil
	},
}

var contextClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the context ledger (requires confirmation)",
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}
		contextDir := filepath.Join(burrowDir, "context")

		fmt.Print("This will delete all context ledger entries. Are you sure? [y/N] ")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return nil
		}
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer != "y" && answer != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}

		for _, sub := range []string{"reports", "results", "sessions", "contacts"} {
			dir := filepath.Join(contextDir, sub)
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("removing %s: %w", sub, err)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("recreating %s: %w", sub, err)
			}
		}

		fmt.Println("Context ledger cleared.")
		return nil
	},
}

// openLedger is a helper to open the context ledger from the standard location.
func openLedger() (*bcontext.Ledger, error) {
	burrowDir, err := config.BurrowDir()
	if err != nil {
		return nil, err
	}
	contextDir := filepath.Join(burrowDir, "context")
	return bcontext.NewLedger(contextDir)
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
