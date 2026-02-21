package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcadam/burrow/pkg/actions"
	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/contacts"
	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/jcadam/burrow/pkg/profile"
	"github.com/jcadam/burrow/pkg/render"
	"github.com/jcadam/burrow/pkg/reports"
	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/synthesis"
	"golang.org/x/term"
)

// readWriter combines separate reader and writer into an io.ReadWriter.
type readWriter struct {
	io.Reader
	io.Writer
}

// interactiveSession holds state for the interactive REPL.
type interactiveSession struct {
	burrowDir string
	cfg       *config.Config
	registry  *services.Registry
	ledger    *bcontext.Ledger
	contacts  *contacts.Store
	profile   *profile.Profile
	provider  synthesis.Provider
	handoff   *actions.Handoff
	term      *term.Terminal // line editor (nil when stdin is not a tty)
	fd        int            // stdin file descriptor
	rawState  *term.State    // saved state for restore
	scanner   *bufio.Scanner // fallback for non-TTY input
}

// out returns the writer for REPL output. In raw mode this is the Terminal
// (which translates \n to \r\n); otherwise it is os.Stdout.
func (s *interactiveSession) out() io.Writer {
	if s.term != nil {
		return s.term
	}
	return os.Stdout
}

// readLine reads one line with proper UTF-8 editing support.
// Falls back to bufio.Scanner when not a TTY.
func (s *interactiveSession) readLine() (string, error) {
	if s.term != nil {
		return s.term.ReadLine()
	}
	if s.scanner.Scan() {
		return s.scanner.Text(), nil
	}
	if err := s.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// suspendTerm restores cooked mode before launching sub-programs (viewer).
func (s *interactiveSession) suspendTerm() {
	if s.rawState != nil {
		term.Restore(s.fd, s.rawState)
	}
}

// resumeTerm re-enters raw mode and creates a fresh Terminal.
func (s *interactiveSession) resumeTerm() error {
	if s.term == nil {
		return nil
	}
	state, err := term.MakeRaw(s.fd)
	if err != nil {
		return fmt.Errorf("re-entering raw mode: %w", err)
	}
	s.rawState = state
	s.term = term.NewTerminal(readWriter{os.Stdin, os.Stdout}, "  > ")
	return nil
}

// runInteractive starts the interactive REPL.
func runInteractive(ctx context.Context) error {
	burrowDir, err := config.BurrowDir()
	if err != nil {
		return err
	}

	// Load config
	cfg, err := config.Load(burrowDir)
	if err != nil {
		fmt.Println("No configuration found. Run 'gd init' to get started.")
		return nil
	}
	config.ResolveEnvVars(cfg)

	if err := config.Validate(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: config issue: %v\n", err)
	}

	// Build registry
	registry, err := buildRegistry(cfg, burrowDir)
	if err != nil {
		return fmt.Errorf("building service registry: %w", err)
	}

	// Open ledger
	contextDir := filepath.Join(burrowDir, "context")
	ledger, err := bcontext.NewLedger(contextDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open context ledger: %v\n", err)
	}

	// Open contacts store
	contactsDir := filepath.Join(burrowDir, "contacts")
	contactStore, _ := contacts.NewStore(contactsDir)

	// Find provider — local only. Interactive mode uses the same policy as
	// "gd ask": zero network requests for reasoning. Remote providers are
	// intentionally excluded to maintain compartmentalization.
	provider := findLocalProvider(cfg)

	// Load user profile (optional)
	prof, _ := profile.Load(burrowDir)

	// Create handoff
	handoff := actions.NewHandoff(cfg.Apps)

	sess := &interactiveSession{
		burrowDir: burrowDir,
		cfg:       cfg,
		registry:  registry,
		ledger:    ledger,
		contacts:  contactStore,
		profile:   prof,
		provider:  provider,
		handoff:   handoff,
	}

	// Print banner (before raw mode, to os.Stdout)
	svcCount := len(cfg.Services)
	fmt.Printf("\n  Burrow %s . %d source(s) configured", version, svcCount)
	if contactStore != nil {
		if n := contactStore.Count(); n > 0 {
			fmt.Printf(" . %d contact(s)", n)
		}
	}
	if prof != nil && prof.Name != "" {
		fmt.Printf(" . Profile: %s", prof.Name)
	}
	fmt.Println()
	if provider != nil {
		fmt.Println("  Local LLM available for reasoning")
	}
	fmt.Println("  Type 'help' for commands, or ask a question.")
	fmt.Println()

	// Set up line editing: raw mode with term.Terminal when stdin is a TTY,
	// plain bufio.Scanner otherwise (piped input).
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("entering raw mode: %w", err)
		}
		sess.fd = fd
		sess.rawState = state
		sess.term = term.NewTerminal(readWriter{os.Stdin, os.Stdout}, "  > ")
		defer term.Restore(fd, state)
	} else {
		sess.scanner = bufio.NewScanner(os.Stdin)
	}

	for {
		line, err := sess.readLine()
		if err != nil {
			fmt.Fprintln(sess.out())
			return nil
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch {
		case line == "quit" || line == "exit":
			return nil
		case line == "help":
			sess.printHelp()
		case line == "sources":
			sess.printSources()
		case strings.HasPrefix(line, "search ") || strings.HasPrefix(line, "query "):
			sess.handleServiceQuery(ctx, line)
		case strings.HasPrefix(line, "view"):
			if err := sess.handleView(strings.TrimSpace(strings.TrimPrefix(line, "view"))); err != nil {
				return err
			}
		case strings.HasPrefix(line, "draft "):
			sess.handleDraft(ctx, strings.TrimPrefix(line, "draft "))
		case strings.HasPrefix(line, "ask "):
			sess.handleAsk(ctx, strings.TrimPrefix(line, "ask "))
		default:
			// Treat anything else as an ask query
			sess.handleAsk(ctx, line)
		}
	}
}

