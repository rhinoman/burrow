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

// fakeProvider captures the prompt sent to the LLM.
type fakeProvider struct {
	lastSystem string
	lastUser   string
}

func (f *fakeProvider) Complete(_ context.Context, system, user string) (string, error) {
	f.lastSystem = system
	f.lastUser = user
	return "# Generated Report\n", nil
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
