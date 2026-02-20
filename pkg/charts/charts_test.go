package charts

import (
	"strings"
	"testing"
)

func TestParseDirectivesBar(t *testing.T) {
	md := "# Report\n\nSome text.\n\n```chart\ntype: bar\ntitle: \"Postings by Agency\"\nx: [\"NGA\", \"NRO\", \"DIA\"]\ny: [12, 4, 2]\n```\n\nMore text.\n"

	directives := ParseDirectives(md)
	if len(directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(directives))
	}

	d := directives[0]
	if d.Type != "bar" {
		t.Errorf("expected type bar, got %q", d.Type)
	}
	if d.Title != "Postings by Agency" {
		t.Errorf("expected title 'Postings by Agency', got %q", d.Title)
	}
	if len(d.Labels) != 3 || d.Labels[0] != "NGA" {
		t.Errorf("unexpected labels: %v", d.Labels)
	}
	if len(d.Values) != 3 || d.Values[0] != 12 {
		t.Errorf("unexpected values: %v", d.Values)
	}
}

func TestParseDirectivesMultiple(t *testing.T) {
	md := "```chart\ntype: bar\ntitle: \"Bar\"\nx: [\"A\"]\ny: [1]\n```\n\ntext\n\n```chart\ntype: line\ntitle: \"Line\"\nx: [\"B\"]\ny: [2]\n```\n"

	directives := ParseDirectives(md)
	if len(directives) != 2 {
		t.Fatalf("expected 2 directives, got %d", len(directives))
	}
	if directives[0].Type != "bar" {
		t.Errorf("expected first type bar, got %q", directives[0].Type)
	}
	if directives[1].Type != "line" {
		t.Errorf("expected second type line, got %q", directives[1].Type)
	}
}

func TestParseDirectivesPie(t *testing.T) {
	md := "```chart\ntype: pie\ntitle: \"Share\"\nlabels: [\"A\", \"B\", \"C\"]\nvalues: [30, 50, 20]\n```\n"

	directives := ParseDirectives(md)
	if len(directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(directives))
	}
	d := directives[0]
	if d.Type != "pie" {
		t.Errorf("expected type pie, got %q", d.Type)
	}
	if len(d.Labels) != 3 {
		t.Errorf("expected 3 labels, got %d", len(d.Labels))
	}
}

func TestParseDirectivesEmpty(t *testing.T) {
	md := "# No charts here\n\nJust text.\n"
	directives := ParseDirectives(md)
	if len(directives) != 0 {
		t.Errorf("expected 0 directives, got %d", len(directives))
	}
}

func TestParseDirectivesMalformed(t *testing.T) {
	// Missing closing ```
	md := "```chart\ntype: bar\ntitle: \"Broken\"\nx: [\"A\"]\ny: [1]\n"
	directives := ParseDirectives(md)
	if len(directives) != 0 {
		t.Errorf("expected 0 directives for unclosed block, got %d", len(directives))
	}
}

func TestParseDirectivesUnknownType(t *testing.T) {
	md := "```chart\ntype: radar\ntitle: \"Unknown\"\nx: [\"A\"]\ny: [1]\n```\n"
	directives := ParseDirectives(md)
	if len(directives) != 0 {
		t.Errorf("expected 0 directives for unknown type, got %d", len(directives))
	}
}

func TestParseDirectivesMissingValues(t *testing.T) {
	md := "```chart\ntype: bar\ntitle: \"No data\"\nx: [\"A\"]\n```\n"
	directives := ParseDirectives(md)
	if len(directives) != 0 {
		t.Errorf("expected 0 directives for missing values, got %d", len(directives))
	}
}

func TestParseDirectivesEmptyBlock(t *testing.T) {
	md := "```chart\n```\n"
	directives := ParseDirectives(md)
	if len(directives) != 0 {
		t.Errorf("expected 0 directives for empty block, got %d", len(directives))
	}
}

func TestParseDirectivesExtraWhitespace(t *testing.T) {
	md := "```chart\n  type:  bar  \n  title:  \"Spacy\"  \n  x:  [\"A\", \"B\"]  \n  y:  [1, 2]  \n```\n"
	directives := ParseDirectives(md)
	if len(directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(directives))
	}
	if directives[0].Title != "Spacy" {
		t.Errorf("expected title 'Spacy', got %q", directives[0].Title)
	}
}

func TestParseDirectivesDefaultTitle(t *testing.T) {
	md := "```chart\ntype: bar\nx: [\"A\"]\ny: [1]\n```\n"
	directives := ParseDirectives(md)
	if len(directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(directives))
	}
	if directives[0].Title != "Chart" {
		t.Errorf("expected default title 'Chart', got %q", directives[0].Title)
	}
}