func (s *interactiveSession) printHelp() {
	fmt.Fprint(s.out(), `
  Commands:
    help                          Show this help
    sources                       List configured services and tools
    search <svc> <tool> [params]  Query a service tool (key=value params)
    query <svc> <tool> [params]   Same as search
    ask <question>                Ask a question over collected context
    view [routine]                View latest report in interactive viewer
    draft <instruction>           Generate a communication draft
    quit / exit                   Exit interactive mode

`)
}

func (s *interactiveSession) printSources() {
	w := s.out()
	if len(s.cfg.Services) == 0 {
		fmt.Fprintln(w, "  No services configured.")
		return
	}
	fmt.Fprintln(w)
	for _, svc := range s.cfg.Services {
		fmt.Fprintf(w, "    %s (%s)\n", svc.Name, svc.Endpoint)
		if svc.Type == "rss" {
			fmt.Fprintf(w, "      - feed: RSS/Atom feed\n")
		}
		for _, tool := range svc.Tools {
			desc := tool.Description
			if desc == "" {
				desc = tool.Method + " " + tool.Path
			}
			fmt.Fprintf(w, "      - %s: %s\n", tool.Name, desc)
		}
	}
	fmt.Fprintln(w)
}

// handleView opens the latest report in the interactive viewer.
func (s *interactiveSession) handleView(routine string) error {
	reportsDir := filepath.Join(s.burrowDir, "reports")

	var report *reports.Report
	var err error

	if routine != "" {
		report, err = resolveReport(reportsDir, routine)
		if err != nil {
			fmt.Fprintf(s.out(), "  %v\n", err)
			return nil
		}
	} else {
		all, err := reports.List(reportsDir)
		if err != nil {
			fmt.Fprintf(s.out(), "  Error listing reports: %v\n", err)
			return nil
		}
		if len(all) == 0 {
			fmt.Fprintln(s.out(), "  No reports found.")
			return nil
		}
		report = all[0]
	}

	title := report.Title
	if title == "" {
		title = report.Routine + " — " + report.Date
	}

	opts := viewerOptions(s.cfg, s.profile)
	if s.ledger != nil {
		opts = append(opts, render.WithLedger(s.ledger))
	}

	s.suspendTerm()
	viewErr := render.RunViewer(title, report.Markdown, opts...)
	if err := s.resumeTerm(); err != nil {
		return err
	}
	if viewErr != nil {
		fmt.Fprintf(s.out(), "  Viewer error: %v\n", viewErr)
	}
	return nil
}

// handleServiceQuery dispatches a query to a named service tool.
func (s *interactiveSession) handleServiceQuery(ctx context.Context, line string) {
	w := s.out()
	svcName, toolName, params := parseServiceQuery(line)
	if svcName == "" || toolName == "" {
		fmt.Fprintln(w, "  Usage: search <service> <tool> [key=value ...]")
		return
	}

	svc, err := s.registry.Get(svcName)
	if err != nil {
		fmt.Fprintf(w, "  Unknown service: %s\n", svcName)
		fmt.Fprintf(w, "  Available: %s\n", strings.Join(s.registry.List(), ", "))
		return
	}

	result, err := svc.Execute(ctx, toolName, params)
	if err != nil {
		fmt.Fprintf(w, "  Error: %v\n", err)
		return
	}

	if result.Error != "" {
		fmt.Fprintf(w, "  Service error: %s\n", result.Error)
		return
	}

	fmt.Fprintf(w, "\n%s\n\n", string(result.Data))

	// Log to context ledger
	if s.ledger != nil {
		s.ledger.Append(bcontext.Entry{
			Type:      bcontext.TypeSession,
			Label:     fmt.Sprintf("Interactive query: %s %s", svcName, toolName),
			Timestamp: time.Now().UTC(),
			Content:   string(result.Data),
		})
	}
}

