package configure

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/render"
)

// --- States ---

type tuiState int

const (
	stateInput      tuiState = iota
	stateProcessing
	stateConfirming
)

// --- Message types ---

// llmResponseMsg carries the result of an async session.ProcessMessage call.
type llmResponseMsg struct {
	response      string
	change        *Change
	profChange    *ProfileChange
	routineChange *RoutineChange
	warnings      []string
	err           error
}

// chatMsg represents a single message in the conversation history.
type chatMsg struct {
	role    string // "user", "assistant", "system"
	content string
}

// pendingConfirm represents a change awaiting y/n confirmation.
type pendingConfirm struct {
	prompt  string
	apply   func() error
	warning string // optional post-apply warning (e.g. remote LLM)
}

// processingTickMsg drives the spinner animation during LLM calls.
// Uses tea.Tick instead of spinner's internal tick chain for robustness.
type processingTickMsg time.Time

func processingTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return processingTickMsg(t)
	})
}

// --- Styles (TokyoNight palette) ---

var (
	userLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7DCFFF"))
	confirmStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E0AF68"))
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E"))
	helpBarStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#565F89"))
	tuiHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5FD7")).PaddingLeft(1)
	systemMsgStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ECE6A"))
)

// --- Model ---

// tuiResult holds state that must survive Bubble Tea's value-receiver copies.
type tuiResult struct {
	appliedConfig *config.Config
}

type configModel struct {
	session *Session
	cancel  context.CancelFunc

	// sendMsg builds a tea.Cmd that calls session.ProcessMessage with the
	// original context. Capturing ctx in this closure (rather than storing it
	// on the struct) avoids a stale-context footgun: Bubble Tea copies the
	// model by value, so a stored ctx would never reflect later changes.
	sendMsg func(input string) tea.Cmd

	// UI components
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	ready    bool // set after first WindowSizeMsg

	// Conversation
	messages []chatMsg
	rendered []string // per-message glamour/styled cache

	// State machine
	state        tuiState
	confirmQueue []pendingConfirm
	err          error

	// Result (for init mode) — pointer survives Bubble Tea value-receiver copies.
	result   *tuiResult
	initMode bool

	width, height int
}

func newConfigModel(ctx context.Context, session *Session, initMode bool) configModel {
	ctx, cancel := context.WithCancel(ctx)

	ta := textarea.New()
	ta.Placeholder = "type here..."
	ta.CharLimit = 4096
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.Focus()
	// Allow Enter to be handled by our Update, not the textarea
	ta.KeyMap.InsertNewline.SetEnabled(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0AF68"))

	title := "Burrow Configure"
	if initMode {
		title = "Burrow Init"
	}

	m := configModel{
		session:  session,
		cancel:   cancel,
		sendMsg:  func(input string) tea.Cmd { return sendMessageCmd(ctx, session, input) },
		textarea: ta,
		spinner:  sp,
		initMode: initMode,
		result:   &tuiResult{},
	}

	// Add welcome message
	welcome := fmt.Sprintf("Welcome to %s. Describe what you'd like to configure, or type \"done\" to finish.", title)
	m.appendMessage("system", welcome)

	return m
}

// --- Bubble Tea interface ---

func (m configModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m configModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 2
		inputHeight := 5 // textarea (3) + help bar (1) + border (1)
		vpHeight := m.height - headerHeight - inputHeight
		if vpHeight < 1 {
			vpHeight = 1
		}

		if !m.ready {
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.YPosition = headerHeight
			m.rebuildViewport()
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
			m.rebuildViewport()
		}
		m.textarea.SetWidth(m.width - 2)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case llmResponseMsg:
		return m.handleLLMResponse(msg)

	case processingTickMsg:
		if m.state == stateProcessing {
			m.spinner, _ = m.spinner.Update(spinner.TickMsg{})
			return m, processingTick()
		}
		return m, nil

	default:
		// Forward only internal messages (e.g. cursor blink) to the textarea.
		if m.state == stateInput {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		}
		return m, nil
	}
}

func (m configModel) View() string {
	if !m.ready {
		return "Loading..."
	}

	title := "Burrow Configure"
	if m.initMode {
		title = "Burrow Init"
	}
	header := tuiHeaderStyle.Render(title)

	vpView := m.viewport.View()

	var inputArea string
	switch m.state {
	case stateProcessing:
		inputArea = fmt.Sprintf("  %s Thinking...", m.spinner.View())
	case stateConfirming:
		inputArea = confirmStyle.Render("  > (y/n) ")
	default:
		inputArea = m.textarea.View()
	}

	helpBar := m.renderHelpBar()

	return strings.Join([]string{header, "", vpView, "", inputArea, helpBar}, "\n")
}

