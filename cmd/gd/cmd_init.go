package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/configure"
	"github.com/jcadam/burrow/pkg/profile"
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
		var prof *profile.Profile

		// Try to detect Ollama for conversational config
		if provider := configure.DetectOllama(); provider != nil {
			session := configure.NewSession(burrowDir, &config.Config{}, provider)
			cfg, err = configure.RunInitTUI(cmd.Context(), session)
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

			// Profile step first (before services)
			prof = wiz.ConfigureProfile()

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

		// Save profile if created
		if prof != nil {
			if err := profile.Save(burrowDir, prof); err != nil {
				return fmt.Errorf("saving profile: %w", err)
			}
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
