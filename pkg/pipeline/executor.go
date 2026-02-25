package pipeline

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jcadam/burrow/pkg/charts"
	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/jcadam/burrow/pkg/debug"
	"github.com/jcadam/burrow/pkg/profile"
	"github.com/jcadam/burrow/pkg/reports"
	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/slug"
	"github.com/jcadam/burrow/pkg/synthesis"
)

// Executor runs routines by querying services and producing reports.
type Executor struct {
	registry    *services.Registry
	synthesizer synthesis.Synthesizer
	reportsDir  string
	ledger      *bcontext.Ledger
	profile     *profile.Profile
	randFunc    func(max int) int
	debug       *debug.Logger
}

// NewExecutor creates an executor with the given dependencies.
func NewExecutor(registry *services.Registry, synthesizer synthesis.Synthesizer, reportsDir string) *Executor {
	return &Executor{
		registry:    registry,
		synthesizer: synthesizer,
		reportsDir:  reportsDir,
		randFunc:    func(max int) int { return rand.IntN(max) },
	}
}

// SetLedger sets the context ledger for indexing results and reports.
func (e *Executor) SetLedger(l *bcontext.Ledger) {
	e.ledger = l
}

// SetProfile sets the user profile for template expansion in routines.
func (e *Executor) SetProfile(p *profile.Profile) {
	e.profile = p
}

// SetRandFunc replaces the random function (for testing jitter).
func (e *Executor) SetRandFunc(f func(max int) int) {
	e.randFunc = f
}

// SetDebug enables debug logging for pipeline execution. Nil disables it.
func (e *Executor) SetDebug(l *debug.Logger) {
	e.debug = l
}

