// Package pipeline handles routine loading and execution.
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Routine defines a scheduled data-collection-and-synthesis job.
type Routine struct {
	Name     string         `yaml:"-"` // derived from filename
	Schedule string         `yaml:"schedule,omitempty"`
	Timezone string         `yaml:"timezone,omitempty"`
	Jitter   int            `yaml:"jitter,omitempty"`
	LLM      string         `yaml:"llm,omitempty"`
	Report   ReportConfig   `yaml:"report"`
	Synthesis SynthesisConfig `yaml:"synthesis,omitempty"`
	Sources  []SourceConfig `yaml:"sources"`
}

// ReportConfig controls report generation.
type ReportConfig struct {
	Title          string `yaml:"title"`
	Style          string `yaml:"style,omitempty"`
	GenerateCharts bool   `yaml:"generate_charts,omitempty"`
	MaxLength      int    `yaml:"max_length,omitempty"`
	CompareWith    string `yaml:"compare_with,omitempty"`
}

// SynthesisConfig holds the LLM system prompt for synthesis.
type SynthesisConfig struct {
	System string `yaml:"system,omitempty"`
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

	if err := validateRoutine(&r); err != nil {
		return nil, fmt.Errorf("validating routine %q: %w", r.Name, err)
	}

	return &r, nil
}

// LoadAllRoutines loads all .yaml files from a directory.
func LoadAllRoutines(dir string) ([]*Routine, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing routines: %w", err)
	}

	var routines []*Routine
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		r, err := LoadRoutine(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		routines = append(routines, r)
	}

	return routines, nil
}

func validateRoutine(r *Routine) error {
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
	return nil
}
