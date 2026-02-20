package reports

import (
	"strings"
	"testing"
)

func TestExportHTML(t *testing.T) {
	md := "# Test Report\n\nSome **bold** content.\n\n- Item 1\n- Item 2\n"
	html, err := ExportHTML(md, "Test Report")
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
	html, err := ExportHTML("# Test\n", `Title with "quotes" & <brackets>`)
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
	html, err := ExportHTML(md, "Code Report")
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	if !strings.Contains(html, "<code") {
		t.Error("expected code element in HTML")
	}
}
