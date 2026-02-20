package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/configure"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Burrow configuration",
	Long:  "Sets up ~/.burrow/ with a config.yaml and required directories. Uses conversational config if a local LLM is available, otherwise falls back to a structured wizard.",
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}

		// Check if config already exists
		configPath := filepath.Join(burrowDir, "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			fmt.Println("Configuration already exists at", configPath)
			fmt.Println("Use 'gd configure' to modify it.")
			return nil
		}

		var cfg *config.Config

		// Try to detect Ollama for conversational config
		if provider := configure.DetectOllama(); provider != nil {
			fmt.Println("  Ollama detected — starting conversational configuration.")
			fmt.Println("  Type your configuration requests, or 'done' to finish.")
			fmt.Println()

			session := configure.NewSession(burrowDir, &config.Config{}, provider)
			cfg, err = runConversationalInit(cmd.Context(), session)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Conversational config failed: %v\n", err)
				fmt.Println("  Falling back to structured wizard.")
				fmt.Println()
				cfg = nil
			}
		}

		// Fallback to structured wizard
		if cfg == nil {
			wiz := configure.NewWizard(os.Stdin, os.Stdout)
			cfg, err = wiz.RunInit()
			if err != nil {
				return fmt.Errorf("configuration wizard: %w", err)
			}
		}

		// Validate
		if err := config.Validate(cfg); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}

		// Save config
		if err := config.Save(burrowDir, cfg); err != nil {
			return fmt.Errorf("saving configuration: %w", err)
		}

		// Create standard directories
		for _, sub := range []string{"routines", "reports", "context", "contacts"} {
			if err := os.MkdirAll(filepath.Join(burrowDir, sub), 0o755); err != nil {
				return fmt.Errorf("creating %s directory: %w", sub, err)
			}
		}

		fmt.Printf("\n  Configuration saved to %s\n", configPath)
		fmt.Println("  Directories created: routines/, reports/, context/, contacts/")
		fmt.Println("\n  Next steps:")
		fmt.Println("  - Add routines to ~/.burrow/routines/")
		fmt.Println("  - Run: gd routines list")
		fmt.Println("  - Run: gd routines run <name>")

		return nil
	},
}

// runConversationalInit runs an LLM-driven init session, returning the final config.
// Returns (nil, nil) if no config was applied, so the caller can fall through to the wizard.
func runConversationalInit(ctx context.Context, session *configure.Session) (*config.Config, error) {
	scanner := bufio.NewScanner(os.Stdin)
	var appliedConfig *config.Config

	for {
		fmt.Print("  > ")
		if !scanner.Scan() {
			break
		}
		input := scanner.Text()
		if input == "done" || input == "quit" || input == "exit" {
			break
		}
		if input == "" {
			continue
		}

		response, change, err := session.ProcessMessage(ctx, input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			continue
		}

		fmt.Println("\n  " + response + "\n")

		if change != nil {
			fmt.Println("  Apply this configuration? (y/n)")
			fmt.Print("  > ")
			if scanner.Scan() && scanner.Text() == "y" {
				if err := session.ApplyChange(change); err != nil {
					fmt.Fprintf(os.Stderr, "  Error applying: %v\n", err)
				} else {
					if change.RemoteLLMWarning {
						fmt.Println()
						fmt.Println("  ⚠ This configuration includes a remote LLM provider.")
						fmt.Println("    Collected results will leave your machine during synthesis.")
						fmt.Println("    For maximum privacy, use a local LLM provider.")
						fmt.Println()
					}
					fmt.Println("  Configuration updated.")
					appliedConfig = change.Config
				}
			}
		}
	}

	// Only return a config that was actually applied and accepted.
	if appliedConfig != nil {
		return appliedConfig, nil
	}
	return nil, nil
}
