package reports

import (
	"fmt"
	"os"
	"os/exec"
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

func TestFindPDFConverterNone(t *testing.T) {
	orig := lookPath
	defer func() { lookPath = orig }()

	lookPath = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}

	conv := findPDFConverter()
	if conv != nil {
		t.Errorf("expected nil converter when no tools available, got %s", conv.Name)
	}
}

func TestFindPDFConverterOrder(t *testing.T) {
	orig := lookPath
	defer func() { lookPath = orig }()

	// Only pandoc is available
	lookPath = func(file string) (string, error) {
		if file == "pandoc" {
			return "/usr/bin/pandoc", nil
		}
		return "", exec.ErrNotFound
	}

	conv := findPDFConverter()
	if conv == nil {
		t.Fatal("expected pandoc converter")
	}
	if conv.Name != "pandoc" {
		t.Errorf("expected pandoc, got %s", conv.Name)
	}
}

func TestFindPDFConverterPreference(t *testing.T) {
	orig := lookPath
	defer func() { lookPath = orig }()

	// Both weasyprint and wkhtmltopdf available — weasyprint should win
	lookPath = func(file string) (string, error) {
		if file == "weasyprint" || file == "wkhtmltopdf" {
			return "/usr/bin/" + file, nil
		}
		return "", exec.ErrNotFound
	}

	conv := findPDFConverter()
	if conv == nil {
		t.Fatal("expected weasyprint converter")
	}
	if conv.Name != "weasyprint" {
		t.Errorf("expected weasyprint, got %s", conv.Name)
	}
}

func TestExportPDFNoTool(t *testing.T) {
	orig := lookPath
	defer func() { lookPath = orig }()

	lookPath = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}

	_, err := ExportPDF("# Test\n", "Test", "")
	if err == nil {
		t.Fatal("expected error when no PDF converter available")
	}
	msg := err.Error()
	for _, tool := range []string{"weasyprint", "wkhtmltopdf", "pandoc"} {
		if !strings.Contains(msg, tool) {
			t.Errorf("error message should mention %s: %s", tool, msg)
		}
	}
}

func TestExportPDFIntegration(t *testing.T) {
	// Only run if a real converter is available
	orig := lookPath
	lookPath = exec.LookPath // use real LookPath
	conv := findPDFConverter()
	lookPath = orig

	if conv == nil {
		t.Skip("no PDF converter installed — skipping integration test")
	}

	// Use real lookPath for the full test
	lookPath = exec.LookPath
	defer func() { lookPath = orig }()

	pdf, err := ExportPDF("# Integration Test\n\nHello, PDF.\n", "Integration Test", "")
	if err != nil {
		t.Fatalf("ExportPDF: %v", err)
	}
	if len(pdf) < 4 || string(pdf[:5]) != "%PDF-" {
		t.Errorf("expected PDF magic bytes, got %q", string(pdf[:min(len(pdf), 10)]))
	}

	// Write to temp file to verify it's a valid file
	outPath := filepath.Join(t.TempDir(), "test.pdf")
	if err := os.WriteFile(outPath, pdf, 0o644); err != nil {
		t.Fatalf("writing PDF: %v", err)
	}
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat PDF: %v", err)
	}
	if info.Size() == 0 {
		t.Error("PDF file is empty")
	}
	fmt.Printf("PDF export successful (%d bytes) using %s\n", info.Size(), conv.Name)
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
