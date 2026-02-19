package synthesis

import (
	"testing"

	"github.com/jcadam/burrow/pkg/config"
)

func TestNewProviderOllama(t *testing.T) {
	p, err := NewProvider(config.ProviderConfig{
		Name:  "local",
		Type:  "ollama",
		Model: "qwen2.5:14b",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if _, ok := p.(*OllamaProvider); !ok {
		t.Errorf("expected *OllamaProvider, got %T", p)
	}
}

func TestNewProviderOllamaNoModel(t *testing.T) {
	_, err := NewProvider(config.ProviderConfig{
		Name: "local",
		Type: "ollama",
	})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestNewProviderOpenRouter(t *testing.T) {
	p, err := NewProvider(config.ProviderConfig{
		Name:   "remote",
		Type:   "openrouter",
		APIKey: "sk-test",
		Model:  "mistral/mistral-7b",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if _, ok := p.(*OpenRouterProvider); !ok {
		t.Errorf("expected *OpenRouterProvider, got %T", p)
	}
}

func TestNewProviderOpenRouterNoKey(t *testing.T) {
	_, err := NewProvider(config.ProviderConfig{
		Name:  "remote",
		Type:  "openrouter",
		Model: "model",
	})
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
}

func TestNewProviderOpenRouterNoModel(t *testing.T) {
	_, err := NewProvider(config.ProviderConfig{
		Name:   "remote",
		Type:   "openrouter",
		APIKey: "sk-test",
	})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestNewProviderPassthrough(t *testing.T) {
	p, err := NewProvider(config.ProviderConfig{
		Name: "none",
		Type: "passthrough",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p != nil {
		t.Errorf("expected nil provider for passthrough, got %T", p)
	}
}

func TestNewProviderEmptyType(t *testing.T) {
	p, err := NewProvider(config.ProviderConfig{Name: "default"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p != nil {
		t.Errorf("expected nil provider for empty type, got %T", p)
	}
}

func TestNewProviderUnknownType(t *testing.T) {
	_, err := NewProvider(config.ProviderConfig{
		Name: "bad",
		Type: "gpt4all",
	})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}
