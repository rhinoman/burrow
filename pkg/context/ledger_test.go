package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jcadam/burrow/pkg/config"
)

func TestAppendAndSearch(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	ts := time.Date(2026, 2, 19, 5, 0, 0, 0, time.UTC)
	err = l.Append(Entry{
		Type:      TypeReport,
		Label:     "Morning Intel Brief",
		Routine:   "morning-intel",
		Timestamp: ts,
		Content:   "# Morning Intel Brief\n\nGeospatial analysis contract found.",
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Search for content
	results, err := l.Search("geospatial")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Label != "Morning Intel Brief" {
		t.Errorf("expected label, got %q", results[0].Label)
	}
	if results[0].Routine != "morning-intel" {
		t.Errorf("expected routine, got %q", results[0].Routine)
	}
	if !results[0].Timestamp.Equal(ts) {
		t.Errorf("expected timestamp %v, got %v", ts, results[0].Timestamp)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	l.Append(Entry{
		Type:      TypeResult,
		Label:     "Test Result",
		Timestamp: time.Now().UTC(),
		Content:   "Found IMPORTANT data here.",
	})

	results, err := l.Search("important")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for case-insensitive search, got %d", len(results))
	}
}

func TestSearchEmpty(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	results, err := l.Search("nothing")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestListByType(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	ts := time.Now().UTC()
	l.Append(Entry{Type: TypeReport, Label: "Report 1", Timestamp: ts.Add(-time.Hour), Content: "first"})
	l.Append(Entry{Type: TypeReport, Label: "Report 2", Timestamp: ts, Content: "second"})
	l.Append(Entry{Type: TypeResult, Label: "Result 1", Timestamp: ts, Content: "result"})

	reports, err := l.List(TypeReport, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	// Newest first
	if reports[0].Label != "Report 2" {
		t.Errorf("expected Report 2 first (newest), got %q", reports[0].Label)
	}

	// Limit
	limited, err := l.List(TypeReport, 1)
	if err != nil {
		t.Fatalf("List(limit=1): %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1 report with limit, got %d", len(limited))
	}
}

func TestGatherContext(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	ts := time.Now().UTC()
	l.Append(Entry{Type: TypeReport, Label: "Report", Timestamp: ts, Content: strings.Repeat("x", 100)})
	l.Append(Entry{Type: TypeResult, Label: "Result", Timestamp: ts.Add(-time.Minute), Content: strings.Repeat("y", 100)})

	// Gather with enough space for both
	ctx, err := l.GatherContext(10000)
	if err != nil {
		t.Fatalf("GatherContext: %v", err)
	}
	if !strings.Contains(ctx, "Report") || !strings.Contains(ctx, "Result") {
		t.Error("expected both entries in gathered context")
	}

	// Gather with limited space — should only include newest
	ctx, err = l.GatherContext(200)
	if err != nil {
		t.Fatalf("GatherContext: %v", err)
	}
	if !strings.Contains(ctx, "Report") {
		t.Error("expected newest entry (Report) in limited context")
	}
}

func TestFileFormat(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	ts := time.Date(2026, 2, 19, 5, 0, 0, 0, time.UTC)
	l.Append(Entry{
		Type:      TypeReport,
		Label:     "Morning Intel",
		Routine:   "morning-intel",
		Timestamp: ts,
		Content:   "Report content here.",
	})

	// Verify file exists and format
	files, err := os.ReadDir(filepath.Join(dir, "reports"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	data, err := os.ReadFile(filepath.Join(dir, "reports", files[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, "---\n") {
		t.Error("expected YAML front matter start")
	}
	if !strings.Contains(content, "type: report") {
		t.Error("expected type in front matter")
	}
	if !strings.Contains(content, `label: "Morning Intel"`) {
		t.Error("expected label in front matter")
	}
	if !strings.Contains(content, "routine: morning-intel") {
		t.Error("expected routine in front matter")
	}
	if !strings.Contains(content, "Report content here.") {
		t.Error("expected content body")
	}
}

func TestAppendFilenameCollision(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	ts := time.Date(2026, 2, 19, 5, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		err := l.Append(Entry{
			Type:      TypeResult,
			Label:     "Same Label",
			Timestamp: ts,
			Content:   fmt.Sprintf("content %d", i),
		})
		if err != nil {
			t.Fatalf("Append(%d): %v", i, err)
		}
	}

	// All 3 files should exist
	files, err := os.ReadDir(filepath.Join(dir, "results"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	// All 3 should be searchable
	results, err := l.Search("content")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 search results, got %d", len(results))
	}
}

func TestParseEntryMissingClosingFrontMatter(t *testing.T) {
	// Simulate a corrupted file with opening --- but no closing ---
	raw := "---\ntype: result\nlabel: \"broken\"\nSome content without closing delimiter"
	entry := parseEntry(raw, "test.md", "results")

	if entry.Content == "" {
		t.Error("expected content to be preserved from malformed front matter")
	}
	if !strings.Contains(entry.Content, "Some content") {
		t.Errorf("expected raw content preserved, got %q", entry.Content)
	}
}

func TestStats(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	// Empty ledger
	stats, err := l.Stats()
	if err != nil {
		t.Fatalf("Stats (empty): %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected empty stats, got %d types", len(stats))
	}

	ts1 := time.Date(2026, 2, 17, 5, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 2, 19, 8, 0, 0, 0, time.UTC)

	l.Append(Entry{Type: TypeReport, Label: "Report A", Timestamp: ts1, Content: "content a"})
	l.Append(Entry{Type: TypeReport, Label: "Report B", Timestamp: ts2, Content: "content b"})
	l.Append(Entry{Type: TypeResult, Label: "Result A", Timestamp: ts1, Content: "result content"})

	stats, err = l.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 types, got %d", len(stats))
	}

	rs, ok := stats[TypeReport]
	if !ok {
		t.Fatal("expected report stats")
	}
	if rs.Count != 2 {
		t.Errorf("expected 2 reports, got %d", rs.Count)
	}
	if !rs.Earliest.Equal(ts1) {
		t.Errorf("expected earliest %v, got %v", ts1, rs.Earliest)
	}
	if !rs.Latest.Equal(ts2) {
		t.Errorf("expected latest %v, got %v", ts2, rs.Latest)
	}
	if rs.Bytes <= 0 {
		t.Error("expected positive byte count")
	}

	res, ok := stats[TypeResult]
	if !ok {
		t.Fatal("expected result stats")
	}
	if res.Count != 1 {
		t.Errorf("expected 1 result, got %d", res.Count)
	}
}

func TestDirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	_, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	for _, sub := range []string{"reports", "results", "sessions", "contacts", "notes"} {
		if _, err := os.Stat(filepath.Join(dir, sub)); os.IsNotExist(err) {
			t.Errorf("expected %s directory to exist", sub)
		}
	}
}

func TestPruneExpiredSessions(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	// Create an old session (100 days ago) and a recent one (1 day ago).
	oldTS := time.Now().UTC().AddDate(0, 0, -100)
	newTS := time.Now().UTC().AddDate(0, 0, -1)

	l.Append(Entry{Type: TypeSession, Label: "Old Session", Timestamp: oldTS, Content: "old"})
	l.Append(Entry{Type: TypeSession, Label: "New Session", Timestamp: newTS, Content: "new"})

	retention := config.RetentionConfig{Sessions: 30, RawResults: 90, Reports: "forever"}
	pruned, err := l.PruneExpired(retention, time.Now())
	if err != nil {
		t.Fatalf("PruneExpired: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}

	// Only the new session should remain.
	entries, err := l.List(TypeSession, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 remaining session, got %d", len(entries))
	}
	if entries[0].Label != "New Session" {
		t.Errorf("expected 'New Session', got %q", entries[0].Label)
	}
}

func TestPruneExpiredRawResults(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	oldTS := time.Now().UTC().AddDate(0, 0, -100)
	l.Append(Entry{Type: TypeResult, Label: "Old Result", Timestamp: oldTS, Content: "old data"})
	l.Append(Entry{Type: TypeResult, Label: "Recent Result", Timestamp: time.Now().UTC(), Content: "recent"})

	retention := config.RetentionConfig{RawResults: 90, Sessions: 30, Reports: "forever"}
	pruned, err := l.PruneExpired(retention, time.Now())
	if err != nil {
		t.Fatalf("PruneExpired: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}

	entries, err := l.List(TypeResult, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 remaining result, got %d", len(entries))
	}
}

func TestPruneExpiredReportsForever(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	oldTS := time.Now().UTC().AddDate(0, 0, -365)
	l.Append(Entry{Type: TypeReport, Label: "Ancient Report", Timestamp: oldTS, Content: "ancient"})

	retention := config.RetentionConfig{Reports: "forever", RawResults: 90, Sessions: 30}
	pruned, err := l.PruneExpired(retention, time.Now())
	if err != nil {
		t.Fatalf("PruneExpired: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 pruned (reports=forever), got %d", pruned)
	}

	entries, err := l.List(TypeReport, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 report preserved, got %d", len(entries))
	}
}

func TestPruneExpiredSkipsContacts(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	oldTS := time.Now().UTC().AddDate(0, 0, -365)
	l.Append(Entry{Type: TypeContact, Label: "Old Contact", Timestamp: oldTS, Content: "contact info"})

	retention := config.RetentionConfig{RawResults: 1, Sessions: 1, Reports: "forever"}
	pruned, err := l.PruneExpired(retention, time.Now())
	if err != nil {
		t.Fatalf("PruneExpired: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 pruned (contacts are never pruned), got %d", pruned)
	}
}

func TestPruneExpiredSkipsNotes(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	oldTS := time.Now().UTC().AddDate(0, 0, -365)
	l.Append(Entry{Type: TypeNote, Label: "Old Note", Timestamp: oldTS, Content: "note content"})

	retention := config.RetentionConfig{RawResults: 1, Sessions: 1, Reports: "forever"}
	pruned, err := l.PruneExpired(retention, time.Now())
	if err != nil {
		t.Fatalf("PruneExpired: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 pruned (notes are never pruned), got %d", pruned)
	}

	entries, err := l.List(TypeNote, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 note preserved, got %d", len(entries))
	}
}

func TestPruneExpiredZeroDays(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	oldTS := time.Now().UTC().AddDate(0, 0, -365)
	l.Append(Entry{Type: TypeResult, Label: "Result", Timestamp: oldTS, Content: "old"})
	l.Append(Entry{Type: TypeSession, Label: "Session", Timestamp: oldTS, Content: "old"})

	// Zero means no pruning for that type.
	retention := config.RetentionConfig{RawResults: 0, Sessions: 0, Reports: "forever"}
	pruned, err := l.PruneExpired(retention, time.Now())
	if err != nil {
		t.Fatalf("PruneExpired: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 pruned (0 days = no pruning), got %d", pruned)
	}
}

func TestPruneExpiredNoConfig(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	oldTS := time.Now().UTC().AddDate(0, 0, -365)
	l.Append(Entry{Type: TypeResult, Label: "Result", Timestamp: oldTS, Content: "old"})

	// Empty retention config — all zeros/empty = no pruning.
	retention := config.RetentionConfig{}
	pruned, err := l.PruneExpired(retention, time.Now())
	if err != nil {
		t.Fatalf("PruneExpired: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 pruned (empty config = no pruning), got %d", pruned)
	}
}

func TestPruneExpiredBoundary(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	// Create an entry at a known timestamp.
	entryTS := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l.Append(Entry{Type: TypeResult, Label: "Boundary", Timestamp: entryTS, Content: "data"})

	retention := config.RetentionConfig{RawResults: 30}

	// At 29 days after: entry is 29 days old, retention is 30 — should NOT be pruned.
	now29 := entryTS.AddDate(0, 0, 29)
	pruned, err := l.PruneExpired(retention, now29)
	if err != nil {
		t.Fatalf("PruneExpired at 29 days: %v", err)
	}
	if pruned != 0 {
		t.Error("expected entry to survive at 29 days (within 30-day retention)")
	}

	// At 31 days after: entry is 31 days old, retention is 30 — should be pruned.
	now31 := entryTS.AddDate(0, 0, 31)
	pruned, err = l.PruneExpired(retention, now31)
	if err != nil {
		t.Fatalf("PruneExpired at 31 days: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected entry pruned at 31 days, got %d", pruned)
	}
}

func TestNoteEntry(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLedger(dir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	ts := time.Now().UTC()
	l.Append(Entry{
		Type:      TypeNote,
		Label:     "Note",
		Timestamp: ts,
		Content:   "Met with CISA team, discussed Phase 2 timeline",
	})

	// Should be searchable.
	results, err := l.Search("CISA")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Type != TypeNote {
		t.Errorf("expected type %q, got %q", TypeNote, results[0].Type)
	}

	// Should appear in GatherContext.
	ctx, err := l.GatherContext(10000)
	if err != nil {
		t.Fatalf("GatherContext: %v", err)
	}
	if !strings.Contains(ctx, "CISA") {
		t.Error("expected note content in gathered context")
	}
}
