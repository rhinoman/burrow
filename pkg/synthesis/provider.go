package synthesis

import (
	"fmt"

	"github.com/jcadam/burrow/pkg/config"
)

// GenerationParams holds optional LLM generation parameters.
// Nil pointer fields mean "use model default".
type GenerationParams struct {
	Temperature *float64
	TopP        *float64
	MaxTokens   int // 0 = model default
}

// NewProvider creates an LLM provider from config. Returns (nil, nil) for
// passthrough type, signaling the caller to use PassthroughSynthesizer.
func NewProvider(cfg config.ProviderConfig) (Provider, error) {
	params := GenerationParams{
		Temperature: cfg.Temperature,
		TopP:        cfg.TopP,
		MaxTokens:   cfg.MaxTokens,
	}

	switch cfg.Type {
	case "ollama":
		if cfg.Model == "" {
			return nil, fmt.Errorf("ollama provider %q requires a model", cfg.Name)
		}
		p := NewOllamaProviderWithTimeout(cfg.Endpoint, cfg.Model, cfg.Timeout, cfg.ContextWindow)
		p.SetGenerationParams(params)
		return p, nil

	case "openrouter":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("openrouter provider %q requires an api_key", cfg.Name)
		}
		if cfg.Model == "" {
			return nil, fmt.Errorf("openrouter provider %q requires a model", cfg.Name)
		}
		p := NewOpenRouterProviderWithTimeout(cfg.Endpoint, cfg.APIKey, cfg.Model, cfg.Timeout)
		p.SetGenerationParams(params)
		return p, nil

	case "passthrough", "":
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown provider type %q for %q", cfg.Type, cfg.Name)
	}
}
