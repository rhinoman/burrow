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
	"github.com/jcadam/burrow/pkg/pipeline"
	"github.com/jcadam/burrow/pkg/reports"
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

	if len(loaded.Services) != 5 {
		t.Fatalf("expected 5 services, got %d", len(loaded.Services))
	}

	// Verify service names and types
	expected := []struct {
		name, typ string
	}{
		{"weather-gov", "rest"},
		{"usgs-earthquakes", "rest"},
		{"open-meteo", "rest"},
		{"hackernews", "rss"},
		{"arxiv-ai", "rss"},
	}
	for i, exp := range expected {
		if loaded.Services[i].Name != exp.name {
			t.Errorf("service %d: expected name %q, got %q", i, exp.name, loaded.Services[i].Name)
		}
		if loaded.Services[i].Type != exp.typ {
			t.Errorf("service %d: expected type %q, got %q", i, exp.typ, loaded.Services[i].Type)
		}
	}

	// Verify weather-gov details
	svc := loaded.Services[0]
	if svc.Auth.Method != "user_agent" {
		t.Errorf("expected user_agent auth, got %q", svc.Auth.Method)
	}
	if len(svc.Tools) != 2 {
		t.Fatalf("expected 2 weather-gov tools, got %d", len(svc.Tools))
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

	loaded, err := pipeline.LoadRoutine(filepath.Join(dir, "daily-brief.yaml"))
	if err != nil {
		t.Fatalf("LoadRoutine: %v", err)
	}

	if loaded.Name != "daily-brief" {
		t.Errorf("expected name daily-brief, got %q", loaded.Name)
	}
	if loaded.LLM != "none" {
		t.Errorf("expected llm none, got %q", loaded.LLM)
	}
	if loaded.Report.Title != "Daily Brief — {{profile.name}}" {
		t.Errorf("expected report title, got %q", loaded.Report.Title)
	}
	if loaded.Synthesis.System != "" {
		t.Errorf("expected no synthesis prompt for llm:none, got %q", loaded.Synthesis.System)
	}
	if len(loaded.Sources) != 6 {
		t.Fatalf("expected 6 sources, got %d", len(loaded.Sources))
	}
	if loaded.Sources[0].Service != "weather-gov" || loaded.Sources[0].Tool != "forecast" {
		t.Errorf("unexpected first source: %+v", loaded.Sources[0])
	}
	if loaded.Sources[5].Service != "arxiv-ai" || loaded.Sources[5].Tool != "feed" {
		t.Errorf("unexpected last source: %+v", loaded.Sources[5])
	}
}

