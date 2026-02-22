package main

import (
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
			// Session uses the unresolved config so YAML output preserves ${ENV_VAR} references.
			session := configure.NewSession(burrowDir, cfg, provider)
			return configure.RunTUI(cmd.Context(), session)
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
