package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRoutine = `
schedule: "05:00"
timezone: "America/Anchorage"
jitter: 300
llm: local/qwen-14b

report:
  title: "Market Intelligence Brief"
  style: executive_summary
  generate_charts: true
  max_length: 2000

synthesis:
  system: |
    You are a business development analyst writing a daily brief.

sources:
  - service: sam-gov
    tool: search_opportunities
    params:
      naics: "541370"
      status: "active"
    context_label: "SAM.gov Postings"

  - service: edgar
    tool: company_filings
    params:
      keywords: "geospatial"
    context_label: "SEC Filings"
`

func TestLoadRoutine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "morning-intel.yaml")
	if err := os.WriteFile(path, []byte(testRoutine), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := LoadRoutine(path)
	if err != nil {
		t.Fatalf("LoadRoutine: %v", err)
	}

	if r.Name != "morning-intel" {
		t.Errorf("expected name morning-intel, got %q", r.Name)
	}
	if r.Schedule != "05:00" {
		t.Errorf("expected schedule 05:00, got %q", r.Schedule)
	}
	if r.Report.Title != "Market Intelligence Brief" {
		t.Errorf("expected title, got %q", r.Report.Title)
	}
	if len(r.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(r.Sources))
	}
	if r.Sources[0].Service != "sam-gov" {
		t.Errorf("expected sam-gov, got %q", r.Sources[0].Service)
	}
	if r.Sources[0].Params["naics"] != "541370" {
		t.Errorf("expected naics 541370, got %q", r.Sources[0].Params["naics"])
	}
	if r.Report.GenerateCharts == nil || *r.Report.GenerateCharts != true {
		t.Errorf("expected generate_charts to be parsed as *true, got %v", r.Report.GenerateCharts)
	}
}

func TestChartsEnabledDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-charts-field.yaml")
	content := `
report:
  title: "Test"
sources:
  - service: test
    tool: fetch
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := LoadRoutine(path)
	if err != nil {
		t.Fatalf("LoadRoutine: %v", err)
	}

	if r.Report.GenerateCharts != nil {
		t.Errorf("expected GenerateCharts to be nil when omitted, got %v", *r.Report.GenerateCharts)
	}
	if !r.Report.ChartsEnabled() {
		t.Error("expected ChartsEnabled() to return true when GenerateCharts is nil")
	}
}

func TestLoadRoutineMissingTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	content := `
report:
  style: executive_summary
sources:
  - service: test
    tool: fetch
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRoutine(path)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestLoadRoutineNoSources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	content := `
report:
  title: "Test"
sources: []
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRoutine(path)
	if err == nil {
		t.Fatal("expected error for no sources")
	}
}

func TestLoadAllRoutines(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"alpha.yaml", "beta.yml", "readme.txt"} {
		content := testRoutine
		if name == "readme.txt" {
			content = "not a routine"
		}
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	}

	routines, err := LoadAllRoutines(dir)
	if err != nil {
		t.Fatalf("LoadAllRoutines: %v", err)
	}
	if len(routines) != 2 {
		t.Errorf("expected 2 routines (skipping .txt), got %d", len(routines))
	}
}

func TestSaveRoutineRoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := &Routine{
		Name:     "test-routine",
		Schedule: "06:00",
		Timezone: "America/New_York",
		Jitter:   120,
		LLM:      "local/test",
		Report: ReportConfig{
			Title: "Test Report",
			Style: "executive_summary",
		},
		Synthesis: SynthesisConfig{
			System: "You are a test analyst.",
		},
		Sources: []SourceConfig{
			{
				Service:      "test-svc",
				Tool:         "search",
				Params:       map[string]string{"q": "test"},
				ContextLabel: "Test Results",
			},
		},
	}

	if err := SaveRoutine(dir, original); err != nil {
		t.Fatalf("SaveRoutine: %v", err)
	}

	loaded, err := LoadRoutine(filepath.Join(dir, "test-routine.yaml"))
	if err != nil {
		t.Fatalf("LoadRoutine after Save: %v", err)
	}

	if loaded.Name != "test-routine" {
		t.Errorf("name: got %q, want test-routine", loaded.Name)
	}
	if loaded.Schedule != "06:00" {
		t.Errorf("schedule: got %q, want 06:00", loaded.Schedule)
	}
	if loaded.Report.Title != "Test Report" {
		t.Errorf("title: got %q, want Test Report", loaded.Report.Title)
	}
	if len(loaded.Sources) != 1 || loaded.Sources[0].Service != "test-svc" {
		t.Errorf("sources round-trip failed: %+v", loaded.Sources)
	}
}

func TestSaveRoutineCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "routines")

	r := &Routine{
		Name:   "test",
		Report: ReportConfig{Title: "T"},
		Sources: []SourceConfig{
			{Service: "s", Tool: "t"},
		},
	}
	if err := SaveRoutine(dir, r); err != nil {
		t.Fatalf("SaveRoutine: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "test.yaml")); os.IsNotExist(err) {
		t.Error("routine file not created")
	}
}

func TestSaveRoutineExcludesName(t *testing.T) {
	dir := t.TempDir()

	r := &Routine{
		Name:   "my-routine",
		Report: ReportConfig{Title: "T"},
		Sources: []SourceConfig{
			{Service: "s", Tool: "t"},
		},
	}
	if err := SaveRoutine(dir, r); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "my-routine.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// Name field has yaml:"-" so it should not appear
	if strings.Contains(string(data), "name: my-routine") {
		t.Error("Name field should be excluded from YAML (yaml:\"-\")")
	}
}

func TestLoadAllRoutinesEmpty(t *testing.T) {
	dir := t.TempDir()
	routines, err := LoadAllRoutines(dir)
	if err != nil {
		t.Fatalf("LoadAllRoutines: %v", err)
	}
	if len(routines) != 0 {
		t.Errorf("expected 0 routines, got %d", len(routines))
	}
}

func TestValidateRoutineStrategyValid(t *testing.T) {
	for _, strategy := range []string{"auto", "single", "multi-stage", ""} {
		r := &Routine{
			Report:    ReportConfig{Title: "T"},
			Synthesis: SynthesisConfig{Strategy: strategy},
			Sources:   []SourceConfig{{Service: "s", Tool: "t"}},
		}
		if err := ValidateRoutine(r); err != nil {
			t.Errorf("strategy %q should be valid, got: %v", strategy, err)
		}
	}
}

func TestValidateRoutineStrategyInvalid(t *testing.T) {
	r := &Routine{
		Report:    ReportConfig{Title: "T"},
		Synthesis: SynthesisConfig{Strategy: "invalid"},
		Sources:   []SourceConfig{{Service: "s", Tool: "t"}},
	}
	err := ValidateRoutine(r)
	if err == nil {
		t.Fatal("expected error for invalid strategy")
	}
	if !strings.Contains(err.Error(), "invalid strategy") {
		t.Errorf("expected strategy error, got: %v", err)
	}
}

func TestLoadRoutineWithSynthesisStrategy(t *testing.T) {
	dir := t.TempDir()
	content := `
