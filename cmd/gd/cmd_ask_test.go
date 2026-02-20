package main

import (
	"testing"

	"github.com/jcadam/burrow/pkg/config"
)

func TestFindLocalProviderNilConfig(t *testing.T) {
	if p := findLocalProvider(nil); p != nil {
		t.Error("expected nil for nil config")
	}
}

func TestFindLocalProviderEmpty(t *testing.T) {
	cfg := &config.Config{}
	if p := findLocalProvider(cfg); p != nil {
		t.Error("expected nil for empty config")
	}
}

func TestFindLocalProviderSkipsRemote(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "cloud", Type: "openrouter", APIKey: "key", Model: "gpt-4", Privacy: "remote"},
			},
		},
	}
	if p := findLocalProvider(cfg); p != nil {
		t.Error("expected nil — should skip remote providers")
	}
}

func TestFindLocalProviderSkipsPassthrough(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "none", Type: "passthrough", Privacy: "local"},
			},
		},
	}
	if p := findLocalProvider(cfg); p != nil {
		t.Error("expected nil — should skip passthrough")
	}
}

func TestFindLocalProviderFindsOllama(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "cloud", Type: "openrouter", APIKey: "key", Model: "gpt-4", Privacy: "remote"},
				{Name: "none", Type: "passthrough", Privacy: "local"},
				{Name: "local/llama", Type: "ollama", Model: "llama3:latest", Privacy: "local"},
			},
		},
	}
	p := findLocalProvider(cfg)
	if p == nil {
		t.Fatal("expected to find local ollama provider")
	}
}

func TestFindLocalProviderFirstLocalWins(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Providers: []config.ProviderConfig{
				{Name: "local/a", Type: "ollama", Model: "a", Privacy: "local"},
				{Name: "local/b", Type: "ollama", Model: "b", Privacy: "local"},
			},
		},
	}
	// Should not panic and should return a provider
	p := findLocalProvider(cfg)
	if p == nil {
		t.Fatal("expected to find provider")
	}
}
