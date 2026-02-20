package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSnippet(t *testing.T) {
	text := "This is a report about geospatial analysis and other topics."

	snippet := extractSnippet(text, "geospatial", 30)
	if snippet == "" {
		t.Fatal("expected non-empty snippet")
	}
	if len([]rune(snippet)) > 30 {
		t.Errorf("snippet too long: %d runes", len([]rune(snippet)))
	}
}

func TestExtractSnippetCaseInsensitive(t *testing.T) {
	text := "Report contains IMPORTANT findings."
	snippet := extractSnippet(text, "important", 40)
	if snippet == "" {
		t.Fatal("expected match for case-insensitive query")
	}
}

func TestExtractSnippetNoMatch(t *testing.T) {
	text := "Nothing relevant here."
	snippet := extractSnippet(text, "nonexistent", 40)
	if snippet != "" {
		t.Errorf("expected empty snippet, got %q", snippet)
	}
}

func TestExtractSnippetUTF8(t *testing.T) {
	// Ensure multi-byte characters aren't split
	text := "Üntersuchung der Ökonomie — eine Übersicht mit Analyse der Märkte"
	snippet := extractSnippet(text, "Übersicht", 20)
	if snippet == "" {
		t.Fatal("expected match for UTF-8 query")
	}
	// Verify snippet is valid UTF-8 by round-tripping through runes
	runes := []rune(snippet)
	if string(runes) != snippet {
		t.Error("snippet contains invalid UTF-8")
	}
}

func TestExtractSnippetNewlineRemoval(t *testing.T) {
	text := "Line one\nLine two\nLine three"
	snippet := extractSnippet(text, "two", 40)
	if snippet == "" {
		t.Fatal("expected non-empty snippet")
	}
	for _, r := range snippet {
		if r == '\n' {
			t.Error("expected newlines to be replaced")
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
	}
	for _, tc := range tests {
		got := formatBytes(tc.input)
		if got != tc.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestResolveReport(t *testing.T) {
	dir := t.TempDir()

	// Create reports
	for _, name := range []string{
		"2026-02-17T0800-morning-intel",
		"2026-02-19T0500-morning-intel",
		"2026-02-18T0900-afternoon-brief",
	} {
		reportDir := filepath.Join(dir, name)
		os.MkdirAll(reportDir, 0o755)
		os.WriteFile(filepath.Join(reportDir, "report.md"), []byte("# Report\n"), 0o644)
	}

	// Exact match
	r, err := resolveReport(dir, "morning-intel")
	if err != nil {
		t.Fatalf("exact match: %v", err)
	}
	if r.Date != "2026-02-19" {
		t.Errorf("expected latest 2026-02-19, got %q", r.Date)
	}

	// Fuzzy match
	r, err = resolveReport(dir, "morning")
	if err != nil {
		t.Fatalf("fuzzy match: %v", err)
	}
	if r.Routine != "morning-intel" {
		t.Errorf("expected morning-intel, got %q", r.Routine)
	}

	// Date prefix match
	r, err = resolveReport(dir, "2026-02-18")
	if err != nil {
		t.Fatalf("date prefix match: %v", err)
	}
	if r.Routine != "afternoon-brief" {
		t.Errorf("expected afternoon-brief, got %q", r.Routine)
	}

	// No match
	_, err = resolveReport(dir, "nonexistent")
	if err == nil {
		t.Error("expected error for no match")
	}
}
