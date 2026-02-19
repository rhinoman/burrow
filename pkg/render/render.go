// Package render provides terminal markdown rendering and a scrollable report viewer.
package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var headerStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("205")).
	PaddingLeft(1)

var footerStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("240")).
	PaddingLeft(1)

// RenderMarkdown renders markdown to styled terminal output using Glamour.
func RenderMarkdown(markdown string, width int) (string, error) {
	if width <= 0 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", fmt.Errorf("creating renderer: %w", err)
	}
	out, err := r.Render(markdown)
	if err != nil {
		return "", fmt.Errorf("rendering markdown: %w", err)
	}
	return out, nil
}

// Viewer is a Bubble Tea model for scrollable report viewing.
type Viewer struct {
	title    string
	content  string
	viewport viewport.Model
	ready    bool
}

// NewViewer creates a viewer with pre-rendered content.
func NewViewer(title string, content string) Viewer {
	return Viewer{
		title:   title,
		content: content,
	}
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

		if !v.ready {
			v.viewport = viewport.New(msg.Width, msg.Height-headerHeight-footerHeight)
			v.viewport.YPosition = headerHeight
			v.viewport.SetContent(v.content)
			v.ready = true
		} else {
			v.viewport.Width = msg.Width
			v.viewport.Height = msg.Height - headerHeight - footerHeight
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return v, tea.Quit
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
	footer := footerStyle.Render(fmt.Sprintf(
		" %3.f%% • q to quit",
		v.viewport.ScrollPercent()*100,
	))

	return strings.Join([]string{header, "", v.viewport.View(), "", footer}, "\n")
}

// RunViewer launches the interactive viewer for a report.
func RunViewer(title string, markdown string) error {
	rendered, err := RenderMarkdown(markdown, 0)
	if err != nil {
		return err
	}

	v := NewViewer(title, rendered)
	p := tea.NewProgram(v, tea.WithAltScreen())

	_, err = p.Run()
	return err
}
