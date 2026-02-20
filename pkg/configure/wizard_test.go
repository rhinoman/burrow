package configure

import (
	"bytes"
	"strings"
	"testing"
)

func TestWizardRunInitOllama(t *testing.T) {
	// Simulate: choose Ollama, accept defaults, skip service, accept privacy, accept app defaults
	input := "1\n\nqwen2.5:14b\n\nn\ny\n\n\n"
	r := strings.NewReader(input)
	var out bytes.Buffer

	wiz := NewWizard(r, &out)
	cfg, err := wiz.RunInit()
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	if len(cfg.LLM.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.LLM.Providers))
	}
	if cfg.LLM.Providers[0].Type != "ollama" {
		t.Errorf("expected ollama, got %q", cfg.LLM.Providers[0].Type)
	}
	if cfg.LLM.Providers[0].Model != "qwen2.5:14b" {
		t.Errorf("expected qwen2.5:14b, got %q", cfg.LLM.Providers[0].Model)
	}
	if !cfg.Privacy.StripReferrers {
		t.Error("expected privacy hardening enabled")
	}
}

func TestWizardRunInitNone(t *testing.T) {
	// Choose none, skip service, accept privacy, accept app defaults
	input := "3\nn\ny\n\n\n"
	r := strings.NewReader(input)
	var out bytes.Buffer

	wiz := NewWizard(r, &out)
	cfg, err := wiz.RunInit()
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	if len(cfg.LLM.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.LLM.Providers))
	}
	if cfg.LLM.Providers[0].Type != "passthrough" {
		t.Errorf("expected passthrough, got %q", cfg.LLM.Providers[0].Type)
	}
}

func TestWizardRunInitWithService(t *testing.T) {
	// Choose none LLM, add a service with API key auth, one tool, 0 params, accept privacy, defaults
	input := "3\ny\ntest-api\nhttps://api.test.com\n1\n${TEST_KEY}\n\nn\ny\n\n\n"
	r := strings.NewReader(input)
	var out bytes.Buffer

	wiz := NewWizard(r, &out)
	cfg, err := wiz.RunInit()
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(cfg.Services))
	}
	if cfg.Services[0].Name != "test-api" {
		t.Errorf("expected test-api, got %q", cfg.Services[0].Name)
	}
	if cfg.Services[0].Auth.Method != "api_key" {
		t.Errorf("expected api_key auth, got %q", cfg.Services[0].Auth.Method)
	}
}

func TestWizardRunInitOpenRouter(t *testing.T) {
	// Choose OpenRouter, skip service, skip privacy, accept app defaults
	input := "2\n${OPENROUTER_KEY}\nopenai/gpt-4\n\nn\ny\n\n\n"
	r := strings.NewReader(input)
	var out bytes.Buffer

	wiz := NewWizard(r, &out)
	cfg, err := wiz.RunInit()
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	if len(cfg.LLM.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.LLM.Providers))
	}
	if cfg.LLM.Providers[0].Type != "openrouter" {
		t.Errorf("expected openrouter, got %q", cfg.LLM.Providers[0].Type)
	}
	if cfg.LLM.Providers[0].APIKey != "${OPENROUTER_KEY}" {
		t.Errorf("expected ${OPENROUTER_KEY}, got %q", cfg.LLM.Providers[0].APIKey)
	}
}

func TestWizardOutputContainsBanner(t *testing.T) {
	input := "3\nn\ny\n\n\n"
	r := strings.NewReader(input)
	var out bytes.Buffer

	wiz := NewWizard(r, &out)
	_, err := wiz.RunInit()
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Burrow") {
		t.Error("expected banner to contain 'Burrow'")
	}
}
