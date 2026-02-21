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
	"github.com/jcadam/burrow/pkg/profile"
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
	text      string
	line      int  // line in fullLines (stable, original position)
	viewLine  int  // line in current visible content (recomputed on rebuild)
	level     int  // heading level (1-6), from # count
	endLine   int  // exclusive end of section content in fullLines
	collapsed bool
}

// Viewer is a Bubble Tea model for scrollable report viewing with section
// navigation and action execution.
type Viewer struct {
	title     string
	raw       string   // original markdown
	content   string   // rendered content (visible, rebuilt on toggle)
	fullLines []string // complete rendered content (all expanded), never modified
	viewport  viewport.Model
	ready     bool

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
	profile  *profile.Profile
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

// WithProfile provides a user profile for draft generation.
func WithProfile(p *profile.Profile) ViewerOption {
	return func(v *Viewer) { v.profile = p }
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
		title:     title,
		raw:       raw,
		content:   rendered,
		fullLines: strings.Split(rendered, "\n"),
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
		case "enter", "tab":
			idx := v.currentHeadingIdx()
			if idx >= 0 {
				v.toggleSection(idx)
			}
			return v, nil
		case "c":
			v.collapseAll()
			return v, nil
		case "e":
			v.expandAll()
			return v, nil
		case "p":
			return v.startPlayAction()
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
			hints += " │ enter fold │ c/e all"
		}
		if len(v.actions) > 0 {
			hints += " │ a actions"
		}
		if v.hasCharts && v.handoff != nil {
			hints += " │ i open chart"
		}
		if v.hasPlayActions() {
			hints += " │ p play"
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

	// Refresh fullLines and headings after chart processing may have changed content
	v.fullLines = strings.Split(v.content, "\n")
	v.headings = extractHeadings(v.raw, v.content)

	p := tea.NewProgram(v, tea.WithAltScreen())

	_, err = p.Run()
	return err
}

// --- Section navigation ---

// headingPattern matches markdown headings in raw markdown.
var headingPattern = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)

// extractHeadings scans raw markdown for headings and maps them to line
// positions in the rendered content. Also computes heading levels and
// section boundaries (endLine).
func extractHeadings(raw, rendered string) []headingPos {
	matches := headingPattern.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil
	}

	renderedLines := strings.Split(rendered, "\n")
	var headings []headingPos
	searchFrom := 0

	for _, m := range matches {
		level := len(m[1]) // number of # characters
		text := strings.TrimSpace(m[2])
		// Find this heading text in rendered output (Glamour preserves heading text)
		for i := searchFrom; i < len(renderedLines); i++ {
			if strings.Contains(renderedLines[i], text) {
				headings = append(headings, headingPos{
					text:     text,
					line:     i,
					viewLine: i,
					level:    level,
				})
				searchFrom = i + 1
				break
			}
		}
	}

	computeEndLines(headings, len(renderedLines))
	return headings
}

// computeEndLines sets endLine for each heading: the next heading at same or
// higher level (lower number), or the total line count.
func computeEndLines(headings []headingPos, totalLines int) {
	for i := range headings {
		headings[i].endLine = totalLines // default: extends to end
		for j := i + 1; j < len(headings); j++ {
			if headings[j].level <= headings[i].level {
				headings[i].endLine = headings[j].line
				break
			}
		}
	}
}

func (v *Viewer) nextHeading() {
	if len(v.headings) == 0 {
		return
	}
	currentLine := v.viewport.YOffset
	for i, h := range v.headings {
		if h.viewLine > currentLine {
			v.currentHead = i
			v.viewport.SetYOffset(h.viewLine)
			return
		}
	}
	v.currentHead = 0
	v.viewport.SetYOffset(v.headings[0].viewLine)
}

func (v *Viewer) prevHeading() {
	if len(v.headings) == 0 {
		return
	}
	currentLine := v.viewport.YOffset
	for i := len(v.headings) - 1; i >= 0; i-- {
		if v.headings[i].viewLine < currentLine {
			v.currentHead = i
			v.viewport.SetYOffset(v.headings[i].viewLine)
			return
		}
	}
	v.currentHead = len(v.headings) - 1
	v.viewport.SetYOffset(v.headings[v.currentHead].viewLine)
}

// --- Expandable sections ---

// stripANSI removes ANSI escape sequences from a string.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// insertAfterANSIPrefix inserts text after any leading ANSI escape sequences
// on a line, so the indicator appears before the visible text but after styling.
func insertAfterANSIPrefix(line, insert string) string {
	// Find the position after all leading ANSI escape codes
	pos := 0
	for {
		loc := ansiPattern.FindStringIndex(line[pos:])
		if loc == nil || loc[0] != 0 {
			break
		}
		pos += loc[1]
	}
	return line[:pos] + insert + line[pos:]
}

// prependIndicator adds a ▸ (collapsed) or ▼ (expanded) indicator to a heading line.
func prependIndicator(line string, collapsed bool) string {
	indicator := "▼ "
	if collapsed {
		indicator = "▸ "
	}
	return insertAfterANSIPrefix(line, indicator)
}

