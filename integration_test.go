package burrow_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jcadam/burrow/pkg/config"
	bcontext "github.com/jcadam/burrow/pkg/context"
	bhttp "github.com/jcadam/burrow/pkg/http"
	"github.com/jcadam/burrow/pkg/pipeline"
	"github.com/jcadam/burrow/pkg/privacy"
	"github.com/jcadam/burrow/pkg/render"
	"github.com/jcadam/burrow/pkg/reports"
	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/synthesis"
)

// TestEndToEnd exercises the full pipeline: config → registry → executor →
// passthrough synthesis → report save → report load → render.
// Zero network access — uses httptest.NewServer.
func TestEndToEnd(t *testing.T) {
	// Stand up fake API servers
	samServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("api_key"); got != "test-key-123" {
			t.Errorf("expected api_key test-key-123, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"opportunitiesData": [
				{"title": "Geospatial Analysis Contract", "agency": "NGA", "value": "$2.5M"},
				{"title": "Remote Sensing Platform", "agency": "CISA", "value": "$800K"}
			]
		}`))
	}))
	defer samServer.Close()

	edgarServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != "burrow/1.0 test@example.com" {
			t.Errorf("expected custom user-agent, got %q", ua)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"filings": [
				{"company": "Maxar Technologies", "type": "10-K", "date": "2026-02-15"}
			]
		}`))
	}))
	defer edgarServer.Close()

	// Create temp burrow directory
	burrowDir := t.TempDir()
	routinesDir := filepath.Join(burrowDir, "routines")
	reportsDir := filepath.Join(burrowDir, "reports")
	os.MkdirAll(routinesDir, 0o755)
	os.MkdirAll(reportsDir, 0o755)

	// Write config.yaml
	configYAML := `
services:
  - name: sam-gov
    type: rest
    endpoint: ` + samServer.URL + `
    auth:
      method: api_key
      key: ${SAM_API_KEY}
    tools:
      - name: search_opportunities
        description: "Search active contract opportunities"
        method: GET
        path: /opportunities/v2/search
        params:
          - name: naics
            type: string
            maps_to: api.ncode
          - name: status
            type: string
            maps_to: api.status

  - name: edgar
    type: rest
    endpoint: ` + edgarServer.URL + `
    auth:
      method: user_agent
      value: "burrow/1.0 test@example.com"
    tools:
      - name: company_filings
        description: "Search SEC filings"
        method: GET
        path: /filings/search
        params:
          - name: keywords
            type: string
            maps_to: q

rendering:
  images: auto
`
	os.WriteFile(filepath.Join(burrowDir, "config.yaml"), []byte(configYAML), 0o644)

	// Write routine YAML
	routineYAML := `
report:
  title: "Market Intelligence Brief"
  style: executive_summary

synthesis:
  system: |
    You are a business development analyst.

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
	os.WriteFile(filepath.Join(routinesDir, "morning-intel.yaml"), []byte(routineYAML), 0o644)

	// 1. Load and validate config
	t.Setenv("SAM_API_KEY", "test-key-123")
	cfg, err := config.Load(burrowDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	config.ResolveEnvVars(cfg)
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("config.Validate: %v", err)
	}

	// 2. Build registry (nil privacy config for basic test)
	registry := services.NewRegistry()
	for _, svcCfg := range cfg.Services {
		svc := bhttp.NewRESTService(svcCfg, nil)
		if err := registry.Register(svc); err != nil {
			t.Fatalf("Register %q: %v", svcCfg.Name, err)
		}
	}
	if names := registry.List(); len(names) != 2 {
		t.Fatalf("expected 2 services registered, got %d", len(names))
	}

	// 3. Load routine
	routine, err := pipeline.LoadRoutine(filepath.Join(routinesDir, "morning-intel.yaml"))
	if err != nil {
		t.Fatalf("LoadRoutine: %v", err)
	}
	if routine.Name != "morning-intel" {
		t.Errorf("expected routine name morning-intel, got %q", routine.Name)
	}

	// 4. Execute pipeline
	synth := synthesis.NewPassthroughSynthesizer()
	executor := pipeline.NewExecutor(registry, synth, reportsDir)

	report, err := executor.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("executor.Run: %v", err)
	}

	// 5. Verify report content
	if !strings.Contains(report.Markdown, "Market Intelligence Brief") {
		t.Error("expected report title in markdown")
	}
	if !strings.Contains(report.Markdown, "Geospatial Analysis Contract") {
		t.Error("expected SAM.gov data in report")
	}
	if !strings.Contains(report.Markdown, "Maxar Technologies") {
		t.Error("expected EDGAR data in report")
	}
	if !strings.Contains(report.Markdown, "**Sources queried:** 2") {
		t.Error("expected source count")
	}
	if !strings.Contains(report.Markdown, "**Successful:** 2") {
		t.Error("expected all sources successful")
	}

	// 6. Verify report on disk
	reportMD := filepath.Join(report.Dir, "report.md")
	if _, err := os.Stat(reportMD); os.IsNotExist(err) {
		t.Fatal("expected report.md on disk")
	}
	dataDir := filepath.Join(report.Dir, "data")
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatalf("reading data dir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 raw result files, got %d", len(entries))
	}

	// 7. Load report back
	loaded, err := reports.Load(report.Dir)
	if err != nil {
		t.Fatalf("reports.Load: %v", err)
	}
	if loaded.Markdown != report.Markdown {
		t.Error("loaded markdown doesn't match saved markdown")
	}
	if loaded.Title != "Market Intelligence Brief" {
		t.Errorf("expected title, got %q", loaded.Title)
	}

	// 8. List reports
	allReports, err := reports.List(reportsDir)
	if err != nil {
		t.Fatalf("reports.List: %v", err)
	}
	if len(allReports) != 1 {
		t.Errorf("expected 1 report, got %d", len(allReports))
	}

	// 9. Find latest by routine
	latest, err := reports.FindLatest(reportsDir, "morning-intel")
	if err != nil {
		t.Fatalf("FindLatest: %v", err)
	}
	if latest == nil {
		t.Fatal("expected to find latest report")
	}

	// 10. Render markdown (non-interactive, just verify it doesn't error)
	rendered, err := render.RenderMarkdown(report.Markdown, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if rendered == "" {
		t.Error("expected non-empty rendered output")
	}
}

// TestPrivacyHeaders verifies that privacy transport strips/rotates headers.
func TestPrivacyHeaders(t *testing.T) {
	var receivedUA string
	var receivedReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		receivedReferer = r.Header.Get("Referer")
		w.Write([]byte(`{"ok": true}`))
	}))
	defer srv.Close()

	privCfg := &privacy.Config{
		StripReferrers:     true,
		RandomizeUserAgent: true,
		MinimizeRequests:   true,
	}

	svc := bhttp.NewRESTService(config.ServiceConfig{
		Name:     "priv-test",
		Type:     "rest",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/data"},
		},
	}, privCfg)

	result, err := svc.Execute(context.Background(), "fetch", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("result error: %s", result.Error)
	}

	if strings.Contains(receivedUA, "Go-http-client") {
		t.Errorf("expected rotated UA, got default Go UA: %q", receivedUA)
	}
	if receivedReferer != "" {
		t.Error("expected Referer stripped")
	}
}

// TestContextLedgerAfterPipeline verifies that context entries are written after a pipeline run.
func TestContextLedgerAfterPipeline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data": "test-value"}`))
	}))
	defer srv.Close()

	burrowDir := t.TempDir()
	reportsDir := filepath.Join(burrowDir, "reports")
	contextDir := filepath.Join(burrowDir, "context")
	os.MkdirAll(reportsDir, 0o755)

	ledger, err := bcontext.NewLedger(contextDir)
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}

	registry := services.NewRegistry()
	svc := bhttp.NewRESTService(config.ServiceConfig{
		Name:     "test-api",
		Type:     "rest",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "none"},
		Tools: []config.ToolConfig{
			{Name: "fetch", Method: "GET", Path: "/data"},
		},
	}, nil)
	registry.Register(svc)

	synth := synthesis.NewPassthroughSynthesizer()
	executor := pipeline.NewExecutor(registry, synth, reportsDir)
	executor.SetLedger(ledger)

	routine := &pipeline.Routine{
		Name:   "context-test",
		Report: pipeline.ReportConfig{Title: "Context Test"},
		Sources: []pipeline.SourceConfig{
			{Service: "test-api", Tool: "fetch"},
		},
	}

	_, err = executor.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify context entries
	contextReports, err := ledger.List(bcontext.TypeReport, 0)
	if err != nil {
		t.Fatalf("List reports: %v", err)
	}
	if len(contextReports) != 1 {
		t.Errorf("expected 1 report in context, got %d", len(contextReports))
	}

	contextResults, err := ledger.List(bcontext.TypeResult, 0)
	if err != nil {
		t.Fatalf("List results: %v", err)
	}
	if len(contextResults) != 1 {
		t.Errorf("expected 1 result in context, got %d", len(contextResults))
	}

	// Search context
	matches, err := ledger.Search("test-value")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Error("expected to find test-value in context search")
	}
}

