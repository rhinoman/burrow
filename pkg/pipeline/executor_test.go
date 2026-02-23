package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/jcadam/burrow/pkg/reports"
	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/synthesis"
)

type mockService struct {
	name     string
	response []byte
	err      error
	delay    time.Duration
}

func (m *mockService) Name() string { return m.name }
func (m *mockService) Execute(ctx context.Context, tool string, _ map[string]string) (*services.Result, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return &services.Result{
		Service:   m.name,
		Tool:      tool,
		Data:      m.response,
		Timestamp: time.Now(),
	}, nil
}

func TestExecutorRun(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{
		name:     "test-api",
		response: []byte(`{"results": [{"title": "Finding A"}]}`),
	})

	synth := synthesis.NewPassthroughSynthesizer()
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name: "test-routine",
		Report: ReportConfig{
			Title: "Test Report",
		},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "search", Params: map[string]string{"q": "test"}},
		},
	}

	report, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(report.Markdown, "Test Report") {
		t.Error("expected report title in markdown")
	}
	if !strings.Contains(report.Markdown, "Finding A") {
		t.Error("expected data content in markdown")
	}

	// Verify report was saved to disk
	reportPath := filepath.Join(report.Dir, "report.md")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Error("expected report file on disk")
	}
}

func TestExecutorRunPartialFailure(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{
		name:     "good-api",
		response: []byte(`{"ok": true}`),
	})
	// "bad-api" is not registered — simulates service not found

	synth := synthesis.NewPassthroughSynthesizer()
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name: "partial",
		Report: ReportConfig{
			Title: "Partial Report",
		},
		Sources: []SourceConfig{
			{Service: "good-api", Tool: "fetch"},
			{Service: "bad-api", Tool: "fetch"},
		},
	}

	report, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run should succeed with partial failures: %v", err)
	}

	if !strings.Contains(report.Markdown, "good-api") {
		t.Error("expected good-api results")
	}
	if !strings.Contains(report.Markdown, "service not found") {
		t.Error("expected error for bad-api")
	}
}

func TestExecutorParallelSpeedup(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	for _, name := range []string{"api-a", "api-b", "api-c"} {
		reg.Register(&mockService{
			name:     name,
			response: []byte(`{"ok": true}`),
			delay:    100 * time.Millisecond,
		})
	}

	synth := synthesis.NewPassthroughSynthesizer()
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name:   "parallel-test",
		Report: ReportConfig{Title: "Parallel"},
		Sources: []SourceConfig{
			{Service: "api-a", Tool: "fetch"},
			{Service: "api-b", Tool: "fetch"},
			{Service: "api-c", Tool: "fetch"},
		},
	}

	start := time.Now()
	report, err := exec.Run(context.Background(), routine)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(report.Markdown, "**Successful:** 3") {
		t.Error("expected all 3 sources successful")
	}
	// 3 services at 100ms each: sequential would take ≥300ms.
	// Parallel should complete well under that. Use generous ceiling for CI.
	if elapsed > 500*time.Millisecond {
		t.Errorf("parallel execution too slow: %v (expected < 500ms, sequential would be ≥300ms)", elapsed)
	}
}

func TestExecutorJitterCalls(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "api-a", response: []byte(`{}`)})
	reg.Register(&mockService{name: "api-b", response: []byte(`{}`)})

	synth := synthesis.NewPassthroughSynthesizer()
	exec := NewExecutor(reg, synth, reportsDir)

	var callCount atomic.Int32
	exec.SetRandFunc(func(max int) int {
		callCount.Add(1)
		if max != 10 {
			t.Errorf("expected jitter max 10, got %d", max)
		}
		return 0 // no actual delay
	})

	routine := &Routine{
		Name:   "jitter-test",
		Jitter: 10,
		Report: ReportConfig{Title: "Jitter"},
		Sources: []SourceConfig{
			{Service: "api-a", Tool: "fetch"},
			{Service: "api-b", Tool: "fetch"},
		},
	}

	_, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := callCount.Load(); got != 2 {
		t.Errorf("expected randFunc called 2 times, got %d", got)
	}
}

