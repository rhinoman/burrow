// Package render provides terminal markdown rendering and a scrollable report viewer.
package render

import (
	"fmt"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

// rendererCacheKey identifies a cached glamour renderer.
type rendererCacheKey struct {
	width         int
	useBurrowStyle bool
}

var (
	rendererMu    sync.Mutex
	rendererCache = make(map[rendererCacheKey]*glamour.TermRenderer)
)

// RenderMarkdown renders markdown to styled terminal output using Glamour.
// An optional ImageTier can be passed to enable the custom Burrow style on
// Tier 1 terminals. When omitted (or TierNone), the default auto-style is used.
//
// The renderer cache is protected by a mutex that covers both cache access and
// the Render call, since glamour.TermRenderer is not safe for concurrent use.
func RenderMarkdown(markdown string, width int, tier ...ImageTier) (string, error) {
	if width <= 0 {
		width = 80
	}

	useBurrow := len(tier) > 0 && tier[0] != TierNone
	key := rendererCacheKey{width: width, useBurrowStyle: useBurrow}

	rendererMu.Lock()
	defer rendererMu.Unlock()

	r, ok := rendererCache[key]
	if !ok {
		var styleOpt glamour.TermRendererOption
		if useBurrow {
			styleOpt = glamour.WithStyles(burrowStyle())
		} else {
			styleOpt = glamour.WithAutoStyle()
		}

		var err error
		r, err = glamour.NewTermRenderer(
			styleOpt,
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return "", fmt.Errorf("creating renderer: %w", err)
		}
		rendererCache[key] = r
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
