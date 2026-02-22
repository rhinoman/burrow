package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

// readMultiLine reads user input, accumulating multiple lines when they
// arrive together (e.g. pasted text). Single-line typed input is returned
// immediately. Returns io.EOF when stdin is closed.
func readMultiLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF && line == "" {
		return "", io.EOF
	}

	var lines []string
	if trimmed := strings.TrimRight(line, "\n\r"); trimmed != "" {
		lines = append(lines, trimmed)
	}

	// Accumulate additional buffered lines (paste detection).
	for reader.Buffered() > 0 {
		extra, err := reader.ReadString('\n')
		if trimmed := strings.TrimRight(extra, "\n\r"); trimmed != "" {
			lines = append(lines, trimmed)
		}
		if err != nil {
			break
		}
	}

	return strings.Join(lines, "\n"), nil
}

// readConfirm reads a single line for y/n confirmation.
func readConfirm(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// runConversationalConfigure runs an LLM-driven config modification loop.
func runConversationalConfigure(ctx context.Context, session *configure.Session) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("  > ")
		input, err := readMultiLine(reader)
		if err != nil {
			return nil
		}
		if input == "done" || input == "quit" || input == "exit" {
			return nil
		}
		if input == "" {
			continue
		}

		response, change, profChange, routineChange, err := session.ProcessMessage(ctx, input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			continue
		}

		fmt.Println("\n  " + response + "\n")

		if profChange != nil {
			fmt.Println("  Apply profile change? (y/n)")
			fmt.Print("  > ")
			if readConfirm(reader) == "y" {
				if err := session.ApplyProfileChange(profChange); err != nil {
					fmt.Fprintf(os.Stderr, "  Error applying profile: %v\n", err)
				} else {
					fmt.Println("  Profile updated.")
				}
			} else {
				fmt.Println("  Profile change discarded.")
			}
		}

		if routineChange != nil {
			action := "Create"
			if !routineChange.IsNew {
				action = "Update"
			}
			fmt.Printf("  %s routine %q? (y/n)\n", action, routineChange.Routine.Name)
			fmt.Print("  > ")
			if readConfirm(reader) == "y" {
				if err := session.ApplyRoutineChange(routineChange); err != nil {
					fmt.Fprintf(os.Stderr, "  Error applying routine: %v\n", err)
				} else {
					fmt.Printf("  Routine %q saved.\n", routineChange.Routine.Name)
				}
			} else {
				fmt.Println("  Routine change discarded.")
			}
		}

		if change != nil {
			fmt.Println("  Apply this configuration change? (y/n)")
			fmt.Print("  > ")
			if readConfirm(reader) == "y" {
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
