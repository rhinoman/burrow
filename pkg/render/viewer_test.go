package render

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

func TestExtractHeadings(t *testing.T) {
	raw := "# Title\n\nSome text.\n\n## Section One\n\nMore text.\n\n## Section Two\n\nEnd.\n"
	rendered, err := RenderMarkdown(raw, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	headings := extractHeadings(raw, rendered)
	if len(headings) != 3 {
		t.Fatalf("expected 3 headings, got %d", len(headings))
	}

	if headings[0].text != "Title" {
		t.Errorf("expected 'Title', got %q", headings[0].text)
	}
	if headings[1].text != "Section One" {
		t.Errorf("expected 'Section One', got %q", headings[1].text)
	}
	if headings[2].text != "Section Two" {
		t.Errorf("expected 'Section Two', got %q", headings[2].text)
	}

	for i := 1; i < len(headings); i++ {
		if headings[i].line <= headings[i-1].line {
			t.Errorf("heading %d (line %d) should be after heading %d (line %d)",
				i, headings[i].line, i-1, headings[i-1].line)
		}
	}
}

func TestExtractHeadingsEmpty(t *testing.T) {
	raw := "No headings here.\n\nJust paragraphs.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	headings := extractHeadings(raw, rendered)
	if len(headings) != 0 {
		t.Errorf("expected 0 headings, got %d", len(headings))
	}
}

func TestExtractHeadingsMultipleLevels(t *testing.T) {
	raw := "# H1\n\n## H2\n\n### H3\n\n#### H4\n"
	rendered, _ := RenderMarkdown(raw, 80)
	headings := extractHeadings(raw, rendered)
	if len(headings) != 4 {
		t.Fatalf("expected 4 headings, got %d", len(headings))
	}
}

func TestViewerWithRawParsesActions(t *testing.T) {
	raw := "# Report\n\n[Draft] Write follow-up email\n\n[Open] Check dashboard (https://example.com)\n"
	rendered, _ := RenderMarkdown(raw, 80)

	v := newViewerWithRaw("Test", raw, rendered)
	if len(v.actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(v.actions))
	}
	if v.actions[0].Type != "draft" {
		t.Errorf("expected draft action, got %q", v.actions[0].Type)
	}
	if v.actions[1].Type != "open" {
		t.Errorf("expected open action, got %q", v.actions[1].Type)
	}
}

func TestViewerActionToggle(t *testing.T) {
	raw := "# Report\n\n[Draft] Write email\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Press 'a' to open actions
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	viewer := m.(Viewer)
	if !viewer.showActions {
		t.Error("expected action overlay to be visible")
	}

	// Press 'esc' to close
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	viewer = m.(Viewer)
	if viewer.showActions {
		t.Error("expected action overlay to be hidden")
	}
}

func TestViewerActionToggleNoActions(t *testing.T) {
	raw := "# Report\n\nNo actions here.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	viewer := m.(Viewer)
	if viewer.showActions {
		t.Error("expected overlay to not open when no actions")
	}
	if viewer.statusMsg == "" {
		t.Error("expected status message about no actions")
	}
}

func TestViewerActionNavigation(t *testing.T) {
	raw := "# Report\n\n[Draft] Write email\n[Open] Check site (https://example.com)\n[Configure] Update settings\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Open actions
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	viewer := m.(Viewer)
	if viewer.actionIdx != 0 {
		t.Errorf("expected actionIdx 0, got %d", viewer.actionIdx)
	}

	// Navigate down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	viewer = m.(Viewer)
	if viewer.actionIdx != 1 {
		t.Errorf("expected actionIdx 1, got %d", viewer.actionIdx)
	}

	// Navigate down again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	viewer = m.(Viewer)
	if viewer.actionIdx != 2 {
		t.Errorf("expected actionIdx 2, got %d", viewer.actionIdx)
	}

	// Navigate up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	viewer = m.(Viewer)
	if viewer.actionIdx != 1 {
		t.Errorf("expected actionIdx 1 after up, got %d", viewer.actionIdx)
	}
}

