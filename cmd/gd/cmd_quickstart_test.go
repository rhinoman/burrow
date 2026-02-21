package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcadam/burrow/pkg/config"
	bhttp "github.com/jcadam/burrow/pkg/http"
	"github.com/jcadam/burrow/pkg/pipeline"
	"github.com/jcadam/burrow/pkg/reports"
	"github.com/jcadam/burrow/pkg/services"
	"github.com/jcadam/burrow/pkg/synthesis"
)

func TestQuickstartConfigValid(t *testing.T) {
	cfg := buildQuickstartConfig()

	if err := config.Validate(cfg); err != nil {
		t.Fatalf("quickstart config should be valid: %v", err)
	}

	// Round-trip through save/load
	dir := t.TempDir()
	if err := config.Save(dir, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(loaded.Services))
	}

	svc := loaded.Services[0]
	if svc.Name != "weather-gov" {
		t.Errorf("expected service name weather-gov, got %q", svc.Name)
	}
	if svc.Type != "rest" {
		t.Errorf("expected type rest, got %q", svc.Type)
	}
	if svc.Endpoint != "https://api.weather.gov" {
		t.Errorf("expected weather.gov endpoint, got %q", svc.Endpoint)
	}
	if svc.Auth.Method != "user_agent" {
		t.Errorf("expected user_agent auth, got %q", svc.Auth.Method)
	}
	if len(svc.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(svc.Tools))
	}
	if svc.Tools[0].Name != "forecast" {
		t.Errorf("expected first tool forecast, got %q", svc.Tools[0].Name)
	}
	if svc.Tools[1].Name != "alerts" {
		t.Errorf("expected second tool alerts, got %q", svc.Tools[1].Name)
	}

	if !loaded.Privacy.StripReferrers {
		t.Error("expected strip_referrers true")
	}
	if !loaded.Privacy.MinimizeRequests {
		t.Error("expected minimize_requests true")
	}
}

func TestQuickstartConfigWithProvider(t *testing.T) {
	cfg := buildQuickstartConfig()

	// Simulate what setupQuickstartLLM does when Ollama is detected
	cfg.LLM.Providers = append(cfg.LLM.Providers, config.ProviderConfig{
		Name:     "local/llama3:latest",
		Type:     "ollama",
		Endpoint: "http://localhost:11434",
		Model:    "llama3:latest",
		Privacy:  "local",
	})

	if err := config.Validate(cfg); err != nil {
		t.Fatalf("config with provider should be valid: %v", err)
	}

	// Round-trip
	dir := t.TempDir()
	if err := config.Save(dir, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.LLM.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(loaded.LLM.Providers))
	}
	prov := loaded.LLM.Providers[0]
	if prov.Name != "local/llama3:latest" {
		t.Errorf("expected provider name local/llama3:latest, got %q", prov.Name)
	}
	if prov.Type != "ollama" {
		t.Errorf("expected type ollama, got %q", prov.Type)
	}
	if prov.Model != "llama3:latest" {
		t.Errorf("expected model llama3:latest, got %q", prov.Model)
	}
	if prov.Privacy != "local" {
		t.Errorf("expected privacy local, got %q", prov.Privacy)
	}
}

func TestQuickstartRoutineNone(t *testing.T) {
	routine := buildQuickstartRoutine("none")

	// Save and reload
	dir := t.TempDir()
	if err := pipeline.SaveRoutine(dir, routine); err != nil {
		t.Fatalf("SaveRoutine: %v", err)
	}

	loaded, err := pipeline.LoadRoutine(filepath.Join(dir, "weather.yaml"))
	if err != nil {
		t.Fatalf("LoadRoutine: %v", err)
	}

	if loaded.Name != "weather" {
		t.Errorf("expected name weather, got %q", loaded.Name)
	}
	if loaded.LLM != "none" {
		t.Errorf("expected llm none, got %q", loaded.LLM)
	}
	if loaded.Report.Title != "Weather Report — Denver/Boulder, CO" {
		t.Errorf("expected report title, got %q", loaded.Report.Title)
	}
	if loaded.Synthesis.System != "" {
		t.Errorf("expected no synthesis prompt for llm:none, got %q", loaded.Synthesis.System)
	}
	if len(loaded.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(loaded.Sources))
	}
	if loaded.Sources[0].Service != "weather-gov" || loaded.Sources[0].Tool != "forecast" {
		t.Errorf("unexpected first source: %+v", loaded.Sources[0])
	}
	if loaded.Sources[1].Service != "weather-gov" || loaded.Sources[1].Tool != "alerts" {
		t.Errorf("unexpected second source: %+v", loaded.Sources[1])
	}
}

func TestQuickstartRoutineWithLLM(t *testing.T) {
	routine := buildQuickstartRoutine("local/llama3:latest")

	// Save and reload
	dir := t.TempDir()
	if err := pipeline.SaveRoutine(dir, routine); err != nil {
		t.Fatalf("SaveRoutine: %v", err)
	}

	loaded, err := pipeline.LoadRoutine(filepath.Join(dir, "weather.yaml"))
	if err != nil {
		t.Fatalf("LoadRoutine: %v", err)
	}

	if loaded.LLM != "local/llama3:latest" {
		t.Errorf("expected llm local/llama3:latest, got %q", loaded.LLM)
	}
	if loaded.Synthesis.System == "" {
		t.Error("expected synthesis prompt for llm provider")
	}
	if !strings.Contains(loaded.Synthesis.System, "weather analyst") {
		t.Errorf("expected weather analyst in synthesis prompt, got %q", loaded.Synthesis.System)
	}
}

func TestQuickstartExistingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BURROW_DIR", dir)

	// Write a dummy config
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("services: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Reset the force flag for this test
	quickstartForce = false

	err := runQuickstart(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Config should NOT have been overwritten
	data, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if strings.Contains(string(data), "weather-gov") {
		t.Error("config was overwritten without --force")
	}
}

// TestQuickstartEndToEnd tests the full pipeline with passthrough (llm: none path).
// The LLM synthesis path is covered by integration tests in pkg/pipeline.
func TestQuickstartEndToEnd(t *testing.T) {
	// Sample NWS forecast response
	forecastJSON := `{
		"properties": {
			"periods": [
				{
					"number": 1,
					"name": "Today",
					"temperature": 45,
					"temperatureUnit": "F",
					"windSpeed": "10 mph",
					"shortForecast": "Partly Sunny",
					"detailedForecast": "Partly sunny with a high near 45."
				},
				{
					"number": 2,
					"name": "Tonight",
					"temperature": 28,
					"temperatureUnit": "F",
					"windSpeed": "5 mph",
					"shortForecast": "Mostly Clear",
					"detailedForecast": "Mostly clear with a low around 28."
				}
			]
		}
	}`

	// Sample NWS alerts response
	alertsJSON := `{
		"features": [
			{
				"properties": {
					"event": "Winter Storm Warning",
					"headline": "Winter Storm Warning issued for Boulder County",
					"severity": "Severe",
					"description": "Heavy snow expected. 8 to 14 inches."
				}
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/gridpoints/"):
			w.Write([]byte(forecastJSON))
		case strings.HasPrefix(r.URL.Path, "/alerts/"):
			w.Write([]byte(alertsJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	burrowDir := t.TempDir()
	reportsDir := filepath.Join(burrowDir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	// Build config pointing at test server
	cfg := buildQuickstartConfig()
	cfg.Services[0].Endpoint = srv.URL

	if err := config.Validate(cfg); err != nil {
		t.Fatalf("config validation: %v", err)
	}

	// Build registry
	registry := services.NewRegistry()
	svc := bhttp.NewRESTService(cfg.Services[0], nil, "")
	if err := registry.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Build routine with passthrough (llm: none)
	routine := buildQuickstartRoutine("none")
	synth := synthesis.NewPassthroughSynthesizer()
	executor := pipeline.NewExecutor(registry, synth, reportsDir)

	// Test sources
	statuses := executor.TestSources(context.Background(), routine)
	for _, s := range statuses {
		if !s.OK {
			t.Errorf("source %s/%s failed: %s", s.Service, s.Tool, s.Error)
		}
	}

	// Run pipeline
	report, err := executor.Run(context.Background(), routine)
	if err != nil {
		t.Fatalf("executor.Run: %v", err)
	}

	// Verify report content
	if !strings.Contains(report.Markdown, "Weather Report") {
		t.Error("expected report title in markdown")
	}
	if !strings.Contains(report.Markdown, "Partly Sunny") {
		t.Error("expected forecast data in report")
	}
	if !strings.Contains(report.Markdown, "Winter Storm Warning") {
		t.Error("expected alerts data in report")
	}
	if !strings.Contains(report.Markdown, "**Sources queried:** 2") {
		t.Error("expected 2 sources queried")
	}
	if !strings.Contains(report.Markdown, "**Successful:** 2") {
		t.Error("expected 2 successful sources")
	}

	// Verify report on disk
	if _, err := os.Stat(filepath.Join(report.Dir, "report.md")); os.IsNotExist(err) {
		t.Error("expected report.md on disk")
	}
	dataDir := filepath.Join(report.Dir, "data")
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatalf("reading data dir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 raw result files, got %d", len(entries))
	}

	// Find report by routine name
	latest, err := reports.FindLatest(reportsDir, "weather")
	if err != nil {
		t.Fatalf("FindLatest: %v", err)
	}
	if latest == nil {
		t.Fatal("expected to find latest weather report")
	}
}

func TestResolveEnvRef(t *testing.T) {
	// Plain value
	if got := resolveEnvRef("sk-abc123"); got != "sk-abc123" {
		t.Errorf("expected plain value unchanged, got %q", got)
	}

	// Braced form: ${VAR}
	t.Setenv("TEST_QS_KEY", "resolved-value")
	if got := resolveEnvRef("${TEST_QS_KEY}"); got != "resolved-value" {
		t.Errorf("expected resolved env var, got %q", got)
	}

	// Bare form: $VAR
	if got := resolveEnvRef("$TEST_QS_KEY"); got != "resolved-value" {
		t.Errorf("expected resolved bare $VAR, got %q", got)
	}

	// Braced form that doesn't exist — returns original
	if got := resolveEnvRef("${NONEXISTENT_QS_VAR}"); got != "${NONEXISTENT_QS_VAR}" {
		t.Errorf("expected unresolved env var returned as-is, got %q", got)
	}

	// Bare form that doesn't exist — returns original
	if got := resolveEnvRef("$NONEXISTENT_QS_VAR"); got != "$NONEXISTENT_QS_VAR" {
		t.Errorf("expected unresolved bare $VAR returned as-is, got %q", got)
	}
}
