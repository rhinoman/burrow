package synthesis

import (
	"context"
	"errors"
	"strings"
	gosync "sync"
	"sync/atomic"
	"testing"

	"github.com/jcadam/burrow/pkg/services"
)

// --- shouldMultiStage tests ---

func TestShouldMultiStageAuto_BelowThreshold(t *testing.T) {
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "auto"})

	results := []*services.Result{
		{Data: make([]byte, 1000)},
	}
	if synth.shouldMultiStage(results) {
		t.Error("expected single-stage for small data")
	}
}

func TestShouldMultiStageAuto_AboveThreshold(t *testing.T) {
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "auto"})

	results := []*services.Result{
		{Data: make([]byte, 10000)},
		{Data: make([]byte, 10000)},
	}
	if !synth.shouldMultiStage(results) {
		t.Error("expected multi-stage for large data")
	}
}

func TestShouldMultiStageAuto_CustomThreshold(t *testing.T) {
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "auto", ThresholdBytes: 500})

	results := []*services.Result{
		{Data: make([]byte, 600)},
	}
	if !synth.shouldMultiStage(results) {
		t.Error("expected multi-stage with custom threshold 500 and 600 bytes of data")
	}
}

func TestShouldMultiStageForceSingle(t *testing.T) {
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "single"})

	results := []*services.Result{
		{Data: make([]byte, 100000)},
	}
	if synth.shouldMultiStage(results) {
		t.Error("expected single-stage when strategy=single even with large data")
	}
}

func TestShouldMultiStageForceMulti(t *testing.T) {
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "multi-stage"})

	results := []*services.Result{
		{Data: make([]byte, 10)},
	}
	if !synth.shouldMultiStage(results) {
		t.Error("expected multi-stage when strategy=multi-stage even with tiny data")
	}
}

func TestShouldMultiStageDefaultStrategy(t *testing.T) {
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	// No SetMultiStage call — defaults to auto behavior

	results := []*services.Result{
		{Data: make([]byte, 100)},
	}
	if synth.shouldMultiStage(results) {
		t.Error("expected single-stage with default config and small data")
	}
}

// --- extractPriorities tests ---

func TestExtractPrioritiesShort(t *testing.T) {
	input := "You are a business analyst."
	got := extractPriorities(input)
	if got != input {
		t.Errorf("expected unchanged short prompt, got %q", got)
	}
}

