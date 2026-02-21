package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/profile"
	"github.com/spf13/cobra"
)

func init() {
	profileCmd.AddCommand(profileEditCmd)
	rootCmd.AddCommand(profileCmd)
}

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Display or edit your user profile",
	Long:  "Shows the current user profile (identity, interests, and domain context). Use 'gd profile edit' to open it in your editor.",
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}

		p, err := profile.Load(burrowDir)
		if err != nil {
			return fmt.Errorf("loading profile: %w", err)
		}
		if p == nil {
			fmt.Println("No profile found.")
			fmt.Println("Create one with: gd init, gd configure, or gd profile edit")
			return nil
		}

		printProfile(p)
		return nil
	},
}

var profileEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open profile.yaml in your editor",
	RunE: func(cmd *cobra.Command, args []string) error {
		burrowDir, err := config.BurrowDir()
		if err != nil {
			return err
		}

		profilePath := filepath.Join(burrowDir, "profile.yaml")

		// Create a starter file if it doesn't exist
		if _, err := os.Stat(profilePath); os.IsNotExist(err) {
			starter := &profile.Profile{
				Raw: map[string]interface{}{
					"name":        "",
					"description": "",
					"interests":   []interface{}{},
				},
			}
			if err := profile.Save(burrowDir, starter); err != nil {
				return fmt.Errorf("creating profile: %w", err)
			}
		}

		editor := findEditor(burrowDir)
		c := exec.Command(editor, profilePath)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

// findEditor returns the editor to use, checking config, $EDITOR, then system default.
func findEditor(burrowDir string) string {
	// Check config apps.editor
	cfg, err := config.Load(burrowDir)
	if err == nil && cfg.Apps.Editor != "" {
		return cfg.Apps.Editor
	}

	// Check $EDITOR
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}

	// System default
	if runtime.GOOS == "darwin" {
		return "open"
	}
	return "xdg-open"
}

// printProfile displays a formatted summary of the user profile.
func printProfile(p *profile.Profile) {
	fmt.Println()
	if p.Name != "" {
		fmt.Printf("  Name: %s\n", p.Name)
	}
	if p.Description != "" {
		fmt.Printf("  Description: %s\n", strings.TrimSpace(p.Description))
	}
	if len(p.Interests) > 0 {
		fmt.Printf("  Interests: %s\n", strings.Join(p.Interests, ", "))
	}

	// Print additional fields from raw map, sorted for stable output.
	if p.Raw != nil {
		keys := make([]string, 0, len(p.Raw))
		for key := range p.Raw {
			switch key {
			case "name", "description", "interests":
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if val, ok := p.Get(key); ok {
				label := strings.ReplaceAll(key, "_", " ")
				if len(label) > 0 {
					label = strings.ToUpper(label[:1]) + label[1:]
				}
				fmt.Printf("  %s: %s\n", label, val)
			}
		}
	}
	fmt.Println()
}