func TestViewerHeadingNavigation(t *testing.T) {
	raw := "# Title\n\nParagraph.\n\n## Section A\n\nText.\n\n## Section B\n\nMore.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	if len(v.headings) < 2 {
		t.Fatalf("expected at least 2 headings, got %d", len(v.headings))
	}

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Press 'n' to go to next heading — should not crash
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	_ = m.(Viewer) // assert correct type
}

func TestViewerViewOutput(t *testing.T) {
	raw := "# Test\n\nContent.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("My Report", raw, rendered)

	view := v.View()
	if view != "Loading..." {
		t.Errorf("expected loading message before ready, got %q", view)
	}
}

func TestViewerFooterWithOptions(t *testing.T) {
	raw := "# Report\n\n## Section\n\n[Draft] Write email\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	viewer := m.(Viewer)

	view := viewer.View()
	if view == "Loading..." {
		t.Fatal("expected rendered view")
	}
}

func TestViewerAsyncDraftReturnsCmd(t *testing.T) {
	raw := "# Report\n\n[Draft] Write email\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Press 'd' — without a provider, this should return a clipboard cmd (not block)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	viewer := m.(Viewer)

	// Without a provider, it copies the instruction to clipboard via async cmd
	if cmd == nil {
		t.Error("expected a tea.Cmd for clipboard operation")
	}
	// Viewer should not be stuck in busy state (no LLM = instant clipboard)
	if viewer.busy {
		t.Error("expected viewer not to be busy for clipboard-only draft")
	}
}

func TestViewerAsyncDraftWithProviderSetsBusy(t *testing.T) {
	raw := "# Report\n\n[Draft] Write email\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)
	// Set a mock provider to trigger the async LLM path
	v.provider = &mockProvider{}

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Press 'd' — with a provider, this should set busy and return a cmd
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	viewer := m.(Viewer)

	if cmd == nil {
		t.Error("expected a tea.Cmd for async draft generation")
	}
	if !viewer.busy {
		t.Error("expected viewer to be busy during LLM draft generation")
	}
}

func TestViewerBusyBlocksKeys(t *testing.T) {
	raw := "# Report\n\n[Draft] Write email\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)
	v.busy = true

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Keys other than q/ctrl+c should be blocked when busy
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	viewer := m.(Viewer)
	// Should not crash, should stay busy
	if !viewer.busy {
		t.Error("expected viewer to remain busy")
	}
}

func TestViewerDraftResultClearsBusy(t *testing.T) {
	raw := "# Report\n\n[Draft] Write email\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)
	v.busy = true

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Simulate draft result message
	m, _ = m.Update(draftResultMsg{raw: "Dear team...", err: nil})
	viewer := m.(Viewer)
	if viewer.busy {
		t.Error("expected busy cleared after draft result")
	}
}

func TestViewerActionResultMsg(t *testing.T) {
	raw := "# Report\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)
	v.busy = true

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = m.Update(actionResultMsg{status: "Opened: https://example.com"})
	viewer := m.(Viewer)
	if viewer.busy {
		t.Error("expected busy cleared after action result")
	}
	if viewer.statusMsg != "Opened: https://example.com" {
		t.Errorf("expected status message, got %q", viewer.statusMsg)
	}
}

func TestExtractHeadingsLevel(t *testing.T) {
	raw := "# H1\n\n## H2\n\n### H3\n\n#### H4\n"
	rendered, _ := RenderMarkdown(raw, 80)
	headings := extractHeadings(raw, rendered)

	if len(headings) != 4 {
		t.Fatalf("expected 4 headings, got %d", len(headings))
	}

	expected := []struct {
		text  string
		level int
	}{
		{"H1", 1},
		{"H2", 2},
		{"H3", 3},
		{"H4", 4},
	}

	for i, e := range expected {
		if headings[i].text != e.text {
			t.Errorf("heading %d: expected text %q, got %q", i, e.text, headings[i].text)
		}
		if headings[i].level != e.level {
			t.Errorf("heading %d: expected level %d, got %d", i, e.level, headings[i].level)
		}
	}
}

