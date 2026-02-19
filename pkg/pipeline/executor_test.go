package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/synthesis"
)

type mockService struct {
	name     string
	response []byte
	err      error
}

func (m *mockService) Name() string { return m.name }
func (m *mockService) Execute(_ context.Context, tool string, _ map[string]string) (*services.Result, error) {
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
