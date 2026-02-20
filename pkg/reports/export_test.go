package reports

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportHTML(t *testing.T) {
	md := "# Test Report\n\nSome **bold** content.\n\n- Item 1\n- Item 2\n"
	html, err := ExportHTML(md, "Test Report", "")
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}

	if !strings.Contains(html, "<title>Test Report</title>") {
		t.Error("expected title in HTML")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("expected DOCTYPE")
	}
	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Error("expected bold tag in HTML")
	}
	if !strings.Contains(html, "<li>Item 1</li>") {
		t.Error("expected list items in HTML")
	}
}

func TestExportHTMLEscapesTitle(t *testing.T) {
	html, err := ExportHTML("# Test\n", `Title with "quotes" & <brackets>`, "")
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	if strings.Contains(html, `<title>Title with "quotes"`) {
		t.Error("expected escaped title")
	}
	if !strings.Contains(html, "&amp;") {
		t.Error("expected HTML-escaped ampersand in title")
	}
}

func TestExportHTMLCodeBlock(t *testing.T) {
	md := "```json\n{\"key\": \"value\"}\n```\n"
	html, err := ExportHTML(md, "Code Report", "")
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	if !strings.Contains(html, "<code") {
		t.Error("expected code element in HTML")
	}
}

func TestExportHTMLWithChartPNG(t *testing.T) {
	dir := t.TempDir()

	// Create a chart PNG file
	chartsDir := filepath.Join(dir, "charts")
	os.MkdirAll(chartsDir, 0o755)
	// Write a minimal valid PNG (8-byte header)
	pngHeader := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	os.WriteFile(filepath.Join(chartsDir, "test-chart.png"), pngHeader, 0o644)

	md := "# Report\n\n```chart\ntype: bar\ntitle: \"Test Chart\"\nx: [\"A\"]\ny: [1]\n```\n"
	html, err := ExportHTML(md, "Chart Report", dir)
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}

	// Should contain base64-encoded image
	if !strings.Contains(html, "data:image/png;base64,") {
		t.Error("expected base64 data URI for chart image")
	}
	if !strings.Contains(html, `alt="Test Chart"`) {
		t.Error("expected alt text for chart image")
	}
	// Should NOT contain the raw chart code block
	if strings.Contains(html, "```chart") {
		t.Error("expected chart block to be replaced")
	}
}

func TestExportHTMLChartFallbackTable(t *testing.T) {
	// No reportDir — charts should fall back to HTML tables
	md := "# Report\n\n```chart\ntype: bar\ntitle: \"Fallback\"\nx: [\"A\", \"B\"]\ny: [10, 20]\n```\n"
	html, err := ExportHTML(md, "Fallback Report", "")
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}

	if !strings.Contains(html, "<table>") {
		t.Error("expected HTML table fallback")
	}
	if !strings.Contains(html, "Fallback") {
		t.Error("expected chart title in table")
	}
	if !strings.Contains(html, ">A<") {
		t.Error("expected label A in table")
	}
}
