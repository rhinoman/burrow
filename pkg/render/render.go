// Package render provides terminal markdown rendering and a scrollable report viewer.
package render

import (
	"fmt"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

// RenderMarkdown renders markdown to styled terminal output using Glamour.
// An optional ImageTier can be passed to enable the custom Burrow style on
// Tier 1 terminals. When omitted (or TierNone), the default auto-style is used.
func RenderMarkdown(markdown string, width int, tier ...ImageTier) (string, error) {
	if width <= 0 {
		width = 80
	}

	var styleOpt glamour.TermRendererOption
	if len(tier) > 0 && tier[0] != TierNone {
		styleOpt = glamour.WithStyles(burrowStyle())
	} else {
		styleOpt = glamour.WithAutoStyle()
	}

	r, err := glamour.NewTermRenderer(
		styleOpt,
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", fmt.Errorf("creating renderer: %w", err)
	}
	out, err := r.Render(markdown)
	if err != nil {
		return "", fmt.Errorf("rendering markdown: %w", err)
	}
	return out, nil
}

// burrowStyle returns a custom Glamour style based on TokyoNight with Burrow
// refinements: subtle H1 background, Unicode horizontal rules, and styled
// block quotes.
func burrowStyle() ansi.StyleConfig {
	s := styles.TokyoNightStyleConfig

	// H1: subtle dark background for a banner effect
	s.H1.BackgroundColor = stringPtr("#1a1b26")

	// Horizontal rule: cleaner Unicode line
	s.HorizontalRule.Format = "\n──────────\n"

	return s
}

func stringPtr(s string) *string { return &s }
