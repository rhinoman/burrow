package render

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jcadam/burrow/pkg/actions"
	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/jcadam/burrow/pkg/synthesis"
)

var headerStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("205")).
	PaddingLeft(1)

var footerStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("240")).
	PaddingLeft(1)

var actionSelectedStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("205"))

var actionNormalStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("252"))

// --- Async message types ---

// actionResultMsg carries the result of an async action execution.
type actionResultMsg struct {
	status string
	err    error
}

// draftResultMsg carries the result of async draft generation.
type draftResultMsg struct {
	raw string
	err error
}

// headingPos tracks a heading's location in the rendered content.
type headingPos struct {
	text string
	line int // line number in rendered content
}

// Viewer is a Bubble Tea model for scrollable report viewing with section
// navigation and action execution.
type Viewer struct {
	title   string
	raw     string // original markdown
	content string // rendered content
	viewport viewport.Model
	ready    bool

	// Section navigation
	headings    []headingPos
	currentHead int

	// Actions
	actions     []actions.Action
	showActions bool
	actionIdx   int
	busy        bool // true while an async action is in flight

	// Optional deps for action execution
	handoff  *actions.Handoff
	provider synthesis.Provider
	ledger   *bcontext.Ledger
	ctx      context.Context

	// Chart rendering
	reportDir   string    // report directory for locating chart PNGs
	imageConfig string    // rendering.images config value
	imageTier   ImageTier // detected terminal image capability
	hasCharts   bool      // whether content contains charts

	statusMsg string
	statusExp time.Time
}

// ViewerOption configures optional Viewer behavior.
type ViewerOption func(*Viewer)

// WithHandoff provides a Handoff for executing open/mail actions.
func WithHandoff(h *actions.Handoff) ViewerOption {
	return func(v *Viewer) { v.handoff = h }
}

// WithProvider provides an LLM provider for draft generation.
func WithProvider(p synthesis.Provider) ViewerOption {
	return func(v *Viewer) { v.provider = p }
}

// WithLedger provides a context ledger for gathering draft context.
func WithLedger(l *bcontext.Ledger) ViewerOption {
	return func(v *Viewer) { v.ledger = l }
}

// WithContext provides a cancellable context for async operations.
func WithContext(ctx context.Context) ViewerOption {
	return func(v *Viewer) { v.ctx = ctx }
}

// WithReportDir provides the report directory for locating chart PNGs.
func WithReportDir(dir string) ViewerOption {
	return func(v *Viewer) { v.reportDir = dir }
}

// WithImageConfig provides the rendering.images config value.
func WithImageConfig(images string) ViewerOption {
	return func(v *Viewer) { v.imageConfig = images }
}

// NewViewer creates a viewer with pre-rendered content.
func NewViewer(title string, content string) Viewer {
	return Viewer{
		title:   title,
		content: content,
	}
}

// newViewerWithRaw creates a viewer with both raw markdown and rendered content.
func newViewerWithRaw(title, raw, rendered string) Viewer {
	v := Viewer{
		title:   title,
		raw:     raw,
		content: rendered,
	}
	v.headings = extractHeadings(raw, rendered)
	v.actions = actions.ParseActions(raw)
	return v
}

// Init initializes the viewer.
func (v Viewer) Init() tea.Cmd {
	return nil
}

// Update handles messages for the viewer.
func (v Viewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		headerHeight := 2
		footerHeight := 2
		if v.showActions {
			footerHeight = v.actionOverlayHeight() + 1
		}

		if !v.ready {
			v.viewport = viewport.New(msg.Width, msg.Height-headerHeight-footerHeight)
			v.viewport.YPosition = headerHeight
			v.viewport.SetContent(v.content)
			v.ready = true
		} else {
			v.viewport.Width = msg.Width
			v.viewport.Height = msg.Height - headerHeight - footerHeight
		}

	case actionResultMsg:
		v.busy = false
		if msg.err != nil {
			v.setStatus("Error: " + msg.err.Error())
		} else {
			v.setStatus(msg.status)
		}
		return v, nil

	case draftResultMsg:
		v.busy = false
		if msg.err != nil {
			v.setStatus("Draft error: " + msg.err.Error())
			return v, nil
		}
		// Copy draft to clipboard asynchronously
		return v, clipboardCmd(msg.raw, "Draft copied to clipboard")

	case tea.KeyMsg:
		if v.busy {
			// Only allow quit while busy
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return v, tea.Quit
			}
			return v, nil
		}
		if v.showActions {
			return v.updateActionOverlay(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return v, tea.Quit
		case "esc":
			return v, tea.Quit
		case "n":
			v.nextHeading()
			return v, nil
		case "N":
			v.prevHeading()
			return v, nil
		case "a":
			if len(v.actions) > 0 {
				v.showActions = true
				v.actionIdx = 0
			} else {
				v.setStatus("No actions found")
			}
			return v, nil
		case "d":
			return v.startDraftAction()
		case "o":
			return v.startOpenAction()
		case "i":
			return v.openFirstChart()
		}
	}

	v.viewport, cmd = v.viewport.Update(msg)
	return v, cmd
}

