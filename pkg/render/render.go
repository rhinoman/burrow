// Package render provides terminal markdown rendering and a scrollable report viewer.
package render

import (
	"fmt"

	"github.com/charmbracelet/glamour"
)

// RenderMarkdown renders markdown to styled terminal output using Glamour.
func RenderMarkdown(markdown string, width int) (string, error) {
	if width <= 0 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
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