func TestComputeEndLines(t *testing.T) {
	raw := "# Title\n\nIntro.\n\n## Section A\n\nA text.\n\n### Subsection\n\nSub text.\n\n## Section B\n\nB text.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	headings := extractHeadings(raw, rendered)

	if len(headings) != 4 {
		t.Fatalf("expected 4 headings, got %d", len(headings))
	}

	// H1 Title: endLine should cover everything
	// ## Section A: endLine should be at ## Section B's line
	// ### Subsection: endLine should be at ## Section B's line (next heading at same or higher level)
	// ## Section B: endLine should be total line count

	// Section A's endLine should equal Section B's line
	if headings[1].endLine != headings[3].line {
		t.Errorf("Section A endLine: expected %d, got %d", headings[3].line, headings[1].endLine)
	}

	// Subsection's endLine should equal Section B's line
	if headings[2].endLine != headings[3].line {
		t.Errorf("Subsection endLine: expected %d, got %d", headings[3].line, headings[2].endLine)
	}
}

func TestToggleSection(t *testing.T) {
	raw := "# Title\n\nIntro.\n\n## Section A\n\nA text line 1.\nA text line 2.\n\n## Section B\n\nB text.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	viewer := m.(Viewer)

	// Find Section A (index 1, level 2)
	sectionAIdx := -1
	for i, h := range viewer.headings {
		if h.text == "Section A" {
			sectionAIdx = i
			break
		}
	}
	if sectionAIdx < 0 {
		t.Fatal("Section A not found in headings")
	}

	originalContent := viewer.content

	// Collapse Section A
	viewer.toggleSection(sectionAIdx)
	if !viewer.headings[sectionAIdx].collapsed {
		t.Error("expected Section A to be collapsed")
	}
	if viewer.content == originalContent {
		t.Error("expected content to change after collapse")
	}
	// Collapsed content should be shorter
	if len(viewer.content) >= len(originalContent) {
		t.Error("expected collapsed content to be shorter")
	}

	// Expand Section A
	viewer.toggleSection(sectionAIdx)
	if viewer.headings[sectionAIdx].collapsed {
		t.Error("expected Section A to be expanded")
	}
}

func TestCollapseExpandAll(t *testing.T) {
	raw := "# Title\n\n## Section A\n\nA text.\n\n## Section B\n\nB text.\n\n### Sub B\n\nSub text.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	viewer := m.(Viewer)

	expandedContent := viewer.content

	// Collapse all
	viewer.collapseAll()
	for _, h := range viewer.headings {
		if h.level > 1 && !h.collapsed {
			t.Errorf("expected heading %q (level %d) to be collapsed", h.text, h.level)
		}
	}
	if viewer.content == expandedContent {
		t.Error("expected content to change after collapse all")
	}

	// Expand all
	viewer.expandAll()
	for _, h := range viewer.headings {
		if h.collapsed {
			t.Errorf("expected heading %q to be expanded", h.text)
		}
	}
}

func TestH1NotCollapsible(t *testing.T) {
	raw := "# Title\n\nIntro.\n\n## Section\n\nText.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	// Find H1
	h1Idx := -1
	for i, h := range v.headings {
		if h.level == 1 {
			h1Idx = i
			break
		}
	}
	if h1Idx < 0 {
		t.Fatal("H1 not found")
	}

	originalContent := v.content
	v.toggleSection(h1Idx) // should be no-op
	if v.headings[h1Idx].collapsed {
		t.Error("H1 should not be collapsible")
	}
	if v.content != originalContent {
		t.Error("content should not change when toggling H1")
	}
}

func TestNestedCollapse(t *testing.T) {
	raw := "# Title\n\n## Outer\n\nOuter text.\n\n### Inner\n\nInner text.\n\n## Next\n\nNext text.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	viewer := m.(Viewer)

	// Find Outer section
	outerIdx := -1
	for i, h := range viewer.headings {
		if h.text == "Outer" {
			outerIdx = i
			break
		}
	}
	if outerIdx < 0 {
		t.Fatal("Outer heading not found")
	}

	// Collapsing Outer should hide both Outer's text AND Inner section
	viewer.toggleSection(outerIdx)
	if !viewer.headings[outerIdx].collapsed {
		t.Error("expected Outer to be collapsed")
	}
	// Inner's content should be hidden (it's within Outer's range)
	if strings.Contains(viewer.content, "Inner text.") {
		t.Error("expected Inner text to be hidden when Outer is collapsed")
	}
}