func TestExtractPrioritiesLong(t *testing.T) {
	input := strings.Repeat("word ", 100) // 500 chars
	got := extractPriorities(input)
	if len(got) > 310 {
		t.Errorf("expected truncated to ~300 chars, got %d chars", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected ellipsis suffix on truncated priorities")
	}
}

// --- truncateSummary tests ---

func TestTruncateSummaryShort(t *testing.T) {
	input := "one two three"
	got := truncateSummary(input, 10)
	if got != input {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestTruncateSummaryLong(t *testing.T) {
	input := "one two three four five six"
	got := truncateSummary(input, 3)
	if got != "one two three..." {
		t.Errorf("expected truncated to 3 words, got %q", got)
	}
}

// --- truncateRawFallback tests ---

func TestTruncateRawFallbackShort(t *testing.T) {
	input := "short data"
	got := truncateRawFallback(input, 100)
	if got != input {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestTruncateRawFallbackLong(t *testing.T) {
	input := "one two three four five six"
	got := truncateRawFallback(input, 3)
	if !strings.HasPrefix(got, "one two three") {
		t.Errorf("expected truncated prefix, got %q", got)
	}
	if !strings.Contains(got, "[... truncated ...]") {
		t.Error("expected truncation marker")
	}
}

// --- recordingProvider captures all LLM calls ---

type recordingProvider struct {
	mu    gosync.Mutex
	calls []providerCall
	// response is the fixed response for all calls
	response string
	// failStage1 makes stage 1 calls fail
	failStage1 bool
	callCount  atomic.Int32
}

type providerCall struct {
	system string
	user   string
}

func (rp *recordingProvider) Complete(_ context.Context, system, user string) (string, error) {
	rp.callCount.Add(1)
	if rp.failStage1 && strings.Contains(system, "data summarization assistant") {
		return "", errors.New("stage 1 failure")
	}

	rp.mu.Lock()
	rp.calls = append(rp.calls, providerCall{system: system, user: user})
	rp.mu.Unlock()

	if rp.response != "" {
		return rp.response, nil
	}
	return "# Summary\nKey facts here.\n", nil
}

func (rp *recordingProvider) getCalls() []providerCall {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	cp := make([]providerCall, len(rp.calls))
	copy(cp, rp.calls)
	return cp
}

// --- Stage 1 prompt tests ---

func TestStage1PromptContents(t *testing.T) {
	provider := &recordingProvider{response: "Summary of source data."}
	synth := NewLLMSynthesizer(provider, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "multi-stage"})

	results := []*services.Result{
		{Service: "nws", Tool: "forecast", Data: []byte(`{"temp": 42}`), ContextLabel: "NWS Forecast"},
	}

	_, err := synth.Synthesize(context.Background(), "Daily Brief", "You are an analyst.", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	calls := provider.getCalls()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 LLM calls (stage 1 + stage 2), got %d", len(calls))
	}

	// Stage 1 call
	stage1 := calls[0]
	if !strings.Contains(stage1.system, "data summarization assistant") {
		t.Error("stage 1 system prompt missing expected content")
	}
	if !strings.Contains(stage1.user, "NWS Forecast") {
		t.Error("stage 1 user prompt missing context label")
	}
	if !strings.Contains(stage1.user, `{"temp": 42}`) {
		t.Error("stage 1 user prompt missing source data")
	}
	if !strings.Contains(stage1.user, "You are an analyst") {
		t.Error("stage 1 user prompt missing priorities excerpt")
	}
}

// --- Stage 2 prompt tests ---

func TestStage2PromptContents(t *testing.T) {
	provider := &recordingProvider{response: "Summary of source data."}
	synth := NewLLMSynthesizer(provider, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "multi-stage"})

	results := []*services.Result{
		{Service: "nws", Tool: "forecast", Data: []byte(`data1`)},
		{Service: "rss", Tool: "fetch", Data: []byte(`data2`)},
	}

	_, err := synth.Synthesize(context.Background(), "Daily Brief", "Be concise.", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	calls := provider.getCalls()
	// Last call should be stage 2
	stage2 := calls[len(calls)-1]

	if !strings.Contains(stage2.user, "Pre-summarized source data extracts") {
		t.Error("stage 2 prompt missing pre-summarized marker")
	}
	if !strings.Contains(stage2.user, "Daily Brief") {
		t.Error("stage 2 prompt missing report title")
	}
	if !strings.Contains(stage2.user, "No preamble, no reasoning") {
		t.Error("stage 2 prompt missing anti-thinking reinforcement")
	}
	if !strings.Contains(stage2.user, "exact URL") {
		t.Error("stage 2 prompt missing URL instruction")
	}
}

// --- Parallel execution test ---

func TestMultiStageRunsStage1(t *testing.T) {
	provider := &recordingProvider{response: "Summarized."}
	synth := NewLLMSynthesizer(provider, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "multi-stage"})

	results := make([]*services.Result, 5)
	for i := range results {
		results[i] = &services.Result{
			Service: "svc",
			Tool:    "tool",
			Data:    []byte("data"),
		}
	}

	_, err := synth.Synthesize(context.Background(), "Report", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// 5 stage 1 calls + 1 stage 2 call = 6 total
	totalCalls := int(provider.callCount.Load())
	if totalCalls != 6 {
		t.Errorf("expected 6 LLM calls (5 stage 1 + 1 stage 2), got %d", totalCalls)
	}
}

func TestMultiStageRespectsCustomConcurrency(t *testing.T) {
	provider := &recordingProvider{response: "Summarized."}
	synth := NewLLMSynthesizer(provider, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "multi-stage", Concurrency: 3})

	results := make([]*services.Result, 5)
	for i := range results {
		results[i] = &services.Result{
			Service: "svc",
			Tool:    "tool",
			Data:    []byte("data"),
		}
	}

	_, err := synth.Synthesize(context.Background(), "Report", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Still 5 stage 1 calls + 1 stage 2 call = 6 total regardless of concurrency
	totalCalls := int(provider.callCount.Load())
	if totalCalls != 6 {
		t.Errorf("expected 6 LLM calls (5 stage 1 + 1 stage 2), got %d", totalCalls)
	}
}

// --- Error fallback tests ---

// selectiveFailProvider fails on the first stage 1 call only.
type selectiveFailProvider struct {
	callIdx  *atomic.Int32
	response string
}

func (p *selectiveFailProvider) Complete(_ context.Context, system, user string) (string, error) {
	idx := p.callIdx.Add(1)
	if strings.Contains(system, "data summarization assistant") && idx == 1 {
		return "", errors.New("simulated stage 1 failure")
	}
	if p.response != "" {
		return p.response, nil
	}
	return "# Report\n", nil
}

func TestMultiStagePartialStage1Failure(t *testing.T) {
	callIdx := atomic.Int32{}
	provider := &selectiveFailProvider{
		callIdx:  &callIdx,
		response: "Summary here.",
	}
	synth := NewLLMSynthesizer(provider, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "multi-stage"})

	results := []*services.Result{
		{Service: "svc1", Tool: "t", Data: []byte("data from svc1")},
		{Service: "svc2", Tool: "t", Data: []byte("data from svc2")},
	}

	md, err := synth.Synthesize(context.Background(), "Report", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Should produce output even with partial failures
	if md == "" {
		t.Error("expected non-empty report even with partial stage 1 failures")
	}
}

func TestMultiStageAllStage1FailsUsesRawFallback(t *testing.T) {
	provider := &recordingProvider{
		response:   "# Report from raw data\n",
		failStage1: true,
	}
	synth := NewLLMSynthesizer(provider, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "multi-stage"})

	results := []*services.Result{
		{Service: "svc1", Tool: "t", Data: []byte("raw data 1")},
		{Service: "svc2", Tool: "t", Data: []byte("raw data 2")},
	}

	md, err := synth.Synthesize(context.Background(), "Report", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Should produce output via stage 2 with truncated raw data (not single-stage)
	if md == "" {
		t.Error("expected non-empty report")
	}

	// Verify stage 2 was called (the one non-stage1 call that succeeded)
	calls := provider.getCalls()
	if len(calls) != 1 {
		t.Errorf("expected exactly 1 successful call (stage 2), got %d", len(calls))
	}
	// Stage 2 prompt should contain truncated raw data from failed sources
	if len(calls) > 0 {
		if !strings.Contains(calls[0].user, "Pre-summarized source data extracts") {
			t.Error("expected stage 2 prompt with pre-summarized marker")
		}
	}
}

// --- Attribution stripping tests ---

func TestMultiStageStripsAttribution(t *testing.T) {
	provider := &recordingProvider{response: "Summarized data."}
	synth := NewLLMSynthesizer(provider, true) // stripAttribution=true
	synth.SetMultiStage(MultiStageConfig{Strategy: "multi-stage"})

	results := []*services.Result{
		{Service: "sam-gov", Tool: "search", Data: []byte(`data from sam-gov`)},
	}

	_, err := synth.Synthesize(context.Background(), "Brief", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	calls := provider.getCalls()
	for i, c := range calls {
		if strings.Contains(c.user, "sam-gov") {
			t.Errorf("call %d: attribution leak — sam-gov appears in prompt", i)
		}
	}

	// Stage 1 should use "Source 1" label
	if len(calls) > 0 {
		if !strings.Contains(calls[0].user, "Source 1") {
			t.Error("stage 1 should use generic label when stripping attribution")
		}
	}
}

// --- End-to-end test ---

// e2eProvider returns different responses for stage 1 vs stage 2.
type e2eProvider struct {
	callNum *atomic.Int32
}

func (p *e2eProvider) Complete(_ context.Context, system, user string) (string, error) {
	p.callNum.Add(1)
	if strings.Contains(system, "data summarization assistant") {
		// Stage 1
		if strings.Contains(user, "Weather") {
			return "Temperature 42F, rainy conditions expected.", nil
		}
		return "Big Event and Market Update are trending headlines.", nil
	}
	// Stage 2
	return "# Morning Brief\n\n## Weather\nTemp 42F, rain.\n\n## News\nBig Event, Market Update.\n", nil
}

func TestMultiStageEndToEnd(t *testing.T) {
	callNum := atomic.Int32{}
	provider := &e2eProvider{callNum: &callNum}
	synth := NewLLMSynthesizer(provider, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "multi-stage", SummaryMaxWords: 100})

	results := []*services.Result{
		{Service: "weather", Tool: "forecast", Data: []byte(`{"temp": 42, "conditions": "rain"}`), ContextLabel: "Weather Forecast"},
		{Service: "news", Tool: "headlines", Data: []byte(`{"headlines": ["Big Event", "Market Update"]}`), ContextLabel: "Top Headlines"},
	}

	md, err := synth.Synthesize(context.Background(), "Morning Brief", "Focus on actionable items.", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if !strings.Contains(md, "Morning Brief") {
		t.Error("expected report title in output")
	}
	// 2 stage 1 calls + 1 stage 2 call = 3 total
	total := int(callNum.Load())
	if total != 3 {
		t.Errorf("expected 3 LLM calls, got %d", total)
	}
}

// --- assembleStage2Prompt tests ---

func TestAssembleStage2Prompt(t *testing.T) {
	synth := NewLLMSynthesizer(&fakeProvider{}, false)

	summaries := []sourceSummary{
		{label: "Source A", summary: "Key fact 1."},
		{label: "Source B", summary: "Key fact 2."},
	}

	prompt := synth.assembleStage2Prompt("Test Report", summaries)

	if !strings.Contains(prompt, "Test Report") {
		t.Error("expected title in prompt")
	}
	if !strings.Contains(prompt, "Pre-summarized source data extracts") {
		t.Error("expected pre-summarized marker")
	}
	if !strings.Contains(prompt, "Source A") {
		t.Error("expected Source A label")
	}
	if !strings.Contains(prompt, "Key fact 1.") {
		t.Error("expected Source A summary")
	}
	if !strings.Contains(prompt, "Source B") {
		t.Error("expected Source B label")
	}
}

// --- summarizeSource with ContextLabel ---

func TestSummarizeSourceUsesContextLabel(t *testing.T) {
	provider := &recordingProvider{response: "Summary."}
	synth := NewLLMSynthesizer(provider, false)
	synth.SetMultiStage(MultiStageConfig{SummaryMaxWords: 100})

	r := &services.Result{
		Service:      "nws",
		Tool:         "forecast",
		Data:         []byte("forecast data"),
		ContextLabel: "NWS 7-Day Forecast — Anchorage",
	}

	s := synth.summarizeSource(context.Background(), 0, r, "priorities")
	if s.label != "NWS 7-Day Forecast — Anchorage" {
		t.Errorf("expected context label, got %q", s.label)
	}
}

func TestSummarizeSourceFallbackLabel(t *testing.T) {
	provider := &recordingProvider{response: "Summary."}
	synth := NewLLMSynthesizer(provider, false)
	synth.SetMultiStage(MultiStageConfig{SummaryMaxWords: 100})

	r := &services.Result{
		Service: "nws",
		Tool:    "forecast",
		Data:    []byte("forecast data"),
	}

	s := synth.summarizeSource(context.Background(), 0, r, "")
	if s.label != "nws — forecast" {
		t.Errorf("expected service-tool label, got %q", s.label)
	}
}

func TestSummarizeSourceError(t *testing.T) {
	provider := &recordingProvider{response: "Summary."}
	synth := NewLLMSynthesizer(provider, false)

	r := &services.Result{
		Service: "svc",
		Tool:    "t",
		Error:   "connection refused",
	}

	s := synth.summarizeSource(context.Background(), 0, r, "")
	if !strings.Contains(s.summary, "connection refused") {
		t.Error("expected error message in summary")
	}
}

func TestSummarizeSourceEmptyData(t *testing.T) {
	provider := &recordingProvider{response: "Summary."}
	synth := NewLLMSynthesizer(provider, false)

	r := &services.Result{
		Service: "svc",
		Tool:    "t",
	}

	s := synth.summarizeSource(context.Background(), 0, r, "")
	if s.summary != "(no data)" {
		t.Errorf("expected (no data), got %q", s.summary)
	}
}

// --- Config defaults ---

func TestMultiStageConfigDefaults(t *testing.T) {
	cfg := MultiStageConfig{}
	if cfg.summaryMaxWords() != 500 {
		t.Errorf("expected default 500, got %d", cfg.summaryMaxWords())
	}
	if cfg.thresholdBytes() != 16384 {
		t.Errorf("expected default 16384, got %d", cfg.thresholdBytes())
	}
	if cfg.maxSourceWords() != 10000 {
		t.Errorf("expected default 10000, got %d", cfg.maxSourceWords())
	}
	if cfg.concurrency() != 1 {
		t.Errorf("expected default concurrency 1, got %d", cfg.concurrency())
	}
}

func TestMultiStageConfigCustom(t *testing.T) {
	cfg := MultiStageConfig{SummaryMaxWords: 200, ThresholdBytes: 8192, MaxSourceWords: 5000, Concurrency: 3}
	if cfg.summaryMaxWords() != 200 {
		t.Errorf("expected 200, got %d", cfg.summaryMaxWords())
	}
	if cfg.thresholdBytes() != 8192 {
		t.Errorf("expected 8192, got %d", cfg.thresholdBytes())
	}
	if cfg.maxSourceWords() != 5000 {
		t.Errorf("expected 5000, got %d", cfg.maxSourceWords())
	}
	if cfg.concurrency() != 3 {
		t.Errorf("expected concurrency 3, got %d", cfg.concurrency())
	}
}

func TestMaxSourceWordsDerivedFromContextWindow(t *testing.T) {
	// When MaxSourceWords is 0 and ContextWindow is set, derive from context window
	cfg := MultiStageConfig{ContextWindow: 8192}
	got := cfg.maxSourceWords()
	// 8192 * 0.4 = 3276 (truncated to int)
	if got != 3276 {
		t.Errorf("expected maxSourceWords 3276 (40%% of 8192), got %d", got)
	}

	// MaxSourceWords takes precedence over ContextWindow
	cfg2 := MultiStageConfig{MaxSourceWords: 5000, ContextWindow: 8192}
	if cfg2.maxSourceWords() != 5000 {
		t.Errorf("expected explicit MaxSourceWords 5000, got %d", cfg2.maxSourceWords())
	}

	// 32K context window
	cfg3 := MultiStageConfig{ContextWindow: 32768}
	got3 := cfg3.maxSourceWords()
	// 32768 * 0.4 = 13107 (truncated to int)
	if got3 != 13107 {
		t.Errorf("expected maxSourceWords 13107 (40%% of 32768), got %d", got3)
	}
}

// --- Auto threshold with context window tests ---

func TestAutoThresholdWithLargeContextWindow(t *testing.T) {
	// Haiku 200K window: threshold = 200000 * 0.5 * 4 = 400,000 bytes
	// 190KB data < 400KB threshold → single-stage
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "auto", ContextWindow: 200000})

	data := make([]byte, 190*1024) // 190KB
	results := []*services.Result{{Data: data}}

	if synth.shouldMultiStage(results) {
		t.Error("expected single-stage for 190KB with 200K context window")
	}
}

func TestAutoThresholdWithSmallContextWindow(t *testing.T) {
	// Local qwen3-32b 65K window: threshold = 65536 * 0.5 * 4 = 131,072 bytes = 128KB
	// 190KB data > 128KB threshold → multi-stage
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	synth.SetMultiStage(MultiStageConfig{Strategy: "auto", ContextWindow: 65536})

	data := make([]byte, 190*1024) // 190KB
	results := []*services.Result{{Data: data}}

	if !synth.shouldMultiStage(results) {
		t.Error("expected multi-stage for 190KB with 65K context window")
	}
}

func TestAutoThresholdExplicitOverride(t *testing.T) {
	// Explicit ThresholdBytes takes precedence over ContextWindow
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	synth.SetMultiStage(MultiStageConfig{
		Strategy:       "auto",
		ThresholdBytes: 100 * 1024, // 100KB explicit
		ContextWindow:  200000,     // would give 400KB if used
	})

	data := make([]byte, 150*1024) // 150KB — above explicit 100KB, below context-derived 400KB
	results := []*services.Result{{Data: data}}

	if !synth.shouldMultiStage(results) {
		t.Error("expected multi-stage: 150KB exceeds explicit 100KB threshold")
	}
}

func TestAutoThresholdDefault8KMatchesLegacy(t *testing.T) {
	// Default local 8K: threshold = 8192 * 0.5 * 4 = 16,384 = defaultThresholdBytes
	cfg := MultiStageConfig{ContextWindow: 8192}
	got := cfg.autoThresholdBytes()
	if got != defaultThresholdBytes {
		t.Errorf("8K context window should produce %d threshold, got %d", defaultThresholdBytes, got)
	}
}

func TestAutoThresholdNoContextWindow(t *testing.T) {
	// No ContextWindow set → falls back to defaultThresholdBytes
	cfg := MultiStageConfig{}
	got := cfg.autoThresholdBytes()
	if got != defaultThresholdBytes {
		t.Errorf("no context window should fall back to %d, got %d", defaultThresholdBytes, got)
	}
}

// --- Stage 2 bounding tests ---

func TestBoundStage2SummariesNoOpWhenFits(t *testing.T) {
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	synth.SetMultiStage(MultiStageConfig{ContextWindow: 200000}) // huge window

	summaries := []sourceSummary{
		{label: "A", summary: "Short summary."},
		{label: "B", summary: "Another short one."},
	}

	bounded := synth.boundStage2Summaries(summaries)
	for i, s := range bounded {
		if s.summary != summaries[i].summary {
			t.Errorf("summary %d was truncated when it should fit: %q", i, s.summary)
		}
	}
}

func TestBoundStage2SummariesTruncatesWhenOverBudget(t *testing.T) {
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	// Tiny context window: 100 tokens → budget = 100 * 0.6 * 4 = 240 bytes
	synth.SetMultiStage(MultiStageConfig{ContextWindow: 100})

	// Each summary is ~200 words → way over 240 byte budget
	longText := strings.Repeat("word ", 200)
	summaries := []sourceSummary{
		{label: "A", summary: longText},
		{label: "B", summary: longText},
	}

	bounded := synth.boundStage2Summaries(summaries)
	for i, s := range bounded {
		if len(s.summary) >= len(longText) {
			t.Errorf("summary %d was not truncated (len %d)", i, len(s.summary))
		}
		if !strings.HasSuffix(s.summary, "...") {
			t.Errorf("summary %d missing truncation ellipsis", i)
		}
	}
}

func TestBoundStage2SummariesNoContextWindow(t *testing.T) {
	synth := NewLLMSynthesizer(&fakeProvider{}, false)
	synth.SetMultiStage(MultiStageConfig{}) // no context window

	summaries := []sourceSummary{
		{label: "A", summary: strings.Repeat("word ", 10000)},
	}

	bounded := synth.boundStage2Summaries(summaries)
	if bounded[0].summary != summaries[0].summary {
		t.Error("should not truncate when no context window is configured")
	}
}

// --- Chunked summarization test ---

func TestSummarizeSourceChunksLargeData(t *testing.T) {
	provider := &recordingProvider{response: "Chunk summary."}
	synth := NewLLMSynthesizer(provider, false)
	synth.SetMultiStage(MultiStageConfig{
		Strategy:        "multi-stage",
		SummaryMaxWords: 100,
		MaxSourceWords:  10, // Very low to force chunking
	})

	// Create data with ~30 words — should be split into multiple chunks at maxSourceWords=10
	words := make([]string, 30)
	for i := range words {
		words[i] = "word"
	}
	bigData := strings.Join(words, " ")

	r := &services.Result{
		Service: "test",
		Tool:    "fetch",
		Data:    []byte(bigData),
	}

	s := synth.summarizeSource(context.Background(), 0, r, "priorities")
	if s.err != nil {
		t.Fatalf("summarizeSource: %v", s.err)
	}

	// Should have made multiple LLM calls (one per chunk)
	calls := provider.getCalls()
	if len(calls) < 2 {
		t.Errorf("expected at least 2 chunk LLM calls, got %d", len(calls))
	}

	// At least one call should have "(part" in its label
	foundPart := false
	for _, c := range calls {
		if strings.Contains(c.user, "(part") {
			foundPart = true
			break
		}
	}
	if !foundPart {
		t.Error("expected chunk calls to include '(part N/M)' in label")
	}

	// Merged summary should not be empty
	if s.summary == "" {
		t.Error("expected non-empty merged summary")
	}
}