// handleAsk queries the local LLM over context data, or falls back to text search.
func (s *interactiveSession) handleAsk(ctx context.Context, question string) {
	w := s.out()
	if s.provider != nil && s.ledger != nil {
		contextData, err := s.ledger.GatherContext(100_000)
		if err != nil {
			fmt.Fprintf(w, "  Error gathering context: %v\n", err)
			return
		}
		if s.contacts != nil {
			if cc := s.contacts.ForContext(); cc != "" {
				contextData += "\n" + cc
			}
		}

		if contextData != "" {
			var userPrompt strings.Builder
			userPrompt.WriteString("Question: ")
			userPrompt.WriteString(question)
			userPrompt.WriteString("\n\nContext data:\n\n")
			userPrompt.WriteString(contextData)

			response, err := s.provider.Complete(ctx, buildAskSystemPrompt(s.profile), userPrompt.String())
			if err != nil {
				fmt.Fprintf(w, "  LLM error: %v\n", err)
				return
			}

			rendered, err := render.RenderMarkdown(response, 78)
			if err != nil {
				fmt.Fprintln(w, response)
			} else {
				fmt.Fprint(w, rendered)
			}

			// Log to ledger
			s.ledger.Append(bcontext.Entry{
				Type:      bcontext.TypeSession,
				Label:     "Interactive ask: " + question,
				Timestamp: time.Now().UTC(),
				Content:   response,
			})
			return
		}
	}

	// Fallback to text search
	if s.ledger == nil {
		fmt.Fprintln(w, "  No context ledger available.")
		return
	}
	entries, err := s.ledger.Search(question)
	if err != nil {
		fmt.Fprintf(w, "  Search error: %v\n", err)
		return
	}
	if len(entries) == 0 {
		fmt.Fprintf(w, "  No results for %q\n", question)
		return
	}
	fmt.Fprintf(w, "  Found %d result(s):\n\n", len(entries))
	for _, e := range entries {
		ts := e.Timestamp.Format("2006-01-02 15:04")
		fmt.Fprintf(w, "    %s  [%s]  %s\n", ts, e.Type, e.Label)
	}
	fmt.Fprintln(w)
}

// handleDraft generates a communication draft and presents an action menu.
func (s *interactiveSession) handleDraft(ctx context.Context, instruction string) {
	w := s.out()
	if s.provider == nil {
		fmt.Fprintln(w, "  Draft generation requires a local LLM. Configure one with 'gd configure'.")
		return
	}

	var contextData string
	if s.ledger != nil {
		contextData, _ = s.ledger.GatherContext(50_000)
	}
	if s.contacts != nil {
		if cc := s.contacts.ForContext(); cc != "" {
			contextData += "\n" + cc
		}
	}

	fmt.Fprintln(w, "  Generating draft...")
	draft, err := actions.GenerateDraft(ctx, s.provider, instruction, contextData, s.profile)
	if err != nil {
		fmt.Fprintf(w, "  Error: %v\n", err)
		return
	}

	// Display draft
	fmt.Fprintln(w)
	if draft.To != "" {
		fmt.Fprintf(w, "  To: %s\n", draft.To)
	}
	if draft.Subject != "" {
		fmt.Fprintf(w, "  Subject: %s\n", draft.Subject)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, draft.Body)
	fmt.Fprintln(w)

	// Action menu
	fmt.Fprintln(w, "  [c]opy  [m]ail  [d]iscard")
	line, err := s.readLine()
	if err != nil {
		return
	}

	switch strings.TrimSpace(line) {
	case "c", "copy":
		if err := actions.CopyToClipboard(draft.Raw); err != nil {
			fmt.Fprintf(w, "  %v\n", err)
		} else {
			fmt.Fprintln(w, "  Copied to clipboard.")
		}
	case "m", "mail":
		if err := s.handoff.OpenMailto(draft.To, draft.Subject, draft.Body); err != nil {
			fmt.Fprintf(w, "  %v\n", err)
		} else {
			fmt.Fprintln(w, "  Opened in email app.")
		}
	case "d", "discard", "":
		fmt.Fprintln(w, "  Discarded.")
	}
}

// parseServiceQuery extracts service name, tool name, and key=value params from
// a "search <svc> <tool> [key=value ...]" line.
func parseServiceQuery(line string) (svc, tool string, params map[string]string) {
	// Strip the command prefix
	line = strings.TrimPrefix(line, "search ")
	line = strings.TrimPrefix(line, "query ")
	line = strings.TrimSpace(line)

	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "", "", nil
	}
	svc = parts[0]
	if len(parts) > 1 {
		tool = parts[1]
	}
	// Remaining parts are key=value params
	params = make(map[string]string)
	if len(parts) <= 2 {
		return svc, tool, params
	}
	for _, p := range parts[2:] {
		k, v, ok := strings.Cut(p, "=")
		if ok {
			params[k] = v
		}
	}
	return svc, tool, params
}