func TestCollapseAllKeybinding(t *testing.T) {
	raw := "# Title\n\n## Section A\n\nA text.\n\n## Section B\n\nB text.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})

	// Press 'c' to collapse all
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	viewer := m.(Viewer)

	for _, h := range viewer.headings {
		if h.level > 1 && !h.collapsed {
			t.Errorf("expected heading %q collapsed after 'c' key", h.text)
		}
	}

	// Press 'e' to expand all
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	viewer = m.(Viewer)

	for _, h := range viewer.headings {
		if h.collapsed {
			t.Errorf("expected heading %q expanded after 'e' key", h.text)
		}
	}
}

func TestIndicatorPrepend(t *testing.T) {
	// Plain text — Tier 2 (TierNone)
	result := prependIndicator("Market Intelligence", false, TierNone)
	if result != "▼ Market Intelligence" {
		t.Errorf("expected '▼ Market Intelligence', got %q", result)
	}

	result = prependIndicator("Market Intelligence", true, TierNone)
	if result != "▸ Market Intelligence" {
		t.Errorf("expected '▸ Market Intelligence', got %q", result)
	}

	// With ANSI prefix — Tier 2
	ansiLine := "\x1b[1m\x1b[35mHeading Text\x1b[0m"
	result = prependIndicator(ansiLine, false, TierNone)
	if !strings.HasPrefix(result, "\x1b[1m\x1b[35m▼ ") {
		t.Errorf("expected indicator after ANSI prefix, got %q", result)
	}
	if !strings.Contains(result, "Heading Text") {
		t.Error("expected original text preserved")
	}
}

func TestIndicatorPrependTier1(t *testing.T) {
	// On Tier 1, indicators should contain ANSI color codes (amber #E0AF68)
	result := prependIndicator("Section Title", false, TierKitty)
	if !strings.Contains(result, "\x1b[") {
		t.Error("expected ANSI styling in Tier 1 indicator")
	}
	if !strings.Contains(result, "Section Title") {
		t.Error("expected original text preserved")
	}

	result = prependIndicator("Section Title", true, TierKitty)
	if !strings.Contains(result, "\x1b[") {
		t.Error("expected ANSI styling in Tier 1 collapsed indicator")
	}
}

func TestInsertAfterANSIPrefix(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		insert string
		want   string
	}{
		{"plain", "hello", ">> ", ">> hello"},
		{"ansi_prefix", "\x1b[1mhello\x1b[0m", ">> ", "\x1b[1m>> hello\x1b[0m"},
		{"multi_ansi", "\x1b[1m\x1b[35mhello", ">> ", "\x1b[1m\x1b[35m>> hello"},
		{"no_text", "", ">> ", ">> "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := insertAfterANSIPrefix(tt.line, tt.insert)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScrollPositionAfterToggle(t *testing.T) {
	raw := "# Title\n\nIntro paragraph.\n\n## Section A\n\nA text line 1.\nA text line 2.\nA text line 3.\n\n## Section B\n\nB text.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	viewer := m.(Viewer)

	// Find Section A
	sectionAIdx := -1
	for i, h := range viewer.headings {
		if h.text == "Section A" {
			sectionAIdx = i
			break
		}
	}
	if sectionAIdx < 0 {
		t.Fatal("Section A not found")
	}

	// Toggle and check scroll position lands on the heading
	viewer.toggleSection(sectionAIdx)
	if viewer.viewport.YOffset != viewer.headings[sectionAIdx].viewLine {
		t.Errorf("expected viewport at heading viewLine %d, got %d",
			viewer.headings[sectionAIdx].viewLine, viewer.viewport.YOffset)
	}
}

func TestViewerPlayActionParsed(t *testing.T) {
	raw := "# Report\n\n[Play] Listen to call (/tmp/earnings.mp3)\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	if len(v.actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(v.actions))
	}
	if v.actions[0].Type != "play" {
		t.Errorf("expected play action, got %q", v.actions[0].Type)
	}
}

func TestViewerPlayFooterHint(t *testing.T) {
	raw := "# Report\n\n[Play] Audio (/tmp/audio.mp3)\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	viewer := m.(Viewer)

	view := viewer.View()
	if !strings.Contains(view, "p play") {
		t.Error("expected 'p play' hint in footer when play actions exist")
	}
}

func TestViewerPlayNoHandoff(t *testing.T) {
	raw := "# Report\n\n[Play] Audio (/tmp/audio.mp3)\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)
	// No handoff configured

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Press 'p' — without handoff, should set status message
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	viewer := m.(Viewer)
	if viewer.statusMsg == "" {
		t.Error("expected status message about no handoff")
	}
}

// --- Link browser tests ---

func TestExtractLinksBasic(t *testing.T) {
	raw := "# Report\n\nCheck https://example.com for details.\n\nAlso see https://other.org/path here.\n"
	links := extractLinks(raw)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0].url != "https://example.com" {
		t.Errorf("expected first URL https://example.com, got %q", links[0].url)
	}
	if links[1].url != "https://other.org/path" {
		t.Errorf("expected second URL https://other.org/path, got %q", links[1].url)
	}
}