func TestExecutorContextCancellation(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "api-a", response: []byte(`{}`)})

	synth := synthesis.NewPassthroughSynthesizer()
	exec := NewExecutor(reg, synth, reportsDir)
	exec.SetRandFunc(func(max int) int { return max }) // maximum jitter

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	routine := &Routine{
		Name:   "cancel-test",
		Jitter: 60,
		Report: ReportConfig{Title: "Cancel"},
		Sources: []SourceConfig{
			{Service: "api-a", Tool: "fetch"},
		},
	}

	_, err := exec.Run(ctx, routine)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got: %v", err)
	}
}

func TestExecutorDuplicateServiceTool(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "api-a", response: []byte(`{"data": "value"}`)})

	synth := synthesis.NewPassthroughSynthesizer()
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name:   "dup-test",
		Report: ReportConfig{Title: "Dup"},
		Sources: []SourceConfig{
			{Service: "api-a", Tool: "search", Params: map[string]string{"q": "first"}},
			{Service: "api-a", Tool: "search", Params: map[string]string{"q": "second"}},
		},
	}

	report, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Both raw results should be saved (check data/ directory)
	dataDir := filepath.Join(report.Dir, "data")
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatalf("ReadDir data: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 raw result files, got %d", len(entries))
	}
}

func TestExecutorOrderPreservation(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "slow", response: []byte(`{"order": "first"}`), delay: 50 * time.Millisecond})
	reg.Register(&mockService{name: "fast", response: []byte(`{"order": "second"}`), delay: 0})

	synth := synthesis.NewPassthroughSynthesizer()
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name:   "order-test",
		Report: ReportConfig{Title: "Order"},
		Sources: []SourceConfig{
			{Service: "slow", Tool: "fetch"},
			{Service: "fast", Tool: "fetch"},
		},
	}

	report, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// "slow" should appear before "fast" in the report because sources are ordered
	slowIdx := strings.Index(report.Markdown, "slow")
	fastIdx := strings.Index(report.Markdown, "fast")
	if slowIdx < 0 || fastIdx < 0 {
		t.Fatal("expected both services in report")
	}
	if slowIdx > fastIdx {
		t.Error("expected slow (source 0) to appear before fast (source 1) in report")
	}
}

func TestExecutorLedgerIndexing(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	contextDir := filepath.Join(dir, "context")
	os.MkdirAll(reportsDir, 0o755)

	ledger, err := bcontext.NewLedger(contextDir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "test-api", response: []byte(`{"data": "value"}`)})

	synth := synthesis.NewPassthroughSynthesizer()
	exec := NewExecutor(reg, synth, reportsDir)
	exec.SetLedger(ledger)

	routine := &Routine{
		Name:   "ledger-test",
		Report: ReportConfig{Title: "Ledger Test"},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	_, err = exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify report was indexed
	reports, err := ledger.List(bcontext.TypeReport, 0)
	if err != nil {
		t.Fatalf("List reports: %v", err)
	}
	if len(reports) != 1 {
		t.Errorf("expected 1 report in ledger, got %d", len(reports))
	}

	// Verify result was indexed
	results, err := ledger.List(bcontext.TypeResult, 0)
	if err != nil {
		t.Fatalf("List results: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result in ledger, got %d", len(results))
	}
}

// panickingService panics when Execute is called, to test panic recovery.
type panickingService struct {
	name string
}

func (p *panickingService) Name() string { return p.name }
func (p *panickingService) Execute(_ context.Context, _ string, _ map[string]string) (*services.Result, error) {
	panic("nil pointer in response parsing")
}