// Run executes a routine: queries all sources in parallel with jitter,
// synthesizes results, saves report, and indexes in context ledger.
func (e *Executor) Run(ctx context.Context, routine *Routine) (*reports.Report, error) {
	e.debug.Section(fmt.Sprintf("Running %q (%d sources, jitter=%ds)", routine.Name, len(routine.Sources), routine.Jitter))

	results := make([]*services.Result, len(routine.Sources))
	rawResults := make(map[string][]byte)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, src := range routine.Sources {
		wg.Add(1)
		go func(idx int, src SourceConfig) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[idx] = &services.Result{
						Service:      src.Service,
						Tool:         src.Tool,
						Timestamp:    time.Now().UTC(),
						Error:        fmt.Sprintf("panic: %v", r),
						ContextLabel: src.ContextLabel,
					}
				}
			}()

			e.debug.Printf("source %d: %s/%s params=%v", idx, src.Service, src.Tool, src.Params)

			// Apply jitter before executing
			if routine.Jitter > 0 {
				jitterSecs := e.randFunc(routine.Jitter)
				if jitterSecs > 0 {
					e.debug.Printf("  jitter: %ds", jitterSecs)
					timer := time.NewTimer(time.Duration(jitterSecs) * time.Second)
					select {
					case <-ctx.Done():
						timer.Stop()
						results[idx] = &services.Result{
							Service:      src.Service,
							Tool:         src.Tool,
							Timestamp:    time.Now().UTC(),
							Error:        ctx.Err().Error(),
							ContextLabel: src.ContextLabel,
						}
						return
					case <-timer.C:
					}
				}
			}

			svc, err := e.registry.Get(src.Service)
			if err != nil {
				results[idx] = &services.Result{
					Service:      src.Service,
					Tool:         src.Tool,
					Timestamp:    time.Now().UTC(),
					Error:        fmt.Sprintf("service not found: %v", err),
					ContextLabel: src.ContextLabel,
				}
				return
			}

			// Expand {{profile.X}} references in params at execution time.
			params := src.Params
			if e.profile != nil && len(params) > 0 {
				expanded, expandErr := profile.ExpandParams(params, e.profile)
				if expandErr != nil {
					fmt.Fprintf(os.Stderr, "warning: profile expansion in %s/%s params: %v\n", src.Service, src.Tool, expandErr)
				}
				params = expanded
			}

			result, err := svc.Execute(ctx, src.Tool, params)
			if err != nil {
				results[idx] = &services.Result{
					Service:      src.Service,
					Tool:         src.Tool,
					Timestamp:    time.Now().UTC(),
					Error:        err.Error(),
					ContextLabel: src.ContextLabel,
				}
				e.debug.Printf("  source %d result: ERROR %v", idx, err)
				return
			}

			results[idx] = result
			results[idx].ContextLabel = src.ContextLabel

			if result.Error != "" {
				e.debug.Printf("  source %d result: FAIL (%s)", idx, result.Error)
			} else {
				e.debug.Printf("  source %d result: OK (%d bytes, url=%s)", idx, len(result.Data), result.URL)
			}

			if len(result.Data) > 0 {
				key := fmt.Sprintf("%d-%s-%s", idx, result.Service, result.Tool)
				mu.Lock()
				rawResults[key] = result.Data
				mu.Unlock()
			}
		}(i, src)
	}

	wg.Wait()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Persist raw results before synthesis (spec §4.1)
	reportDir, err := reports.Create(e.reportsDir, routine.Name, rawResults)
	if err != nil {
		return nil, fmt.Errorf("saving raw results: %w", err)
	}

	// Expand {{profile.X}} references in synthesis system prompt and report title.
	synthesisSystem := routine.Synthesis.System
	if e.profile != nil {
		if expanded, err := profile.Expand(synthesisSystem, e.profile); err != nil {
			fmt.Fprintf(os.Stderr, "warning: profile expansion in synthesis system: %v\n", err)
			synthesisSystem = expanded // partial expansion is still useful
		} else {
			synthesisSystem = expanded
		}
	}
	reportTitle := routine.Report.Title
	if e.profile != nil {
		if expanded, err := profile.Expand(reportTitle, e.profile); err != nil {
			fmt.Fprintf(os.Stderr, "warning: profile expansion in report title: %v\n", err)
			reportTitle = expanded
		} else {
			reportTitle = expanded
		}
	}

	// Inject comparison context if compare_with is set (spec §5.3).
	if routine.Report.CompareWith != "" {
		prevReport, findErr := reports.FindLatest(e.reportsDir, routine.Report.CompareWith)
		if findErr != nil {
			fmt.Fprintf(os.Stderr, "warning: compare_with %q: %v\n", routine.Report.CompareWith, findErr)
		} else if prevReport != nil {
			synthesisSystem = synthesisSystem + "\n\n" + buildComparisonContext(prevReport)
		}
		// If prevReport is nil (no previous report exists), skip silently — first run.
	}

	// Inject chart generation instructions if enabled (spec §4.5).
	if routine.Report.ChartsEnabled() {
		synthesisSystem = synthesisSystem + "\n\n" + chartInstructions
	}

	// Synthesize
	markdown, err := e.synthesizer.Synthesize(ctx, reportTitle, synthesisSystem, results)
	if err != nil {
		return nil, fmt.Errorf("synthesis failed: %w", err)
	}

	// Generate chart PNGs if enabled
	if routine.Report.ChartsEnabled() {
		directives := charts.ParseDirectives(markdown)
		if len(directives) > 0 {
			chartsDir := filepath.Join(reportDir, "charts")
			if mkErr := os.MkdirAll(chartsDir, 0o755); mkErr != nil {
				fmt.Fprintf(os.Stderr, "warning: creating charts dir: %v\n", mkErr)
			} else {
				for i, d := range directives {
					w, h := 800, 400
					if d.Type == "pie" {
						w = 600
					}
					png, renderErr := charts.RenderPNG(d, w, h)
					if renderErr != nil {
						fmt.Fprintf(os.Stderr, "warning: chart %q: %v\n", d.Title, renderErr)
						continue
					}
					name := slug.Sanitize(d.Title)
					if name == "chart" {
						name = fmt.Sprintf("chart-%d", i)
					}
					if writeErr := os.WriteFile(filepath.Join(chartsDir, name+".png"), png, 0o644); writeErr != nil {
						fmt.Fprintf(os.Stderr, "warning: writing chart %q: %v\n", name, writeErr)
					}
				}
			}
		}
	}

	// Write synthesized report
	report, err := reports.Finish(reportDir, routine.Name, markdown)
	if err != nil {
		return nil, fmt.Errorf("saving report: %w", err)
	}

	// Index in context ledger (best-effort)
	if e.ledger != nil {
		e.indexContext(routine, report, results)
	}

	return report, nil
}