func TestExtractLinksMarkdownSyntax(t *testing.T) {
	raw := "# Report\n\n[Dashboard](https://example.com/dash) is live.\n"
	links := extractLinks(raw)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].url != "https://example.com/dash" {
		t.Errorf("expected URL https://example.com/dash, got %q", links[0].url)
	}
	if links[0].label != "Dashboard" {
		t.Errorf("expected label 'Dashboard', got %q", links[0].label)
	}
}

func TestExtractLinksDedup(t *testing.T) {
	raw := "Link: https://example.com\nAgain: https://example.com\n"
	links := extractLinks(raw)
	if len(links) != 1 {
		t.Fatalf("expected 1 link after dedup, got %d", len(links))
	}
}

func TestExtractLinksEmpty(t *testing.T) {
	raw := "# Report\n\nNo links here.\n"
	links := extractLinks(raw)
	if len(links) != 0 {
		t.Errorf("expected 0 links, got %d", len(links))
	}
}

func TestViewerLinkToggle(t *testing.T) {
	raw := "# Report\n\nSee https://example.com for info.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Press 'l' to open links
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	viewer := m.(Viewer)
	if !viewer.showLinks {
		t.Error("expected link overlay to be visible")
	}

	// Press 'esc' to close
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	viewer = m.(Viewer)
	if viewer.showLinks {
		t.Error("expected link overlay to be hidden")
	}
}

func TestViewerLinkToggleNoLinks(t *testing.T) {
	raw := "# Report\n\nNo links here.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	viewer := m.(Viewer)
	if viewer.showLinks {
		t.Error("expected overlay to not open when no links")
	}
	if viewer.statusMsg == "" {
		t.Error("expected status message about no links")
	}
}

func TestViewerLinkNavigation(t *testing.T) {
	raw := "# Report\n\nSee https://one.com and https://two.com and https://three.com for details.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Open links
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	viewer := m.(Viewer)
	if viewer.linkIdx != 0 {
		t.Errorf("expected linkIdx 0, got %d", viewer.linkIdx)
	}

	// Navigate down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	viewer = m.(Viewer)
	if viewer.linkIdx != 1 {
		t.Errorf("expected linkIdx 1, got %d", viewer.linkIdx)
	}

	// Navigate down again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	viewer = m.(Viewer)
	if viewer.linkIdx != 2 {
		t.Errorf("expected linkIdx 2, got %d", viewer.linkIdx)
	}

	// Navigate past end — should stay at 2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	viewer = m.(Viewer)
	if viewer.linkIdx != 2 {
		t.Errorf("expected linkIdx 2 (clamped), got %d", viewer.linkIdx)
	}

	// Navigate up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	viewer = m.(Viewer)
	if viewer.linkIdx != 1 {
		t.Errorf("expected linkIdx 1 after up, got %d", viewer.linkIdx)
	}
}

