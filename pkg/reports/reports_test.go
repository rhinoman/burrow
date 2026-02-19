package reports

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	markdown := "# Test Report\n\nSome content here.\n"
	rawResults := map[string][]byte{
		"sam-gov-search": []byte(`{"results": []}`),
		"edgar-filings":  []byte(`{"filings": []}`),
	}

	report, err := Save(dir, "morning-intel", markdown, rawResults)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if report.Routine != "morning-intel" {
		t.Errorf("expected routine morning-intel, got %q", report.Routine)
	}
	if report.Markdown != markdown {
		t.Error("markdown content mismatch")
	}
	if len(report.Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(report.Sources))
	}

	// Verify files on disk
	reportPath := filepath.Join(report.Dir, "report.md")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("reading report file: %v", err)
	}
	if string(data) != markdown {
		t.Error("on-disk content mismatch")
	}

	// Load it back
	loaded, err := Load(report.Dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Markdown != markdown {
		t.Error("loaded markdown mismatch")
	}
	if loaded.Title != "Test Report" {
		t.Errorf("expected title 'Test Report', got %q", loaded.Title)
	}
	if loaded.Routine != "morning-intel" {
		t.Errorf("expected routine morning-intel, got %q", loaded.Routine)
	}
	if len(loaded.Sources) != 2 {
		t.Errorf("expected 2 loaded sources, got %d", len(loaded.Sources))
	}
}

func TestSaveNoRawResults(t *testing.T) {
	dir := t.TempDir()

	report, err := Save(dir, "simple", "# Simple\n", nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(report.Sources) != 0 {
		t.Errorf("expected 0 sources, got %d", len(report.Sources))
	}

	// data/ dir should not exist
	dataDir := filepath.Join(report.Dir, "data")
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Error("expected no data directory when no raw results")
	}
}

func TestListOrdering(t *testing.T) {
	dir := t.TempDir()

	// Mix of new timestamp format and legacy date-only format
	for _, name := range []string{"2026-02-17T0800-alpha", "2026-02-19T1400-beta", "2026-02-18-gamma"} {
		reportDir := filepath.Join(dir, name)
		os.MkdirAll(reportDir, 0o755)
		os.WriteFile(filepath.Join(reportDir, "report.md"), []byte("# "+name+"\n"), 0o644)
	}

	reports, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(reports) != 3 {
		t.Fatalf("expected 3 reports, got %d", len(reports))
	}

	// Should be newest first
	if reports[0].Date != "2026-02-19" {
		t.Errorf("expected newest first, got %q", reports[0].Date)
	}
	if reports[2].Date != "2026-02-17" {
		t.Errorf("expected oldest last, got %q", reports[2].Date)
	}
}

func TestListSameDayOrdering(t *testing.T) {
	dir := t.TempDir()

	// Three reports on the same day at different times
	for _, name := range []string{"2026-02-19T0800-daily", "2026-02-19T1400-daily", "2026-02-19T0500-daily"} {
		reportDir := filepath.Join(dir, name)
		os.MkdirAll(reportDir, 0o755)
		os.WriteFile(filepath.Join(reportDir, "report.md"), []byte("# "+name+"\n"), 0o644)
	}

	reports, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(reports) != 3 {
		t.Fatalf("expected 3 reports, got %d", len(reports))
	}

	// Should be ordered by full timestamp, not just date
	if filepath.Base(reports[0].Dir) != "2026-02-19T1400-daily" {
		t.Errorf("expected 1400 first, got %q", filepath.Base(reports[0].Dir))
	}
	if filepath.Base(reports[2].Dir) != "2026-02-19T0500-daily" {
		t.Errorf("expected 0500 last, got %q", filepath.Base(reports[2].Dir))
	}
}

func TestSaveNoClobber(t *testing.T) {
	dir := t.TempDir()

	r1, err := Save(dir, "daily", "# Report 1\n", nil)
	if err != nil {
		t.Fatalf("Save 1: %v", err)
	}

	// Sleep 1 second to guarantee different second-precision timestamps
	// (In practice, routines never run this close together, but the test
	// should verify the guarantee.)
	time.Sleep(1 * time.Second)

	r2, err := Save(dir, "daily", "# Report 2\n", nil)
	if err != nil {
		t.Fatalf("Save 2: %v", err)
	}

	if r1.Dir == r2.Dir {
		t.Fatal("expected different directories for sequential saves")
	}

	// Both reports should be independently loadable
	loaded1, err := Load(r1.Dir)
	if err != nil {
		t.Fatalf("Load r1: %v", err)
	}
	if loaded1.Markdown != "# Report 1\n" {
		t.Errorf("r1 content mismatch: got %q", loaded1.Markdown)
	}

	loaded2, err := Load(r2.Dir)
	if err != nil {
		t.Fatalf("Load r2: %v", err)
	}
	if loaded2.Markdown != "# Report 2\n" {
		t.Errorf("r2 content mismatch: got %q", loaded2.Markdown)
	}

	all, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 reports, got %d", len(all))
	}
}

func TestListEmpty(t *testing.T) {
	dir := t.TempDir()
	reports, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected 0 reports, got %d", len(reports))
	}
}

func TestListNonexistent(t *testing.T) {
	reports, err := List("/nonexistent/path")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if reports != nil {
		t.Error("expected nil for nonexistent directory")
	}
}

func TestFindLatest(t *testing.T) {
	dir := t.TempDir()

	// Create reports with timestamp format, different dates and times
	for _, name := range []string{"2026-02-17T0800-morning-intel", "2026-02-19T0500-morning-intel", "2026-02-18T0900-other-routine"} {
		reportDir := filepath.Join(dir, name)
		os.MkdirAll(reportDir, 0o755)
		os.WriteFile(filepath.Join(reportDir, "report.md"), []byte("# Report\n"), 0o644)
	}

	report, err := FindLatest(dir, "morning-intel")
	if err != nil {
		t.Fatalf("FindLatest: %v", err)
	}
	if report == nil {
		t.Fatal("expected to find a report")
	}
	if report.Date != "2026-02-19" {
		t.Errorf("expected latest date 2026-02-19, got %q", report.Date)
	}
	if report.Routine != "morning-intel" {
		t.Errorf("expected routine morning-intel, got %q", report.Routine)
	}
}

func TestFindLatestMissing(t *testing.T) {
	dir := t.TempDir()
	report, err := FindLatest(dir, "nonexistent")
	if err != nil {
		t.Fatalf("FindLatest: %v", err)
	}
	if report != nil {
		t.Error("expected nil for missing routine")
	}
}
