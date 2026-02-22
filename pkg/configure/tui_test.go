package configure

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/pipeline"
)

// newTestModel creates a configModel in a testable state (ready = true).
// NOTE: session and sendMsg are nil — tests that trigger sendMessageCmd
// (i.e. tests that submit non-quit input) must set sendMsg explicitly.
func newTestModel(initMode bool) configModel {
	ta := textarea.New()
	ta.SetWidth(78)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.Focus()
	ta.KeyMap.InsertNewline.SetEnabled(false)

	_, cancel := context.WithCancel(context.Background())

	m := configModel{
		state:    stateInput,
		initMode: initMode,
		ready:    true,
		width:    80,
		height:   24,
		textarea: ta,
		viewport: viewport.New(80, 16),
		cancel:   cancel,
		result:   &tuiResult{},
	}
	return m
}

func TestInitialState(t *testing.T) {
	m := newTestModel(false)
	if m.state != stateInput {
		t.Errorf("initial state = %d, want stateInput", m.state)
	}
}

func TestQuitCommands(t *testing.T) {
	for _, cmd := range []string{"done", "quit", "exit", "Done", "QUIT"} {
		t.Run(cmd, func(t *testing.T) {
			m := newTestModel(false)
			m.textarea.SetValue(cmd)

			result, teaCmd := m.handleInputKey(tea.KeyMsg{Type: tea.KeyEnter})
			model := result.(configModel)

			// Should have called cancel and returned quit
			_ = model
			if teaCmd == nil {
				t.Fatal("expected quit command, got nil")
			}
		})
	}
}

