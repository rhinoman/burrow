package pipeline

import (
	"os"
	"path/filepath"
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