func TestViewerLinkCloseWithL(t *testing.T) {
	raw := "# Report\n\nSee https://example.com here.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Open with 'l'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	viewer := m.(Viewer)
	if !viewer.showLinks {
		t.Fatal("expected link overlay open")
	}

	// Close with 'l'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	viewer = m.(Viewer)
	if viewer.showLinks {
		t.Error("expected link overlay closed with 'l'")
	}
}

func TestViewerLinkYankReturnsCmd(t *testing.T) {
	raw := "# Report\n\nSee https://example.com here.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Open links
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	// Press 'y' to yank
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Error("expected a tea.Cmd for clipboard copy")
	}
}

func TestViewerLinkFooterHint(t *testing.T) {
	raw := "# Report\n\nSee https://example.com here.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	viewer := m.(Viewer)

	view := viewer.View()
	if !strings.Contains(view, "l links") {
		t.Error("expected 'l links' hint in footer when links exist")
	}
}

func TestViewerLinkFooterHintAbsent(t *testing.T) {
	raw := "# Report\n\nNo links here.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	viewer := m.(Viewer)

	view := viewer.View()
	if strings.Contains(view, "l links") {
		t.Error("expected no 'l links' hint when no links exist")
	}
}

func TestExtractLinksContextTruncation(t *testing.T) {
	longLine := "See https://example.com in this very long line that should be truncated because it exceeds sixty characters total"
	raw := "# Report\n\n" + longLine + "\n"
	links := extractLinks(raw)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if len(links[0].label) > 60 {
		t.Errorf("expected label truncated to 60 chars, got %d", len(links[0].label))
	}
	if !strings.HasSuffix(links[0].label, "...") {
		t.Error("expected truncated label to end with '...'")
	}
}

// --- BubbleZone / clickable URL tests ---

func TestWrapURLsForViewWithZones(t *testing.T) {
	v := Viewer{imageTier: TierNone}
	zm := zone.New()
	defer zm.Close()
	v.zones = zm
	v.zoneState = &zoneState{urls: make(map[string]string)}

	input := "Check https://example.com and https://other.org here."
	result := v.wrapURLsForView(input)

	// Zone marks should be present (invisible ANSI markers)
	if result == input {
		t.Error("expected zone marks to modify output")
	}

	// zoneState.urls should have 2 entries
	if len(v.zoneState.urls) != 2 {
		t.Fatalf("expected 2 zone URLs, got %d", len(v.zoneState.urls))
	}
	if v.zoneState.urls["url-0"] != "https://example.com" {
		t.Errorf("expected url-0 = https://example.com, got %q", v.zoneState.urls["url-0"])
	}
	if v.zoneState.urls["url-1"] != "https://other.org" {
		t.Errorf("expected url-1 = https://other.org, got %q", v.zoneState.urls["url-1"])
	}
}

func TestWrapURLsForViewResolvesWrappedURL(t *testing.T) {
	// Simulate Glamour word-wrapping a long URL across two lines.
	// The regex matches only the first-line fragment; resolveFullURL
	// should recover the full URL from v.links.
	fullURL := "https://example.com/very/long/path/to/something/important"
	v := Viewer{
		imageTier: TierNone,
		links:     []linkEntry{{url: fullURL}},
	}
	zm := zone.New()
	defer zm.Close()
	v.zones = zm
	v.zoneState = &zoneState{urls: make(map[string]string)}

	// Viewport output has the URL truncated at a line break
	input := "See https://example.com/very/long/path/to/so\nmething/important for details."
	v.wrapURLsForView(input)

	// The zone should map to the full URL, not the fragment
	if len(v.zoneState.urls) < 1 {
		t.Fatal("expected at least 1 zone URL")
	}
	if v.zoneState.urls["url-0"] != fullURL {
		t.Errorf("expected full URL %q, got %q", fullURL, v.zoneState.urls["url-0"])
	}
}

