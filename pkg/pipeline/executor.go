package pipeline

import (
	"context"
	"fmt"

	"github.com/jcadam/burrow/pkg/reports"
	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/synthesis"
)

// Executor runs routines by querying services and producing reports.
type Executor struct {
	registry    *services.Registry
	synthesizer synthesis.Synthesizer
	reportsDir  string
}

// NewExecutor creates an executor with the given dependencies.
func NewExecutor(registry *services.Registry, synthesizer synthesis.Synthesizer, reportsDir string) *Executor {
	return &Executor{
		registry:    registry,
		synthesizer: synthesizer,
		reportsDir:  reportsDir,
	}
}

// Run executes a routine: queries all sources, synthesizes results, saves report.
func (e *Executor) Run(ctx context.Context, routine *Routine) (*reports.Report, error) {
	// Phase 1: sequential execution (parallel with jitter is Phase 2)
	var results []*services.Result
	rawResults := make(map[string][]byte)

	for _, src := range routine.Sources {
		svc, err := e.registry.Get(src.Service)
		if err != nil {
			results = append(results, &services.Result{
				Service: src.Service,
				Tool:    src.Tool,
				Error:   fmt.Sprintf("service not found: %v", err),
			})
			continue
		}

		result, err := svc.Execute(ctx, src.Tool, src.Params)
		if err != nil {
			results = append(results, &services.Result{
				Service: src.Service,
				Tool:    src.Tool,
				Error:   err.Error(),
			})
			continue
		}

		results = append(results, result)

		// Store raw data for archival
		if len(result.Data) > 0 {
			key := result.Service + "-" + result.Tool
			rawResults[key] = result.Data
		}
	}

	// Synthesize
	markdown, err := e.synthesizer.Synthesize(ctx, routine.Report.Title, routine.Synthesis.System, results)
	if err != nil {
		return nil, fmt.Errorf("synthesis failed: %w", err)
	}

	// Save report
	report, err := reports.Save(e.reportsDir, routine.Name, markdown, rawResults)
	if err != nil {
		return nil, fmt.Errorf("saving report: %w", err)
	}

	return report, nil
}
