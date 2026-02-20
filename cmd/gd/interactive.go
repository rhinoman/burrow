package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcadam/burrow/pkg/actions"
	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/contacts"
	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/jcadam/burrow/pkg/render"
	"github.com/jcadam/burrow/pkg/reports"
	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/synthesis"
)

// interactiveSession holds state for the interactive REPL.
type interactiveSession struct {
	burrowDir string
	cfg       *config.Config
	registry  *services.Registry
	ledger    *bcontext.Ledger
	contacts  *contacts.Store
	provider  synthesis.Provider
	handoff   *actions.Handoff
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

	// Create handoff
	handoff := actions.NewHandoff(cfg.Apps)

	sess := &interactiveSession{
		burrowDir: burrowDir,
		cfg:       cfg,
		registry:  registry,
		ledger:    ledger,
		contacts:  contactStore,
		provider:  provider,
		handoff:   handoff,
	}

	// Print banner
	svcCount := len(cfg.Services)
	fmt.Printf("\n  Burrow %s . %d source(s) configured", version, svcCount)
	if contactStore != nil {
		if n := contactStore.Count(); n > 0 {
			fmt.Printf(" . %d contact(s)", n)
		}
	}
	fmt.Println()
	if provider != nil {
		fmt.Println("  Local LLM available for reasoning")
	}
	fmt.Println("  Type 'help' for commands, or ask a question.")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("  > ")
		if !scanner.Scan() {
			fmt.Println()
			return nil
		}
		line := strings.TrimSpace(scanner.Text())
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
			sess.handleView(strings.TrimSpace(strings.TrimPrefix(line, "view")))
		case strings.HasPrefix(line, "draft "):
			sess.handleDraft(ctx, scanner, strings.TrimPrefix(line, "draft "))
		case strings.HasPrefix(line, "ask "):
			sess.handleAsk(ctx, strings.TrimPrefix(line, "ask "))
		default:
			// Treat anything else as an ask query
			sess.handleAsk(ctx, line)
		}
	}
}

func (s *interactiveSession) printHelp() {
	fmt.Print(`
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
	if len(s.cfg.Services) == 0 {
		fmt.Println("  No services configured.")
		return
	}
	fmt.Println()
	for _, svc := range s.cfg.Services {
		fmt.Printf("    %s (%s)\n", svc.Name, svc.Endpoint)
		for _, tool := range svc.Tools {
			desc := tool.Description
			if desc == "" {
				desc = tool.Method + " " + tool.Path
			}
			fmt.Printf("      - %s: %s\n", tool.Name, desc)
		}
	}
	fmt.Println()
}

// handleView opens the latest report in the interactive viewer.
func (s *interactiveSession) handleView(routine string) {
	reportsDir := filepath.Join(s.burrowDir, "reports")

	var report *reports.Report
	var err error

	if routine != "" {
		report, err = resolveReport(reportsDir, routine)
		if err != nil {
			fmt.Printf("  %v\n", err)
			return
		}
	} else {
		all, err := reports.List(reportsDir)
		if err != nil {
			fmt.Printf("  Error listing reports: %v\n", err)
			return
		}
		if len(all) == 0 {
			fmt.Println("  No reports found.")
			return
		}
		report = all[0]
	}

	title := report.Title
	if title == "" {
		title = report.Routine + " — " + report.Date
	}

	opts := viewerOptions(s.cfg)
	if s.ledger != nil {
		opts = append(opts, render.WithLedger(s.ledger))
	}

	if err := render.RunViewer(title, report.Markdown, opts...); err != nil {
		fmt.Printf("  Viewer error: %v\n", err)
	}
}

// handleServiceQuery dispatches a query to a named service tool.
func (s *interactiveSession) handleServiceQuery(ctx context.Context, line string) {
	svcName, toolName, params := parseServiceQuery(line)
	if svcName == "" || toolName == "" {
		fmt.Println("  Usage: search <service> <tool> [key=value ...]")
		return
	}

	svc, err := s.registry.Get(svcName)
	if err != nil {
		fmt.Printf("  Unknown service: %s\n", svcName)
		fmt.Printf("  Available: %s\n", strings.Join(s.registry.List(), ", "))
		return
	}

	result, err := svc.Execute(ctx, toolName, params)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}

	if result.Error != "" {
		fmt.Printf("  Service error: %s\n", result.Error)
		return
	}

	fmt.Printf("\n%s\n\n", string(result.Data))

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
	if s.provider != nil && s.ledger != nil {
		contextData, err := s.ledger.GatherContext(100_000)
		if err != nil {
			fmt.Printf("  Error gathering context: %v\n", err)
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

			response, err := s.provider.Complete(ctx, askSystemPrompt, userPrompt.String())
			if err != nil {
				fmt.Printf("  LLM error: %v\n", err)
				return
			}

			rendered, err := render.RenderMarkdown(response, 78)
			if err != nil {
				fmt.Println(response)
			} else {
				fmt.Print(rendered)
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
		fmt.Println("  No context ledger available.")
		return
	}
	entries, err := s.ledger.Search(question)
	if err != nil {
		fmt.Printf("  Search error: %v\n", err)
		return
	}
	if len(entries) == 0 {
		fmt.Printf("  No results for %q\n", question)
		return
	}
	fmt.Printf("  Found %d result(s):\n\n", len(entries))
	for _, e := range entries {
		ts := e.Timestamp.Format("2006-01-02 15:04")
		fmt.Printf("    %s  [%s]  %s\n", ts, e.Type, e.Label)
	}
	fmt.Println()
}

// handleDraft generates a communication draft and presents an action menu.
func (s *interactiveSession) handleDraft(ctx context.Context, scanner *bufio.Scanner, instruction string) {
	if s.provider == nil {
		fmt.Println("  Draft generation requires a local LLM. Configure one with 'gd configure'.")
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

	fmt.Println("  Generating draft...")
	draft, err := actions.GenerateDraft(ctx, s.provider, instruction, contextData)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}

	// Display draft
	fmt.Println()
	if draft.To != "" {
		fmt.Printf("  To: %s\n", draft.To)
	}
	if draft.Subject != "" {
		fmt.Printf("  Subject: %s\n", draft.Subject)
	}
	fmt.Println()
	fmt.Println(draft.Body)
	fmt.Println()

	// Action menu
	fmt.Println("  [c]opy  [m]ail  [d]iscard")
	fmt.Print("  > ")
	if !scanner.Scan() {
		return
	}

	switch strings.TrimSpace(scanner.Text()) {
	case "c", "copy":
		if err := actions.CopyToClipboard(draft.Raw); err != nil {
			fmt.Printf("  %v\n", err)
		} else {
			fmt.Println("  Copied to clipboard.")
		}
	case "m", "mail":
		if err := s.handoff.OpenMailto(draft.To, draft.Subject, draft.Body); err != nil {
			fmt.Printf("  %v\n", err)
		} else {
			fmt.Println("  Opened in email app.")
		}
	case "d", "discard", "":
		fmt.Println("  Discarded.")
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