// SourceStatus holds the result of testing a single source's connectivity.
type SourceStatus struct {
	Service string
	Tool    string
	URL     string
	OK      bool
	Error   string
	Latency time.Duration
}

// TestSources checks connectivity for each source in a routine.
// Sources are tested sequentially with no jitter, synthesis, or persistence.
func (e *Executor) TestSources(ctx context.Context, routine *Routine) []SourceStatus {
	statuses := make([]SourceStatus, len(routine.Sources))

	for i, src := range routine.Sources {
		status := SourceStatus{
			Service: src.Service,
			Tool:    src.Tool,
		}

		svc, err := e.registry.Get(src.Service)
		if err != nil {
			status.Error = fmt.Sprintf("service not found: %v", err)
			statuses[i] = status
			continue
		}

		// Expand {{profile.X}} references in params.
		params := src.Params
		if e.profile != nil && len(params) > 0 {
			expanded, expandErr := profile.ExpandParams(params, e.profile)
			if expandErr != nil {
				fmt.Fprintf(os.Stderr, "warning: profile expansion in %s/%s params: %v\n", src.Service, src.Tool, expandErr)
			}
			params = expanded
		}

		start := time.Now()
		result, err := svc.Execute(ctx, src.Tool, params)
		status.Latency = time.Since(start)

		if err != nil {
			status.Error = err.Error()
		} else if result != nil {
			status.URL = result.URL
			if result.Error != "" {
				status.Error = result.Error
			} else {
				status.OK = true
			}
		}

		statuses[i] = status
	}

	return statuses
}

// indexContext writes report and raw results to the context ledger.
func (e *Executor) indexContext(routine *Routine, report *reports.Report, results []*services.Result) {
	now := time.Now().UTC()

	// Index the report
	reportEntry := bcontext.Entry{
		Type:      bcontext.TypeReport,
		Label:     routine.Report.Title,
		Routine:   routine.Name,
		Timestamp: now,
		Content:   report.Markdown,
	}
	if err := e.ledger.Append(reportEntry); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to index report in context: %v\n", err)
	}

	// Index raw results
	for _, r := range results {
		if r.Error != "" || len(r.Data) == 0 {
			continue
		}
		label := r.Service + " — " + r.Tool
		entry := bcontext.Entry{
			Type:      bcontext.TypeResult,
			Label:     label,
			Routine:   routine.Name,
			Timestamp: now,
			Content:   string(r.Data),
		}
		if err := e.ledger.Append(entry); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to index result %q in context: %v\n", label, err)
		}
	}
}

const chartInstructions = `Data visualization: When source data contains numerical comparisons, trends over time, ` +
	`or proportional breakdowns, include chart directives in fenced code blocks. Format:` + "\n\n" +
	"```chart\n" +
	"type: bar\n" +
	`title: "Descriptive Title"` + "\n" +
	`x: ["Label1", "Label2", "Label3"]` + "\n" +
	`y: [10, 20, 30]` + "\n" +
	"```\n\n" +
	`Supported types: bar (comparisons), line (trends over time), pie (proportional breakdowns). ` +
	`Use "labels" and "values" as alternative keys for pie charts. ` +
	`Only include charts when the data clearly supports visualization — do not force charts on qualitative summaries.`

const maxCompareRunes = 50_000

// buildComparisonContext formats a previous report for injection into the synthesis prompt.
func buildComparisonContext(prev *reports.Report) string {
	content := prev.Markdown
	runes := []rune(content)
	if len(runes) > maxCompareRunes {
		content = string(runes[:maxCompareRunes]) + "\n\n[... truncated ...]\n"
	}
	return fmt.Sprintf(`## Previous Report for Comparison

The following is the most recent report from %q (%s). Focus your analysis on what has CHANGED since this report — new items, updates, removals, and emerging trends. Do not simply repeat information from the previous report.

---
%s
---`, prev.Routine, prev.Date, content)
}
