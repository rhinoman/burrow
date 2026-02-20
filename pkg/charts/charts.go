// Package charts handles parsing chart directives from markdown and rendering
// them as PNG images or ASCII text tables.
package charts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-analyze/charts"
	"github.com/jcadam/burrow/pkg/slug"
)

// ChartDirective represents a parsed chart directive from a fenced code block.
type ChartDirective struct {
	Type   string    // bar, line, pie
	Title  string
	Labels []string  // x-axis labels (bar/line) or slice labels (pie)
	Values []float64 // y-axis values (bar/line) or slice values (pie)
}

// ParseDirectives scans markdown for ```chart fenced code blocks and returns
// the parsed directives in order. Malformed or unknown-type blocks are skipped.
func ParseDirectives(markdown string) []ChartDirective {
	var directives []ChartDirective
	lines := strings.Split(markdown, "\n")

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "```chart" {
			continue
		}
		// Collect lines until closing ```
		i++
		var block []string
		closed := false
		for i < len(lines) {
			if strings.TrimSpace(lines[i]) == "```" {
				closed = true
				break
			}
			block = append(block, lines[i])
			i++
		}
		if !closed {
			continue
		}
		if d, ok := parseBlock(block); ok {
			directives = append(directives, d)
		}
	}
	return directives
}

// ReplaceDirectives replaces each ```chart block in markdown with the
// corresponding string from replacements (indexed by directive order).
// Blocks without a replacement entry are left unchanged.
func ReplaceDirectives(markdown string, replacements map[int]string) string {
	lines := strings.Split(markdown, "\n")
	var out []string
	idx := 0

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "```chart" {
			out = append(out, lines[i])
			continue
		}
		// Find closing ```
		start := i
		i++
		for i < len(lines) {
			if strings.TrimSpace(lines[i]) == "```" {
				break
			}
			i++
		}
		// i now points at closing ``` (or past end)
		if rep, ok := replacements[idx]; ok {
			out = append(out, rep)
		} else {
			// Keep original block
			out = append(out, lines[start:i+1]...)
		}
		idx++
	}
	return strings.Join(out, "\n")
}

// RenderPNG renders a chart directive as a PNG image using go-analyze/charts.
// Returns raw PNG bytes.
func RenderPNG(d ChartDirective, width, height int) ([]byte, error) {
	switch d.Type {
	case "bar":
		return renderBar(d, width, height)
	case "line":
		return renderLine(d, width, height)
	case "pie":
		return renderPie(d, width, height)
	default:
		return nil, fmt.Errorf("unsupported chart type: %q", d.Type)
	}
}

// RenderTextTable formats a chart directive as an ASCII table for terminals
// that do not support inline images.
func RenderTextTable(d ChartDirective) string {
	if len(d.Labels) == 0 || len(d.Values) == 0 {
		return ""
	}

	// Find column widths
	maxLabel := 0
	maxValue := 0
	valueStrs := make([]string, len(d.Values))
	for i, v := range d.Values {
		if i < len(d.Labels) && len(d.Labels[i]) > maxLabel {
			maxLabel = len(d.Labels[i])
		}
		valueStrs[i] = formatValue(v)
		if len(valueStrs[i]) > maxValue {
			maxValue = len(valueStrs[i])
		}
	}
	if maxLabel < 1 {
		maxLabel = 1
	}
	if maxValue < 1 {
		maxValue = 1
	}

	var b strings.Builder

	// Title
	if d.Title != "" {
		b.WriteString("  " + d.Title + "\n")
	}

	// Top border
	b.WriteString(fmt.Sprintf("  \u250c%s\u252c%s\u2510\n",
		strings.Repeat("\u2500", maxLabel+2),
		strings.Repeat("\u2500", maxValue+2)))

	// Rows
	count := len(d.Labels)
	if len(d.Values) < count {
		count = len(d.Values)
	}
	for i := 0; i < count; i++ {
		b.WriteString(fmt.Sprintf("  \u2502 %-*s \u2502 %*s \u2502\n",
			maxLabel, d.Labels[i],
			maxValue, valueStrs[i]))
	}

	// Bottom border
	b.WriteString(fmt.Sprintf("  \u2514%s\u2534%s\u2518",
		strings.Repeat("\u2500", maxLabel+2),
		strings.Repeat("\u2500", maxValue+2)))

	return b.String()
}

