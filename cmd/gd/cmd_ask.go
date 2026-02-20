package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jcadam/burrow/pkg/config"
	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/jcadam/burrow/pkg/render"
	"github.com/jcadam/burrow/pkg/synthesis"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(askCmd)
}

const askSystemPrompt = `You are a research analyst answering questions from collected intelligence data.
Cite specific dates, numbers, and sources from the context when available.
If the context doesn't contain relevant information, say so clearly.
Be concise and actionable. Format your response as markdown.`

var askCmd = &cobra.Command{
	Use:   "ask <query>",
	Short: "Search local context (zero network access)",
	Long:  "Searches the context ledger for entries matching the query. Uses a local LLM for reasoning if available, otherwise falls back to text search. This is a purely local operation — no network requests are made.",
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

		// Try to load config for local LLM
		cfg, cfgErr := config.Load(burrowDir)
		if cfgErr == nil {
			config.ResolveEnvVars(cfg)
		}

		// Find local LLM provider (spec: zero network requests for gd ask)
		provider := findLocalProvider(cfg)
		if provider != nil {
			return askWithLLM(cmd, provider, ledger, query)
		}

		// Fallback to text search
		return askTextSearch(ledger, query)
	},
}

// findLocalProvider returns the first local (non-remote, non-passthrough) LLM provider
// from the config. Returns nil if none found.
func findLocalProvider(cfg *config.Config) synthesis.Provider {
	if cfg == nil {
		return nil
	}
	for _, p := range cfg.LLM.Providers {
		if p.Privacy != "local" {
			continue
		}
		if p.Type == "" || p.Type == "passthrough" {
			continue
		}
		provider, err := synthesis.NewProvider(p)
		if err != nil || provider == nil {
			continue
		}
		return provider
	}
	return nil
}

// askWithLLM gathers context and queries a local LLM for a reasoned answer.
func askWithLLM(cmd *cobra.Command, provider synthesis.Provider, ledger *bcontext.Ledger, query string) error {
	contextData, err := ledger.GatherContext(100_000)
	if err != nil {
		return fmt.Errorf("gathering context: %w", err)
	}

	if contextData == "" {
		fmt.Println("No context data available. Run some routines first.")
		return nil
	}

	var userPrompt strings.Builder
	userPrompt.WriteString("Question: ")
	userPrompt.WriteString(query)
	userPrompt.WriteString("\n\nContext data:\n\n")
	userPrompt.WriteString(contextData)

	fmt.Fprintf(os.Stderr, "Reasoning over %d bytes of context...\n", len(contextData))

	response, err := provider.Complete(cmd.Context(), askSystemPrompt, userPrompt.String())
	if err != nil {
		return fmt.Errorf("LLM error: %w", err)
	}

	rendered, err := render.RenderMarkdown(response, 80)
	if err != nil {
		// Fallback to plain text
		fmt.Println(response)
		return nil
	}
	fmt.Print(rendered)
	return nil
}

// askTextSearch is the original text-search fallback.
func askTextSearch(ledger *bcontext.Ledger, query string) error {
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
}
