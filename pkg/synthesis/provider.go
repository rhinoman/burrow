package synthesis

import (
	"fmt"

	"github.com/jcadam/burrow/pkg/config"
)

// NewProvider creates an LLM provider from config. Returns (nil, nil) for
// passthrough type, signaling the caller to use PassthroughSynthesizer.
func NewProvider(cfg config.ProviderConfig) (Provider, error) {
	switch cfg.Type {
	case "ollama":
		if cfg.Model == "" {
			return nil, fmt.Errorf("ollama provider %q requires a model", cfg.Name)
		}
		return NewOllamaProviderWithTimeout(cfg.Endpoint, cfg.Model, cfg.Timeout, cfg.ContextWindow), nil

	case "openrouter":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("openrouter provider %q requires an api_key", cfg.Name)
		}
		if cfg.Model == "" {
			return nil, fmt.Errorf("openrouter provider %q requires a model", cfg.Name)
		}
		return NewOpenRouterProviderWithTimeout(cfg.Endpoint, cfg.APIKey, cfg.Model, cfg.Timeout), nil

	case "passthrough", "":
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown provider type %q for %q", cfg.Type, cfg.Name)
	}
}