// View renders the viewer.
func (v Viewer) View() string {
	if !v.ready {
		return "Loading..."
	}

	header := headerStyle.Render(v.title)

	var footer string
	if v.showActions {
		footer = v.renderActionOverlay()
	} else {
		status := ""
		if v.busy {
			status = " • Working..."
		} else if v.statusMsg != "" && time.Now().Before(v.statusExp) {
			status = " • " + v.statusMsg
		}

		hints := " %3.f%%"
		if len(v.headings) > 0 {
			hints += " │ n/N sections"
		}
		if len(v.actions) > 0 {
			hints += " │ a actions"
		}
		if v.hasCharts && v.handoff != nil {
			hints += " │ i open chart"
		}
		hints += " │ q quit"

		footer = footerStyle.Render(fmt.Sprintf(hints+status, v.viewport.ScrollPercent()*100))
	}

	return strings.Join([]string{header, "", v.viewport.View(), "", footer}, "\n")
}

// RunViewer launches the interactive viewer for a report.
func RunViewer(title string, markdown string, opts ...ViewerOption) error {
	rendered, err := RenderMarkdown(markdown, 0)
	if err != nil {
		return err
	}

	v := newViewerWithRaw(title, markdown, rendered)
	for _, opt := range opts {
		opt(&v)
	}

	// Process charts after options are applied (need reportDir and imageConfig)
	v.imageTier = DetectImageTier(v.imageConfig)
	v.content = processCharts(v.raw, v.content, v.reportDir, v.imageTier)
	v.hasCharts = hasChartDirectives(v.raw)

	p := tea.NewProgram(v, tea.WithAltScreen())

	_, err = p.Run()
	return err
}

// --- Section navigation ---

// headingPattern matches markdown headings in raw markdown.
var headingPattern = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)

// extractHeadings scans raw markdown for headings and maps them to line
// positions in the rendered content.
func extractHeadings(raw, rendered string) []headingPos {
	matches := headingPattern.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil
	}

	renderedLines := strings.Split(rendered, "\n")
	var headings []headingPos
	searchFrom := 0

	for _, m := range matches {
		text := strings.TrimSpace(m[1])
		// Find this heading text in rendered output (Glamour preserves heading text)
		for i := searchFrom; i < len(renderedLines); i++ {
			if strings.Contains(renderedLines[i], text) {
				headings = append(headings, headingPos{text: text, line: i})
				searchFrom = i + 1
				break
			}
		}
	}
	return headings
}

func (v *Viewer) nextHeading() {
	if len(v.headings) == 0 {
		return
	}
	currentLine := v.viewport.YOffset
	for i, h := range v.headings {
		if h.line > currentLine {
			v.currentHead = i
			v.viewport.SetYOffset(h.line)
			return
		}
	}
	v.currentHead = 0
	v.viewport.SetYOffset(v.headings[0].line)
}

func (v *Viewer) prevHeading() {
	if len(v.headings) == 0 {
		return
	}
	currentLine := v.viewport.YOffset
	for i := len(v.headings) - 1; i >= 0; i-- {
		if v.headings[i].line < currentLine {
			v.currentHead = i
			v.viewport.SetYOffset(v.headings[i].line)
			return
		}
	}
	v.currentHead = len(v.headings) - 1
	v.viewport.SetYOffset(v.headings[v.currentHead].line)
}

// --- Action overlay ---

func (v *Viewer) actionOverlayHeight() int {
	count := len(v.actions)
	if count > 8 {
		count = 8
	}
	return count + 2
}

