package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jcadam/burrow/pkg/config"
	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(askCmd)
}

var askCmd = &cobra.Command{
	Use:   "ask <query>",
	Short: "Search local context (zero network access)",
	Long:  "Searches the context ledger for entries matching the query. This is a purely local operation — no network requests are made.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")

		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}

		contextDir := filepath.Join(burrowDir, "context")
		ledger, err := bcontext.NewLedger(contextDir)
		if err != nil {
			return fmt.Errorf("opening context ledger: %w", err)
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
