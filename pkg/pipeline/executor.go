package pipeline

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"time"

	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/jcadam/burrow/pkg/reports"
	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/synthesis"
)

// Executor runs routines by querying services and producing reports.
type Executor struct {
	registry    *services.Registry
	synthesizer synthesis.Synthesizer
	reportsDir  string
	ledger      *bcontext.Ledger
	randFunc    func(max int) int
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

// SetRandFunc replaces the random function (for testing jitter).
func (e *Executor) SetRandFunc(f func(max int) int) {
	e.randFunc = f
}

// Run executes a routine: queries all sources in parallel with jitter,
// synthesizes results, saves report, and indexes in context ledger.
func (e *Executor) Run(ctx context.Context, routine *Routine) (*reports.Report, error) {
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
						Service:   src.Service,
						Tool:      src.Tool,
						Timestamp: time.Now().UTC(),
						Error:     fmt.Sprintf("panic: %v", r),
					}
				}
			}()

			// Apply jitter before executing
			if routine.Jitter > 0 {
				jitterSecs := e.randFunc(routine.Jitter)
				if jitterSecs > 0 {
					timer := time.NewTimer(time.Duration(jitterSecs) * time.Second)
					select {
					case <-ctx.Done():
						timer.Stop()
						results[idx] = &services.Result{
							Service:   src.Service,
							Tool:      src.Tool,
							Timestamp: time.Now().UTC(),
							Error:     ctx.Err().Error(),
						}
						return
					case <-timer.C:
					}
				}
			}

			svc, err := e.registry.Get(src.Service)
			if err != nil {
				results[idx] = &services.Result{
					Service:   src.Service,
					Tool:      src.Tool,
					Timestamp: time.Now().UTC(),
					Error:     fmt.Sprintf("service not found: %v", err),
				}
				return
			}

			result, err := svc.Execute(ctx, src.Tool, src.Params)
			if err != nil {
				results[idx] = &services.Result{
					Service:   src.Service,
					Tool:      src.Tool,
					Timestamp: time.Now().UTC(),
					Error:     err.Error(),
				}
				return
			}

			results[idx] = result

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

	// Synthesize
	markdown, err := e.synthesizer.Synthesize(ctx, routine.Report.Title, routine.Synthesis.System, results)
	if err != nil {
		return nil, fmt.Errorf("synthesis failed: %w", err)
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