func (v Viewer) renderActionOverlay() string {
	var b strings.Builder
	b.WriteString(footerStyle.Render(" Actions (↑↓ navigate, enter execute, esc close):"))
	b.WriteString("\n")

	maxShow := 8
	if len(v.actions) < maxShow {
		maxShow = len(v.actions)
	}

	for i := 0; i < maxShow; i++ {
		a := v.actions[i]
		label := fmt.Sprintf("  [%s] %s", a.Type, a.Description)
		if a.Target != "" {
			label += " (" + a.Target + ")"
		}
		if i == v.actionIdx {
			b.WriteString(actionSelectedStyle.Render("▸ " + label))
		} else {
			b.WriteString(actionNormalStyle.Render("  " + label))
		}
		if i < maxShow-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (v Viewer) updateActionOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "a":
		v.showActions = false
		return v, nil
	case "q", "ctrl+c":
		return v, tea.Quit
	case "up", "k":
		if v.actionIdx > 0 {
			v.actionIdx--
		}
		return v, nil
	case "down", "j":
		if v.actionIdx < len(v.actions)-1 {
			v.actionIdx++
		}
		return v, nil
	case "enter":
		a := v.actions[v.actionIdx]
		v.showActions = false
		return v.startAction(a)
	}
	return v, nil
}

// --- Async action execution ---

// viewerContext returns the viewer's context or a background context if none set.
func (v *Viewer) viewerContext() context.Context {
	if v.ctx != nil {
		return v.ctx
	}
	return context.Background()
}

// startAction dispatches an action asynchronously based on its type.
func (v Viewer) startAction(a actions.Action) (tea.Model, tea.Cmd) {
	switch a.Type {
	case actions.ActionOpen:
		return v.startOpenActionFor(a)
	case actions.ActionDraft:
		return v.startDraftFromAction(a)
	case actions.ActionConfigure:
		v.setStatus("Configure: " + a.Description)
		return v, nil
	}
	return v, nil
}

// startOpenAction finds and executes the first open action.
func (v Viewer) startOpenAction() (tea.Model, tea.Cmd) {
	for _, a := range v.actions {
		if a.Type == actions.ActionOpen {
			return v.startOpenActionFor(a)
		}
	}
	v.setStatus("No open actions found")
	return v, nil
}

// startOpenActionFor opens a URL via handoff asynchronously.
func (v Viewer) startOpenActionFor(a actions.Action) (tea.Model, tea.Cmd) {
	if v.handoff == nil || a.Target == "" {
		v.setStatus("No handoff configured or no target URL")
		return v, nil
	}
	handoff := v.handoff
	target := a.Target
	v.busy = true
	return v, func() tea.Msg {
		err := handoff.OpenURL(target)
		if err != nil {
			return actionResultMsg{err: err}
		}
		return actionResultMsg{status: "Opened: " + target}
	}
}

// startDraftAction finds and executes the first draft action.
func (v Viewer) startDraftAction() (tea.Model, tea.Cmd) {
	for _, a := range v.actions {
		if a.Type == actions.ActionDraft {
			return v.startDraftFromAction(a)
		}
	}
	v.setStatus("No draft actions found")
	return v, nil
}

// startDraftFromAction starts async draft generation or copies instruction to clipboard.
func (v Viewer) startDraftFromAction(a actions.Action) (tea.Model, tea.Cmd) {
	instruction := a.Description
	if a.Target != "" {
		instruction = a.Target
	}

	if v.provider == nil {
		// No LLM — copy instruction to clipboard (fast, no async needed)
		return v, clipboardCmd(instruction, "Draft instruction copied to clipboard")
	}

	// Async LLM draft generation
	provider := v.provider
	ledger := v.ledger
	ctx := v.viewerContext()
	v.busy = true
	v.setStatus("Generating draft...")

	return v, func() tea.Msg {
		var contextData string
		if ledger != nil {
			contextData, _ = ledger.GatherContext(50_000)
		}
		draft, err := actions.GenerateDraft(ctx, provider, instruction, contextData)
		if err != nil {
			return draftResultMsg{err: err}
		}
		return draftResultMsg{raw: draft.Raw}
	}
}

// clipboardCmd returns a tea.Cmd that copies text to clipboard and reports the result.
func clipboardCmd(text, successMsg string) tea.Cmd {
	return func() tea.Msg {
		if err := actions.CopyToClipboard(text); err != nil {
			return actionResultMsg{err: fmt.Errorf("clipboard: %w", err)}
		}
		return actionResultMsg{status: successMsg}
	}
}

func (v *Viewer) setStatus(msg string) {
	v.statusMsg = msg
	v.statusExp = time.Now().Add(5 * time.Second)
}
