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
	rootCmd.AddCommand(configureCmd)
}

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Modify Burrow configuration",
	Long:  "Interactively modify your Burrow configuration. Uses conversational mode if an LLM is available, otherwise falls back to a structured wizard.",
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}

		// Require existing config
		configPath := filepath.Join(burrowDir, "config.yaml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			fmt.Println("No configuration found. Run 'gd init' first.")
			return nil
		}

		cfg, err := config.Load(burrowDir)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// Try conversational config with detected provider.
		// Resolve env vars on a copy so we can construct providers,
		// but keep the original cfg with ${ENV_VAR} references intact for saving.
		resolvedCfg := cfg.DeepCopy()
		config.ResolveEnvVars(resolvedCfg)

		provider := configure.DetectProvider(resolvedCfg)
		if provider == nil {
			provider = configure.DetectOllama()
		}

		if provider != nil {
			fmt.Println("  LLM available — starting conversational configuration.")
			fmt.Println("  Describe what you want to change, or 'done' to finish.")
			fmt.Println()

			// Session uses the unresolved config so YAML output preserves ${ENV_VAR} references.
			session := configure.NewSession(burrowDir, cfg, provider)
			return runConversationalConfigure(cmd.Context(), session)
		}

		// Fallback to wizard — operates on unresolved config to preserve ${ENV_VAR} references.
		wiz := configure.NewWizard(os.Stdin, os.Stdout)
		if err := wiz.RunModify(cfg); err != nil {
			return fmt.Errorf("configuration wizard: %w", err)
		}

		if err := config.Validate(cfg); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}

		if err := config.Save(burrowDir, cfg); err != nil {
			return fmt.Errorf("saving configuration: %w", err)
		}

		fmt.Printf("\n  Configuration saved to %s\n", configPath)
		return nil
	},
}

// runConversationalConfigure runs an LLM-driven config modification loop.
func runConversationalConfigure(ctx context.Context, session *configure.Session) error {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("  > ")
		if !scanner.Scan() {
			return nil
		}
		input := scanner.Text()
		if input == "done" || input == "quit" || input == "exit" {
			return nil
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
			fmt.Println("  Apply this change? (y/n)")
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
				}
			} else {
				fmt.Println("  Change discarded.")
			}
		}
	}
}