// LoadPNG loads a chart PNG from a charts directory by matching the directive's
// title (slugified) to a filename. Falls back to "chart-N" for generic titles.
func LoadPNG(chartsDir, title string, idx int) []byte {
	name := slug.Sanitize(title)
	if name == "chart" {
		name = fmt.Sprintf("chart-%d", idx)
	}
	data, err := os.ReadFile(filepath.Join(chartsDir, name+".png"))
	if err != nil {
		return nil
	}
	return data
}

// parseBlock parses key-value lines from a chart block.
func parseBlock(lines []string) (ChartDirective, bool) {
	var d ChartDirective
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := parseKV(line)
		if !ok {
			continue
		}
		switch key {
		case "type":
			d.Type = strings.Trim(value, `"'`)
		case "title":
			d.Title = strings.Trim(value, `"'`)
		case "x", "labels":
			d.Labels = parseStringArray(value)
		case "y", "values":
			d.Values = parseFloatArray(value)
		}
	}

	// Require at minimum a type and some data
	if d.Type == "" || len(d.Values) == 0 {
		return d, false
	}
	// Only accept known types
	switch d.Type {
	case "bar", "line", "pie":
	default:
		return d, false
	}
	if d.Title == "" {
		d.Title = "Chart"
	}
	return d, true
}

// parseKV splits "key: value" lines.
func parseKV(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

// parseStringArray parses JSON-style string arrays: ["a", "b", "c"]
func parseStringArray(s string) []string {
	s = strings.TrimSpace(s)
	var result []string
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// parseFloatArray parses JSON-style number arrays: [1, 2.5, 3]
func parseFloatArray(s string) []float64 {
	s = strings.TrimSpace(s)
	var result []float64
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// formatValue formats a float64 for display, omitting decimal places for integers.
func formatValue(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}

// renderBar creates a bar chart PNG.
func renderBar(d ChartDirective, width, height int) ([]byte, error) {
	values := make([]float64, len(d.Values))
	copy(values, d.Values)

	p, err := charts.BarRender(
		[][]float64{values},
		charts.TitleTextOptionFunc(d.Title),
		charts.XAxisLabelsOptionFunc(d.Labels),
		charts.DimensionsOptionFunc(width, height),
		charts.PNGOutputOptionFunc(),
	)
	if err != nil {
		return nil, fmt.Errorf("rendering bar chart: %w", err)
	}
	buf, err := p.Bytes()
	if err != nil {
		return nil, fmt.Errorf("encoding bar chart PNG: %w", err)
	}
	return buf, nil
}

// renderLine creates a line chart PNG.
func renderLine(d ChartDirective, width, height int) ([]byte, error) {
	values := make([]float64, len(d.Values))
	copy(values, d.Values)

	p, err := charts.LineRender(
		[][]float64{values},
		charts.TitleTextOptionFunc(d.Title),
		charts.XAxisLabelsOptionFunc(d.Labels),
		charts.DimensionsOptionFunc(width, height),
		charts.PNGOutputOptionFunc(),
	)
	if err != nil {
		return nil, fmt.Errorf("rendering line chart: %w", err)
	}
	buf, err := p.Bytes()
	if err != nil {
		return nil, fmt.Errorf("encoding line chart PNG: %w", err)
	}
	return buf, nil
}

// renderPie creates a pie chart PNG.
func renderPie(d ChartDirective, width, height int) ([]byte, error) {
	pieValues := make([]float64, len(d.Values))
	copy(pieValues, d.Values)

	p, err := charts.PieRender(
		pieValues,
		charts.TitleTextOptionFunc(d.Title),
		charts.LegendLabelsOptionFunc(d.Labels),
		charts.DimensionsOptionFunc(width, height),
		charts.PNGOutputOptionFunc(),
	)
	if err != nil {
		return nil, fmt.Errorf("rendering pie chart: %w", err)
	}
	buf, err := p.Bytes()
	if err != nil {
		return nil, fmt.Errorf("encoding pie chart PNG: %w", err)
	}
	return buf, nil
}