func TestEmptyInputIgnored(t *testing.T) {
	m := newTestModel(false)
	m.textarea.SetValue("")

	result, cmd := m.handleInputKey(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(configModel)

	if model.state != stateInput {
		t.Errorf("state = %d after empty input, want stateInput", model.state)
	}
	if cmd != nil {
		t.Error("expected nil cmd for empty input")
	}
}

func TestBackslashContinuation(t *testing.T) {
	m := newTestModel(false)
	m.textarea.SetValue("hello\\")

	result, _ := m.handleInputKey(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(configModel)

	// Should stay in input state (not submitted)
	if model.state != stateInput {
		t.Errorf("state = %d after backslash, want stateInput", model.state)
	}

	// Textarea should have the backslash removed and a newline added
	val := model.textarea.Value()
	if !strings.Contains(val, "\n") {
		t.Errorf("textarea value %q should contain newline", val)
	}
	if strings.Contains(val, "\\") {
		t.Errorf("textarea value %q should not contain backslash", val)
	}
}

func TestConfirmKeyY(t *testing.T) {
	m := newTestModel(false)
	m.state = stateConfirming

	applied := false
	m.confirmQueue = []pendingConfirm{
		{
			prompt: "Apply? (y/n)",
			apply:  func() error { applied = true; return nil },
		},
	}

	result, _ := m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model := result.(configModel)

	if !applied {
		t.Error("expected apply to be called on 'y'")
	}
	if model.state != stateInput {
		t.Errorf("state = %d after confirm, want stateInput", model.state)
	}
}

func TestConfirmKeyN(t *testing.T) {
	m := newTestModel(false)
	m.state = stateConfirming

	applied := false
	m.confirmQueue = []pendingConfirm{
		{
			prompt: "Apply? (y/n)",
			apply:  func() error { applied = true; return nil },
		},
	}

	result, _ := m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model := result.(configModel)

	if applied {
		t.Error("expected apply to NOT be called on 'n'")
	}
	if model.state != stateInput {
		t.Errorf("state = %d after decline, want stateInput", model.state)
	}
}

func TestConfirmKeyOtherIgnored(t *testing.T) {
	m := newTestModel(false)
	m.state = stateConfirming
	m.confirmQueue = []pendingConfirm{
		{prompt: "Apply? (y/n)", apply: func() error { return nil }},
	}

	result, _ := m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model := result.(configModel)

	if model.state != stateConfirming {
		t.Errorf("state = %d after invalid key, want stateConfirming", model.state)
	}
	if len(model.confirmQueue) != 1 {
		t.Errorf("confirmQueue len = %d, want 1 (unchanged)", len(model.confirmQueue))
	}
}

func TestConfirmQueueMultiple(t *testing.T) {
	m := newTestModel(false)
	m.state = stateConfirming

	order := []string{}
	m.confirmQueue = []pendingConfirm{
		{prompt: "First? (y/n)", apply: func() error { order = append(order, "first"); return nil }},
		{prompt: "Second? (y/n)", apply: func() error { order = append(order, "second"); return nil }},
	}

	// Confirm first
	result, _ := m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model := result.(configModel)

	if model.state != stateConfirming {
		t.Errorf("state = %d after first confirm, want stateConfirming (more in queue)", model.state)
	}
	if len(model.confirmQueue) != 1 {
		t.Errorf("confirmQueue len = %d, want 1 after first confirm", len(model.confirmQueue))
	}

	// Confirm second
	result, _ = model.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = result.(configModel)

	if model.state != stateInput {
		t.Errorf("state = %d after all confirms, want stateInput", model.state)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("order = %v, want [first second]", order)
	}
}

func TestLLMResponseNoChanges(t *testing.T) {
	m := newTestModel(false)
	m.state = stateProcessing

	result, _ := m.handleLLMResponse(llmResponseMsg{
		response: "Here's some info for you.",
	})
	model := result.(configModel)

	if model.state != stateInput {
		t.Errorf("state = %d after no-change response, want stateInput", model.state)
	}
}

func TestLLMResponseWithError(t *testing.T) {
	m := newTestModel(false)
	m.state = stateProcessing

	result, _ := m.handleLLMResponse(llmResponseMsg{
		err: fmt.Errorf("connection refused"),
	})
	model := result.(configModel)

	if model.state != stateInput {
		t.Errorf("state = %d after error response, want stateInput", model.state)
	}
	// Should have an error message appended
	if len(model.messages) == 0 {
		t.Fatal("expected error message to be appended")
	}
	last := model.messages[len(model.messages)-1]
	if last.role != "system" {
		t.Errorf("last message role = %q, want system", last.role)
	}
}

func TestLLMResponseWithConfigChange(t *testing.T) {
	m := newTestModel(false)
	m.state = stateProcessing

	result, _ := m.handleLLMResponse(llmResponseMsg{
		response: "I'll update the config.",
		change: &Change{
			Description: "test change",
			Config:      &config.Config{},
		},
	})
	model := result.(configModel)

	if model.state != stateConfirming {
		t.Errorf("state = %d after change response, want stateConfirming", model.state)
	}
	if len(model.confirmQueue) != 1 {
		t.Errorf("confirmQueue len = %d, want 1", len(model.confirmQueue))
	}
}

func TestLLMResponseMultipleChanges(t *testing.T) {
	m := newTestModel(false)
	m.state = stateProcessing

	result, _ := m.handleLLMResponse(llmResponseMsg{
		response:      "I'll update everything.",
		change:        &Change{Description: "config", Config: &config.Config{}},
		profChange:    &ProfileChange{Description: "profile"},
		routineChange: &RoutineChange{Description: "routine", Routine: &pipeline.Routine{Name: "test"}, IsNew: true},
	})
	model := result.(configModel)

	if model.state != stateConfirming {
		t.Errorf("state = %d, want stateConfirming", model.state)
	}
	// Should have 3 items: profile, routine, config (in that order)
	if len(model.confirmQueue) != 3 {
		t.Errorf("confirmQueue len = %d, want 3", len(model.confirmQueue))
	}
}

func TestInitModeTracksAppliedConfig(t *testing.T) {
	m := newTestModel(true)
	m.state = stateConfirming
	m.result = &tuiResult{}

	expectedCfg := &config.Config{}
	sharedResult := m.result
	m.confirmQueue = []pendingConfirm{
		{
			prompt: "Apply? (y/n)",
			apply: func() error {
				sharedResult.appliedConfig = expectedCfg
				return nil
			},
		},
	}

	result, _ := m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model := result.(configModel)

	if model.result.appliedConfig != expectedCfg {
		t.Error("expected appliedConfig to be set in init mode")
	}
}

func TestAppendMessageRendering(t *testing.T) {
	m := newTestModel(false)

	m.appendMessage("user", "hello world")
	if len(m.rendered) != 1 {
		t.Fatalf("rendered len = %d, want 1", len(m.rendered))
	}
	if !strings.Contains(m.rendered[0], "You:") {
		t.Errorf("user message should contain 'You:', got %q", m.rendered[0])
	}

	m.appendMessage("system", "Applied.")
	if len(m.rendered) != 2 {
		t.Fatalf("rendered len = %d, want 2", len(m.rendered))
	}
}

func TestViewContainsHeader(t *testing.T) {
	m := newTestModel(false)
	// Create a minimal viewport so View() works
	m.viewport = viewport.New(80, 20)
	m.ready = true

	view := m.View()
	if !strings.Contains(view, "Burrow Configure") {
		t.Error("view should contain 'Burrow Configure' header")
	}
}

func TestViewInitMode(t *testing.T) {
	m := newTestModel(true)
	m.viewport = viewport.New(80, 20)
	m.ready = true

	view := m.View()
	if !strings.Contains(view, "Burrow Init") {
		t.Error("view should contain 'Burrow Init' header for init mode")
	}
}

func TestHelpBarInput(t *testing.T) {
	m := newTestModel(false)
	m.state = stateInput
	bar := m.renderHelpBar()
	if !strings.Contains(bar, "enter send") {
		t.Errorf("help bar should show 'enter send', got %q", bar)
	}
}

func TestHelpBarConfirming(t *testing.T) {
	m := newTestModel(false)
	m.state = stateConfirming
	bar := m.renderHelpBar()
	if !strings.Contains(bar, "y apply") {
		t.Errorf("help bar should show 'y apply', got %q", bar)
	}
}