func TestExecutorPanicRecovery(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&panickingService{name: "panic-api"})
	reg.Register(&mockService{name: "good-api", response: []byte(`{"ok": true}`)})

	synth := synthesis.NewPassthroughSynthesizer()
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name:   "panic-test",
		Report: ReportConfig{Title: "Panic Test"},
		Sources: []SourceConfig{
			{Service: "panic-api", Tool: "fetch"},
			{Service: "good-api", Tool: "fetch"},
		},
	}

	// Should not panic — the goroutine should recover
	report, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run should not fail: %v", err)
	}

	// Panicking service should produce an error result
	if !strings.Contains(report.Markdown, "panic:") {
		t.Error("expected panic error in report")
	}
	// Good service should still succeed
	if !strings.Contains(report.Markdown, "good-api") {
		t.Error("expected good-api results despite panic in other service")
	}
}

func TestTestSources(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{
		name:     "good-api",
		response: []byte(`{"ok": true}`),
		delay:    10 * time.Millisecond,
	})
	reg.Register(&mockService{
		name: "bad-api",
		err:  fmt.Errorf("connection refused"),
	})

	synth := synthesis.NewPassthroughSynthesizer()
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name: "test-sources",
		Sources: []SourceConfig{
			{Service: "good-api", Tool: "fetch"},
			{Service: "bad-api", Tool: "fetch"},
			{Service: "missing-api", Tool: "fetch"},
		},
	}

	statuses := exec.TestSources(context.Background(), routine)
	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}

	// good-api should succeed
	if !statuses[0].OK {
		t.Errorf("expected good-api OK, got error: %s", statuses[0].Error)
	}
	if statuses[0].Latency < 10*time.Millisecond {
		t.Error("expected latency >= 10ms for good-api")
	}

	// bad-api should fail
	if statuses[1].OK {
		t.Error("expected bad-api to fail")
	}
	if !strings.Contains(statuses[1].Error, "connection refused") {
		t.Errorf("expected connection refused error, got: %s", statuses[1].Error)
	}

	// missing-api should fail with service not found
	if statuses[2].OK {
		t.Error("expected missing-api to fail")
	}
	if !strings.Contains(statuses[2].Error, "service not found") {
		t.Errorf("expected service not found error, got: %s", statuses[2].Error)
	}
}

// capturingSynthesizer records the system prompt it receives.
type capturingSynthesizer struct {
	systemPrompt string
	results      []*services.Result
}

func (c *capturingSynthesizer) Synthesize(_ context.Context, title string, systemPrompt string, results []*services.Result) (string, error) {
	c.systemPrompt = systemPrompt
	c.results = results
	return "# " + title + "\n\nSynthesized.\n", nil
}

func TestExecutorCompareWith(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	// Seed a previous report for the "compare-target" routine.
	prevMarkdown := "# Previous Report\n\nOld findings here.\n"
	_, err := reports.Save(reportsDir, "compare-target", prevMarkdown, nil)
	if err != nil {
		t.Fatalf("saving seed report: %v", err)
	}

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "test-api", response: []byte(`{"data": "new"}`)})

	synth := &capturingSynthesizer{}
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name: "current-routine",
		Report: ReportConfig{
			Title:       "Current Report",
			CompareWith: "compare-target",
		},
		Synthesis: SynthesisConfig{System: "You are an analyst."},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	_, err = exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// System prompt should contain comparison context.
	if !strings.Contains(synth.systemPrompt, "Previous Report for Comparison") {
		t.Error("expected comparison context in system prompt")
	}
	if !strings.Contains(synth.systemPrompt, "Old findings here.") {
		t.Error("expected previous report content in system prompt")
	}
	if !strings.Contains(synth.systemPrompt, "You are an analyst.") {
		t.Error("expected original system prompt preserved")
	}
}

