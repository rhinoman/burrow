package reports

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jcadam/burrow/pkg/charts"
	"github.com/yuin/goldmark"
)

// lookPath is an injectable wrapper around exec.LookPath for testing.
var lookPath = exec.LookPath

// PDFConverter describes an external tool that can convert HTML to PDF.
type PDFConverter struct {
	Name string
	Args func(inPath, outPath string) []string
}

// findPDFConverter probes PATH for a supported HTML-to-PDF converter.
// Returns nil if none found. Preference order: weasyprint, wkhtmltopdf, pandoc.
func findPDFConverter() *PDFConverter {
	converters := []PDFConverter{
		{
			Name: "weasyprint",
			Args: func(in, out string) []string { return []string{in, out} },
		},
		{
			Name: "wkhtmltopdf",
			Args: func(in, out string) []string { return []string{"--quiet", in, out} },
		},
		{
			Name: "pandoc",
			Args: func(in, out string) []string { return []string{in, "-o", out} },
		},
	}
	for _, c := range converters {
		if _, err := lookPath(c.Name); err == nil {
			found := c // copy
			return &found
		}
	}
	return nil
}

// ExportPDF converts markdown to PDF via an external converter.
// It first renders self-contained HTML (with embedded charts), then shells out
// to a converter. If no converter is found, it returns an error with install
// instructions.
func ExportPDF(markdown, title, reportDir string) ([]byte, error) {
	conv := findPDFConverter()
	if conv == nil {
		return nil, fmt.Errorf("no PDF converter found.\n\nInstall one of:\n  weasyprint  — pip install weasyprint\n  wkhtmltopdf — https://wkhtmltopdf.org\n  pandoc      — https://pandoc.org")
	}

	htmlContent, err := ExportHTML(markdown, title, reportDir)
	if err != nil {
		return nil, fmt.Errorf("generating HTML for PDF: %w", err)
	}

	return convertHTMLToPDF(conv, htmlContent)
}

// convertHTMLToPDF writes HTML to a temp file, runs the converter, and returns
// the resulting PDF bytes.
func convertHTMLToPDF(conv *PDFConverter, htmlContent string) ([]byte, error) {
	inFile, err := os.CreateTemp("", "burrow-export-*.html")
	if err != nil {
		return nil, fmt.Errorf("creating temp HTML file: %w", err)
	}
	defer os.Remove(inFile.Name())

	if _, err := inFile.WriteString(htmlContent); err != nil {
		inFile.Close()
		return nil, fmt.Errorf("writing temp HTML: %w", err)
	}
	inFile.Close()

	outFile, err := os.CreateTemp("", "burrow-export-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("creating temp PDF file: %w", err)
	}
	outFile.Close()
	defer os.Remove(outFile.Name())

	args := conv.Args(inFile.Name(), outFile.Name())
	cmd := exec.Command(conv.Name, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := stderr.String()
		if detail != "" {
			return nil, fmt.Errorf("%s failed: %w\n%s", conv.Name, err, detail)
		}
		return nil, fmt.Errorf("%s failed: %w", conv.Name, err)
	}

	pdfData, err := os.ReadFile(outFile.Name())
	if err != nil {
		return nil, fmt.Errorf("reading PDF output: %w", err)
	}

	return pdfData, nil
}

// ExportHTML converts markdown to a self-contained HTML document.
// If reportDir is non-empty and contains a charts/ subdirectory, chart
// fenced code blocks are replaced with embedded PNG images (base64 data URIs).
func ExportHTML(markdown, title, reportDir string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("converting markdown to HTML: %w", err)
	}

	// Post-process: replace chart code blocks in the HTML output
	body := replaceChartCodeBlocks(buf.String(), markdown, reportDir)

	escaped := html.EscapeString(title)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
  body { max-width: 48em; margin: 2em auto; padding: 0 1em; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; line-height: 1.6; color: #1a1a1a; }
  h1, h2, h3 { margin-top: 1.5em; }
  code { background: #f4f4f4; padding: 0.15em 0.3em; border-radius: 3px; font-size: 0.9em; }
  pre { background: #f4f4f4; padding: 1em; border-radius: 4px; overflow-x: auto; }
  pre code { background: none; padding: 0; }
  blockquote { border-left: 3px solid #ccc; margin-left: 0; padding-left: 1em; color: #555; }
  table { border-collapse: collapse; width: 100%%; }
  th, td { border: 1px solid #ddd; padding: 0.5em; text-align: left; }
  th { background: #f4f4f4; }
  img { max-width: 100%%; height: auto; }
</style>
</head>
<body>
%s
</body>
</html>`, escaped, body), nil
}

// chartCodeBlockPattern matches goldmark's output for ```chart code blocks.
// Goldmark renders them as <pre><code class="language-chart">...</code></pre>.
var chartCodeBlockPattern = regexp.MustCompile(`(?s)<pre><code class="language-chart">.*?</code></pre>`)

// replaceChartCodeBlocks finds chart code blocks in the HTML output and replaces
// them with embedded images or HTML tables.
func replaceChartCodeBlocks(htmlBody, rawMarkdown, reportDir string) string {
	directives := charts.ParseDirectives(rawMarkdown)
	if len(directives) == 0 {
		return htmlBody
	}

	matches := chartCodeBlockPattern.FindAllStringIndex(htmlBody, -1)
	if len(matches) == 0 {
		return htmlBody
	}

	chartsDir := ""
	if reportDir != "" {
		chartsDir = filepath.Join(reportDir, "charts")
	}

	// Replace in reverse order to preserve indices
	result := htmlBody
	for i := len(matches) - 1; i >= 0; i-- {
		if i >= len(directives) {
			continue
		}
		d := directives[i]
		var replacement string

		if chartsDir != "" {
			if pngData := charts.LoadPNG(chartsDir, d.Title, i); pngData != nil {
				b64 := base64.StdEncoding.EncodeToString(pngData)
				alt := html.EscapeString(d.Title)
				replacement = fmt.Sprintf(
					`<img src="data:image/png;base64,%s" alt="%s">`,
					b64, alt,
				)
			}
		}
		if replacement == "" {
			replacement = chartToHTMLTable(d)
		}

		result = result[:matches[i][0]] + replacement + result[matches[i][1]:]
	}

	return result
}

// chartToHTMLTable renders a chart directive as an HTML table.
func chartToHTMLTable(d charts.ChartDirective) string {
	var b strings.Builder
	if d.Title != "" {
		b.WriteString(fmt.Sprintf("<h4>%s</h4>\n", html.EscapeString(d.Title)))
	}
	b.WriteString("<table><thead><tr><th>Label</th><th>Value</th></tr></thead><tbody>\n")
	count := len(d.Labels)
	if len(d.Values) < count {
		count = len(d.Values)
	}
	for i := 0; i < count; i++ {
		b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>\n",
			html.EscapeString(d.Labels[i]),
			formatHTMLValue(d.Values[i])))
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

// formatHTMLValue formats a float64 for HTML display.
func formatHTMLValue(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}
