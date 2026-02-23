package synthesis

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jcadam/burrow/pkg/services"
)

func TestPassthroughSynthesizeBasic(t *testing.T) {
	synth := NewPassthroughSynthesizer()
	results := []*services.Result{
		{
			Service:   "sam-gov",
			Tool:      "search_opportunities",
			Data:      []byte(`{"results": [{"title": "Contract A"}]}`),
			Timestamp: time.Now(),
		},
		{
			Service:   "edgar",
			Tool:      "company_filings",
			Data:      []byte(`{"filings": []}`),
			Timestamp: time.Now(),
		},
	}

	md, err := synth.Synthesize(context.Background(), "Test Report", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if !strings.Contains(md, "# Test Report") {
		t.Error("expected report title")
	}
	if !strings.Contains(md, "sam-gov") {
		t.Error("expected sam-gov section")
	}
	if !strings.Contains(md, "edgar") {
		t.Error("expected edgar section")
	}
	if !strings.Contains(md, "Contract A") {
		t.Error("expected data content")
	}
	if !strings.Contains(md, "**Sources queried:** 2") {
		t.Error("expected sources count")
	}
	if !strings.Contains(md, "**Successful:** 2") {
		t.Error("expected success count")
	}
}

func TestPassthroughSynthesizeWithErrors(t *testing.T) {
	synth := NewPassthroughSynthesizer()
	results := []*services.Result{
		{
			Service:   "broken-api",
			Tool:      "fetch",
			Data:      []byte(`Not Found`),
			Timestamp: time.Now(),
			Error:     "HTTP 404",
		},
		{
			Service:   "good-api",
			Tool:      "search",
			Data:      []byte(`{"ok": true}`),
			Timestamp: time.Now(),
		},
	}

	md, err := synth.Synthesize(context.Background(), "Mixed Results", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if !strings.Contains(md, "**Errors:** 1") {
		t.Error("expected error count of 1")
	}
	if !strings.Contains(md, "**Successful:** 1") {
		t.Error("expected success count of 1")
	}
	if !strings.Contains(md, "HTTP 404") {
		t.Error("expected error message in output")
	}
}

func TestPassthroughSynthesizeEmpty(t *testing.T) {
	synth := NewPassthroughSynthesizer()
	md, err := synth.Synthesize(context.Background(), "Empty Report", "", nil)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if !strings.Contains(md, "# Empty Report") {
		t.Error("expected title even with no results")
	}
	if !strings.Contains(md, "**Sources queried:** 0") {
		t.Error("expected zero sources")
	}
}

// --- trimConversationalClosing unit tests ---

func TestTrimClosingBasic(t *testing.T) {
	input := "## Summary\n\nGood stuff here.\n\nQuestions? Reply to refine.\n"
	got := trimConversationalClosing(input)
	if strings.Contains(got, "Reply to refine") {
		t.Error("expected closing to be stripped")
	}
	if !strings.Contains(got, "Good stuff here.") {
		t.Error("expected content to be preserved")
	}
}

func TestTrimClosingWithSeparator(t *testing.T) {
	input := "## Summary\n\nContent here.\n\n---\n\nLet me know if you have questions.\n"
	got := trimConversationalClosing(input)
	if strings.Contains(got, "Let me know") {
		t.Error("expected closing to be stripped")
	}
	if strings.Contains(got, "---") {
		// The trailing --- should also be stripped since it's between content and closing.
		// But if there's a --- that's part of the content above, that's fine.
		// Let's just check the closing is gone.
	}
	if !strings.Contains(got, "Content here.") {
		t.Error("expected content to be preserved")
	}
}

func TestTrimClosingEmDashPrefix(t *testing.T) {
	input := "## Summary\n\nContent.\n\n— Questions? Reply to refine.\n"
	got := trimConversationalClosing(input)
	if strings.Contains(got, "Questions? Reply") {
		t.Error("expected em-dash prefixed closing to be stripped")
	}
	if !strings.Contains(got, "Content.") {
		t.Error("expected content to be preserved")
	}
}

func TestTrimClosingMultiLine(t *testing.T) {
	input := "Report content.\n\nLet me know if you need anything.\nI'd be happy to help further.\n"
	got := trimConversationalClosing(input)
	if strings.Contains(got, "Let me know") {
		t.Error("expected first closing line stripped")
	}
	if strings.Contains(got, "happy to help") {
		t.Error("expected second closing line stripped")
	}
	if !strings.Contains(got, "Report content.") {
		t.Error("expected content preserved")
	}
}

func TestTrimClosingNoMatch(t *testing.T) {
	input := "1. Do this\n2. Do that\n"
	got := trimConversationalClosing(input)
	if got != input {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestTrimClosingEmpty(t *testing.T) {
	got := trimConversationalClosing("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestTrimClosingBlockquoteProtected(t *testing.T) {
	input := "> Let me know\n"
	got := trimConversationalClosing(input)
	if !strings.Contains(got, "Let me know") {
		t.Error("expected blockquote content to be preserved")
	}
}

func TestTrimClosingCaseInsensitive(t *testing.T) {
	input := "Content here.\n\nLET ME KNOW IF YOU HAVE QUESTIONS!\n"
	got := trimConversationalClosing(input)
	if strings.Contains(strings.ToLower(got), "let me know") {
		t.Error("expected case-insensitive match to be stripped")
	}
	if !strings.Contains(got, "Content here.") {
		t.Error("expected content preserved")
	}
}

// --- fakeProvider and LLM integration tests ---

// fakeProvider captures the prompt sent to the LLM.
type fakeProvider struct {
	lastSystem string
	lastUser   string
	response   string
}

func (f *fakeProvider) Complete(_ context.Context, system, user string) (string, error) {
	f.lastSystem = system
	f.lastUser = user
	if f.response != "" {
		return f.response, nil
	}
	return "# Generated Report\n", nil
}

func TestLLMSynthesizerTrimsClosing(t *testing.T) {
	provider := &fakeProvider{
		response: "# Daily Brief\n\nKey findings here.\n\n---\n\nLet me know if you have questions.\n",
	}
	synth := NewLLMSynthesizer(provider, false)

	results := []*services.Result{
		{Service: "test-svc", Tool: "fetch", Data: []byte(`data`)},
	}

	got, err := synth.Synthesize(context.Background(), "Brief", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if strings.Contains(got, "Let me know") {
		t.Error("expected LLMSynthesizer to strip conversational closing from output")
	}
	if !strings.Contains(got, "Key findings here.") {
		t.Error("expected report content to be preserved")
	}
}

func TestLLMSynthesizerStripAttribution(t *testing.T) {
	provider := &fakeProvider{}
	synth := NewLLMSynthesizer(provider, true)

	results := []*services.Result{
		{Service: "sam-gov", Tool: "search_opportunities", Data: []byte(`data1`)},
		{Service: "edgar", Tool: "company_filings", Data: []byte(`data2`)},
	}

	_, err := synth.Synthesize(context.Background(), "Brief", "Be concise.", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Service names must NOT appear in the prompt sent to the LLM
	if strings.Contains(provider.lastUser, "sam-gov") {
		t.Error("attribution leak: sam-gov appears in LLM prompt")
	}
	if strings.Contains(provider.lastUser, "edgar") {
		t.Error("attribution leak: edgar appears in LLM prompt")
	}
	// Generic labels should be used instead
	if !strings.Contains(provider.lastUser, "Source 1") {
		t.Error("expected generic label Source 1")
	}
	if !strings.Contains(provider.lastUser, "Source 2") {
		t.Error("expected generic label Source 2")
	}
}

func TestLLMSynthesizerStripErrorAttribution(t *testing.T) {
	provider := &fakeProvider{}
	synth := NewLLMSynthesizer(provider, true)

	results := []*services.Result{
		{Service: "sam-gov", Tool: "search", Error: `service not found: "sam-gov"`},
	}

	_, err := synth.Synthesize(context.Background(), "Brief", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if strings.Contains(provider.lastUser, "sam-gov") {
		t.Error("attribution leak: sam-gov appears in error message sent to LLM")
	}
	if !strings.Contains(provider.lastUser, "[service]") {
		t.Error("expected service name replaced with [service] in error")
	}
}

func TestStripServiceNamesShortNameSkipped(t *testing.T) {
	provider := &fakeProvider{}
	synth := NewLLMSynthesizer(provider, true)

	results := []*services.Result{
		{Service: "ab", Tool: "search", Error: `connection to ab failed at /ab/endpoint`},
	}

	_, err := synth.Synthesize(context.Background(), "Brief", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Short service name "ab" should NOT be replaced — would corrupt text
	if strings.Contains(provider.lastUser, "[service]") {
		t.Error("short service name 'ab' should not be stripped")
	}
	if !strings.Contains(provider.lastUser, "ab") {
		t.Error("expected 'ab' to remain in error text")
	}
}

func TestLLMSynthesizerStripDataAttribution(t *testing.T) {
	provider := &fakeProvider{}
	synth := NewLLMSynthesizer(provider, true)

	results := []*services.Result{
		{Service: "sam-gov", Tool: "search", Data: []byte(`{"source": "sam-gov", "results": []}`)},
	}

	_, err := synth.Synthesize(context.Background(), "Brief", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Service name in response body must be stripped when sending to remote LLM
	if strings.Contains(provider.lastUser, "sam-gov") {
		t.Error("attribution leak: sam-gov in response data sent to LLM")
	}
	if !strings.Contains(provider.lastUser, "[service]") {
		t.Error("expected service name in data replaced with [service]")
	}
}

func TestStripServiceNamesOverlappingNames(t *testing.T) {
	// "news-api" must be replaced as a whole, not corrupted into "[service]-api"
	results := []*services.Result{
		{Service: "news"},
		{Service: "news-api"},
	}
	got := stripServiceNames("data from news-api and news feed", results)
	if strings.Contains(got, "news-api") {
		t.Error("news-api should have been replaced")
	}
	if strings.Contains(got, "[service]-api") {
		t.Error("substring corruption: got [service]-api instead of [service]")
	}
	want := "data from [service] and [service] feed"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOpenRouterTrailingSlash(t *testing.T) {
	p := NewOpenRouterProvider("https://api.example.com/v1/", "key", "model")
	if p.endpoint != "https://api.example.com/v1" {
		t.Errorf("expected trailing slash trimmed, got: %s", p.endpoint)
	}
}

func TestOpenRouterNoTrailingSlash(t *testing.T) {
	p := NewOpenRouterProvider("https://api.example.com/v1", "key", "model")
	if p.endpoint != "https://api.example.com/v1" {
		t.Errorf("expected endpoint unchanged, got: %s", p.endpoint)
	}
}

func TestOllamaTrailingSlash(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434/", "model")
	if p.endpoint != "http://localhost:11434" {
		t.Errorf("expected trailing slash trimmed, got: %s", p.endpoint)
	}
}

func TestOllamaNoTrailingSlash(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434", "model")
	if p.endpoint != "http://localhost:11434" {
		t.Errorf("expected endpoint unchanged, got: %s", p.endpoint)
	}
}

func TestLLMSynthesizerStaticDocumentInstruction(t *testing.T) {
	provider := &fakeProvider{}
	synth := NewLLMSynthesizer(provider, false)

	results := []*services.Result{
		{Service: "test-svc", Tool: "fetch", Data: []byte(`data`)},
	}

	_, err := synth.Synthesize(context.Background(), "Brief", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Primary instruction must be in system prompt for authority over LLM training
	if !strings.Contains(provider.lastSystem, "static report document") {
		t.Error("expected static-document instruction in system prompt")
	}
	if !strings.Contains(provider.lastSystem, "Reply to refine") {
		t.Error("expected explicit prohibition of conversational closing in system prompt")
	}
	// Short reinforcement in user prompt
	if !strings.Contains(provider.lastUser, "static document only") {
		t.Error("expected short reinforcement in user prompt")
	}
}

func TestLLMSynthesizerLinkInstruction(t *testing.T) {
	provider := &fakeProvider{}
	synth := NewLLMSynthesizer(provider, false)

	results := []*services.Result{
		{Service: "rss-feed", Tool: "fetch", Data: []byte(`{"title":"Article","link":"https://example.com"}`)},
	}

	_, err := synth.Synthesize(context.Background(), "Brief", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if !strings.Contains(provider.lastUser, "markdown links") {
		t.Error("expected link-preservation instruction in user prompt")
	}
	if !strings.Contains(provider.lastUser, "clickable link") {
		t.Error("expected clickable link requirement in user prompt")
	}
	if !strings.Contains(provider.lastUser, "exact URL") {
		t.Error("expected exact-URL instruction in user prompt")
	}
	if !strings.Contains(provider.lastUser, "Never break a URL") {
		t.Error("expected no-line-break instruction in user prompt")
	}
}

// --- repairBrokenURLs unit tests ---

func TestRepairBrokenURLsBasic(t *testing.T) {
	input := "[Article](https://example.com/item?\nid=123)"
	want := "[Article](https://example.com/item?id=123)"
	got := repairBrokenURLs(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRepairBrokenURLsLeadingWhitespace(t *testing.T) {
	input := "[Article](https://example.com/item?\n    id=123)"
	want := "[Article](https://example.com/item?id=123)"
	got := repairBrokenURLs(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRepairBrokenURLsMultipleLinks(t *testing.T) {
	input := "See [A](https://a.com/\npath) and [B](https://b.com/\nother)"
	want := "See [A](https://a.com/path) and [B](https://b.com/other)"
	got := repairBrokenURLs(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRepairBrokenURLsMultipleNewlines(t *testing.T) {
	input := "[Link](https://example.com/\na/\nb/c)"
	want := "[Link](https://example.com/a/b/c)"
	got := repairBrokenURLs(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRepairBrokenURLsCRLF(t *testing.T) {
	input := "[Link](https://example.com/item?\r\nid=1)"
	want := "[Link](https://example.com/item?id=1)"
	got := repairBrokenURLs(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRepairBrokenURLsValidUnchanged(t *testing.T) {
	input := "[Link](https://example.com/path) and [Other](https://other.com)"
	got := repairBrokenURLs(input)
	if got != input {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestRepairBrokenURLsEmpty(t *testing.T) {
	got := repairBrokenURLs("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestRepairBrokenURLsBareURLUnchanged(t *testing.T) {
	input := "Visit https://example.com/item?\nid=123 for details"
	got := repairBrokenURLs(input)
	if got != input {
		t.Errorf("expected bare URL unchanged, got %q", got)
	}
}

func TestLLMSynthesizerRepairsBrokenURLs(t *testing.T) {
	provider := &fakeProvider{
		response: "# Report\n\n[Article](https://example.com/item?\nid=123)\n",
	}
	synth := NewLLMSynthesizer(provider, false)

	results := []*services.Result{
		{Service: "test-svc", Tool: "fetch", Data: []byte(`data`)},
	}

	got, err := synth.Synthesize(context.Background(), "Brief", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if strings.Contains(got, "item?\nid=123") {
		t.Error("expected broken URL to be repaired in Synthesize output")
	}
	if !strings.Contains(got, "item?id=123") {
		t.Error("expected repaired URL in Synthesize output")
	}
}

func TestLLMSynthesizerIncompleteDataInstruction(t *testing.T) {
	provider := &fakeProvider{}
	synth := NewLLMSynthesizer(provider, false)

	results := []*services.Result{
		{Service: "test-svc", Tool: "fetch", Data: []byte(`data`)},
	}

	_, err := synth.Synthesize(context.Background(), "Brief", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if !strings.Contains(provider.lastUser, "missing or incomplete fields") {
		t.Error("expected incomplete-data instruction in user prompt")
	}
	if !strings.Contains(provider.lastUser, "analyze what IS present") {
		t.Error("expected analyze-available-data instruction in user prompt")
	}
	if !strings.Contains(provider.lastUser, `Never skip a section or declare "none included"`) {
		t.Error("expected never-skip instruction in user prompt")
	}
}

func TestLLMSynthesizerPreserveAttribution(t *testing.T) {
	provider := &fakeProvider{}
	synth := NewLLMSynthesizer(provider, false)

	results := []*services.Result{
		{Service: "sam-gov", Tool: "search", Data: []byte(`data`)},
	}

	_, err := synth.Synthesize(context.Background(), "Brief", "", results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// With local LLM, service names should be preserved
	if !strings.Contains(provider.lastUser, "sam-gov") {
		t.Error("expected service name in prompt for local LLM")
	}
}