func TestReplaceDirectives(t *testing.T) {
	md := "before\n\n```chart\ntype: bar\nx: [\"A\"]\ny: [1]\n```\n\nmiddle\n\n```chart\ntype: line\nx: [\"B\"]\ny: [2]\n```\n\nafter"

	replacements := map[int]string{
		0: "[CHART 0]",
		1: "[CHART 1]",
	}

	result := ReplaceDirectives(md, replacements)
	if !strings.Contains(result, "[CHART 0]") {
		t.Error("expected [CHART 0] in result")
	}
	if !strings.Contains(result, "[CHART 1]") {
		t.Error("expected [CHART 1] in result")
	}
	if strings.Contains(result, "```chart") {
		t.Error("expected chart blocks to be replaced")
	}
	if !strings.Contains(result, "before") || !strings.Contains(result, "after") {
		t.Error("expected surrounding text preserved")
	}
}

func TestReplaceDirectivesPartial(t *testing.T) {
	md := "```chart\ntype: bar\nx: [\"A\"]\ny: [1]\n```\n\n```chart\ntype: line\nx: [\"B\"]\ny: [2]\n```\n"

	// Only replace first chart
	replacements := map[int]string{
		0: "[REPLACED]",
	}

	result := ReplaceDirectives(md, replacements)
	if !strings.Contains(result, "[REPLACED]") {
		t.Error("expected replacement")
	}
	// Second chart block should be preserved
	if !strings.Contains(result, "type: line") {
		t.Error("expected second chart block preserved")
	}
}

func TestRenderPNGBar(t *testing.T) {
	d := ChartDirective{
		Type:   "bar",
		Title:  "Test Bar",
		Labels: []string{"A", "B", "C"},
		Values: []float64{10, 20, 30},
	}

	png, err := RenderPNG(d, 800, 400)
	if err != nil {
		t.Fatalf("RenderPNG bar: %v", err)
	}
	if len(png) == 0 {
		t.Fatal("expected non-empty PNG data")
	}
	// Verify PNG header
	if len(png) < 8 || string(png[1:4]) != "PNG" {
		t.Error("expected valid PNG header")
	}
}

func TestRenderPNGLine(t *testing.T) {
	d := ChartDirective{
		Type:   "line",
		Title:  "Test Line",
		Labels: []string{"Jan", "Feb", "Mar"},
		Values: []float64{5, 15, 10},
	}

	png, err := RenderPNG(d, 800, 400)
	if err != nil {
		t.Fatalf("RenderPNG line: %v", err)
	}
	if len(png) == 0 {
		t.Fatal("expected non-empty PNG data")
	}
	if len(png) < 8 || string(png[1:4]) != "PNG" {
		t.Error("expected valid PNG header")
	}
}

func TestRenderPNGPie(t *testing.T) {
	d := ChartDirective{
		Type:   "pie",
		Title:  "Test Pie",
		Labels: []string{"X", "Y", "Z"},
		Values: []float64{40, 35, 25},
	}

	png, err := RenderPNG(d, 600, 400)
	if err != nil {
		t.Fatalf("RenderPNG pie: %v", err)
	}
	if len(png) == 0 {
		t.Fatal("expected non-empty PNG data")
	}
	if len(png) < 8 || string(png[1:4]) != "PNG" {
		t.Error("expected valid PNG header")
	}
}

func TestRenderPNGUnsupportedType(t *testing.T) {
	d := ChartDirective{
		Type:   "radar",
		Title:  "Unsupported",
		Labels: []string{"A"},
		Values: []float64{1},
	}

	_, err := RenderPNG(d, 800, 400)
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestRenderTextTableBar(t *testing.T) {
	d := ChartDirective{
		Type:   "bar",
		Title:  "Postings by Agency",
		Labels: []string{"NGA", "NRO", "DIA", "CISA"},
		Values: []float64{12, 4, 2, 1},
	}

	table := RenderTextTable(d)
	if table == "" {
		t.Fatal("expected non-empty table")
	}
	if !strings.Contains(table, "Postings by Agency") {
		t.Error("expected title in table")
	}
	if !strings.Contains(table, "NGA") {
		t.Error("expected label NGA in table")
	}
	if !strings.Contains(table, "12") {
		t.Error("expected value 12 in table")
	}
}

func TestRenderTextTableEmpty(t *testing.T) {
	d := ChartDirective{
		Type:  "bar",
		Title: "Empty",
	}
	table := RenderTextTable(d)
	if table != "" {
		t.Error("expected empty string for no data")
	}
}

func TestRenderTextTableFloat(t *testing.T) {
	d := ChartDirective{
		Type:   "bar",
		Title:  "Floats",
		Labels: []string{"A", "B"},
		Values: []float64{1.5, 2.0},
	}

	table := RenderTextTable(d)
	if !strings.Contains(table, "1.5") {
		t.Error("expected 1.5 in table")
	}
	if !strings.Contains(table, "2") {
		t.Error("expected 2 in table")
	}
}
