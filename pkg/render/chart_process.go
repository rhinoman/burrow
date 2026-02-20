package render

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jcadam/burrow/pkg/charts"
)

// chartMarkerPrefix is the marker used to identify chart positions in rendered output.
// Uses hyphens (not underscores) to avoid CommonMark strong emphasis interpretation.
const chartMarkerPrefix = "BURROW-CHART-"

// hasChartDirectives returns true if the markdown contains any chart code blocks.
func hasChartDirectives(markdown string) bool {
	return strings.Contains(markdown, "```chart")
}

// processCharts replaces chart fenced blocks in the rendered content with
// either inline images (Tier 1) or text tables (Tier 2).
//
// Strategy:
//  1. Parse chart directives from raw markdown.
//  2. Create a copy of raw markdown with chart blocks replaced by unique markers.
//  3. Render the marked-up markdown through Glamour.
//  4. In the Glamour output, replace markers with chart content.
func processCharts(raw, rendered, reportDir string, tier ImageTier) string {
	directives := charts.ParseDirectives(raw)
	if len(directives) == 0 {
		return rendered
	}

	// Build a marker-injected copy of the raw markdown
	replacements := make(map[int]string)
	for i := range directives {
		replacements[i] = fmt.Sprintf("%s%d", chartMarkerPrefix, i)
	}
	markedMD := charts.ReplaceDirectives(raw, replacements)

	// Render the marked-up markdown
	markedRendered, err := RenderMarkdown(markedMD, 0)
	if err != nil {
		// Fall back to original rendered content
		return rendered
	}

	chartsDir := ""
	if reportDir != "" {
		chartsDir = filepath.Join(reportDir, "charts")
	}

	// Now replace each marker in the rendered output with chart content
	for i, d := range directives {
		marker := fmt.Sprintf("%s%d", chartMarkerPrefix, i)
		var replacement string

		if tier != TierNone && chartsDir != "" {
			pngData := charts.LoadPNG(chartsDir, d.Title, i)
			if pngData != nil {
				var buf bytes.Buffer
				if err := WriteInlineImage(&buf, pngData, tier); err == nil {
					replacement = buf.String() + "\n"
				}
			}
		}

		// Fall back to text table if no inline image was produced
		if replacement == "" {
			replacement = charts.RenderTextTable(d)
		}

		markedRendered = strings.Replace(markedRendered, marker, replacement, 1)
	}

	return markedRendered
}

// openFirstChart opens the first chart PNG in an external viewer.
func (v Viewer) openFirstChart() (tea.Model, tea.Cmd) {
	if v.handoff == nil || v.reportDir == "" {
		v.setStatus("No charts available")
		return v, nil
	}

	chartsDir := filepath.Join(v.reportDir, "charts")
	entries, err := os.ReadDir(chartsDir)
	if err != nil || len(entries) == 0 {
		v.setStatus("No chart files found")
		return v, nil
	}

	chartPath := filepath.Join(chartsDir, entries[0].Name())
	handoff := v.handoff
	v.busy = true
	return v, func() tea.Msg {
		err := handoff.OpenFile(chartPath)
		if err != nil {
			return actionResultMsg{err: err}
		}
		return actionResultMsg{status: "Opened: " + entries[0].Name()}
	}
}