report:
  title: "Test"
synthesis:
  system: "Be concise."
  strategy: multi-stage
  summary_max_words: 300
sources:
  - service: test
    tool: fetch
`
	path := filepath.Join(dir, "strat.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := LoadRoutine(path)
	if err != nil {
		t.Fatalf("LoadRoutine: %v", err)
	}

	if r.Synthesis.Strategy != "multi-stage" {
		t.Errorf("expected strategy multi-stage, got %q", r.Synthesis.Strategy)
	}
	if r.Synthesis.SummaryMaxWords != 300 {
		t.Errorf("expected summary_max_words 300, got %d", r.Synthesis.SummaryMaxWords)
	}
}

func TestLoadRoutineWithMaxSourceWords(t *testing.T) {
	dir := t.TempDir()
	content := `
report:
  title: "Test"
synthesis:
  system: "Be concise."
  strategy: multi-stage
  summary_max_words: 300
  max_source_words: 8000
sources:
  - service: test
    tool: fetch
`
	path := filepath.Join(dir, "chunked.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := LoadRoutine(path)
	if err != nil {
		t.Fatalf("LoadRoutine: %v", err)
	}

	if r.Synthesis.MaxSourceWords != 8000 {
		t.Errorf("expected max_source_words 8000, got %d", r.Synthesis.MaxSourceWords)
	}
	if r.Synthesis.SummaryMaxWords != 300 {
		t.Errorf("expected summary_max_words 300, got %d", r.Synthesis.SummaryMaxWords)
	}
}

func TestLoadAllRoutinesSkipsBadFiles(t *testing.T) {
	dir := t.TempDir()

	// Write a valid routine
	os.WriteFile(filepath.Join(dir, "good.yaml"), []byte(testRoutine), 0o644)

	// Write an invalid routine (missing title and sources)
	os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("junk: true\n"), 0o644)

	var warnings strings.Builder
	routines, err := LoadAllRoutines(dir, &warnings)
	if err != nil {
		t.Fatalf("LoadAllRoutines: %v", err)
	}
	if len(routines) != 1 {
		t.Errorf("expected 1 valid routine, got %d", len(routines))
	}
	if routines[0].Name != "good" {
		t.Errorf("expected good routine, got %q", routines[0].Name)
	}
	if !strings.Contains(warnings.String(), "skipping bad.yaml") {
		t.Errorf("expected warning about bad.yaml, got: %q", warnings.String())
	}
}