// --- Key handling ---

func (m configModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global quit
	if key == "ctrl+c" {
		m.cancel()
		return m, tea.Quit
	}

	switch m.state {
	case stateInput:
		return m.handleInputKey(msg)
	case stateConfirming:
		return m.handleConfirmKey(msg)
	case stateProcessing:
		// Allow scrolling while waiting for LLM response.
		if key == "pgup" || key == "pgdown" || key == "ctrl+u" || key == "ctrl+d" {
			return m.scrollViewport(msg)
		}
		return m, nil
	}

	return m, nil
}

func (m configModel) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		input := strings.TrimSpace(m.textarea.Value())
		if input == "" {
			return m, nil
		}

		// Check for backslash continuation: if text ends with \, strip it and insert newline
		if strings.HasSuffix(input, "\\") {
			// Remove trailing backslash and add a newline in the textarea
			m.textarea.SetValue(strings.TrimSuffix(m.textarea.Value(), "\\") + "\n")
			// Move cursor to end
			m.textarea.CursorEnd()
			return m, nil
		}

		// Check for quit commands
		lower := strings.ToLower(input)
		if lower == "done" || lower == "quit" || lower == "exit" {
			m.cancel()
			return m, tea.Quit
		}

		// Submit message
		m.textarea.Reset()
		m.textarea.Blur()
		m.appendMessage("user", input)
		m.rebuildViewport()
		m.state = stateProcessing

		return m, tea.Batch(
			m.sendMsg(input),
			processingTick(),
		)

	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		return m.scrollViewport(msg)
	}

	// Filter escape sequence fragments that leak through Bubble Tea's input
	// parser. Two patterns:
	//
	// 1. Alt+rune: the \x1b byte is consumed as Alt=true, remaining bytes
	//    become runes (e.g. OSC fragments like Alt+]).
	// 2. CSI parameter residue: \x1b[ is consumed as a CSI prefix but the
	//    parameter bytes + final byte leak as plain KeyRunes (e.g. "50;1R"
	//    from a cursor position report \x1b[50;1R).
	if msg.Type == tea.KeyRunes && msg.Alt {
		if len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'f', 'b', 'd', 'l', 'u', 'c':
				// Known textarea bindings (word nav, case change) — allow
			default:
				return m, nil
			}
		} else {
			return m, nil
		}
	}
	if msg.Type == tea.KeyRunes && !msg.Alt && len(msg.Runes) > 1 && looksLikeEscapeFragment(msg.Runes) {
		return m, nil
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// scrollViewport forwards a key event to the viewport for scrolling.
func (m configModel) scrollViewport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.ready {
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// looksLikeEscapeFragment returns true if a multi-rune KeyRunes message looks
// like terminal escape sequence residue rather than real user input.
//
// When Bubble Tea's parser consumes the leading \x1b (or \x1b[ / \x1b]) of a
// terminal response, the payload bytes leak through as KeyRunes. Examples:
//
//   - CSI cursor position report  \x1b[50;1R       → "50;1R"
//   - OSC color response          \x1b]11;rgb:…\x07 → "11;rgb:2e2e/3434/4040"
//   - Device attributes           \x1b[?64;1c       → "?64;1c"
//
// These payloads never contain spaces and always contain semicolons, colons,
// or other separator characters that don't appear in normal single-burst
// typing. Normal user input arrives one rune at a time (or as pasted text
// that virtually always contains spaces or common prose punctuation).
func looksLikeEscapeFragment(runes []rune) bool {
	if len(runes) < 2 {
		return false
	}
	hasSeparator := false
	for _, r := range runes {
		switch {
		case r == ' ', r == '\t':
			// Spaces → almost certainly real text (paste), not an escape payload.
			return false
		case r == ';', r == ':':
			hasSeparator = true
		}
	}
	return hasSeparator
}

func (m configModel) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Allow scrolling while confirming.
	switch key {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		return m.scrollViewport(msg)
	}

	if key != "y" && key != "n" {
		return m, nil
	}

	if len(m.confirmQueue) == 0 {
		m.state = stateInput
		cmd := m.textarea.Focus()
		return m, cmd
	}

	confirm := m.confirmQueue[0]
	m.confirmQueue = m.confirmQueue[1:]

	if key == "y" {
		if err := confirm.apply(); err != nil {
			m.appendMessage("system", errorStyle.Render("Error: "+err.Error()))
		} else {
			m.appendMessage("system", "Applied.")
			if confirm.warning != "" {
				m.appendMessage("system", confirmStyle.Render(confirm.warning))
			}
		}
	} else {
		m.appendMessage("system", "Discarded.")
	}

	// Check for more confirmations
	if len(m.confirmQueue) > 0 {
		m.appendMessage("system", m.confirmQueue[0].prompt)
		m.rebuildViewport()
		return m, nil
	}

	m.state = stateInput
	cmd := m.textarea.Focus()
	m.rebuildViewport()
	return m, cmd
}

// --- LLM response handling ---

func (m configModel) handleLLMResponse(msg llmResponseMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.appendMessage("system", errorStyle.Render("Error: "+msg.err.Error()))
		m.state = stateInput
		cmd := m.textarea.Focus()
		m.rebuildViewport()
		return m, cmd
	}

	m.appendMessage("assistant", msg.response)

	for _, w := range msg.warnings {
		m.appendMessage("system", errorStyle.Render("Warning: "+w))
	}

	// Build confirmation queue
	m.confirmQueue = nil

	if msg.profChange != nil {
		pc := msg.profChange
		m.confirmQueue = append(m.confirmQueue, pendingConfirm{
			prompt: "Apply profile change? (y/n)",
			apply: func() error {
				return m.session.ApplyProfileChange(pc)
			},
		})
	}

	if msg.routineChange != nil {
		rc := msg.routineChange
		action := "Create"
		if !rc.IsNew {
			action = "Update"
		}
		m.confirmQueue = append(m.confirmQueue, pendingConfirm{
			prompt: fmt.Sprintf("%s routine %q? (y/n)", action, rc.Routine.Name),
			apply: func() error {
				return m.session.ApplyRoutineChange(rc)
			},
		})
	}

	if msg.change != nil {
		ch := msg.change
		sess := m.session
		result := m.result
		initMode := m.initMode
		m.confirmQueue = append(m.confirmQueue, pendingConfirm{
			prompt: "Apply this configuration change? (y/n)",
			apply: func() error {
				if err := sess.ApplyChange(ch); err != nil {
					return err
				}
				if initMode {
					result.appliedConfig = ch.Config
				}
				return nil
			},
			warning: func() string {
				if ch.RemoteLLMWarning {
					return "Warning: This configuration includes a remote LLM provider. " +
						"Collected results will leave your machine during synthesis. " +
						"For maximum privacy, use a local LLM provider."
				}
				return ""
			}(),
		})
	}

	var cmd tea.Cmd
	if len(m.confirmQueue) > 0 {
		m.state = stateConfirming
		m.appendMessage("system", m.confirmQueue[0].prompt)
	} else {
		m.state = stateInput
		cmd = m.textarea.Focus()
	}

	m.rebuildViewport()
	return m, cmd
}

