package render

import (
	"strings"
	"testing"
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

func TestNewViewer(t *testing.T) {
	v := NewViewer("Test Title", "Some content")
	if v.title != "Test Title" {
		t.Errorf("expected title, got %q", v.title)
	}
}