func TestResolveFullURL(t *testing.T) {
	v := Viewer{
		links: []linkEntry{
			{url: "https://example.com/a/b/c"},
			{url: "https://other.org/path"},
		},
	}

	// Exact match
	if got := v.resolveFullURL("https://example.com/a/b/c"); got != "https://example.com/a/b/c" {
		t.Errorf("exact match: expected full URL, got %q", got)
	}

	// Prefix match (truncated by line wrap)
	if got := v.resolveFullURL("https://example.com/a/b"); got != "https://example.com/a/b/c" {
		t.Errorf("prefix match: expected full URL, got %q", got)
	}

	// No match — returns input as-is
	if got := v.resolveFullURL("https://unknown.com"); got != "https://unknown.com" {
		t.Errorf("no match: expected input back, got %q", got)
	}

	// Ambiguous prefix — should pick the longest match
	v2 := Viewer{
		links: []linkEntry{
			{url: "https://example.com/a"},
			{url: "https://example.com/abc"},
		},
	}
	if got := v2.resolveFullURL("https://example.com/a"); got != "https://example.com/abc" {
		t.Errorf("ambiguous prefix: expected longest URL %q, got %q", "https://example.com/abc", got)
	}
}

func TestWrapURLsForViewNoZones(t *testing.T) {
	v := Viewer{imageTier: TierNone}
	// zones is nil — should fall back to processHyperlinks (which is a no-op on TierNone)

	input := "Visit https://example.com today."
	result := v.wrapURLsForView(input)

	if result != input {
		t.Errorf("expected unchanged output on TierNone with nil zones, got %q", result)
	}
}

func TestWrapURLsForViewTier1(t *testing.T) {
	v := Viewer{imageTier: TierKitty}
	zm := zone.New()
	defer zm.Close()
	v.zones = zm
	v.zoneState = &zoneState{urls: make(map[string]string)}

	input := "Visit https://example.com today."
	result := v.wrapURLsForView(input)

	// Should contain OSC 8 hyperlink (SetHyperlink includes \x1b])
	if !strings.Contains(result, "\x1b]8;") {
		t.Error("expected OSC 8 hyperlink escape in Tier 1 output")
	}

	// Should also have zone marks (invisible markers change the string)
	if result == input {
		t.Error("expected zone marks to modify output")
	}
}

func TestViewerMouseClickNoHandoff(t *testing.T) {
	raw := "# Report\n\nSee https://example.com for details.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)
	// Initialize zones (simulating what RunViewer does)
	v.zones = zone.New()
	defer v.zones.Close()
	v.zoneState = &zoneState{urls: map[string]string{"url-0": "https://example.com"}}
	// No handoff configured

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Simulate left-click release — should set "No handoff configured" status
	// Note: without actual Scan() positioning, the zone won't be InBounds,
	// so we test the handler branching by checking it doesn't crash
	m, _ = m.Update(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      10, Y: 5,
	})
	_ = m.(Viewer) // should not panic
}

func TestViewerMouseClickOverlayBlocks(t *testing.T) {
	raw := "# Report\n\nSee https://example.com for details.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)
	v.zones = zone.New()
	defer v.zones.Close()
	v.zoneState = &zoneState{urls: map[string]string{"url-0": "https://example.com"}}
	v.showLinks = true // overlay active

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Click should be ignored when overlay is shown (passed to viewport)
	m, _ = m.Update(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      10, Y: 5,
	})
	viewer := m.(Viewer)
	// Status should NOT have "No handoff configured" since zone check was skipped
	if viewer.statusMsg == "No handoff configured" {
		t.Error("expected click to be ignored when overlay is active")
	}
}

func TestViewerMouseWheelPassthrough(t *testing.T) {
	raw := "# Report\n\nLine 1.\nLine 2.\nLine 3.\nLine 4.\nLine 5.\nLine 6.\nLine 7.\nLine 8.\nLine 9.\nLine 10.\n"
	rendered, _ := RenderMarkdown(raw, 80)
	v := newViewerWithRaw("Test", raw, rendered)
	v.zones = zone.New()
	defer v.zones.Close()
	v.zoneState = &zoneState{urls: make(map[string]string)}

	var m tea.Model = v
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 6})

	// Send a mouse wheel down event — should not panic and should reach viewport
	m, _ = m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
		X:      10, Y: 3,
	})
	_ = m.(Viewer) // should not panic
}

// mockProvider implements synthesis.Provider for testing.
type mockProvider struct{}

func (m *mockProvider) Complete(_ context.Context, _, _ string) (string, error) {
	return "mock draft response", nil
}