// --- Async command ---

func sendMessageCmd(ctx context.Context, session *Session, input string) tea.Cmd {
	return func() tea.Msg {
		response, change, profChange, routineChange, warnings, err := session.ProcessMessage(ctx, input)
		return llmResponseMsg{response, change, profChange, routineChange, warnings, err}
	}
}

// --- Message rendering ---

func (m *configModel) appendMessage(role, content string) {
	m.messages = append(m.messages, chatMsg{role: role, content: content})

	// Render the message
	var rendered string
	switch role {
	case "user":
		rendered = userLabelStyle.Render("You:") + " " + content
	case "assistant":
		// Render markdown via glamour
		md, err := render.RenderMarkdown(content, m.renderWidth())
		if err != nil {
			md = content
		}
		rendered = md
	case "system":
		rendered = systemMsgStyle.Render("  " + content)
	}

	m.rendered = append(m.rendered, rendered)
}

func (m *configModel) renderWidth() int {
	if m.width > 4 {
		return m.width - 4
	}
	return 76
}

func (m *configModel) rebuildViewport() {
	if !m.ready {
		return
	}
	content := strings.Join(m.rendered, "\n\n")
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

// --- Help bar ---

func (m configModel) renderHelpBar() string {
	if m.state == stateConfirming {
		return helpBarStyle.Render("  y apply  n discard  ctrl+c quit")
	}
	return helpBarStyle.Render("  enter send  \\ newline  pgup/pgdn scroll  ctrl+c quit")
}

// --- Public API ---

// isTerminal reports whether fd is a terminal.
func isTerminal(fd int) bool {
	return term.IsTerminal(fd)
}

// RunTUI starts the configure TUI. Falls back to plain REPL if not a TTY.
func RunTUI(ctx context.Context, session *Session) error {
	if !isTerminal(int(os.Stdin.Fd())) {
		return runPlainREPL(ctx, session, false)
	}

	m := newConfigModel(ctx, session, false)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// RunInitTUI starts the init TUI. Returns the applied config (or nil).
// Falls back to plain REPL if not a TTY.
func RunInitTUI(ctx context.Context, session *Session) (*config.Config, error) {
	if !isTerminal(int(os.Stdin.Fd())) {
		return runPlainREPLInit(ctx, session)
	}

	m := newConfigModel(ctx, session, true)
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, err
	}

	if fm, ok := result.(configModel); ok && fm.result != nil {
		return fm.result.appliedConfig, nil
	}
	return nil, nil
}

// --- Plain REPL fallback (non-TTY) ---

func runPlainREPL(ctx context.Context, session *Session, initMode bool) error {
	_, err := runPlainREPLInner(ctx, session, initMode)
	return err
}

func runPlainREPLInit(ctx context.Context, session *Session) (*config.Config, error) {
	return runPlainREPLInner(ctx, session, true)
}

func runPlainREPLInner(ctx context.Context, session *Session, initMode bool) (*config.Config, error) {
	reader := bufio.NewReader(os.Stdin)
	var appliedConfig *config.Config

	fmt.Println("  Describe what you want to change, or 'done' to finish.")
	fmt.Println()

	for {
		fmt.Print("  > ")
		input, err := readPlainLine(reader)
		if err != nil {
			break
		}
		lower := strings.ToLower(strings.TrimSpace(input))
		if lower == "done" || lower == "quit" || lower == "exit" {
			break
		}
		if input == "" {
			continue
		}

		response, change, profChange, routineChange, warnings, err := session.ProcessMessage(ctx, input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			continue
		}

		fmt.Println("\n  " + response + "\n")

		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  Warning: %s\n", w)
		}

		if profChange != nil {
			fmt.Println("  Apply profile change? (y/n)")
			fmt.Print("  > ")
			if readPlainConfirm(reader) == "y" {
				if err := session.ApplyProfileChange(profChange); err != nil {
					fmt.Fprintf(os.Stderr, "  Error applying profile: %v\n", err)
				} else {
					fmt.Println("  Profile updated.")
				}
			} else {
				fmt.Println("  Profile change discarded.")
			}
		}

		if routineChange != nil {
			action := "Create"
			if !routineChange.IsNew {
				action = "Update"
			}
			fmt.Printf("  %s routine %q? (y/n)\n", action, routineChange.Routine.Name)
			fmt.Print("  > ")
			if readPlainConfirm(reader) == "y" {
				if err := session.ApplyRoutineChange(routineChange); err != nil {
					fmt.Fprintf(os.Stderr, "  Error applying routine: %v\n", err)
				} else {
					fmt.Printf("  Routine %q saved.\n", routineChange.Routine.Name)
				}
			} else {
				fmt.Println("  Routine change discarded.")
			}
		}

		if change != nil {
			fmt.Println("  Apply this configuration change? (y/n)")
			fmt.Print("  > ")
			if readPlainConfirm(reader) == "y" {
				if err := session.ApplyChange(change); err != nil {
					fmt.Fprintf(os.Stderr, "  Error applying: %v\n", err)
				} else {
					if change.RemoteLLMWarning {
						fmt.Println()
						fmt.Println("  Warning: This configuration includes a remote LLM provider.")
						fmt.Println("    Collected results will leave your machine during synthesis.")
						fmt.Println("    For maximum privacy, use a local LLM provider.")
						fmt.Println()
					}
					fmt.Println("  Configuration updated.")
					if initMode {
						appliedConfig = change.Config
					}
				}
			} else {
				fmt.Println("  Change discarded.")
			}
		}
	}

	if initMode && appliedConfig != nil {
		return appliedConfig, nil
	}
	return nil, nil
}

// readPlainLine reads a single line from the reader, stripping trailing newlines.
func readPlainLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF && line == "" {
		return "", io.EOF
	}

	// Accumulate additional buffered lines (paste detection).
	var lines []string
	if trimmed := strings.TrimRight(line, "\n\r"); trimmed != "" {
		lines = append(lines, trimmed)
	}
	for reader.Buffered() > 0 {
		extra, err := reader.ReadString('\n')
		if trimmed := strings.TrimRight(extra, "\n\r"); trimmed != "" {
			lines = append(lines, trimmed)
		}
		if err != nil {
			break
		}
	}

	return strings.Join(lines, "\n"), nil
}

// readPlainConfirm reads a single line for y/n confirmation.
func readPlainConfirm(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}
