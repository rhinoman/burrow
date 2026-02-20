package render

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

// mockProvider implements synthesis.Provider for testing.
type mockProvider struct{}

func (m *mockProvider) Complete(_ context.Context, _, _ string) (string, error) {
	return "mock draft response", nil
}
