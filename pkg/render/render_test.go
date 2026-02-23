package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/glamour"
)

func TestRenderMarkdown(t *testing.T) {
	md := "# Hello World\n\nThis is a **test**.\n\n- Item 1\n- Item 2\n"

	out, err := RenderMarkdown(md, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(out, "Hello World") {
		t.Error("expected title in output")
	}
}

func TestRenderMarkdownDefaultWidth(t *testing.T) {
	md := "# Test\n\nContent.\n"
	out, err := RenderMarkdown(md, 0)
	if err != nil {
		t.Fatalf("RenderMarkdown with zero width: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestRenderMarkdownCodeBlock(t *testing.T) {
	md := "```json\n{\"key\": \"value\"}\n```\n"
	out, err := RenderMarkdown(md, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown code block: %v", err)
	}
	if !strings.Contains(out, "key") {
		t.Error("expected code block content in output")
	}
}

func TestRenderMarkdownCachedRenderer(t *testing.T) {
	// Clear cache to isolate this test.
	rendererMu.Lock()
	rendererCache = make(map[rendererCacheKey]*glamour.TermRenderer)
	rendererMu.Unlock()

	md := "# Cached\n\nTest content.\n"

	out1, err := RenderMarkdown(md, 80)
	if err != nil {
		t.Fatalf("first RenderMarkdown: %v", err)
	}

	// Cache should now have one entry.
	rendererMu.Lock()
	cacheLen := len(rendererCache)
	rendererMu.Unlock()
	if cacheLen != 1 {
		t.Fatalf("expected 1 cached renderer, got %d", cacheLen)
	}

	out2, err := RenderMarkdown(md, 80)
	if err != nil {
		t.Fatalf("second RenderMarkdown: %v", err)
	}

	if out1 != out2 {
		t.Error("expected identical output from cached renderer")
	}

	// Different width should create a second cache entry.
	_, err = RenderMarkdown(md, 120)
	if err != nil {
		t.Fatalf("third RenderMarkdown: %v", err)
	}
	rendererMu.Lock()
	cacheLen = len(rendererCache)
	rendererMu.Unlock()
	if cacheLen != 2 {
		t.Fatalf("expected 2 cached renderers, got %d", cacheLen)
	}
}

func TestNewViewer(t *testing.T) {
	v := NewViewer("Test Title", "Some content")
	if v.title != "Test Title" {
		t.Errorf("expected title, got %q", v.title)
	}
}