// TestParallelExecution verifies parallel execution with timing.
func TestParallelExecution(t *testing.T) {
	// Create 3 servers that each take 100ms to respond
	makeSlowServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Write([]byte(`{"ok": true}`))
		}))
	}

	srv1 := makeSlowServer()
	srv2 := makeSlowServer()
	srv3 := makeSlowServer()
	defer srv1.Close()
	defer srv2.Close()
	defer srv3.Close()

	burrowDir := t.TempDir()
	reportsDir := filepath.Join(burrowDir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	registry := services.NewRegistry()
	for i, srv := range []*httptest.Server{srv1, srv2, srv3} {
		name := []string{"api-a", "api-b", "api-c"}[i]
		svc := bhttp.NewRESTService(config.ServiceConfig{
			Name:     name,
			Type:     "rest",
			Endpoint: srv.URL,
			Auth:     config.AuthConfig{Method: "none"},
			Tools: []config.ToolConfig{
				{Name: "fetch", Method: "GET", Path: "/data"},
			},
		}, nil)
		registry.Register(svc)
	}

	synth := synthesis.NewPassthroughSynthesizer()
	executor := pipeline.NewExecutor(registry, synth, reportsDir)

	routine := &pipeline.Routine{
		Name:   "parallel-test",
		Report: pipeline.ReportConfig{Title: "Parallel Test"},
		Sources: []pipeline.SourceConfig{
			{Service: "api-a", Tool: "fetch"},
			{Service: "api-b", Tool: "fetch"},
			{Service: "api-c", Tool: "fetch"},
		},
	}

	start := time.Now()
	report, err := executor.Run(context.Background(), routine)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(report.Markdown, "**Successful:** 3") {
		t.Error("expected 3 successful sources")
	}
	// 3 services at 100ms each, parallel should complete well under 300ms
	if elapsed > 250*time.Millisecond {
		t.Errorf("expected parallel execution, took %v (should be < 250ms)", elapsed)
	}
}