func TestExecutorCompareWithNoPrevious(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "test-api", response: []byte(`{"data": "value"}`)})

	synth := &capturingSynthesizer{}
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name: "first-run",
		Report: ReportConfig{
			Title:          "First Report",
			CompareWith:    "nonexistent-routine",
			GenerateCharts: boolPtr(false),
		},
		Synthesis: SynthesisConfig{System: "You are an analyst."},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	// Should succeed without error — no previous report is fine (first run).
	_, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// System prompt should NOT contain comparison context.
	if strings.Contains(synth.systemPrompt, "Previous Report") {
		t.Error("did not expect comparison context when no previous report exists")
	}
	if synth.systemPrompt != "You are an analyst." {
		t.Errorf("expected unmodified system prompt, got %q", synth.systemPrompt)
	}
}

func TestExecutorNoCompareWith(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "test-api", response: []byte(`{"data": "value"}`)})

	synth := &capturingSynthesizer{}
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name:      "no-compare",
		Report:    ReportConfig{Title: "Normal Report", GenerateCharts: boolPtr(false)},
		Synthesis: SynthesisConfig{System: "You are an analyst."},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	_, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// System prompt should be unchanged.
	if synth.systemPrompt != "You are an analyst." {
		t.Errorf("expected unmodified system prompt, got %q", synth.systemPrompt)
	}
}

func boolPtr(b bool) *bool { return &b }

// chartSynthesizer returns markdown with chart directives.
type chartSynthesizer struct {
	markdown string
}

func (c *chartSynthesizer) Synthesize(_ context.Context, _ string, _ string, _ []*services.Result) (string, error) {
	return c.markdown, nil
}

func TestExecutorChartGeneration(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "test-api", response: []byte(`{"data": "value"}`)})

	chartMD := "# Report\n\n```chart\ntype: bar\ntitle: \"Postings by Agency\"\nx: [\"NGA\", \"NRO\", \"DIA\"]\ny: [12, 4, 2]\n```\n\nSome text.\n"

	synth := &chartSynthesizer{markdown: chartMD}
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name: "chart-test",
		Report: ReportConfig{
			Title:          "Chart Report",
			GenerateCharts: boolPtr(true),
		},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	report, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify charts directory was created with a PNG
	chartsDir := filepath.Join(report.Dir, "charts")
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		t.Fatalf("ReadDir charts: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 chart file, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".png") {
		t.Errorf("expected .png file, got %q", entries[0].Name())
	}

	// Verify the PNG file has valid content
	pngData, err := os.ReadFile(filepath.Join(chartsDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("reading PNG: %v", err)
	}
	if len(pngData) < 8 || string(pngData[1:4]) != "PNG" {
		t.Error("expected valid PNG header")
	}

	// Verify report.md is unchanged (chart blocks preserved)
	if !strings.Contains(report.Markdown, "```chart") {
		t.Error("expected chart block preserved in markdown")
	}

	// Verify Charts field is populated
	if len(report.Charts) != 1 {
		t.Errorf("expected 1 chart in report, got %d", len(report.Charts))
	}
}

func TestExecutorChartGenerationDisabled(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "test-api", response: []byte(`{"data": "value"}`)})

	chartMD := "# Report\n\n```chart\ntype: bar\ntitle: \"Test\"\nx: [\"A\"]\ny: [1]\n```\n"

	synth := &chartSynthesizer{markdown: chartMD}
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name: "no-charts",
		Report: ReportConfig{
			Title:          "No Charts",
			GenerateCharts: boolPtr(false),
		},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	report, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Charts directory should not exist
	chartsDir := filepath.Join(report.Dir, "charts")
	if _, err := os.Stat(chartsDir); !os.IsNotExist(err) {
		t.Error("expected no charts directory when generate_charts is false")
	}
	if len(report.Charts) != 0 {
		t.Errorf("expected 0 charts, got %d", len(report.Charts))
	}
}

