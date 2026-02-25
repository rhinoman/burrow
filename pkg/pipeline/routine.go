// Package pipeline handles routine loading and execution.
package pipeline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Routine defines a scheduled data-collection-and-synthesis job.
type Routine struct {
	Name      string          `yaml:"-"` // derived from filename
	Schedule  string          `yaml:"schedule,omitempty"`
	Timezone  string          `yaml:"timezone,omitempty"`
	Jitter    int             `yaml:"jitter,omitempty"`
	LLM       string          `yaml:"llm,omitempty"`
	Report    ReportConfig    `yaml:"report"`
	Synthesis SynthesisConfig `yaml:"synthesis,omitempty"`
	Sources   []SourceConfig  `yaml:"sources"`
}

// ReportConfig controls report generation.
type ReportConfig struct {
	Title          string `yaml:"title"`
	Style          string `yaml:"style,omitempty"`
	GenerateCharts *bool  `yaml:"generate_charts,omitempty"`
	MaxLength      int    `yaml:"max_length,omitempty"`
	CompareWith    string `yaml:"compare_with,omitempty"` // Routine name to compare with for longitudinal analysis
}

// ChartsEnabled returns whether chart generation is enabled.
// Charts are enabled by default (nil = true). Only an explicit false disables them.
func (rc ReportConfig) ChartsEnabled() bool {
	return rc.GenerateCharts == nil || *rc.GenerateCharts
}

// SynthesisConfig holds the LLM system prompt for synthesis.
type SynthesisConfig struct {
	System          string `yaml:"system,omitempty"`
	Strategy        string `yaml:"strategy,omitempty"`           // auto | single | multi-stage
	SummaryMaxWords int    `yaml:"summary_max_words,omitempty"`  // target words per summary (default: 500)
	MaxSourceWords  int    `yaml:"max_source_words,omitempty"`   // max words per source before chunking (default: 10000)
	Concurrency     int    `yaml:"concurrency,omitempty"`        // max concurrent stage 1 LLM calls (default: 1)
	Preprocess      *bool  `yaml:"preprocess,omitempty"`         // nil=auto (local), true=always, false=never
}

// SourceConfig defines a single data source within a routine.
type SourceConfig struct {
	Service      string            `yaml:"service"`
	Tool         string            `yaml:"tool"`
	Params       map[string]string `yaml:"params"`
	ContextLabel string            `yaml:"context_label,omitempty"`
}

// LoadRoutine reads and parses a single routine YAML file.
func LoadRoutine(path string) (*Routine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading routine: %w", err)
	}

	var r Routine
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing routine: %w", err)
	}

	// Derive name from filename without extension
	base := filepath.Base(path)
	r.Name = strings.TrimSuffix(base, filepath.Ext(base))

	if err := ValidateRoutine(&r); err != nil {
		return nil, fmt.Errorf("validating routine %q: %w", r.Name, err)
	}

	return &r, nil
}

// LoadAllRoutines loads all .yaml files from a directory.
// Invalid routine files are skipped with a warning to warnWriter (if non-nil).
// Use nil for warnWriter to discard warnings.
func LoadAllRoutines(dir string, warnWriter ...io.Writer) ([]*Routine, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing routines: %w", err)
	}

	var w io.Writer
	if len(warnWriter) > 0 && warnWriter[0] != nil {
		w = warnWriter[0]
	}

	var routines []*Routine
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		r, err := LoadRoutine(filepath.Join(dir, e.Name()))
		if err != nil {
			if w != nil {
				fmt.Fprintf(w, "warning: skipping %s: %v\n", e.Name(), err)
			}
			continue
		}
		routines = append(routines, r)
	}

	return routines, nil
}

// SaveRoutine marshals a routine to YAML and writes it to the routines directory.
// The Name field is excluded (yaml:"-") since it's derived from the filename.
func SaveRoutine(routinesDir string, r *Routine) error {
	if err := os.MkdirAll(routinesDir, 0o755); err != nil {
		return fmt.Errorf("creating routines directory: %w", err)
	}

	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshaling routine: %w", err)
	}

	path := filepath.Join(routinesDir, r.Name+".yaml")
	return os.WriteFile(path, data, 0o644)
}

// ValidateRoutine checks that a routine has the required fields.
func ValidateRoutine(r *Routine) error {
	if r.Report.Title == "" {
		return fmt.Errorf("missing report.title")
	}
	if len(r.Sources) == 0 {
		return fmt.Errorf("no sources defined")
	}
	for i, s := range r.Sources {
		if s.Service == "" {
			return fmt.Errorf("source[%d] missing service", i)
		}
		if s.Tool == "" {
			return fmt.Errorf("source[%d] missing tool", i)
		}
	}
	if r.Synthesis.Strategy != "" {
		validStrategies := map[string]bool{"auto": true, "single": true, "multi-stage": true}
		if !validStrategies[r.Synthesis.Strategy] {
			return fmt.Errorf("invalid strategy %q (must be auto, single, or multi-stage)", r.Synthesis.Strategy)
		}
	}
	return nil
}