// rebuildContent constructs visible content from fullLines, skipping collapsed
// section bodies and adding fold indicators to headings.
func (v *Viewer) rebuildContent() {
	if len(v.fullLines) == 0 {
		return
	}

	// Build a set of line ranges to skip (collapsed section bodies)
	type skipRange struct{ start, end int }
	var skips []skipRange
	for _, h := range v.headings {
		if h.collapsed && h.level > 1 { // H1 is never collapsible
			skips = append(skips, skipRange{h.line + 1, h.endLine})
		}
	}

	isSkipped := func(lineIdx int) bool {
		for _, s := range skips {
			if lineIdx >= s.start && lineIdx < s.end {
				return true
			}
		}
		return false
	}

	// Map from fullLines index to heading index (for indicator prepend)
	headingAtLine := make(map[int]int, len(v.headings))
	for i, h := range v.headings {
		headingAtLine[h.line] = i
	}

	var visible []string
	viewIdx := 0
	for i, line := range v.fullLines {
		if isSkipped(i) {
			continue
		}
		if hIdx, ok := headingAtLine[i]; ok {
			h := v.headings[hIdx]
			if h.level > 1 { // Only show indicators on collapsible headings
				line = prependIndicator(line, h.collapsed)
			}
			v.headings[hIdx].viewLine = viewIdx
		}
		visible = append(visible, line)
		viewIdx++
	}

	v.content = strings.Join(visible, "\n")
	if v.ready {
		v.viewport.SetContent(v.content)
	}
}

// currentHeadingIdx returns the index of the collapsible heading (level > 1) at
// or just before the current viewport offset. Returns -1 if none found.
func (v *Viewer) currentHeadingIdx() int {
	if len(v.headings) == 0 {
		return -1
	}
	yOff := v.viewport.YOffset
	best := -1
	for i, h := range v.headings {
		if h.level <= 1 {
			continue // H1 not collapsible
		}
		if h.viewLine <= yOff {
			best = i
		}
	}
	// If we haven't found one at/before cursor, find the first one after
	if best == -1 {
		for i, h := range v.headings {
			if h.level > 1 {
				return i
			}
		}
	}
	return best
}

// toggleSection flips the collapsed state of a heading and rebuilds content.
func (v *Viewer) toggleSection(idx int) {
	if idx < 0 || idx >= len(v.headings) || v.headings[idx].level <= 1 {
		return
	}
	v.headings[idx].collapsed = !v.headings[idx].collapsed
	v.rebuildContent()
	// Keep the toggled heading visible
	v.viewport.SetYOffset(v.headings[idx].viewLine)
}

// collapseAll collapses all sections (## and below).
func (v *Viewer) collapseAll() {
	changed := false
	for i := range v.headings {
		if v.headings[i].level > 1 && !v.headings[i].collapsed {
			v.headings[i].collapsed = true
			changed = true
		}
	}
	if changed {
		v.rebuildContent()
	}
}

// expandAll expands all sections.
func (v *Viewer) expandAll() {
	changed := false
	for i := range v.headings {
		if v.headings[i].collapsed {
			v.headings[i].collapsed = false
			changed = true
		}
	}
	if changed {
		v.rebuildContent()
	}
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
	case actions.ActionPlay:
		return v.startPlayActionFor(a)
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
	prof := v.profile
	ctx := v.viewerContext()
	v.busy = true
	v.setStatus("Generating draft...")

	return v, func() tea.Msg {
		var contextData string
		if ledger != nil {
			contextData, _ = ledger.GatherContext(50_000)
		}
		draft, err := actions.GenerateDraft(ctx, provider, instruction, contextData, prof)
		if err != nil {
			return draftResultMsg{err: err}
		}
		return draftResultMsg{raw: draft.Raw}
	}
}

// hasPlayActions returns true if any actions are of type ActionPlay.
func (v *Viewer) hasPlayActions() bool {
	for _, a := range v.actions {
		if a.Type == actions.ActionPlay {
			return true
		}
	}
	return false
}

// startPlayAction finds and executes the first play action.
func (v Viewer) startPlayAction() (tea.Model, tea.Cmd) {
	for _, a := range v.actions {
		if a.Type == actions.ActionPlay {
			return v.startPlayActionFor(a)
		}
	}
	v.setStatus("No play actions found")
	return v, nil
}

// startPlayActionFor plays a media file via handoff asynchronously.
func (v Viewer) startPlayActionFor(a actions.Action) (tea.Model, tea.Cmd) {
	if v.handoff == nil || a.Target == "" {
		v.setStatus("No handoff configured or no media target")
		return v, nil
	}
	handoff := v.handoff
	target := a.Target
	v.busy = true
	return v, func() tea.Msg {
		err := handoff.PlayMedia(target)
		if err != nil {
			return actionResultMsg{err: err}
		}
		return actionResultMsg{status: "Playing: " + target}
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