func TestExecutorChartInstructionsInjected(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "test-api", response: []byte(`{"data": "value"}`)})

	synth := &capturingSynthesizer{}
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name: "chart-instructions",
		Report: ReportConfig{
			Title:          "Chart Instructions Test",
			GenerateCharts: boolPtr(true),
		},
		Synthesis: SynthesisConfig{System: "You are an analyst."},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	_, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// System prompt should contain chart instructions.
	if !strings.Contains(synth.systemPrompt, "Data visualization:") {
		t.Error("expected chart instructions in system prompt")
	}
	if !strings.Contains(synth.systemPrompt, "```chart") {
		t.Error("expected chart directive example in system prompt")
	}
	if !strings.Contains(synth.systemPrompt, "bar") || !strings.Contains(synth.systemPrompt, "line") || !strings.Contains(synth.systemPrompt, "pie") {
		t.Error("expected all chart types mentioned in system prompt")
	}
	// Original system prompt should still be present.
	if !strings.Contains(synth.systemPrompt, "You are an analyst.") {
		t.Error("expected original system prompt preserved")
	}
}

func TestExecutorChartDefaultEnabled(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "test-api", response: []byte(`{"data": "value"}`)})

	chartMD := "# Report\n\n```chart\ntype: bar\ntitle: \"Default Charts\"\nx: [\"A\", \"B\"]\ny: [10, 20]\n```\n\nText.\n"

	synth := &chartSynthesizer{markdown: chartMD}
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name: "default-charts",
		Report: ReportConfig{
			Title: "Default Charts Report",
			// GenerateCharts omitted (nil) — should default to enabled
		},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	report, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Charts directory should be created with a PNG (charts enabled by default)
	chartsDir := filepath.Join(report.Dir, "charts")
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		t.Fatalf("ReadDir charts: %v (expected charts dir to exist when GenerateCharts is nil)", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 chart file, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".png") {
		t.Errorf("expected .png file, got %q", entries[0].Name())
	}
}

func TestExecutorChartInstructionsNotInjectedWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "test-api", response: []byte(`{"data": "value"}`)})

	synth := &capturingSynthesizer{}
	exec := NewExecutor(reg, synth, reportsDir)

	routine := &Routine{
		Name: "no-chart-instructions",
		Report: ReportConfig{
			Title:          "No Chart Instructions",
			GenerateCharts: boolPtr(false),
		},
		Synthesis: SynthesisConfig{System: "You are an analyst."},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	_, err := exec.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// System prompt should NOT contain chart instructions.
	if strings.Contains(synth.systemPrompt, "Data visualization:") {
		t.Error("did not expect chart instructions when GenerateCharts is false")
	}
	if synth.systemPrompt != "You are an analyst." {
		t.Errorf("expected unmodified system prompt, got %q", synth.systemPrompt)
	}
}

type failingSynthesizer struct{}

func (f *failingSynthesizer) Synthesize(_ context.Context, _ string, _ string, _ []*services.Result) (string, error) {
	return "", fmt.Errorf("LLM timeout")
}

func TestExecutorSynthesisFailurePreservesRawData(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	reg := services.NewRegistry()
	reg.Register(&mockService{name: "test-api", response: []byte(`{"important": "data"}`)})

	exec := NewExecutor(reg, &failingSynthesizer{}, reportsDir)

	routine := &Routine{
		Name:   "fail-synth",
		Report: ReportConfig{Title: "Should Fail"},
		Sources: []SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	_, err := exec.Run(context.Background(), routine)
	if err == nil {
		t.Fatal("expected synthesis failure error")
	}
	if !strings.Contains(err.Error(), "synthesis failed") {
		t.Errorf("expected synthesis failed error, got: %v", err)
	}

	// Raw results must still be on disk despite synthesis failure
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 report directory, got %d", len(entries))
	}

	dataDir := filepath.Join(reportsDir, entries[0].Name(), "data")
	dataEntries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatalf("ReadDir data: %v", err)
	}
	if len(dataEntries) != 1 {
		t.Errorf("expected 1 raw result file, got %d", len(dataEntries))
	}
}
