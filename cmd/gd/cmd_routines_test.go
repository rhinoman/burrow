package main

import (
	"strings"
	"testing"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/pipeline"
	"github.com/jcadam/burrow/pkg/synthesis"
)

func TestBuildSynthesizerEmpty(t *testing.T) {
	routine := &pipeline.Routine{LLM: ""}
	cfg := &config.Config{}

	synth, err := buildSynthesizer(routine, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := synth.(*synthesis.PassthroughSynthesizer); !ok {
		t.Errorf("expected PassthroughSynthesizer, got %T", synth)
	}
}

func TestBuildSynthesizerNone(t *testing.T) {
	routine := &pipeline.Routine{LLM: "none"}
	cfg := &config.Config{}

	synth, err := buildSynthesizer(routine, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := synth.(*synthesis.PassthroughSynthesizer); !ok {
		t.Errorf("expected PassthroughSynthesizer, got %T", synth)
	}
}

func TestBuildSynthesizerOllama(t *testing.T) {
	routine := &pipeline.Routine{LLM: "local/qwen"}
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{
					Name:     "local/qwen",
					Type:     "ollama",
					Endpoint: "http://localhost:11434",
					Model:    "qwen2.5:14b",
					Privacy:  "local",
				},
			},
		},
	}

	synth, err := buildSynthesizer(routine, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := synth.(*synthesis.LLMSynthesizer); !ok {
		t.Errorf("expected LLMSynthesizer, got %T", synth)
	}
}

func TestBuildSynthesizerOpenRouterRemote(t *testing.T) {
	routine := &pipeline.Routine{LLM: "cloud/gpt"}
	cfg := &config.Config{
		Privacy: config.PrivacyConfig{StripAttributionForRemote: true},
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{
					Name:     "cloud/gpt",
					Type:     "openrouter",
					Endpoint: "https://openrouter.ai/api/v1",
					APIKey:   "test-key",
					Model:    "openai/gpt-4",
					Privacy:  "remote",
				},
			},
		},
	}

	synth, err := buildSynthesizer(routine, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := synth.(*synthesis.LLMSynthesizer); !ok {
		t.Errorf("expected LLMSynthesizer, got %T", synth)
	}
}

func TestBuildSynthesizerPassthrough(t *testing.T) {
	routine := &pipeline.Routine{LLM: "passthrough"}
	cfg := &config.Config{}

	synth, err := buildSynthesizer(routine, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := synth.(*synthesis.PassthroughSynthesizer); !ok {
		t.Errorf("expected PassthroughSynthesizer, got %T", synth)
	}
}

func TestBuildSynthesizerPassthroughProvider(t *testing.T) {
	routine := &pipeline.Routine{LLM: "noop"}
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{
					Name: "noop",
					Type: "passthrough",
				},
			},
		},
	}

	synth, err := buildSynthesizer(routine, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := synth.(*synthesis.PassthroughSynthesizer); !ok {
		t.Errorf("expected PassthroughSynthesizer, got %T", synth)
	}
}

func TestBuildSynthesizerUnknownProvider(t *testing.T) {
	routine := &pipeline.Routine{LLM: "nonexistent"}
	cfg := &config.Config{}

	_, err := buildSynthesizer(routine, cfg)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}