func TestQuickstartRoutineWithLLM(t *testing.T) {
	routine := buildQuickstartRoutine("local/llama3:latest")

	// Save and reload
	dir := t.TempDir()
	if err := pipeline.SaveRoutine(dir, routine); err != nil {
		t.Fatalf("SaveRoutine: %v", err)
	}

	loaded, err := pipeline.LoadRoutine(filepath.Join(dir, "daily-brief.yaml"))
	if err != nil {
		t.Fatalf("LoadRoutine: %v", err)
	}

	if loaded.LLM != "local/llama3:latest" {
		t.Errorf("expected llm local/llama3:latest, got %q", loaded.LLM)
	}
	if loaded.Synthesis.System == "" {
		t.Error("expected synthesis prompt for llm provider")
	}
	if !strings.Contains(loaded.Synthesis.System, "personal research analyst") {
		t.Errorf("expected 'personal research analyst' in synthesis prompt, got %q", loaded.Synthesis.System)
	}
	if !strings.Contains(loaded.Synthesis.System, "{{profile.name}}") {
		t.Error("expected profile template variables in synthesis prompt")
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
					"temperature": 15,
					"temperatureUnit": "F",
					"windSpeed": "10 mph",
					"shortForecast": "Partly Sunny",
					"detailedForecast": "Partly sunny with a high near 15."
				}
			]
		}
	}`

	// Sample NWS alerts response
	alertsJSON := `{
		"features": [
			{
				"properties": {
					"event": "Wind Advisory",
					"headline": "Wind Advisory for Anchorage",
					"severity": "Moderate",
					"description": "Gusts up to 50 mph expected."
				}
			}
		]
	}`

	// Sample USGS earthquakes response
	earthquakeJSON := `{
		"type": "FeatureCollection",
		"features": [
			{
				"properties": {
					"mag": 3.2,
					"place": "50km S of Anchorage, Alaska",
					"time": 1700000000000
				}
			}
		]
	}`

	// Sample Open-Meteo response
	openMeteoJSON := `{
		"hourly": {"temperature_2m": [10, 12, 14]},
		"daily": {"temperature_2m_max": [20], "temperature_2m_min": [5]}
	}`

	// RSS feed for Hacker News
	hnRSS := `<?xml version="1.0" encoding="UTF-8"?>
	<rss version="2.0">
		<channel>
			<title>Hacker News</title>
			<item>
				<title>Show HN: A cool project</title>
				<link>https://example.com/cool</link>
				<description>A cool project description</description>
			</item>
		</channel>
	</rss>`

	// RSS feed for ArXiv
	arxivRSS := `<?xml version="1.0" encoding="UTF-8"?>
	<rss version="2.0">
		<channel>
			<title>ArXiv CS.AI</title>
			<item>
				<title>Attention Is All You Need (Again)</title>
				<link>https://arxiv.org/abs/1234.5678</link>
				<description>A new transformer paper</description>
			</item>
		</channel>
	</rss>`

	// REST mock server (weather-gov, usgs, open-meteo)
	restSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/gridpoints/"):
			w.Write([]byte(forecastJSON))
		case strings.HasPrefix(r.URL.Path, "/alerts/"):
			w.Write([]byte(alertsJSON))
		case strings.HasPrefix(r.URL.Path, "/fdsnws/"):
			w.Write([]byte(earthquakeJSON))
		case strings.HasPrefix(r.URL.Path, "/v1/forecast"):
			w.Write([]byte(openMeteoJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer restSrv.Close()

	// RSS mock server (hackernews + arxiv)
	rssSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		switch {
		case strings.Contains(r.URL.Path, "/hn"):
			w.Write([]byte(hnRSS))
		case strings.Contains(r.URL.Path, "/arxiv"):
			w.Write([]byte(arxivRSS))
		default:
			http.NotFound(w, r)
		}
	}))
	defer rssSrv.Close()

	burrowDir := t.TempDir()
	reportsDir := filepath.Join(burrowDir, "reports")
	os.MkdirAll(reportsDir, 0o755)

	// Build config pointing at test servers
	cfg := buildQuickstartConfig()
	cfg.Services[0].Endpoint = restSrv.URL // weather-gov
	cfg.Services[1].Endpoint = restSrv.URL // usgs-earthquakes
	cfg.Services[2].Endpoint = restSrv.URL // open-meteo
	cfg.Services[3].Endpoint = rssSrv.URL + "/hn"  // hackernews
	cfg.Services[4].Endpoint = rssSrv.URL + "/arxiv" // arxiv-ai

	if err := config.Validate(cfg); err != nil {
		t.Fatalf("config validation: %v", err)
	}

	// Build registry with all services
	registry, err := buildRegistry(cfg, burrowDir)
	if err != nil {
		t.Fatalf("buildRegistry: %v", err)
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
	if !strings.Contains(report.Markdown, "Daily Brief") {
		t.Error("expected report title in markdown")
	}
	if !strings.Contains(report.Markdown, "Partly Sunny") {
		t.Error("expected forecast data in report")
	}
	if !strings.Contains(report.Markdown, "Wind Advisory") {
		t.Error("expected alerts data in report")
	}
	if !strings.Contains(report.Markdown, "**Sources queried:** 6") {
		t.Error("expected 6 sources queried")
	}
	if !strings.Contains(report.Markdown, "**Successful:** 6") {
		t.Error("expected 6 successful sources")
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
	if len(entries) != 6 {
		t.Errorf("expected 6 raw result files, got %d", len(entries))
	}

	// Find report by routine name
	latest, err := reports.FindLatest(reportsDir, "daily-brief")
	if err != nil {
		t.Fatalf("FindLatest: %v", err)
	}
	if latest == nil {
		t.Fatal("expected to find latest daily-brief report")
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
