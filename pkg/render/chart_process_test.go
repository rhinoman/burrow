package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasChartDirectives(t *testing.T) {
	if !hasChartDirectives("# Report\n\n```chart\ntype: bar\n```\n") {
		t.Error("expected true for markdown with chart block")
	}
	if hasChartDirectives("# Report\n\nNo charts here.\n") {
		t.Error("expected false for markdown without chart block")
	}
}

func TestProcessChartsTextTable(t *testing.T) {
	raw := "# Report\n\n```chart\ntype: bar\ntitle: \"Postings\"\nx: [\"NGA\", \"NRO\"]\ny: [12, 4]\n```\n\nMore text.\n"

	rendered, err := RenderMarkdown(raw, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	result := processCharts(raw, rendered, "", TierNone)

	// Should contain the text table
	if !strings.Contains(result, "Postings") {
		t.Error("expected chart title in output")
	}
	if !strings.Contains(result, "NGA") {
		t.Error("expected label NGA in output")
	}
	if !strings.Contains(result, "12") {
		t.Error("expected value 12 in output")
	}

	// Should NOT contain the raw chart block or mangled markers
	if strings.Contains(result, "```chart") {
		t.Error("expected chart block to be replaced")
	}
	if strings.Contains(result, "BURROW-CHART-") {
		t.Error("expected markers to be replaced with chart content")
	}
}

func TestProcessChartsMarkerNotMangled(t *testing.T) {
	// This test directly verifies the HIGH bug fix:
	// markers must survive Glamour rendering without being interpreted as markdown.
	raw := "# Report\n\n```chart\ntype: bar\ntitle: \"Test\"\nx: [\"A\"]\ny: [1]\n```\n"

	rendered, err := RenderMarkdown(raw, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	result := processCharts(raw, rendered, "", TierNone)

	// If markers were mangled by Glamour (e.g., double underscores → bold),
	// the replacement wouldn't happen and markers would remain in the output.
	if strings.Contains(result, "BURROW") {
		t.Error("marker text found in output — markers were not properly replaced (likely mangled by Glamour)")
	}

	// The text table should be present instead
	if !strings.Contains(result, "Test") {
		t.Error("expected chart title in text table output")
	}
}

func TestProcessChartsNoDirectives(t *testing.T) {
	raw := "# Report\n\nNo charts.\n"
	rendered, _ := RenderMarkdown(raw, 80)

	result := processCharts(raw, rendered, "", TierNone)
	if result != rendered {
		t.Error("expected unchanged output when no chart directives")
	}
}

func TestProcessChartsMultiple(t *testing.T) {
	raw := "# Report\n\n```chart\ntype: bar\ntitle: \"First\"\nx: [\"A\"]\ny: [1]\n```\n\nMiddle text.\n\n```chart\ntype: line\ntitle: \"Second\"\nx: [\"B\"]\ny: [2]\n```\n"

	rendered, err := RenderMarkdown(raw, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	result := processCharts(raw, rendered, "", TierNone)

	if !strings.Contains(result, "First") {
		t.Error("expected first chart title")
	}
	if !strings.Contains(result, "Second") {
		t.Error("expected second chart title")
	}
	if strings.Contains(result, "BURROW-CHART-") {
		t.Error("expected all markers replaced")
	}
}

func TestProcessChartsWithPNG(t *testing.T) {
	dir := t.TempDir()
	chartsDir := filepath.Join(dir, "charts")
	os.MkdirAll(chartsDir, 0o755)

	// Write a fake PNG (WriteInlineImage with TierNone will no-op,
	// so this tests the loading path and fallback behavior)
	os.WriteFile(filepath.Join(chartsDir, "test.png"), []byte("fake png data"), 0o644)

	raw := "# Report\n\n```chart\ntype: bar\ntitle: \"Test\"\nx: [\"A\"]\ny: [1]\n```\n"
	rendered, err := RenderMarkdown(raw, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	// With TierNone, even though PNG exists, it should fall back to text table
	result := processCharts(raw, rendered, dir, TierNone)
	if !strings.Contains(result, "Test") {
		t.Error("expected text table fallback")
	}
}

func TestProcessChartsTierNoneUsedInViewport(t *testing.T) {
	// Documents the deliberate design choice: the TUI viewer always passes
	// TierNone to processCharts, even on Tier 1 terminals. Kitty/iTerm
	// floating images don't scroll with BubbleTea's line-based viewport,
	// so we render text tables inline and let the user press 'i' to open
	// the full PNG in an external viewer.
	dir := t.TempDir()
	chartsDir := filepath.Join(dir, "charts")
	os.MkdirAll(chartsDir, 0o755)

	// Create a valid-header PNG so LoadPNG would succeed on a real tier
	pngHeader := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1 pixel
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, // RGB, CRC
	}
	os.WriteFile(filepath.Join(chartsDir, "postings_0.png"), pngHeader, 0o644)

	raw := "# Report\n\n```chart\ntype: bar\ntitle: \"Postings\"\nx: [\"NGA\", \"NRO\"]\ny: [12, 4]\n```\n\nAnalysis follows.\n"
	rendered, err := RenderMarkdown(raw, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	// TierNone must produce a text table, not image escape sequences
	result := processCharts(raw, rendered, dir, TierNone)

	if !strings.Contains(result, "Postings") {
		t.Error("expected text table with chart title")
	}
	if !strings.Contains(result, "NGA") {
		t.Error("expected text table with label NGA")
	}
	// Must not contain Kitty or iTerm2 image escape sequences
	if strings.Contains(result, "\x1b_G") {
		t.Error("TierNone must not produce Kitty graphics escape sequences")
	}
	if strings.Contains(result, "\x1b]1337") {
		t.Error("TierNone must not produce iTerm2 image escape sequences")
	}
	if strings.Contains(result, "BURROW-CHART-") {
		t.Error("expected all markers replaced")
	}
}
